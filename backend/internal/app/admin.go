package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"fragwatch/internal/store"
)

// Admin: the people side of the install (accounts, plans, credits, payments, activity). The data
// side (collectors, re-rating, crawler stats) lives on the existing /api/jobs and /api/llm routes,
// which are admin-only too (see access()).
func (a *App) adminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/overview", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		st, _ := a.Store.Stats(ctx)
		writeJSON(w, map[string]any{
			"users": a.Store.CountUsers(ctx), "totals": a.Store.UserTotals(ctx), "paid": a.Store.PaidTotals(ctx), "llm_usage": a.LLMUsage(), "stats": st,
			"gateways": map[string]bool{"ton": a.gateway("ton") != nil, "stars": a.gateway("stars") != nil},
			"bot":      a.botName(), "daily_credits": a.Cfg.DailyCredits, "support_waiting": a.Store.SupportWaiting(ctx),
		})
	})
	// ?days=30: daily series for the charts, plus where the AI spend and the activity went
	mux.HandleFunc("GET /api/admin/stats", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		days, _ := strconv.Atoi(r.URL.Query().Get("days"))
		if days < 7 || days > 180 {
			days = 30
		}
		since := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -(days - 1)).Unix()
		daily, err := a.Store.Daily(ctx, days)
		if err != nil {
			httpErr(w, err, 500)
			return
		}
		features, _ := a.Store.FeatureUsage(ctx, since)
		kinds, _ := a.Store.ActivityKinds(ctx, since)
		writeJSON(w, map[string]any{"days": days, "daily": daily, "features": features, "kinds": kinds})
	})
	mux.HandleFunc("GET /api/admin/users", func(w http.ResponseWriter, r *http.Request) {
		us, err := a.Store.ListUsers(r.Context(), r.URL.Query().Get("q"), 200)
		if err != nil {
			httpErr(w, err, 500)
			return
		}
		out := make([]map[string]any, 0, len(us))
		for _, u := range us {
			j := a.userJSON(u)
			j["seen_at"] = u.SeenAt
			out = append(out, j)
		}
		writeJSON(w, map[string]any{"users": out})
	})
	// one account in full: profile, what it made, its money and its trail
	mux.HandleFunc("GET /api/admin/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		u, err := a.Store.UserByID(ctx, id)
		if err != nil || u == nil {
			httpErr(w, errNotFound, 404)
			return
		}
		j := a.userJSON(u)
		j["seen_at"], j["login"], j["email"], j["tg_2fa"] = u.SeenAt, u.Login, u.Email, u.TG2FA
		led, _ := a.Store.Ledger(ctx, id, 200)
		pays, _ := a.Store.Payments(ctx, id, 100)
		act, _ := a.Store.Activity(ctx, store.ActivityFilter{UserID: id, Limit: 200})
		writeJSON(w, map[string]any{"user": j, "footprint": a.Store.UserFootprint(ctx, id), "ledger": led, "payments": pays, "activity": act})
	})
	// {"role":"admin"|"user", "plan":"pro"|"free", "grant": 500, "set_credits": 1000, "pro_days": 30, "note": "why"}: any subset
	mux.HandleFunc("POST /api/admin/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		me := currentUser(ctx)
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		u, err := a.Store.UserByID(ctx, id)
		if err != nil || u == nil {
			httpErr(w, errNotFound, 404)
			return
		}
		var b struct {
			Role       string `json:"role"`
			Plan       string `json:"plan"`
			Grant      int    `json:"grant"`
			SetCredits *int   `json:"set_credits"`
			ProDays    int    `json:"pro_days"`
			Note       string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			httpErr(w, err, 400)
			return
		}
		if b.Grant < -1_000_000 || b.Grant > 1_000_000 {
			httpErr(w, apiError("grant must be within ±1,000,000"), 400)
			return
		}
		if b.SetCredits != nil && (*b.SetCredits < 0 || *b.SetCredits > 10_000_000) {
			httpErr(w, apiError("balance must be between 0 and 10,000,000"), 400)
			return
		}
		if b.ProDays < -3650 || b.ProDays > 3650 {
			httpErr(w, apiError("Pro days must be within ±3650"), 400)
			return
		}
		if b.Role == "user" && id == me.ID {
			httpErr(w, apiError("you cannot take admin from yourself"), 400)
			return
		}
		note := "by " + me.Label()
		if n := clip(b.Note, 200); n != "" {
			note += ": " + n
		}
		log := func(kind, detail string) { a.Store.Log(ctx, id, me.ID, kind, detail) }
		if (b.Role == "admin" || b.Role == "user") && b.Role != u.Role {
			a.Store.SetRole(ctx, id, b.Role)
			log("admin.role", u.Role+" → "+b.Role)
		}
		if (b.Plan == "pro" || b.Plan == "free") && b.Plan != u.PlanRaw {
			a.Store.SetPlan(ctx, id, b.Plan)
			log("admin.plan", map[string]string{"pro": "Pro granted", "free": "Pro grant removed"}[b.Plan])
		}
		if b.ProDays != 0 {
			if err := a.Store.AddProDays(ctx, id, b.ProDays); err != nil {
				httpErr(w, err, 500)
				return
			}
			log("admin.pro_days", fmt.Sprintf("%+d days", b.ProDays))
		}
		if b.SetCredits != nil {
			d, err := a.Store.SetCredits(ctx, id, *b.SetCredits, note)
			if err != nil {
				httpErr(w, err, 500)
				return
			}
			if d != 0 {
				log("admin.credits", fmt.Sprintf("set to %d (%+d)%s", *b.SetCredits, d, noteTail(b.Note)))
			}
		} else if b.Grant != 0 {
			a.Store.AddCredits(ctx, id, store.LedgerEntry{Feature: "grant", Credits: b.Grant, Note: note})
			log("admin.credits", fmt.Sprintf("%+d%s", b.Grant, noteTail(b.Note)))
		}
		u, _ = a.Store.UserByID(ctx, id)
		j := a.userJSON(u)
		j["seen_at"] = u.SeenAt
		writeJSON(w, map[string]any{"user": j})
	})
	// sign an account out on every device
	mux.HandleFunc("POST /api/admin/users/{id}/logout", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err := a.Store.DeleteSessions(ctx, id, ""); err != nil {
			httpErr(w, err, 500)
			return
		}
		a.Store.Log(ctx, id, uid(ctx), "admin.logout", "signed out everywhere")
		writeJSON(w, map[string]bool{"ok": true})
	})
	// ?user=&kind=&before=&limit=: the activity feed, newest first
	mux.HandleFunc("GET /api/admin/activity", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		atoi := func(k string) int64 { n, _ := strconv.ParseInt(q.Get(k), 10, 64); return n }
		act, err := a.Store.Activity(r.Context(), store.ActivityFilter{UserID: atoi("user"), Kind: q.Get("kind"), Before: atoi("before"), Limit: int(atoi("limit"))})
		if err != nil {
			httpErr(w, err, 500)
			return
		}
		writeJSON(w, map[string]any{"activity": act})
	})
	mux.HandleFunc("GET /api/admin/payments", func(w http.ResponseWriter, r *http.Request) {
		ps, err := a.Store.Payments(r.Context(), 0, 300)
		if err != nil {
			httpErr(w, err, 500)
			return
		}
		labels := map[int64]string{}
		for _, p := range ps {
			if _, ok := labels[p.UserID]; !ok {
				if u, _ := a.Store.UserByID(r.Context(), p.UserID); u != nil {
					labels[p.UserID] = u.Label()
				}
			}
			p.User = labels[p.UserID]
		}
		writeJSON(w, map[string]any{"payments": ps})
	})
}

func noteTail(n string) string {
	if n = clip(n, 200); n != "" {
		return " · " + n
	}
	return ""
}
