package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"fragwatch/internal/llm"
	"fragwatch/internal/store"
)

func (a *App) aiRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/ai/expand", a.aiExpand)
	mux.HandleFunc("POST /api/ai/suggest", a.aiSuggest)
	mux.HandleFunc("GET /api/market/medians", func(w http.ResponseWriter, r *http.Request) {
		m, err := a.Store.ScoreMedians(r.Context())
		if err != nil {
			httpErr(w, err, 500)
			return
		}
		writeJSON(w, m)
	})
}

func (a *App) addUsage(u llm.Usage) {
	a.llmMu.Lock()
	a.llmUsage.PromptTokens += u.PromptTokens
	a.llmUsage.CompletionTokens += u.CompletionTokens
	a.llmUsage.Cost += u.Cost
	a.llmMu.Unlock()
}

// aiExpand turns a topic into search stems for the Topic filter.
func (a *App) aiExpand(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Topic string `json:"topic"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || strings.TrimSpace(b.Topic) == "" {
		httpErr(w, apiError("topic is required"), 400)
		return
	}
	topic := strings.ToLower(strings.Join(strings.Fields(b.Topic), " "))
	cacheKey := "topic:" + topic
	if cached := a.Store.GetKV(r.Context(), cacheKey); cached != "" {
		var kw []string
		if json.Unmarshal([]byte(cached), &kw) == nil {
			writeJSON(w, map[string]any{"keywords": kw, "cached": true})
			return
		}
	}
	if a.paywall(w, r, "topic") {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	kw, usage, err := a.LLM.Expand(ctx, topic)
	a.addUsage(usage)
	if err != nil {
		httpErr(w, err, 502)
		return
	}
	a.charge(ctx, "topic", usage, topic)
	if js, err := json.Marshal(kw); err == nil {
		a.Store.SetKV(r.Context(), cacheKey, string(js))
	}
	writeJSON(w, map[string]any{"keywords": kw})
}

type suggestPick struct {
	Fit    int            `json:"fit"`
	Reason string         `json:"reason"`
	Item   store.Username `json:"item"`
}

type suggestIdea struct {
	Name   string          `json:"name"`
	Status string          `json:"status"` // fragment status, or "unknown" when Fragment has no page
	Item   *store.Username `json:"item,omitempty"`
}

// aiSuggest answers "I need a username for …": the model plans search stems and invents ideas,
// the database supplies existing names that match, the model ranks them, and invented ideas are
// checked live on Fragment.
func (a *App) aiSuggest(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Brief        string   `json:"brief"`
		MaxPrice     *float64 `json:"max_price"`
		BuyableOnly  bool     `json:"buyable_only"`
		ExcludeNames []string `json:"exclude"` // names already shown, for "more like this"
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || strings.TrimSpace(b.Brief) == "" {
		httpErr(w, apiError("brief is required"), 400)
		return
	}
	if a.paywall(w, r, "ask") {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	plan, usage, err := a.LLM.PlanSearch(ctx, b.Brief)
	a.addUsage(usage)
	if err != nil {
		httpErr(w, fmt.Errorf("planning: %w", err), 502)
		return
	}
	total := usage

	// Candidates: the best-rated matches overall, plus listed matches so buyable names always make it in.
	exclude := map[string]bool{}
	for _, n := range b.ExcludeNames {
		exclude[n] = true
	}
	minScore := 3
	base := store.Filter{Keywords: plan.Keywords, MinScore: &minScore, Unscored: true, HideJunk: true, Sort: "score", MaxLen: 20}
	if b.MaxPrice != nil {
		base.MaxBuyout = b.MaxPrice
	}
	var queries []store.Filter
	listed := base
	listed.Statuses = []string{"auction", "sale", "market"}
	listed.Limit = 80
	queries = append(queries, listed)
	if !b.BuyableOnly && b.MaxPrice == nil {
		all := base
		all.Limit = 120
		queries = append(queries, all)
	}
	byName := map[string]store.Username{}
	var order []string
	for _, f := range queries {
		items, _, err := a.query(ctx, f)
		if err != nil {
			httpErr(w, err, 500)
			return
		}
		for _, u := range items {
			if _, ok := byName[u.Name]; !ok && !exclude[u.Name] {
				byName[u.Name] = u
				order = append(order, u.Name)
			}
		}
	}
	// Invented ideas that already exist in the database are candidates too.
	var unknownIdeas []string
	for _, idea := range plan.Ideas {
		if u, _ := a.Store.Get(ctx, idea); u != nil {
			if _, ok := byName[idea]; !ok && !exclude[idea] {
				byName[idea] = *u
				order = append(order, idea)
			}
		} else {
			unknownIdeas = append(unknownIdeas, idea)
		}
	}

	picks := []suggestPick{}
	if len(order) > 0 {
		lines := make([]string, 0, len(order))
		for _, n := range order {
			u := byName[n]
			lines = append(lines, fmt.Sprintf("%s | %s | %s", n, deref(u.Meaning, "-"), availability(u)))
		}
		ranked, usage, err := a.LLM.Rank(ctx, b.Brief, lines)
		a.addUsage(usage)
		if err != nil {
			httpErr(w, fmt.Errorf("ranking: %w", err), 502)
			return
		}
		total.PromptTokens += usage.PromptTokens
		total.CompletionTokens += usage.CompletionTokens
		total.Cost += usage.Cost
		for _, p := range ranked {
			if u, ok := byName[p.Name]; ok && p.Fit >= 4 {
				picks = append(picks, suggestPick{Fit: p.Fit, Reason: p.Reason, Item: u})
				delete(byName, p.Name) // no duplicates if the model repeats a name
			}
		}
		sort.SliceStable(picks, func(i, j int) bool { return picks[i].Fit > picks[j].Fit })
	}

	// Check invented ideas on Fragment right now; anything Fragment knows gets stored.
	ideas := []suggestIdea{}
	found := map[string]store.FragRow{}
	if len(unknownIdeas) > 0 {
		a.Frag.DetailMany(ctx, unknownIdeas, a.Cfg.DetailWorkers, func(rows []store.FragRow) error {
			for _, r := range rows {
				found[r.Name] = r
			}
			return a.Store.UpsertFragment(ctx, rows)
		}, nil)
	}
	for _, idea := range plan.Ideas {
		if exclude[idea] {
			continue
		}
		row, ok := found[idea]
		if !ok {
			if inPicks(picks, idea) {
				continue
			}
			if u, _ := a.Store.Get(ctx, idea); u != nil { // already in the DB, shown via picks or not a fit
				continue
			}
			ideas = append(ideas, suggestIdea{Name: idea, Status: "unknown"})
			continue
		}
		u, _ := a.Store.Get(ctx, idea)
		ideas = append(ideas, suggestIdea{Name: idea, Status: row.Status, Item: u})
	}

	a.charge(ctx, "ask", total, b.Brief)
	writeJSON(w, map[string]any{
		"credits":  a.balance(ctx),
		"summary":  plan.Summary,
		"keywords": plan.Keywords,
		"picks":    picks,
		"ideas":    ideas,
		"searched": len(order),
	})
}

func inPicks(picks []suggestPick, name string) bool {
	for _, p := range picks {
		if p.Item.Name == name {
			return true
		}
	}
	return false
}

// availability describes how a name can be had, for the ranking prompt.
func availability(u store.Username) string {
	switch u.Status {
	case "auction":
		s := "auction"
		if u.Price != nil {
			s += fmt.Sprintf(", bid %g TON", *u.Price)
		}
		if u.Buyout != nil {
			s += fmt.Sprintf(", buy now %g TON", *u.Buyout)
		}
		return s
	case "sale":
		return fmt.Sprintf("for sale %g TON", ptrf(u.Price))
	case "market":
		return fmt.Sprintf("listed on %s %g TON", deref(u.ChainMarket, "a marketplace"), ptrf(u.Price))
	default:
		return "owned, offer only"
	}
}

func ptrf(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
