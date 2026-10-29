package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
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
	"filter":  1,  // AI filter: a request in words → table rules
}

// ProCredits is Pro for ProDays paid from the balance instead of a gateway: a bit cheaper than
// the pro30 pack, so buying credits first is never the worse deal.
const (
	ProCredits = 1500
	ProDays    = 30
)

const ledgerMax = 60

var ErrNoCredits = errors.New("not enough credits")

type CreditsResp struct {
	Balance int                 `json:"balance"`
	Daily   int                 `json:"daily"`
	Full    int                 `json:"full_daily"` // the allowance once they have paid
	Pro     map[string]int      `json:"pro"`        // Pro from the balance: credits, days
	Claim   ClaimInfo           `json:"claim"`
	Prices  map[string]int      `json:"prices"`
	Ledger  []store.LedgerEntry `json:"ledger"`
}

// ClaimInfo is today's free allowance: whether it is still there to claim, what claiming adds,
// and the Turnstile site key the claim must pass (empty = press-and-hold).
type ClaimInfo struct {
	Available bool   `json:"available"`
	Adds      int    `json:"adds"`
	Turnstile string `json:"turnstile,omitempty"`
}

// balance is the signed-in account's credits. The free allowance is not added here: they claim it.
func (a *App) balance(ctx context.Context) int {
	if id := uid(ctx); id != 0 {
		return a.Store.Credits(ctx, id)
	}
	return 0
}

func today() string { return time.Now().Format("2006-01-02") }

func (a *App) claimInfo(ctx context.Context, bal int) ClaimInfo {
	return ClaimInfo{Available: a.Store.RefillDay(ctx, uid(ctx)) != today(), Adds: max(0, a.daily(ctx)-bal), Turnstile: a.Cfg.TurnstileSite}
}

// daily is the account's allowance: the full one once they have paid, a taste before that.
func (a *App) daily(ctx context.Context) int {
	if currentUser(ctx).Unlocked() {
		return a.Cfg.DailyCredits
	}
	return a.Cfg.FreeDailyCredits
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
	a.Store.Log(ctx, uid(ctx), 0, "ai."+feature, note)
}

func (a *App) creditsResp(ctx context.Context) CreditsResp {
	led, _ := a.Store.Ledger(ctx, uid(ctx), ledgerMax)
	if led == nil {
		led = []store.LedgerEntry{}
	}
	bal := a.balance(ctx)
	return CreditsResp{Balance: bal, Daily: a.daily(ctx), Full: a.Cfg.DailyCredits, Claim: a.claimInfo(ctx, bal),
		Pro: map[string]int{"credits": ProCredits, "days": ProDays}, Prices: Prices, Ledger: led}
}

func (a *App) creditsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/credits", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.creditsResp(r.Context())) })
	mux.HandleFunc("POST /api/credits/claim", a.claimDaily)
}

// claimDaily hands out today's allowance to someone who is here: the app asks them to pass a
// Turnstile check (or hold a button) and sends the result along.
func (a *App) claimDaily(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var b struct {
		Token string `json:"token"`
	}
	json.NewDecoder(r.Body).Decode(&b)
	if a.Cfg.TurnstileSecret != "" {
		if err := a.turnstile(ctx, b.Token, clientIP(r)); err != nil {
			httpErr(w, err, 403)
			return
		}
	}
	add, err := a.Store.ClaimDaily(ctx, uid(ctx), a.daily(ctx), today())
	switch {
	case errors.Is(err, store.ErrClaimed):
		httpErr(w, apiError("already claimed today"), 409)
		return
	case err != nil:
		httpErr(w, err, 500)
		return
	}
	a.Store.Log(ctx, uid(ctx), 0, "credits.claim", "+"+itoa(add))
	writeJSON(w, map[string]any{"added": add, "credits": a.creditsResp(ctx)})
}

// turnstile asks Cloudflare whether the widget's token is a fresh pass from a person.
func (a *App) turnstile(ctx context.Context, token, ip string) error {
	if token == "" {
		return apiError("the human check is missing")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	form := url.Values{"secret": {a.Cfg.TurnstileSecret}, "response": {token}, "remoteip": {ip}}
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return apiError("the human check could not be verified; try again")
	}
	defer resp.Body.Close()
	var out struct {
		Success bool `json:"success"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil || !out.Success {
		return apiError("the human check failed; try again")
	}
	return nil
}

// paywall turns a reserve failure into the 402 the UI shows as "top up".
func (a *App) paywall(w http.ResponseWriter, r *http.Request, feature string) bool {
	if err := a.reserve(r.Context(), feature); err != nil {
		httpErr(w, apiError("not enough credits: "+feature+" costs "+itoa(Prices[feature])), 402)
		return true
	}
	return false
}
