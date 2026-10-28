package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"fragwatch/internal/llm"
	"fragwatch/internal/store"
)

// Credits meter every AI feature. Prices sit well above token cost, so the hosted version has
// margin; the real cost is logged next to each charge to tune them. Cached answers are free.
var Prices = map[string]int{
	"ideas":   10, // one run of concepts over ~110 names
	"rotate":  3,  // replace one concept
	"refine":  0,  // bend one concept their way ("I'd say …"): free, it makes the idea theirs
	"imagine": 2,  // one full project for one name
	"chat":    2,  // one message in a concept's chat (the thread and the concept ride along)
	"angle":   1,  // "why this name" on a card
	"scan":    5,  // "why" for a whole grid (up to 12 names) in one call
	"profile": 2,  // taste profile, once per change of taste
	"ask":     5,  // Ask AI: plan + rank
	"topic":   1,  // topic → stems
}

const ledgerMax = 60

var ErrNoCredits = errors.New("not enough credits")

type CreditsResp struct {
	Balance int                 `json:"balance"`
	Daily   int                 `json:"daily"`
	Prices  map[string]int      `json:"prices"`
	Ledger  []store.LedgerEntry `json:"ledger"`
}

// balance is the signed-in account's credits, after topping up to the daily allowance once a day
// (the free tier). Bought credits sit on top of it.
func (a *App) balance(ctx context.Context) int {
	id := uid(ctx)
	if id == 0 {
		return 0
	}
	a.Store.Refill(ctx, id, a.Cfg.DailyCredits, time.Now().Format("2006-01-02"))
	return a.Store.Credits(ctx, id)
}

func itoa(n int) string {
	js, _ := json.Marshal(n)
	return string(js)
}

// reserve checks the price up front so a run never starts that cannot be paid for.
func (a *App) reserve(ctx context.Context, feature string) error {
	if a.balance(ctx) < Prices[feature] {
		return ErrNoCredits
	}
	return nil
}

// charge debits after the work is done and writes the ledger line with the real token spend.
func (a *App) charge(ctx context.Context, feature string, usage llm.Usage, note string) {
	a.Store.Spend(ctx, uid(ctx), store.LedgerEntry{Feature: feature, Credits: Prices[feature],
		Tokens: usage.PromptTokens + usage.CompletionTokens, Cached: usage.CachedTokens, Cost: usage.Cost, Note: note})
}

func (a *App) creditsResp(ctx context.Context) CreditsResp {
	led, _ := a.Store.Ledger(ctx, uid(ctx), ledgerMax)
	if led == nil {
		led = []store.LedgerEntry{}
	}
	return CreditsResp{Balance: a.balance(ctx), Daily: a.Cfg.DailyCredits, Prices: Prices, Ledger: led}
}

func (a *App) creditsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/credits", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.creditsResp(r.Context())) })
}

// paywall turns a reserve failure into the 402 the UI shows as "top up".
func (a *App) paywall(w http.ResponseWriter, r *http.Request, feature string) bool {
	if err := a.reserve(r.Context(), feature); err != nil {
		httpErr(w, apiError("not enough credits: "+feature+" costs "+itoa(Prices[feature])), 402)
		return true
	}
	return false
}
