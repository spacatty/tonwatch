package app

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"fragwatch/internal/auth"
	"fragwatch/internal/store"
)

// Accounts and sign-in. Telegram is the only way in, through three doors:
//   - the Mini App: Telegram signs the launch data (initData); no form at all
//   - the web, "Continue with Telegram": a bot deep link, confirmed with a button in the bot
//   - the web, Telegram Login Widget (needs a public https domain set in @BotFather)
// Every door ends in a session: an HttpOnly cookie on the web, a bearer token in the Mini App
// (it runs in Telegram's webview, where cookies are unreliable).

const (
	sessionCookie = "fw_session"
	sessionTTL    = 60 * 24 * time.Hour
	handshakeTTL  = 5 * time.Minute
	initDataAge   = 24 * time.Hour
)

type ctxKey int

const (
	userKey ctxKey = iota
	sessionKey
)

func withUser(ctx context.Context, u *store.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

func currentUser(ctx context.Context) *store.User {
	u, _ := ctx.Value(userKey).(*store.User)
	return u
}

// uid is the signed-in account, 0 when there is none (background jobs set one per user).
func uid(ctx context.Context) int64 {
	if u := currentUser(ctx); u != nil {
		return u.ID
	}
	return 0
}

// access decides who may call a path: anyone, any signed-in account, or admins only.
func access(path string) string {
	switch {
	case strings.HasPrefix(path, "/api/auth/"), path == "/api/meta", path == "/api/billing/packs":
		return "public"
	case strings.HasPrefix(path, "/api/jobs"), strings.HasPrefix(path, "/api/llm/"), strings.HasPrefix(path, "/api/admin/"):
		return "admin"
	}
	return "user"
}

func sessionToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

// authenticate puts the session's account on the request and enforces access().
func (a *App) authenticate(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if tok := sessionToken(r); tok != "" {
			hash := auth.HashToken(tok)
			if u, err := a.Store.SessionUser(ctx, hash); err == nil && u != nil {
				ctx = context.WithValue(withUser(ctx, u), sessionKey, hash)
			}
		}
		u := currentUser(ctx)
		switch access(r.URL.Path) {
		case "user":
			if u == nil {
				httpErr(w, apiError("sign in first"), 401)
				return
			}
		case "admin":
			if u == nil {
				httpErr(w, apiError("sign in first"), 401)
				return
			}
			if !u.Admin() {
				httpErr(w, apiError("admins only"), 403)
				return
			}
		}
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *App) authRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auth/me", a.me)
	mux.HandleFunc("POST /api/auth/logout", a.logout)
	mux.HandleFunc("POST /api/auth/logout-all", a.logoutAll)
	mux.HandleFunc("POST /api/auth/telegram/webapp", a.webAppLogin)
	mux.HandleFunc("POST /api/auth/telegram/widget", a.widgetLogin)
	mux.HandleFunc("POST /api/auth/telegram/start", a.botHandshake)
	mux.HandleFunc("GET /api/auth/telegram/poll/{secret}", a.pollHandshake)
}

// userJSON is the account as the app sees it.
func (a *App) userJSON(u *store.User) map[string]any {
	photo := u.TgPhoto
	if photo == "" {
		photo = a.avatarURL(u.ID, u.TgID) // signed in through the bot: no photo_url, read it via the Bot API
	}
	return map[string]any{
		"id": u.ID, "tg_id": u.TgID, "tg_username": u.TgUsername, "tg_name": u.TgName, "tg_photo": photo,
		"role": u.Role, "plan": u.Plan(), "plan_until": u.PlanUntil, "credits": u.Credits, "created_at": u.CreatedAt,
		"label": u.Label(),
	}
}

// authInfo tells the sign-in screen which doors are open.
func (a *App) authInfo() map[string]any {
	bot := a.botName()
	return map[string]any{
		"telegram":   bot != "",
		"bot":        bot,
		"widget":     bot != "" && strings.HasPrefix(a.Cfg.PublicURL, "https://"),
		"public_url": a.Cfg.PublicURL,
	}
}

func (a *App) me(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"user": nil, "auth": a.authInfo()}
	if u := currentUser(r.Context()); u != nil {
		a.balance(r.Context()) // the daily refill shows up the moment they open the app
		u, _ = a.Store.UserByID(r.Context(), u.ID)
		out["user"] = a.userJSON(u)
	}
	writeJSON(w, out)
}

// --- rate limiting: sign-in handshakes, per client IP ---

var (
	limMu    sync.Mutex
	limiters = map[string]*rate.Limiter{}
)

func clientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		ip, _, _ := strings.Cut(xf, ",")
		return strings.TrimSpace(ip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// throttled allows ~10 attempts a minute per IP (bursts of 10).
func throttled(w http.ResponseWriter, r *http.Request) bool {
	limMu.Lock()
	if len(limiters) > 10000 {
		limiters = map[string]*rate.Limiter{}
	}
	l, ok := limiters[clientIP(r)]
	if !ok {
		l = rate.NewLimiter(rate.Every(6*time.Second), 10)
		limiters[clientIP(r)] = l
	}
	limMu.Unlock()
	if !l.Allow() {
		httpErr(w, apiError("too many attempts; wait a minute"), 429)
		return true
	}
	return false
}

// --- sessions ---

func (a *App) startSession(w http.ResponseWriter, r *http.Request, u *store.User, client string) {
	tok := auth.Token(32)
	if err := a.Store.CreateSession(r.Context(), auth.HashToken(tok), u.ID, client, sessionTTL); err != nil {
		httpErr(w, err, 500)
		return
	}
	if client == "web" {
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: tok, Path: "/", MaxAge: int(sessionTTL.Seconds()),
			HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: strings.HasPrefix(a.Cfg.PublicURL, "https://")})
	}
	u, _ = a.Store.UserByID(r.Context(), u.ID)
	// the Mini App keeps the token in memory and sends it as a bearer; the web relies on the cookie.
	// "status" lets the Telegram handshake poll tell this success apart from "pending"/"expired".
	writeJSON(w, map[string]any{"status": "ok", "user": a.userJSON(u), "token": tok})
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if h, ok := r.Context().Value(sessionKey).(string); ok {
		a.Store.DeleteSession(r.Context(), h)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *App) logoutAll(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if u == nil {
		httpErr(w, apiError("sign in first"), 401)
		return
	}
	keep, _ := r.Context().Value(sessionKey).(string)
	a.Store.DeleteSessions(r.Context(), u.ID, keep)
	writeJSON(w, map[string]bool{"ok": true})
}

// newAccount creates an account for a Telegram identity. Admins are the ids in ADMIN_TELEGRAM_IDS;
// when that is empty, the very first account is the owner of the install. The first admin inherits
// everything made before accounts existed.
func (a *App) newAccount(ctx context.Context, tg store.TgProfile) (*store.User, error) {
	role := "user"
	if ids := a.Cfg.AdminTelegramIDs; slices.Contains(ids, tg.ID) || (len(ids) == 0 && a.Store.CountUsers(ctx) == 0) {
		role = "admin"
	}
	firstAdmin := role == "admin" && a.Store.CountAdmins(ctx) == 0
	id, err := a.Store.CreateUser(ctx, "", "", "", &tg, role)
	if err != nil {
		return nil, err
	}
	if firstAdmin {
		if err := a.Store.ClaimLegacy(ctx, id); err != nil {
			log.Printf("claim legacy data: %v", err)
		}
	}
	return a.Store.UserByID(ctx, id)
}

// --- Telegram ---

func profileOf(t auth.TgUser) store.TgProfile {
	return store.TgProfile{ID: t.ID, Username: t.Username, Name: t.Name(), Photo: t.PhotoURL}
}

// tgAccount finds the account behind a Telegram identity, refreshing its profile, or creates it.
func (a *App) tgAccount(ctx context.Context, p store.TgProfile) (*store.User, error) {
	u, err := a.Store.UserByTg(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return a.newAccount(ctx, p)
	}
	a.Store.SetUserTg(ctx, u.ID, p)
	if !u.Admin() && slices.Contains(a.Cfg.AdminTelegramIDs, p.ID) {
		a.Store.SetRole(ctx, u.ID, "admin")
	}
	return a.Store.UserByID(ctx, u.ID)
}

func (a *App) webAppLogin(w http.ResponseWriter, r *http.Request) {
	var b struct {
		InitData string `json:"init_data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpErr(w, err, 400)
		return
	}
	tu, err := auth.VerifyWebApp(b.InitData, a.Cfg.TelegramBotToken, initDataAge, time.Now())
	if err != nil {
		httpErr(w, err, 401)
		return
	}
	u, err := a.tgAccount(r.Context(), profileOf(tu))
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	a.startSession(w, r, u, "miniapp")
}

func (a *App) widgetLogin(w http.ResponseWriter, r *http.Request) {
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		httpErr(w, err, 400)
		return
	}
	fields := map[string]string{}
	for k, v := range raw {
		switch x := v.(type) {
		case string:
			fields[k] = x
		case float64:
			fields[k] = strconv.FormatFloat(x, 'f', -1, 64) // id and auth_date arrive as JSON numbers
		}
	}
	tu, err := auth.VerifyWidget(fields, a.Cfg.TelegramBotToken, initDataAge, time.Now())
	if err != nil {
		httpErr(w, err, 401)
		return
	}
	u, err := a.tgAccount(r.Context(), profileOf(tu))
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	a.startSession(w, r, u, "web")
}

// botHandshake starts "Continue with Telegram": the browser gets a t.me link to open and a secret
// to poll while the person approves in the bot.
func (a *App) botHandshake(w http.ResponseWriter, r *http.Request) {
	if throttled(w, r) {
		return
	}
	bot := a.botName()
	if bot == "" {
		httpErr(w, apiError("the Telegram bot is not configured"), 503)
		return
	}
	t := store.AuthToken{Secret: auth.Token(32), Code: auth.Token(12), Purpose: "login", Client: clientLabel(r)}
	if err := a.Store.CreateAuthToken(r.Context(), t, handshakeTTL); err != nil {
		httpErr(w, err, 500)
		return
	}
	writeJSON(w, map[string]any{"secret": t.Secret, "url": "https://t.me/" + bot + "?start=" + t.Purpose + "_" + t.Code,
		"expires_in": int(handshakeTTL.Seconds())})
}

// pollHandshake is what the browser asks every couple of seconds while the person answers in the
// bot. "ok" is answered once, with the session.
func (a *App) pollHandshake(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t, err := a.Store.AuthTokenBySecret(ctx, r.PathValue("secret"))
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	if t == nil {
		writeJSON(w, map[string]string{"status": "expired"})
		return
	}
	if t.Status != "ok" {
		writeJSON(w, map[string]string{"status": t.Status})
		return
	}
	if t, err = a.Store.TakeAuthToken(ctx, t.Secret); err != nil || t == nil {
		writeJSON(w, map[string]string{"status": "expired"})
		return
	}
	u, err := a.Store.UserByID(ctx, t.UserID)
	if err != nil || u == nil {
		httpErr(w, apiError("account not found"), 404)
		return
	}
	a.startSession(w, r, u, "web")
}

// clientLabel describes the browser asking, for the bot's "is this you?" prompt.
func clientLabel(r *http.Request) string {
	ua := r.UserAgent()
	browser := "a browser"
	for _, b := range []struct{ key, name string }{{"Edg/", "Edge"}, {"OPR/", "Opera"}, {"Firefox/", "Firefox"}, {"Chrome/", "Chrome"}, {"Safari/", "Safari"}} {
		if strings.Contains(ua, b.key) {
			browser = b.name
			break
		}
	}
	osName := ""
	for _, o := range []struct{ key, name string }{{"Windows", "Windows"}, {"iPhone", "iPhone"}, {"iPad", "iPad"}, {"Android", "Android"}, {"Mac OS", "macOS"}, {"Linux", "Linux"}} {
		if strings.Contains(ua, o.key) {
			osName = " on " + o.name
			break
		}
	}
	return browser + osName + ", IP " + clientIP(r)
}
