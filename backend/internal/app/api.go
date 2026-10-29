package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"fragwatch/internal/llm"
	"fragwatch/internal/store"
)

var nameRe = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/usernames", a.listUsernames)
	mux.HandleFunc("GET /api/usernames/{name}", a.getUsername)
	mux.HandleFunc("POST /api/usernames/{name}/refresh", a.refreshUsername)
	mux.HandleFunc("POST /api/usernames/{name}/watch", a.watchUsername)
	mux.HandleFunc("POST /api/usernames/{name}/hide", a.hideUsername)
	mux.HandleFunc("GET /api/events", a.listEvents)
	mux.HandleFunc("GET /api/stats", a.stats)
	mux.HandleFunc("GET /api/jobs", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.Jobs.Statuses()) })
	mux.HandleFunc("POST /api/jobs/{name}/run", a.runJob)
	mux.HandleFunc("POST /api/jobs/{name}/stop", func(w http.ResponseWriter, r *http.Request) {
		a.Jobs.Stop(r.PathValue("name"))
		writeJSON(w, map[string]bool{"ok": true})
	})
	// re-rate: forget the recorded facts so the model reads every name again. Non-destructive:
	// scores, kinds and meanings stay until each name's new facts land.
	mux.HandleFunc("POST /api/llm/reset", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Store.ResetFacts(r.Context()); err != nil {
			httpErr(w, err, 500)
			return
		}
		a.Jobs.Trigger("llm")
		writeJSON(w, map[string]bool{"ok": true})
	})
	// rescore: recompute every score from the stored facts with the current formula; no model calls
	mux.HandleFunc("POST /api/llm/rescore", func(w http.ResponseWriter, r *http.Request) {
		started, err := a.Jobs.Trigger("rescore")
		if err != nil {
			httpErr(w, err, 500)
			return
		}
		writeJSON(w, map[string]bool{"ok": true, "started": started})
	})
	mux.HandleFunc("GET /api/meta", func(w http.ResponseWriter, r *http.Request) {
		m := map[string]any{"categories": llm.Categories, "llm_enabled": a.LLM.Enabled(), "probe_min_score": a.Cfg.ProbeMinScore, "chain_off": a.Cfg.SkipChain}
		if currentUser(r.Context()).Admin() {
			m["model"], m["proxies"] = a.LLM.Model, len(a.Cfg.Proxies)
		}
		writeJSON(w, m)
	})
	a.trackerRoutes(mux)
	a.aiRoutes(mux)
	a.studioRoutes(mux)
	a.authRoutes(mux)
	a.avatarRoutes(mux)
	a.billingRoutes(mux)
	a.adminRoutes(mux)
	return cors(a.authenticate(mux))
}

// query and count run a name search with the signed-in account's stars and hidden names applied.
func (a *App) query(ctx context.Context, f store.Filter) ([]store.Username, int, error) {
	f.UserID = uid(ctx)
	return a.Store.Query(ctx, f)
}

func (a *App) count(ctx context.Context, f store.Filter) (int, error) {
	f.UserID = uid(ctx)
	return a.Store.Count(ctx, f)
}

// uget/uset read and write the signed-in account's own key/value state.
func (a *App) uget(ctx context.Context, k string) string { return a.Store.GetUserKV(ctx, uid(ctx), k) }

func (a *App) uset(ctx context.Context, k, v string) { a.Store.SetUserKV(ctx, uid(ctx), k, v) }

func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			return
		}
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func httpErr(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func list(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func fptr(v string) *float64 {
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return &f
	}
	return nil
}

// parseFilter turns /api/usernames query parameters into a store filter. Trackers store the
// same query string, so they are evaluated exactly like the table.
func parseFilter(q url.Values) store.Filter {
	rules, _ := store.ParseRules(q.Get("rules")) // callers that take rules from people check them first with badRules
	atoi := func(k string) int { n, _ := strconv.Atoi(q.Get(k)); return n }
	f := store.Filter{
		Q: q.Get("q"), Statuses: list(q.Get("status")), Categories: list(q.Get("category")), NotCategories: list(q.Get("not_category")),
		Keywords: list(q.Get("keywords")), Digits: q.Get("digits"), DigitMode: q.Get("digit_mode"),
		MinPrice: fptr(q.Get("min_price")), MaxPrice: fptr(q.Get("max_price")),
		MinBuyout: fptr(q.Get("min_buyout")), MaxBuyout: fptr(q.Get("max_buyout")), BuyNow: q.Get("buy_now") == "1",
		MinLen: atoi("min_len"), MaxLen: atoi("max_len"),
		Unscored: q.Get("unscored") == "1", HideJunk: q.Get("hide_junk") == "1",
		NoDigits: q.Get("no_digits") == "1", NoUnder: q.Get("no_under") == "1",
		EndingIn: int64(atoi("ending_in")), QuietDays: atoi("quiet_days"), Rules: rules, Watched: q.Get("watched") == "1", Hidden: q.Get("hidden") == "1", Live: q.Get("live") == "1",
		ChangedSince: int64(atoi("changed_since")),
		Sort:         q.Get("sort"), Dir: q.Get("dir"), Limit: atoi("limit"), Offset: atoi("offset"),
	}
	if v := q.Get("min_score"); v != "" {
		n, _ := strconv.Atoi(v)
		f.MinScore = &n
	}
	return f
}

// badRules rejects a malformed rule tree with a 400: silently dropping it would show everything.
func badRules(w http.ResponseWriter, params string) bool {
	q, _ := url.ParseQuery(params)
	if _, err := store.ParseRules(q.Get("rules")); err != nil {
		httpErr(w, apiError(err.Error()), 400)
		return true
	}
	return false
}

// Accounts that have never paid browse every name, but only the first FreeRows of any search and
// without the power filters. AI features stay open to them: those are paid per use in credits.
const (
	FreeRows     = 300
	FreeTrackers = 1
)

// gate trims a filter to what the account may run and says what it took out ("rules",
// "quiet_days", "keywords", "depth"), so the UI can show it as locked rather than silently missing.
func (a *App) gate(ctx context.Context, f *store.Filter) []string {
	if currentUser(ctx).Unlocked() {
		return nil
	}
	locked := []string{}
	if f.Rules != nil {
		f.Rules, locked = nil, append(locked, "rules")
	}
	if f.QuietDays > 0 {
		f.QuietDays, locked = 0, append(locked, "quiet_days")
	}
	if len(f.Keywords) > 0 {
		f.Keywords, locked = nil, append(locked, "keywords")
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 100
	}
	if f.Offset+f.Limit > FreeRows {
		f.Limit = max(0, FreeRows-f.Offset)
		locked = append(locked, "depth")
	}
	return locked
}

func (a *App) listUsernames(w http.ResponseWriter, r *http.Request) {
	if badRules(w, r.URL.RawQuery) {
		return
	}
	f := parseFilter(r.URL.Query())
	locked := a.gate(r.Context(), &f)
	items := []store.Username{}
	total := 0
	var err error
	if f.Limit > 0 {
		items, total, err = a.query(r.Context(), f)
	} else {
		total, err = a.count(r.Context(), f)
	}
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	out := map[string]any{"items": items, "total": total}
	if locked != nil {
		out["locked"], out["cap"] = locked, FreeRows
	}
	writeJSON(w, out)
}

func cleanName(s string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(s), "@"))
}

func (a *App) getUsername(w http.ResponseWriter, r *http.Request) {
	name := cleanName(r.PathValue("name"))
	u, err := a.Store.GetFor(r.Context(), uid(r.Context()), name)
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	if u == nil {
		httpErr(w, errNotFound, 404)
		return
	}
	ev, _ := a.Store.Events(r.Context(), uid(r.Context()), name, 100)
	writeJSON(w, map[string]any{"item": u, "events": ev})
}

type apiError string

func (e apiError) Error() string { return string(e) }

const errNotFound = apiError("not found")

func (a *App) refreshUsername(w http.ResponseWriter, r *http.Request) {
	name := cleanName(r.PathValue("name"))
	if !nameRe.MatchString(name) {
		httpErr(w, apiError("invalid username"), 400)
		return
	}
	if _, err := a.RefreshOne(r.Context(), name); err != nil {
		httpErr(w, err, 502)
		return
	}
	u, _ := a.Store.GetFor(r.Context(), uid(r.Context()), name) // with their star and note
	writeJSON(w, map[string]any{"item": u})
}

func (a *App) watchUsername(w http.ResponseWriter, r *http.Request) {
	name := cleanName(r.PathValue("name"))
	var body struct {
		Watched bool    `json:"watched"`
		Note    *string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpErr(w, err, 400)
		return
	}
	if !nameRe.MatchString(name) {
		httpErr(w, apiError("invalid username"), 400)
		return
	}
	if err := a.Store.SetUserWatched(r.Context(), uid(r.Context()), name, body.Watched, body.Note); err != nil {
		httpErr(w, err, 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// hideUsername archives a name out of every list ({"hidden":true}), or brings it back. Hiding
// is also the strongest "not for me" the taste can learn from.
func (a *App) hideUsername(w http.ResponseWriter, r *http.Request) {
	name := cleanName(r.PathValue("name"))
	var body struct {
		Hidden bool `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpErr(w, err, 400)
		return
	}
	if !nameRe.MatchString(name) {
		httpErr(w, apiError("invalid username"), 400)
		return
	}
	if err := a.Store.SetUserHidden(r.Context(), uid(r.Context()), name, body.Hidden); err != nil {
		httpErr(w, err, 500)
		return
	}
	if body.Hidden {
		a.recordSignal(r.Context(), name, "dismiss")
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *App) listEvents(w http.ResponseWriter, r *http.Request) {
	ev, err := a.Store.Events(r.Context(), uid(r.Context()), "", 200)
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	writeJSON(w, ev)
}

// stats: what is tracked and rated is everyone's to see; the crawler's own numbers (rating queue,
// junk, LLM spend, chain progress) are for admins.
func (a *App) stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, err := a.Store.Stats(ctx)
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	st.Watched, st.Hidden = a.Store.UserNameCounts(ctx, uid(ctx))
	if !currentUser(ctx).Admin() {
		st.Queue, st.Junk = 0, 0
		writeJSON(w, map[string]any{"stats": st})
		return
	}
	writeJSON(w, map[string]any{"stats": st, "llm_usage": a.LLMUsage(), "chain_done_at": a.Store.GetKV(ctx, "chain_done_at")})
}

func (a *App) runJob(w http.ResponseWriter, r *http.Request) {
	started, err := a.Jobs.Trigger(r.PathValue("name"))
	if err != nil {
		httpErr(w, err, 404)
		return
	}
	writeJSON(w, map[string]bool{"started": started})
}
