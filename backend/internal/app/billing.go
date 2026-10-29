package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"fragwatch/internal/auth"
	"fragwatch/internal/i18n"
	"fragwatch/internal/jobs"
	"fragwatch/internal/store"
)

// Billing: packs bought through payment gateways. A gateway turns a pack into a price in its own
// currency and tells the client how to pay; the money is then confirmed by the gateway's own
// signal (a TON transfer seen on chain, a Stars successful_payment from Telegram) and delivered by
// store.CompletePayment, which is idempotent. Adding a gateway (USDT on TON, a card processor) is
// one more implementation of gateway plus wherever its confirmation arrives.

// Pack is something to buy. Prices are per gateway; a zero price means "not sold there".
type Pack struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Note    string  `json:"note,omitempty"`
	Credits int     `json:"credits,omitempty"`
	ProDays int     `json:"pro_days,omitempty"`
	TON     float64 `json:"ton"`
	Stars   int     `json:"stars"`
}

var Packs = []Pack{
	{ID: "c1000", Title: "1,000 credits", Credits: 1000, TON: 1, Stars: 150},
	{ID: "c5000", Title: "5,000 credits", Note: "20% off", Credits: 5000, TON: 4, Stars: 600},
	{ID: "c20000", Title: "20,000 credits", Note: "30% off", Credits: 20000, TON: 14, Stars: 2100},
	{ID: "pro30", Title: "Pro, 30 days", Note: "Telegram alerts", ProDays: 30, TON: 2, Stars: 250}, // about $5; or ProCredits from the balance
}

func packByID(id string) (Pack, bool) {
	for _, p := range Packs {
		if p.ID == id {
			return p, true
		}
	}
	return Pack{}, false
}

type gateway interface {
	ID() string
	Label() string
	Enabled() bool
	// Price is the pack's price in the gateway's currency; ok=false when it is not sold there.
	Price(p Pack) (amount float64, currency string, ok bool)
	// TTL is how long a checkout waits for the money.
	TTL() time.Duration
	// Checkout returns what the client needs to pay a freshly created payment.
	Checkout(ctx context.Context, pay *store.Payment, p Pack) (map[string]any, error)
}

func (a *App) gateways() []gateway {
	return []gateway{tonGateway{a}, starsGateway{a}}
}

func (a *App) gateway(id string) gateway {
	for _, g := range a.gateways() {
		if g.ID() == id && g.Enabled() {
			return g
		}
	}
	return nil
}

func (a *App) billingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/billing/packs", a.listPacks)
	mux.HandleFunc("POST /api/billing/checkout", a.checkout)
	mux.HandleFunc("GET /api/billing/payments", func(w http.ResponseWriter, r *http.Request) {
		ps, err := a.Store.Payments(r.Context(), uid(r.Context()), 50)
		if err != nil {
			httpErr(w, err, 500)
			return
		}
		writeJSON(w, map[string]any{"payments": ps})
	})
	mux.HandleFunc("GET /api/billing/payments/{id}", a.getPayment)
	mux.HandleFunc("POST /api/billing/pro", a.buyProWithCredits)
}

// buyProWithCredits pays for Pro from the balance: credits they bought are as good as money here.
func (a *App) buyProWithCredits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch err := a.Store.BuyProWithCredits(ctx, uid(ctx), ProCredits, ProDays); {
	case errors.Is(err, store.ErrShort):
		httpErr(w, apiError("not enough credits: pro costs "+itoa(ProCredits)), 402)
		return
	case err != nil:
		httpErr(w, err, 500)
		return
	}
	a.Store.Log(ctx, uid(ctx), 0, "pay.credits", fmt.Sprintf("pro %dd for %d credits", ProDays, ProCredits))
	u, _ := a.Store.UserByID(ctx, uid(ctx))
	writeJSON(w, map[string]any{"user": a.userJSON(u), "credits": a.creditsResp(withUser(ctx, u))})
}

func (a *App) listPacks(w http.ResponseWriter, r *http.Request) {
	type gw struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	gws := []gw{}
	for _, g := range a.gateways() {
		if g.Enabled() {
			gws = append(gws, gw{g.ID(), g.Label()})
		}
	}
	writeJSON(w, map[string]any{"packs": Packs, "gateways": gws})
}

func (a *App) checkout(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Pack    string `json:"pack"`
		Gateway string `json:"gateway"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpErr(w, err, 400)
		return
	}
	ctx := r.Context()
	pack, ok := packByID(b.Pack)
	g := a.gateway(b.Gateway)
	if !ok || g == nil {
		httpErr(w, apiError("unknown pack or payment method"), 400)
		return
	}
	amount, currency, ok := g.Price(pack)
	if !ok {
		httpErr(w, apiError("this pack is not sold for "+g.Label()), 400)
		return
	}
	pay := store.Payment{UserID: uid(ctx), Gateway: g.ID(), Pack: pack.ID, Credits: pack.Credits, ProDays: pack.ProDays,
		Amount: amount, Currency: currency, Memo: "fw-" + strings.ToLower(auth.Token(6))}
	id, err := a.Store.CreatePayment(ctx, pay, g.TTL())
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	p, _ := a.Store.Payment(ctx, id)
	how, err := g.Checkout(ctx, p, pack)
	if err != nil {
		httpErr(w, err, 502)
		return
	}
	a.Store.Log(ctx, uid(ctx), 0, "pay.checkout", fmt.Sprintf("%s: %g %s via %s", pack.ID, amount, currency, g.ID()))
	writeJSON(w, map[string]any{"payment": p, "pay": how})
}

// getPayment is what the checkout screen polls. A TON payment still waiting nudges the watcher,
// so the credits show up within seconds of the transfer landing.
func (a *App) getPayment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	p, err := a.Store.Payment(ctx, id)
	if err != nil || p == nil || p.UserID != uid(ctx) {
		httpErr(w, errNotFound, 404)
		return
	}
	if p.Status == "pending" && p.Gateway == "ton" {
		a.Jobs.Trigger("payments")
	}
	u, _ := a.Store.UserByID(ctx, p.UserID)
	writeJSON(w, map[string]any{"payment": p, "user": a.userJSON(u)})
}

// --- TON: a transfer to our own wallet carrying the checkout's comment ---

type tonGateway struct{ a *App }

func (g tonGateway) ID() string         { return "ton" }
func (g tonGateway) Label() string      { return "TON" }
func (g tonGateway) Enabled() bool      { return g.a.Cfg.TonWallet != "" }
func (g tonGateway) TTL() time.Duration { return 2 * time.Hour }

func (g tonGateway) Price(p Pack) (float64, string, bool) { return p.TON, "TON", p.TON > 0 }

func (g tonGateway) Checkout(ctx context.Context, pay *store.Payment, p Pack) (map[string]any, error) {
	nano := strconv.FormatInt(int64(math.Round(pay.Amount*1e9)), 10)
	q := "amount=" + nano + "&text=" + url.QueryEscape(pay.Memo)
	addr := g.a.Cfg.TonWallet
	return map[string]any{
		"address": addr, "amount": pay.Amount, "comment": pay.Memo,
		// ton:// opens any wallet app on the device; Tonkeeper's link also works from a desktop browser
		"ton_link":       "ton://transfer/" + addr + "?" + q,
		"tonkeeper_link": "https://app.tonkeeper.com/transfer/" + addr + "?" + q,
	}, nil
}

// runPayments is the job that watches the wallet for transfers matching open TON checkouts.
func (a *App) runPayments(ctx context.Context, j *jobs.Job) error {
	if a.Cfg.TonWallet == "" {
		j.Progress("off: set TON_WALLET in .env")
		return nil
	}
	pending, err := a.Store.PendingPayments(ctx, "ton")
	if err != nil || len(pending) == 0 {
		j.Progress("no open TON checkouts")
		return err
	}
	byMemo := map[string]*store.Payment{}
	for _, p := range pending {
		byMemo[strings.ToLower(p.Memo)] = p
	}
	txs, err := a.Chain.Incoming(ctx, a.Cfg.TonWallet, 100)
	if err != nil {
		return err
	}
	paid := 0
	for _, t := range txs {
		p := byMemo[strings.ToLower(t.Comment)]
		if p == nil || t.Nano < int64(math.Round(p.Amount*1e9)) {
			continue
		}
		ok, err := a.Store.CompletePayment(ctx, p.ID, t.Hash)
		if err != nil {
			return fmt.Errorf("payment %d: %w", p.ID, err)
		}
		if ok {
			paid++
			a.notifyPaid(ctx, p.ID)
		}
	}
	j.Progress("%d open TON checkout%s, %d paid now", len(pending), plural(len(pending)), paid)
	return nil
}

// --- Telegram Stars: an invoice link the bot issues; the bot sees the payment ---

type starsGateway struct{ a *App }

func (g starsGateway) ID() string    { return "stars" }
func (g starsGateway) Label() string { return "Telegram Stars" }
func (g starsGateway) Enabled() bool {
	return g.a.Cfg.StarsEnabled && g.a.botName() != "" && g.a.Cfg.BotPoll
}
func (g starsGateway) TTL() time.Duration { return 24 * time.Hour }

func (g starsGateway) Price(p Pack) (float64, string, bool) {
	return float64(p.Stars), "XTR", p.Stars > 0
}

func (g starsGateway) Checkout(ctx context.Context, pay *store.Payment, p Pack) (map[string]any, error) {
	// the invoice is shown in Telegram, so it speaks the payer's language
	l := g.a.userLang(ctx, pay.UserID)
	title, desc := p.Title, p.Title+" for TonWatch"
	if p.Credits > 0 {
		n := i18n.Num(l, p.Credits)
		title = i18n.T(l, "inv_title_credits", n, i18n.N(l, "w_credits_short", p.Credits))
		desc = i18n.T(l, "inv_credits", n, i18n.N(l, "w_credits", p.Credits))
	} else if p.ProDays > 0 {
		days := i18n.N(l, "w_days", p.ProDays)
		title = i18n.T(l, "inv_title_pro", p.ProDays, days)
		desc = i18n.T(l, "inv_pro", p.ProDays, days)
	}
	body, err := g.a.tgCall(ctx, "createInvoiceLink", map[string]any{
		"title": title, "description": desc, "payload": starsPayload(pay.ID), "currency": "XTR",
		"prices": []map[string]any{{"label": title, "amount": int(pay.Amount)}},
	})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result string `json:"result"`
	}
	json.Unmarshal(body, &resp)
	return map[string]any{"invoice_link": resp.Result}, nil
}
