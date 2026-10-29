package app

import (
	"context"
	"fmt"
	"html"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fragwatch/internal/i18n"
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
		st.Why = i18n.T(i18n.From(ctx), "why_no_bot")
	case s.Plan != "pro":
		st.Why = i18n.T(i18n.From(ctx), "why_not_pro")
	case s.TelegramChat == "":
		st.Why = i18n.T(i18n.From(ctx), "why_no_tg")
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
		if err := a.tgSend(r.Context(), i18n.T(a.userLang(r.Context(), uid(r.Context())), "alerts_on"), nil); err != nil {
			if strings.Contains(err.Error(), "blocked") || strings.Contains(err.Error(), "chat not found") {
				err = apiError("the bot cannot write to you yet: open @" + st.Bot + " in Telegram and press Start")
			}
			httpErr(w, err, 502)
			return
		}
		a.sendPreview(r.Context())
		writeJSON(w, a.alertsStatus(r.Context()))
	})
}

// sendPreview shows what a "new names" alert looks like, with a few names that fit the settings now.
func (a *App) sendPreview(ctx context.Context) {
	s := a.settings(ctx)
	l := a.userLang(ctx, uid(ctx))
	items, total, err := a.query(ctx, a.newNamesFilter(s, 0, 3))
	if err != nil || len(items) == 0 {
		return
	}
	cards := make([]string, len(items))
	for i, u := range items {
		cards[i] = nameCard(l, u, "")
	}
	foot := ""
	if total > len(items) {
		foot = i18n.T(l, "alerts_more", total-len(items))
	}
	a.tgSend(ctx, alertMessage(i18n.T(l, "alerts_new", len(items), i18n.N(l, "w_new_names", len(items))), i18n.T(l, "alerts_new_sub"), cards, foot), a.alertButtons(l, items))
}

// newNamesFilter: buyable-now names inside the settings, changed since a moment (0: any time).
func (a *App) newNamesFilter(s Settings, since int64, limit int) store.Filter {
	minScore := s.MinScore
	f := store.Filter{Statuses: []string{"auction", "sale", "market"}, Live: true, BuyNow: true, MinScore: &minScore, HideJunk: true,
		Categories: s.Kinds, ChangedSince: since, Sort: "value", Limit: limit}
	if s.MaxBuyout > 0 {
		f.MaxBuyout = &s.MaxBuyout
	}
	return f
}

// tgSend writes an alert to the signed-in (or job-selected) account's Telegram.
func (a *App) tgSend(ctx context.Context, text string, buttons [][]map[string]any) error {
	u := currentUser(ctx)
	if u == nil || u.TgID == 0 {
		return apiError("no Telegram linked")
	}
	err := a.tgText(ctx, u.TgID, text, buttons)
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
	l := a.userLang(ctx, uid(ctx))
	now := time.Now().Unix()
	last, _ := strconv.ParseInt(a.uget(ctx, kvAlertsLast), 10, 64)
	a.uset(ctx, kvAlertsLast, strconv.FormatInt(now, 10))
	if last == 0 {
		// first run: take a baseline so the chat is not flooded with everything already listed
		a.baselinePrices(ctx, s)
		return 0, nil
	}

	// 1. names that entered the settings since the last run
	f := a.newNamesFilter(s, last, maxNewPerMsg)
	if items, total, err := a.query(ctx, f); err == nil && len(items) > 0 {
		cards := make([]string, len(items))
		for i, u := range items {
			cards[i] = nameCard(l, u, "")
		}
		foot := ""
		if total > len(items) {
			foot = i18n.T(l, "alerts_more", total-len(items))
		}
		head := i18n.T(l, "alerts_new", total, i18n.N(l, "w_new_names", total))
		if err := a.tgSend(ctx, alertMessage(head, i18n.T(l, "alerts_new_sub"), cards, foot), a.alertButtons(l, items)); err != nil {
			return sent, err
		}
		sent++
	}

	// 2. tracked auctions an hour from closing (once per auction)
	ending := store.Filter{Statuses: []string{"auction"}, Watched: true, Live: true, EndingIn: endingWithin, Sort: "ending", Limit: 20}
	if items, _, err := a.query(ctx, ending); err == nil {
		var lines []string
		var closing []store.Username
		for _, u := range items {
			if u.EndsAt == nil {
				continue
			}
			key := "alerts:ending:" + u.Name
			if a.uget(ctx, key) == strconv.FormatInt(*u.EndsAt, 10) {
				continue
			}
			a.uset(ctx, key, strconv.FormatInt(*u.EndsAt, 10))
			lines = append(lines, nameCard(l, u, i18n.T(l, "closes_in", timeLeft(l, *u.EndsAt-now))))
			closing = append(closing, u)
		}
		if len(lines) > 0 {
			if err := a.tgSend(ctx, alertMessage(i18n.T(l, "alerts_end"), i18n.T(l, "alerts_end_sub"), lines, ""), a.alertButtons(l, closing)); err != nil {
				return sent, err
			}
			sent++
		}
	}

	// 3. price drops on names they liked, starred or put in the portfolio
	var drops []string
	var dropped []store.Username
	for _, name := range a.followedNames(ctx) {
		u, _ := a.Store.Get(ctx, name)
		if u == nil || u.Buyout == nil {
			continue
		}
		key := "alerts:price:" + name
		prev, _ := strconv.ParseFloat(a.uget(ctx, key), 64)
		a.uset(ctx, key, strconv.FormatFloat(*u.Buyout, 'g', -1, 64))
		if prev > 0 && *u.Buyout <= prev*(1-dropMin) {
			drop := fmt.Sprintf("📉 <s>%s TON</s> → <b>%s TON</b>  −%d%%", ton(l, prev), ton(l, *u.Buyout), int(math.Round((1-*u.Buyout/prev)*100)))
			drops = append(drops, nameCard(l, *u, drop))
			dropped = append(dropped, *u)
		}
	}
	if len(drops) > 0 {
		if err := a.tgSend(ctx, alertMessage(i18n.T(l, "alerts_drops"), i18n.T(l, "alerts_drops_sub"), drops, ""), a.alertButtons(l, dropped)); err != nil {
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

// ton is a TON amount with thousands grouped: 10,000.5 in English, 10 000,5 in Russian.
func ton(l string, v float64) string {
	whole, frac, _ := strings.Cut(strconv.FormatFloat(v, 'f', -1, 64), ".")
	sep, dot := ",", "."
	if l == i18n.RU {
		sep, dot = " ", "," // a no-break space keeps "10 000" on one line
	}
	neg := strings.HasPrefix(whole, "-")
	whole = strings.TrimPrefix(whole, "-")
	var b strings.Builder
	for i, c := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteString(sep)
		}
		b.WriteRune(c)
	}
	s := b.String()
	if frac != "" {
		s += dot + frac
	}
	if neg {
		s = "-" + s
	}
	return s
}

// timeLeft is how long until an auction closes, to the minute: "47 min", "1 h 05 min".
func timeLeft(l string, secs int64) string {
	m := int((secs + 30) / 60)
	if m < 60 {
		return i18n.T(l, "min", m)
	}
	return i18n.T(l, "h_min", m/60, m%60)
}

// alertMessage lays an alert out with room to breathe: a bold title, a quiet subtitle, then one
// quoted card per name (Telegram draws each <blockquote> as its own block with an accent bar).
func alertMessage(title, sub string, cards []string, foot string) string {
	var b strings.Builder
	b.WriteString(title + "\n" + sub + "\n\n")
	b.WriteString(strings.Join(cards, "\n"))
	if foot != "" {
		b.WriteString("\n" + foot)
	}
	return b.String()
}

// nameCard is one name as a card: the handle, then price · score · where it is sold, then what
// it means, then an optional extra line (time left, the price drop).
func nameCard(l string, u store.Username, extra string) string {
	price := i18n.T(l, "no_price")
	if u.Buyout != nil {
		price = "💎 <b>" + ton(l, *u.Buyout) + " TON</b>"
	} else if u.Price != nil {
		price = "💎 " + i18n.T(l, "bid", ton(l, *u.Price))
	}
	facts := []string{price}
	if u.Score != nil {
		facts = append(facts, scoreBar(*u.Score))
	}
	switch u.Status {
	case "auction":
		facts = append(facts, i18n.T(l, "st_auction"))
	case "sale":
		facts = append(facts, i18n.T(l, "st_sale"))
	case "market":
		m := "market"
		if u.ChainMarket != nil && *u.ChainMarket != "" {
			m = *u.ChainMarket
		}
		facts = append(facts, i18n.T(l, "st_market", html.EscapeString(m)))
	}
	lines := []string{fmt.Sprintf(`<b><a href="%s">@%s</a></b>`, nameURL(u), u.Name), strings.Join(facts, "   ")}
	if u.Meaning != nil && *u.Meaning != "" {
		lines = append(lines, "<i>"+html.EscapeString(*u.Meaning)+"</i>")
	}
	if extra != "" {
		lines = append(lines, extra)
	}
	return "<blockquote>" + strings.Join(lines, "\n") + "</blockquote>"
}

// scoreBar draws the AI score as five stars: ★★★★☆ 8/10.
func scoreBar(s int) string {
	full := min(5, max(0, (s+1)/2))
	return strings.Repeat("★", full) + strings.Repeat("☆", 5-full) + fmt.Sprintf(" %d/10", s)
}

func nameURL(u store.Username) string { return "https://fragment.com/username/" + u.Name }

// alertButtons: one tap per name, two to a row, and a way into the app.
func (a *App) alertButtons(l string, items []store.Username) [][]map[string]any {
	var rows [][]map[string]any
	var row []map[string]any
	for i, u := range items {
		if i == 6 {
			break
		}
		row = append(row, map[string]any{"text": "@" + u.Name, "url": nameURL(u)})
		if len(row) == 2 {
			rows, row = append(rows, row), nil
		}
	}
	if row != nil {
		rows = append(rows, row)
	}
	if btn := a.appButton(i18n.T(l, "btn_open")); btn != nil {
		rows = append(rows, btn)
	}
	return rows
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
