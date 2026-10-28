package app

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"fragwatch/internal/auth"
	"fragwatch/internal/store"
)

// Accounts and sign-in. One account, many doors:
//   - the Mini App: Telegram signs the launch data (initData); no password, no form
//   - the web, "Continue with Telegram": a bot deep link, confirmed with a button in the bot
//   - the web, Telegram Login Widget (needs a public https domain set in @BotFather)
//   - the web, login + password, optionally approved in the bot (Telegram 2FA)
// Every door ends in a session: an HttpOnly cookie on the web, a bearer token in the Mini App
// (it runs in Telegram's webview, where cookies are unreliable).

const (
	sessionCookie = "fw_session"
	sessionTTL    = 60 * 24 * time.Hour
	handshakeTTL  = 5 * time.Minute
	initDataAge   = 24 * time.Hour
)

var loginRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{2,31}$`)

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
	mux.HandleFunc("POST /api/auth/register", a.register)
	mux.HandleFunc("POST /api/auth/login", a.passwordLogin)
	mux.HandleFunc("POST /api/auth/logout", a.logout)
	mux.HandleFunc("POST /api/auth/logout-all", a.logoutAll)
	mux.HandleFunc("POST /api/auth/telegram/webapp", a.webAppLogin)
	mux.HandleFunc("POST /api/auth/telegram/widget", a.widgetLogin)
	mux.HandleFunc("POST /api/auth/telegram/start", a.botHandshake)
	mux.HandleFunc("GET /api/auth/telegram/poll/{secret}", a.pollHandshake)
	mux.HandleFunc("POST /api/auth/telegram/unlink", a.unlinkTelegram)
	mux.HandleFunc("PUT /api/auth/account", a.updateAccount)
}

// userJSON is the account as the app sees it.
func (a *App) userJSON(u *store.User) map[string]any {
	return map[string]any{
		"id": u.ID, "login": u.Login, "email": u.Email, "has_password": u.HasPassword,
		"tg_id": u.TgID, "tg_username": u.TgUsername, "tg_name": u.TgName, "tg_photo": u.TgPhoto, "tg_2fa": u.TG2FA,
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

// --- rate limiting: password guesses and handshakes, per client IP ---

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
	// the Mini App keeps the token in memory and sends it as a bearer; the web relies on the cookie
	writeJSON(w, map[string]any{"user": a.userJSON(u), "token": tok})
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

// newAccount creates an account. The very first one is the owner of the install: it becomes
// admin and inherits everything made before accounts existed.
func (a *App) newAccount(ctx context.Context, login, email, passHash string, tg *store.TgProfile) (*store.User, error) {
	first := a.Store.CountUsers(ctx) == 0
	role := "user"
	if first || (tg != nil && slices.Contains(a.Cfg.AdminTelegramIDs, tg.ID)) {
		role = "admin"
	}
	id, err := a.Store.CreateUser(ctx, login, email, passHash, tg, role)
	if err != nil {
		return nil, err
	}
	if first {
		if err := a.Store.ClaimLegacy(ctx, id); err != nil {
			log.Printf("claim legacy data: %v", err)
		}
	}
	return a.Store.UserByID(ctx, id)
}

// --- password accounts ---

type credsBody struct {
	Login    string `json:"login"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func checkPassword(pw string) error {
	if len(pw) < 8 {
		return apiError("the password needs at least 8 characters")
	}
	if len(pw) > 200 {
		return apiError("the password is too long")
	}
	return nil
}

func cleanEmail(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", nil
	}
	if len(s) > 200 || !strings.Contains(s, "@") || strings.ContainsAny(s, " \t<>") {
		return "", apiError("that email does not look right")
	}
	return s, nil
}

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	if throttled(w, r) {
		return
	}
	var b credsBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpErr(w, err, 400)
		return
	}
	b.Login = strings.TrimSpace(b.Login)
	if !loginRe.MatchString(b.Login) {
		httpErr(w, apiError("pick a login of 3–32 letters, digits or _, starting with a letter"), 400)
		return
	}
	email, err := cleanEmail(b.Email)
	if err == nil {
		err = checkPassword(b.Password)
	}
	if err != nil {
		httpErr(w, err, 400)
		return
	}
	hash, err := auth.HashPassword(b.Password)
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	u, err := a.newAccount(r.Context(), b.Login, email, hash, nil)
	if errors.Is(err, store.ErrTaken) {
		httpErr(w, apiError("that login or email is taken"), 409)
		return
	}
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	a.startSession(w, r, u, "web")
}

func (a *App) passwordLogin(w http.ResponseWriter, r *http.Request) {
	if throttled(w, r) {
		return
	}
	var b credsBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpErr(w, err, 400)
		return
	}
	u, err := a.Store.UserByLogin(r.Context(), b.Login)
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	if u == nil || !u.HasPassword || !auth.CheckPassword(u.PassHash, b.Password) {
		httpErr(w, auth.ErrBadPassword, 401)
		return
	}
	if u.TG2FA && u.TgID != 0 && a.botName() != "" {
		secret, err := a.ask2FA(r, u)
		if err != nil {
			httpErr(w, err, 502)
			return
		}
		writeJSON(w, map[string]any{"challenge": secret, "expires_in": int(handshakeTTL.Seconds())})
		return
	}
	a.startSession(w, r, u, "web")
}

// updateAccount changes login, email, password or the 2FA switch. Setting a password when one
// exists needs the current one; a Telegram-only account sets its first password freely (it is
// already proven by Telegram).
func (a *App) updateAccount(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if u == nil {
		httpErr(w, apiError("sign in first"), 401)
		return
	}
	var b struct {
		Login           *string `json:"login"`
		Email           *string `json:"email"`
		Password        *string `json:"password"`
		CurrentPassword string  `json:"current_password"`
		TG2FA           *bool   `json:"tg_2fa"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpErr(w, err, 400)
		return
	}
	var hash *string
	if b.Login != nil {
		*b.Login = strings.TrimSpace(*b.Login)
		if *b.Login == "" && u.HasPassword {
			httpErr(w, apiError("an account with a password needs a login"), 400)
			return
		}
		if *b.Login != "" && !loginRe.MatchString(*b.Login) {
			httpErr(w, apiError("pick a login of 3–32 letters, digits or _, starting with a letter"), 400)
			return
		}
	}
	if b.Email != nil {
		e, err := cleanEmail(*b.Email)
		if err != nil {
			httpErr(w, err, 400)
			return
		}
		b.Email = &e
	}
	if b.Password != nil {
		if u.HasPassword && !auth.CheckPassword(u.PassHash, b.CurrentPassword) {
			httpErr(w, apiError("the current password is wrong"), 403)
			return
		}
		if err := checkPassword(*b.Password); err != nil {
			httpErr(w, err, 400)
			return
		}
		if u.Login == "" && (b.Login == nil || *b.Login == "") {
			httpErr(w, apiError("pick a login to go with the password"), 400)
			return
		}
		h, err := auth.HashPassword(*b.Password)
		if err != nil {
			httpErr(w, err, 500)
			return
		}
		hash = &h
	}
	if b.TG2FA != nil && *b.TG2FA && u.TgID == 0 {
		httpErr(w, apiError("link Telegram first"), 400)
		return
	}
	if err := a.Store.UpdateUser(r.Context(), u.ID, b.Login, b.Email, hash, b.TG2FA); err != nil {
		if errors.Is(err, store.ErrTaken) {
			httpErr(w, apiError("that login or email is taken"), 409)
			return
		}
		httpErr(w, err, 500)
		return
	}
	if hash != nil {
		// a new password signs every other device out
		keep, _ := r.Context().Value(sessionKey).(string)
		a.Store.DeleteSessions(r.Context(), u.ID, keep)
	}
	u, _ = a.Store.UserByID(r.Context(), u.ID)
	writeJSON(w, map[string]any{"user": a.userJSON(u)})
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
		return a.newAccount(ctx, "", "", "", &p)
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
	// a signed-in visitor using the widget is linking, not signing in
	if cur := currentUser(r.Context()); cur != nil {
		if err := a.Store.SetUserTg(r.Context(), cur.ID, profileOf(tu)); err != nil {
			httpErr(w, linkErr(err), 409)
			return
		}
		u, _ := a.Store.UserByID(r.Context(), cur.ID)
		writeJSON(w, map[string]any{"user": a.userJSON(u)})
		return
	}
	u, err := a.tgAccount(r.Context(), profileOf(tu))
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	a.startSession(w, r, u, "web")
}

func linkErr(err error) error {
	if errors.Is(err, store.ErrTaken) {
		return apiError("this Telegram account is already linked to another BidSpot account")
	}
	return err
}

// botHandshake starts "Continue with Telegram" ({"purpose":"login"}) or linking Telegram to the
// signed-in account ({"purpose":"link"}): the browser gets a t.me link to open and a secret to poll.
func (a *App) botHandshake(w http.ResponseWriter, r *http.Request) {
	if throttled(w, r) {
		return
	}
	bot := a.botName()
	if bot == "" {
		httpErr(w, apiError("the Telegram bot is not configured"), 503)
		return
	}
	var b struct {
		Purpose string `json:"purpose"`
	}
	json.NewDecoder(r.Body).Decode(&b)
	t := store.AuthToken{Secret: auth.Token(32), Code: auth.Token(12), Purpose: "login", Client: clientLabel(r)}
	if b.Purpose == "link" {
		u := currentUser(r.Context())
		if u == nil {
			httpErr(w, apiError("sign in first"), 401)
			return
		}
		t.Purpose, t.UserID = "link", u.ID
	}
	if err := a.Store.CreateAuthToken(r.Context(), t, handshakeTTL); err != nil {
		httpErr(w, err, 500)
		return
	}
	writeJSON(w, map[string]any{"secret": t.Secret, "url": "https://t.me/" + bot + "?start=" + t.Purpose + "_" + t.Code,
		"expires_in": int(handshakeTTL.Seconds())})
}

// pollHandshake is what the browser asks every couple of seconds while the person answers in the
// bot. "ok" is answered once: login and 2FA get their session, link gets the updated account.
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
	if t.Purpose == "link" {
		writeJSON(w, map[string]any{"status": "ok", "user": a.userJSON(u)})
		return
	}
	a.startSession(w, r, u, "web")
}

func (a *App) unlinkTelegram(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if u == nil {
		httpErr(w, apiError("sign in first"), 401)
		return
	}
	if !u.HasPassword {
		httpErr(w, apiError("set a password first, or you could not sign in again"), 400)
		return
	}
	if err := a.Store.UnlinkTg(r.Context(), u.ID); err != nil {
		httpErr(w, err, 500)
		return
	}
	u, _ = a.Store.UserByID(r.Context(), u.ID)
	writeJSON(w, map[string]any{"user": a.userJSON(u)})
}

// ask2FA sends the approve/deny prompt for a password sign-in and returns the secret to poll.
func (a *App) ask2FA(r *http.Request, u *store.User) (string, error) {
	t := store.AuthToken{Secret: auth.Token(32), Code: auth.Token(12), Purpose: "2fa", UserID: u.ID, TgID: u.TgID, Client: clientLabel(r)}
	ctx := r.Context()
	if err := a.Store.CreateAuthToken(ctx, t, handshakeTTL); err != nil {
		return "", err
	}
	if _, err := a.Store.ResolveAuthToken(ctx, t.Code, "pending", "waiting", 0, 0); err != nil {
		return "", err
	}
	if err := a.sendConfirm(ctx, u.TgID, t); err != nil {
		return "", apiError("could not reach you in Telegram: " + err.Error())
	}
	return t.Secret, nil
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
