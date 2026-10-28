package app

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fragwatch/internal/jobs"
	"fragwatch/internal/store"
)

// Telegram alerts: the Pro tier. The bot pings each Pro account's own Telegram when a name enters
// their sniper settings, when an auction they track is an hour from closing, and when a name they
// follow drops in price. The chat is the account's linked Telegram; the plan is bought (billing.go)
// or granted by an admin.
const (
	kvAlertsLast = "alerts:last_run"
	kvAlertsSent = "alerts:sent"
	alertEvery   = 5 * time.Minute
	endingWithin = 3600 // seconds
	dropMin      = 0.10 // a price drop smaller than this is noise
	maxNewPerMsg = 8
)

type alertsStatus struct {
	BotSet  bool   `json:"bot_set"`  // the install has a bot
	Bot     string `json:"bot"`      // its @username, to message it
	Plan    string `json:"plan"`     // free | pro
	Chat    string `json:"chat"`     // their Telegram id: where alerts go
	Active  bool   `json:"active"`   // everything in place: the job sends
	LastRun int64  `json:"last_run"` // unix; 0 = never
	Sent    int    `json:"sent"`     // messages sent so far
	Why     string `json:"why"`      // what is missing when not active
}

func (a *App) alertsStatus(ctx context.Context) alertsStatus {
	s := a.settings(ctx)
	st := alertsStatus{BotSet: a.Cfg.TelegramBotToken != "", Bot: a.botName(), Plan: s.Plan, Chat: s.TelegramChat}
	st.LastRun, _ = strconv.ParseInt(a.uget(ctx, kvAlertsLast), 10, 64)
	st.Sent, _ = strconv.Atoi(a.uget(ctx, kvAlertsSent))
	switch {
	case !st.BotSet:
		st.Why = "this install has no Telegram bot (TELEGRAM_BOT_TOKEN)"
	case s.Plan != "pro":
		st.Why = "alerts are a Pro feature"
	case s.TelegramChat == "":
		st.Why = "link your Telegram account first"
	default:
		st.Active = true
	}
	return st
}

func (a *App) alertRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/alerts", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.alertsStatus(r.Context())) })
	mux.HandleFunc("POST /api/alerts/test", func(w http.ResponseWriter, r *http.Request) {
		st := a.alertsStatus(r.Context())
		if !st.Active {
			httpErr(w, apiError(st.Why), 403)
			return
		}
		if err := a.tgSend(r.Context(), "✅ <b>TON Watch</b> alerts are on. You will hear about new names inside your settings, tracked auctions an hour from closing, and price drops on names you liked."); err != nil {
			if strings.Contains(err.Error(), "blocked") || strings.Contains(err.Error(), "chat not found") {
				err = apiError("the bot cannot write to you yet: open @" + st.Bot + " in Telegram and press Start")
			}
			httpErr(w, err, 502)
			return
		}
		writeJSON(w, a.alertsStatus(r.Context()))
	})
}

// tgSend writes an alert to the signed-in (or job-selected) account's Telegram.
func (a *App) tgSend(ctx context.Context, text string) error {
	u := currentUser(ctx)
	if u == nil || u.TgID == 0 {
		return apiError("no Telegram linked")
	}
	err := a.tgText(ctx, u.TgID, text, nil)
	if err == nil {
		n, _ := strconv.Atoi(a.uget(ctx, kvAlertsSent))
		a.uset(ctx, kvAlertsSent, strconv.Itoa(n+1))
	}
	return err
}

// runAlerts is the job: every Pro account with Telegram linked gets its own run.
func (a *App) runAlerts(ctx context.Context, j *jobs.Job) error {
	if a.Cfg.TelegramBotToken == "" {
		j.Progress("off: no TELEGRAM_BOT_TOKEN")
		return nil
	}
	users, err := a.Store.UsersWithPro(ctx)
	if err != nil {
		return err
	}
	sent, failed := 0, 0
	for _, u := range users {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, err := a.alertUser(withUser(ctx, u))
		sent += n
		if err != nil {
			failed++
			j.Progress("%s: %v", u.Label(), err)
		}
	}
	j.Progress("%d Pro account%s, %d message%s sent, %d failed", len(users), plural(len(users)), sent, plural(sent), failed)
	return nil
}

// alertUser: three checks for one account, one message each, nothing when there is nothing new.
func (a *App) alertUser(ctx context.Context) (sent int, err error) {
	s := a.settings(ctx)
	now := time.Now().Unix()
	last, _ := strconv.ParseInt(a.uget(ctx, kvAlertsLast), 10, 64)
	a.uset(ctx, kvAlertsLast, strconv.FormatInt(now, 10))
	if last == 0 {
		// first run: take a baseline so the chat is not flooded with everything already listed
		a.baselinePrices(ctx, s)
		return 0, nil
	}

	// 1. names that entered the settings since the last run
	minScore := s.MinScore
	f := store.Filter{Statuses: []string{"auction", "sale", "market"}, Live: true, BuyNow: true, MinScore: &minScore, HideJunk: true,
		Categories: s.Kinds, ChangedSince: last, Sort: "value", Limit: maxNewPerMsg}
	if s.MaxBuyout > 0 {
		f.MaxBuyout = &s.MaxBuyout
	}
	if items, total, err := a.query(ctx, f); err == nil && len(items) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "🎯 <b>%d new name%s inside your settings</b>\n", total, plural(total))
		for _, u := range items {
			fmt.Fprintf(&b, "%s\n", nameLine(u))
		}
		if total > len(items) {
			fmt.Fprintf(&b, "…and %d more in the app", total-len(items))
		}
		if err := a.tgSend(ctx, b.String()); err != nil {
			return sent, err
		}
		sent++
	}

	// 2. tracked auctions an hour from closing (once per auction)
	ending := store.Filter{Statuses: []string{"auction"}, Watched: true, Live: true, EndingIn: endingWithin, Sort: "ending", Limit: 20}
	if items, _, err := a.query(ctx, ending); err == nil {
		var lines []string
		for _, u := range items {
			if u.EndsAt == nil {
				continue
			}
			key := "alerts:ending:" + u.Name
			if a.uget(ctx, key) == strconv.FormatInt(*u.EndsAt, 10) {
				continue
			}
			a.uset(ctx, key, strconv.FormatInt(*u.EndsAt, 10))
			left := time.Duration(*u.EndsAt-now) * time.Second
			lines = append(lines, fmt.Sprintf("%s — closes in %s", nameLine(u), left.Round(time.Minute)))
		}
		if len(lines) > 0 {
			if err := a.tgSend(ctx, "⏳ <b>Tracked auctions closing</b>\n"+strings.Join(lines, "\n")); err != nil {
				return sent, err
			}
			sent++
		}
	}

	// 3. price drops on names they liked, starred or put in the portfolio
	var drops []string
	for _, name := range a.followedNames(ctx) {
		u, _ := a.Store.Get(ctx, name)
		if u == nil || u.Buyout == nil {
			continue
		}
		key := "alerts:price:" + name
		prev, _ := strconv.ParseFloat(a.uget(ctx, key), 64)
		a.uset(ctx, key, strconv.FormatFloat(*u.Buyout, 'g', -1, 64))
		if prev > 0 && *u.Buyout <= prev*(1-dropMin) {
			drops = append(drops, fmt.Sprintf("%s — was %g TON", nameLine(*u), prev))
		}
	}
	if len(drops) > 0 {
		if err := a.tgSend(ctx, "📉 <b>Price drops on names you follow</b>\n"+strings.Join(drops, "\n")); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

// followedNames: explicit likes, soft likes, starred names and the portfolio, deduplicated.
func (a *App) followedNames(ctx context.Context) []string {
	t := a.loadTaste(ctx)
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, n := range t.Likes {
		add(n)
	}
	for _, n := range t.SoftLikes {
		add(n)
	}
	if items, _, err := a.query(ctx, store.Filter{Watched: true, Limit: 200}); err == nil {
		for _, u := range items {
			add(u.Name)
		}
	}
	if rows, err := a.Store.Portfolio(ctx, uid(ctx)); err == nil {
		for _, r := range rows {
			add(r.Name)
		}
	}
	return out
}

func (a *App) baselinePrices(ctx context.Context, s Settings) {
	for _, name := range a.followedNames(ctx) {
		if u, _ := a.Store.Get(ctx, name); u != nil && u.Buyout != nil {
			a.uset(ctx, "alerts:price:"+name, strconv.FormatFloat(*u.Buyout, 'g', -1, 64))
		}
	}
}

func nameLine(u store.Username) string {
	price := "no price"
	if u.Buyout != nil {
		price = fmt.Sprintf("%g TON", *u.Buyout)
	} else if u.Price != nil {
		price = fmt.Sprintf("bid %g TON", *u.Price)
	}
	score := ""
	if u.Score != nil {
		score = fmt.Sprintf(" · %d/10", *u.Score)
	}
	meaning := ""
	if u.Meaning != nil && *u.Meaning != "" {
		meaning = " — " + html.EscapeString(*u.Meaning)
	}
	return fmt.Sprintf(`<a href="https://fragment.com/username/%s">@%s</a> %s%s%s`, u.Name, u.Name, price, score, meaning)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
