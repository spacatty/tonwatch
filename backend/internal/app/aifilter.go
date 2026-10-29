package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"fragwatch/internal/i18n"
	"fragwatch/internal/llm"
	"fragwatch/internal/store"
)

const (
	kvAIFilters   = "ai:filters" // the account's recent AI filters, newest first
	aiFiltersKept = 12
)

// aiFilter is one request turned into table rules: the view, the filters (in the shape the
// dashboard keeps them, so they drop straight into the table, a tracker or the chips) and a
// one-line restatement shown above the list.
type aiFilter struct {
	Prompt  string         `json:"prompt"`
	View    string         `json:"view"`
	Filters map[string]any `json:"filters"`
	Summary  string         `json:"summary"`
	CouldNot string         `json:"could_not,omitempty"` // what the request asked for that no rule can say
	At       int64          `json:"at"`
}

func (a *App) aiFilterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/ai/filter", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"recent": a.recentAIFilters(r)})
	})
	mux.HandleFunc("POST /api/ai/filter", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || strings.TrimSpace(b.Prompt) == "" {
			httpErr(w, apiError("describe what to look for"), 400)
			return
		}
		prompt := clip(strings.Join(strings.Fields(b.Prompt), " "), 500)
		ctx := r.Context()
		// the same request in the same language always reads into the same rules: answered once, free after
		key := aiFilterCacheKey(i18n.From(ctx), prompt)
		var f aiFilter
		if cached := a.Store.GetKV(ctx, key); cached != "" && json.Unmarshal([]byte(cached), &f) == nil {
			f.Prompt, f.At = prompt, time.Now().Unix()
			a.rememberAIFilter(r, f)
			writeJSON(w, map[string]any{"filter": f, "cached": true, "credits": a.balance(ctx)})
			return
		}
		if a.paywall(w, r, "filter") {
			return
		}
		pctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		spec, usage, err := a.LLM.ParseFilter(pctx, prompt)
		a.addUsage(usage)
		if err != nil {
			httpErr(w, err, 502)
			return
		}
		a.charge(ctx, "filter", usage, prompt)
		view, filters := filterFromSpec(spec)
		f = aiFilter{Prompt: prompt, View: view, Filters: filters, Summary: clip(spec.Summary, 160), CouldNot: clip(spec.CouldNot, 200), At: time.Now().Unix()}
		if js, err := json.Marshal(f); err == nil {
			a.Store.SetKV(ctx, key, string(js))
		}
		a.rememberAIFilter(r, f)
		writeJSON(w, map[string]any{"filter": f, "credits": a.balance(ctx)})
	})
}

// aiFilterCacheKey keys the shared cache of AI filters by the prompt's version, the reader's
// language (the summary is written in it) and the request with case, spacing and end
// punctuation evened out.
func aiFilterCacheKey(lang, prompt string) string {
	norm := strings.TrimRight(strings.ToLower(strings.Join(strings.Fields(prompt), " ")), ".!?…, ")
	h := sha256.Sum256([]byte(norm))
	return "aifilter:" + llm.FilterPromptVersion() + ":" + lang + ":" + hex.EncodeToString(h[:12])
}

func (a *App) recentAIFilters(r *http.Request) []aiFilter {
	var out []aiFilter
	json.Unmarshal([]byte(a.uget(r.Context(), kvAIFilters)), &out)
	if out == nil {
		out = []aiFilter{}
	}
	return out
}

func (a *App) rememberAIFilter(r *http.Request, f aiFilter) {
	recent := slices.DeleteFunc(a.recentAIFilters(r), func(x aiFilter) bool { return strings.EqualFold(x.Prompt, f.Prompt) })
	recent = append([]aiFilter{f}, recent...)
	if len(recent) > aiFiltersKept {
		recent = recent[:aiFiltersKept]
	}
	js, _ := json.Marshal(recent)
	a.uset(r.Context(), kvAIFilters, string(js))
}

var aiViews = []string{"buyable", "auction", "ending", "sale", "market", "owned", "all"}
var aiSorts = map[string]string{"score": "desc", "buyout": "asc", "price": "asc", "ending": "asc", "len": "asc", "recent": "desc", "value": "asc"}

// filterFromSpec checks everything the model said and keeps only what fits: kinds from the
// list, numbers in range, a known view and sort. Anything left unsaid stays at the defaults.
func filterFromSpec(s llm.FilterSpec) (string, map[string]any) {
	view := "buyable"
	if slices.Contains(aiViews, s.View) {
		view = s.View
	}
	f := map[string]any{}
	if s.MinScore != nil {
		f["minScore"] = min(10, max(0, *s.MinScore))
		f["unscored"] = *s.MinScore == 0
	}
	kinds := func(in []string) []string {
		out := []string{}
		for _, k := range in {
			if k = strings.ToLower(strings.TrimSpace(k)); slices.Contains(llm.Categories, k) && !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
		return out
	}
	if ks := kinds(s.Kinds); len(ks) > 0 {
		f["categories"] = ks
	}
	if ks := kinds(s.NotKinds); len(ks) > 0 {
		f["notCategories"] = ks
	}
	money := func(key string, v *float64) {
		if v != nil && *v > 0 && *v < 1e7 {
			f[key] = strconv.FormatFloat(*v, 'f', -1, 64)
		}
	}
	money("maxBuyout", s.MaxBuyout)
	money("minBuyout", s.MinBuyout)
	money("maxPrice", s.MaxPrice)
	if s.MinLen != nil || s.MaxLen != nil {
		lo, hi := 4, 32
		if s.MinLen != nil {
			lo = min(32, max(4, *s.MinLen))
		}
		if s.MaxLen != nil {
			hi = min(32, max(4, *s.MaxLen))
		}
		if lo <= hi {
			f["len"] = []int{lo, hi}
		}
	}
	if s.NoDigits {
		f["noDigits"] = true
	}
	if s.NoUnder {
		f["noUnder"] = true
	}
	if s.BuyNow {
		f["buyNow"] = true
	}
	if s.EndingIn != nil {
		for _, w := range []int64{900, 3600, 21600, 86400} { // the windows the ending chip offers
			if *s.EndingIn <= w {
				f["endingIn"] = strconv.FormatInt(w, 10)
				if view == "buyable" || view == "all" {
					view = "auction"
				}
				break
			}
		}
	}
	if s.QuietDays != nil && *s.QuietDays > 0 {
		f["quietDays"] = strconv.Itoa(min(90, *s.QuietDays))
	}
	if len(s.Keywords) > 0 {
		f["keywords"] = s.Keywords
		f["topic"] = clip(s.Summary, 40)
	}
	if q := strings.ToLower(strings.TrimSpace(s.Query)); q != "" && len(q) <= 33 && strings.Trim(q, "^$abcdefghijklmnopqrstuvwxyz0123456789_") == "" {
		f["q"] = q
	}
	// the tree is checked like any other rules; one the store would refuse is left out entirely
	if len(s.Rules) > 0 && string(s.Rules) != "null" {
		if r, err := store.ParseRules(string(s.Rules)); err == nil && r != nil && (r.Field != "" || len(r.Rules) > 0) {
			f["rules"] = r.Simplify()
		}
	}
	if dir, ok := aiSorts[s.Sort]; ok {
		f["sorts"] = []map[string]string{{"col": s.Sort, "dir": dir}}
	}
	return view, f
}
