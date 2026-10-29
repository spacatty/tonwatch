package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"fragwatch/internal/llm"
	"fragwatch/internal/store"
)

func (a *App) aiRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/ai/expand", a.aiExpand)
	a.aiFilterRoutes(mux)
	mux.HandleFunc("POST /api/ai/suggest", a.aiSuggest)
	mux.HandleFunc("GET /api/ai/chat", a.aiChat)
	mux.HandleFunc("POST /api/ai/chat", a.aiAsk)
	mux.HandleFunc("DELETE /api/ai/chat", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Store.ClearAITurns(r.Context(), uid(r.Context())); err != nil {
			httpErr(w, err, 500)
			return
		}
		writeJSON(w, map[string]any{"turns": []store.AITurn{}})
	})
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
	var b suggestReq
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || strings.TrimSpace(b.Brief) == "" {
		httpErr(w, apiError("brief is required"), 400)
		return
	}
	b.clean()
	if a.paywall(w, r, "ask") {
		return
	}
	res, err := a.suggest(r.Context(), b)
	if err != nil {
		httpErr(w, err, 502)
		return
	}
	writeJSON(w, res)
}

type suggestReq struct {
	Brief        string   `json:"brief"`
	MaxPrice     *float64 `json:"max_price,omitempty"`
	BuyableOnly  bool     `json:"buyable_only"`
	ExcludeNames []string `json:"exclude,omitempty"` // names already shown, for "more like this"
}

// clean bounds what one request may send: the brief goes into three prompts, the exclusions into a map.
func (b *suggestReq) clean() {
	b.Brief = clip(b.Brief, 1000)
	if len(b.ExcludeNames) > 200 {
		b.ExcludeNames = b.ExcludeNames[:200]
	}
}

// maxCandidates caps the lines sent to ranking: listed keyword matches first, then matches by
// meaning, then the rest of the keyword matches.
const maxCandidates = 220

func (a *App) aiChat(w http.ResponseWriter, r *http.Request) {
	turns, err := a.Store.AITurns(r.Context(), uid(r.Context()), 30)
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	writeJSON(w, map[string]any{"turns": turns})
}

// aiAsk saves the question and answers it in the background: the request returns at once, and
// the answer lands in the conversation whether or not the page that asked is still open.
func (a *App) aiAsk(w http.ResponseWriter, r *http.Request) {
	var b suggestReq
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || strings.TrimSpace(b.Brief) == "" {
		httpErr(w, apiError("brief is required"), 400)
		return
	}
	b.clean()
	ctx := r.Context()
	if a.Store.PendingAITurns(ctx, uid(ctx)) >= 2 {
		httpErr(w, apiError("wait for the answer you already asked for"), 429)
		return
	}
	if a.paywall(w, r, "ask") {
		return
	}
	id, err := a.Store.AddAITurn(ctx, uid(ctx), b.Brief, map[string]any{"buyable_only": b.BuyableOnly, "max_price": b.MaxPrice})
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	// detached from the request (it outlives it) but keeps the account and the language
	bg := context.WithoutCancel(ctx)
	go func() {
		res, err := a.suggest(bg, b)
		why := ""
		if err != nil {
			why = err.Error()
		}
		if err := a.Store.FinishAITurn(bg, id, res, why); err != nil {
			log.Printf("ai chat: save turn %d: %v", id, err)
		}
	}()
	a.aiChat(w, r)
}

// suggest is one consultation, charged to the account in ctx.
func (a *App) suggest(ctx context.Context, b suggestReq) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()

	plan, usage, err := a.LLM.PlanSearch(ctx, b.Brief)
	a.addUsage(usage)
	if err != nil {
		return nil, fmt.Errorf("planning: %w", err)
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
	var lists [][]store.Username
	for _, f := range queries {
		items, _, err := a.query(ctx, f)
		if err != nil {
			return nil, err
		}
		lists = append(lists, items)
	}
	// Names close in meaning to the request, from the semantic index: they find "harbor" for a
	// shipping project even when no keyword stem is inside the name. Best effort: no index, no names.
	if near, usage, err := a.semantic(ctx, b.Brief+"\n"+plan.Summary, 150); err != nil {
		log.Printf("ask: semantic search: %v", err)
	} else if len(near) > 0 {
		a.addUsage(usage)
		total.Cost += usage.Cost
		f := base
		f.Keywords, f.Names, f.Limit = nil, near, len(near)
		if b.BuyableOnly {
			f.Statuses = listed.Statuses
		}
		items, _, err := a.query(ctx, f)
		if err != nil {
			return nil, err
		}
		rank := map[string]int{}
		for i, n := range near {
			rank[n] = i
		}
		sort.SliceStable(items, func(i, j int) bool { return rank[items[i].Name] < rank[items[j].Name] })
		if len(items) > 90 {
			items = items[:90]
		}
		// after the listed keyword matches, before the rest
		lists = append(lists[:1], append([][]store.Username{items}, lists[1:]...)...)
	}
	byName := map[string]store.Username{}
	var order []string
	for _, items := range lists {
		for _, u := range items {
			if _, ok := byName[u.Name]; !ok && !exclude[u.Name] && len(order) < maxCandidates {
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
			return nil, fmt.Errorf("ranking: %w", err)
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
	return map[string]any{
		"credits":  a.balance(ctx),
		"summary":  plan.Summary,
		"keywords": plan.Keywords,
		"picks":    picks,
		"ideas":    ideas,
		"searched": len(order),
	}, nil
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
