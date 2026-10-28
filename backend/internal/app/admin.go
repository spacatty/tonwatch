package app

import (
	"encoding/json"
	"net/http"
	"strconv"

	"fragwatch/internal/store"
)

// Admin: the people side of the install (accounts, plans, credits, payments). The data side
// (collectors, re-rating, crawler stats) lives on the existing /api/jobs and /api/llm routes,
// which are admin-only too (see access()).
func (a *App) adminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/overview", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		st, _ := a.Store.Stats(ctx)
		writeJSON(w, map[string]any{
			"users": a.Store.CountUsers(ctx), "paid": a.Store.PaidTotals(ctx), "llm_usage": a.LLMUsage(), "stats": st,
			"gateways": map[string]bool{"ton": a.gateway("ton") != nil, "stars": a.gateway("stars") != nil},
			"bot": a.botName(),
		})
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
	// {"role":"admin"|"user", "plan":"pro"|"free", "grant": 500}: any subset
	mux.HandleFunc("POST /api/admin/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		u, err := a.Store.UserByID(ctx, id)
		if err != nil || u == nil {
			httpErr(w, errNotFound, 404)
			return
		}
		var b struct {
			Role  string `json:"role"`
			Plan  string `json:"plan"`
			Grant int    `json:"grant"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			httpErr(w, err, 400)
			return
		}
		if b.Role == "admin" || b.Role == "user" {
			if b.Role == "user" && id == uid(ctx) {
				httpErr(w, apiError("you cannot take admin from yourself"), 400)
				return
			}
			a.Store.SetRole(ctx, id, b.Role)
		}
		if b.Plan == "pro" || b.Plan == "free" {
			a.Store.SetPlan(ctx, id, b.Plan)
		}
		if b.Grant != 0 {
			if b.Grant < -1_000_000 || b.Grant > 1_000_000 {
				httpErr(w, apiError("grant must be within ±1,000,000"), 400)
				return
			}
			a.Store.AddCredits(ctx, id, store.LedgerEntry{Feature: "grant", Credits: b.Grant, Note: "by " + currentUser(ctx).Label()})
		}
		u, _ = a.Store.UserByID(ctx, id)
		writeJSON(w, map[string]any{"user": a.userJSON(u)})
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
