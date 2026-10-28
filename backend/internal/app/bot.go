package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fragwatch/internal/store"
)

// The bot is the Telegram side of the app: it opens the Mini App, confirms web sign-ins and
// account links, approves password sign-ins (2FA), takes Stars payments and sends alerts. This
// process reads its updates by long polling (getUpdates), so it works without a public URL.

type tgFrom struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

func (f tgFrom) profile() store.TgProfile {
	return store.TgProfile{ID: f.ID, Username: f.Username, Name: strings.TrimSpace(f.FirstName + " " + f.LastName)}
}

type tgMessage struct {
	MessageID int64   `json:"message_id"`
	From      *tgFrom `json:"from"`
	Chat      struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"chat"`
	Text              string `json:"text"`
	SuccessfulPayment *struct {
		Currency       string `json:"currency"`
		TotalAmount    int    `json:"total_amount"`
		InvoicePayload string `json:"invoice_payload"`
		ChargeID       string `json:"telegram_payment_charge_id"`
	} `json:"successful_payment"`
}

type tgUpdate struct {
	UpdateID      int64      `json:"update_id"`
	Message       *tgMessage `json:"message"`
	CallbackQuery *struct {
		ID      string     `json:"id"`
		From    tgFrom     `json:"from"`
		Data    string     `json:"data"`
		Message *tgMessage `json:"message"`
	} `json:"callback_query"`
	PreCheckoutQuery *struct {
		ID             string `json:"id"`
		From           tgFrom `json:"from"`
		Currency       string `json:"currency"`
		TotalAmount    int    `json:"total_amount"`
		InvoicePayload string `json:"invoice_payload"`
	} `json:"pre_checkout_query"`
}

func (a *App) botName() string {
	a.botMu.RLock()
	defer a.botMu.RUnlock()
	return a.botUser
}

func (a *App) tgCall(ctx context.Context, method string, body map[string]any) ([]byte, error) {
	return a.tgCallT(ctx, 20*time.Second, method, body)
}

func (a *App) tgCallT(ctx context.Context, timeout time.Duration, method string, body map[string]any) ([]byte, error) {
	if a.Cfg.TelegramBotToken == "" {
		return nil, apiError("TELEGRAM_BOT_TOKEN is not set")
	}
	js, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.telegram.org/bot"+a.Cfg.TelegramBotToken+"/"+method, bytes.NewReader(js))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var tg struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	json.Unmarshal(out, &tg)
	if !tg.OK {
		if tg.Description == "" {
			tg.Description = fmt.Sprintf("telegram http %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("telegram: %s", tg.Description)
	}
	return out, nil
}

// tgText sends an HTML message to a chat, with optional inline buttons.
func (a *App) tgText(ctx context.Context, chat int64, text string, buttons [][]map[string]any) error {
	body := map[string]any{"chat_id": chat, "text": text, "parse_mode": "HTML", "link_preview_options": map[string]bool{"is_disabled": true}}
	if len(buttons) > 0 {
		body["reply_markup"] = map[string]any{"inline_keyboard": buttons}
	}
	_, err := a.tgCall(ctx, "sendMessage", body)
	return err
}

// appURL is where the bot's buttons send people. Telegram refuses buttons to localhost, and the
// Mini App only opens over https.
func (a *App) appButton(label string) []map[string]any {
	u := a.Cfg.PublicURL
	switch {
	case strings.HasPrefix(u, "https://"):
		return []map[string]any{{"text": label, "web_app": map[string]string{"url": u}}}
	case u != "" && !strings.Contains(u, "localhost") && !strings.Contains(u, "127.0.0.1"):
		return []map[string]any{{"text": label, "url": u}}
	}
	return nil
}

// RunBot reads the bot's updates until ctx ends. Without a token, or with BOT_POLL=0, it only
// learns the bot's name (so links can be built) and returns.
func (a *App) RunBot(ctx context.Context) {
	if a.Cfg.TelegramBotToken == "" {
		return
	}
	for ctx.Err() == nil {
		body, err := a.tgCall(ctx, "getMe", nil)
		if err == nil {
			var me struct {
				Result struct {
					Username string `json:"username"`
				} `json:"result"`
			}
			json.Unmarshal(body, &me)
			a.botMu.Lock()
			a.botUser = me.Result.Username
			a.botMu.Unlock()
			break
		}
		log.Printf("bot: getMe: %v (retrying in 30s)", err)
		sleepCtx(ctx, 30*time.Second)
	}
	if !a.Cfg.BotPoll || ctx.Err() != nil {
		return
	}
	log.Printf("bot: @%s is listening", a.botName())
	if btn := a.appButton("Open"); btn != nil && btn[0]["web_app"] != nil {
		// the menu button next to the message field opens the Mini App straight away
		a.tgCall(ctx, "setChatMenuButton", map[string]any{"menu_button": map[string]any{"type": "web_app", "text": "Open", "web_app": btn[0]["web_app"]}})
	}
	a.tgCall(ctx, "setMyCommands", map[string]any{"commands": []map[string]string{
		{"command": "start", "description": "Open BidSpot"},
		{"command": "balance", "description": "Credits and plan"},
	}})

	var offset int64
	for ctx.Err() == nil {
		body, err := a.tgCallT(ctx, 40*time.Second, "getUpdates", map[string]any{"offset": offset, "timeout": 25,
			"allowed_updates": []string{"message", "callback_query", "pre_checkout_query"}})
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("bot: getUpdates: %v", err)
				sleepCtx(ctx, 5*time.Second)
			}
			continue
		}
		var resp struct {
			Result []tgUpdate `json:"result"`
		}
		json.Unmarshal(body, &resp)
		for _, u := range resp.Result {
			offset = u.UpdateID + 1
			a.handleUpdate(ctx, u)
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func (a *App) handleUpdate(ctx context.Context, u tgUpdate) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("bot: update %d: panic: %v", u.UpdateID, r)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	switch {
	case u.PreCheckoutQuery != nil:
		a.preCheckout(ctx, u)
	case u.CallbackQuery != nil:
		a.onCallback(ctx, u)
	case u.Message != nil && u.Message.SuccessfulPayment != nil:
		a.onStarsPaid(ctx, u.Message)
	case u.Message != nil && u.Message.From != nil && u.Message.Chat.Type == "private":
		a.onMessage(ctx, u.Message)
	}
}

func (a *App) onMessage(ctx context.Context, m *tgMessage) {
	cmd, arg, _ := strings.Cut(strings.TrimSpace(m.Text), " ")
	cmd, _, _ = strings.Cut(cmd, "@")
	switch cmd {
	case "/start":
		if purpose, code, ok := strings.Cut(arg, "_"); ok && (purpose == "login" || purpose == "link") {
			a.onHandshake(ctx, m, purpose, code)
			return
		}
		a.welcome(ctx, m)
	case "/balance":
		u, _ := a.Store.UserByTg(ctx, m.From.ID)
		if u == nil {
			a.tgText(ctx, m.Chat.ID, "You have no BidSpot account yet. Open the app to make one.", rows(a.appButton("Open BidSpot")))
			return
		}
		ctx = withUser(ctx, u)
		plan := "Free"
		if u.Plan() == "pro" {
			plan = "Pro"
			if u.PlanRaw != "pro" {
				plan += " until " + time.Unix(u.PlanUntil, 0).UTC().Format("2 Jan 2006")
			}
		}
		a.tgText(ctx, m.Chat.ID, fmt.Sprintf("<b>%d</b> AI credits · %s plan", a.balance(ctx), plan), rows(a.appButton("Top up")))
	default:
		a.welcome(ctx, m)
	}
}

func rows(btn []map[string]any) [][]map[string]any {
	if btn == nil {
		return nil
	}
	return [][]map[string]any{btn}
}

func (a *App) welcome(ctx context.Context, m *tgMessage) {
	text := "<b>BidSpot</b> tracks every Telegram username on Fragment and the TON chain, rates each with AI, " +
		"and tells you which ones are worth buying.\n\nOpen the app to browse names, save trackers and get project ideas."
	if a.appButton("x") == nil {
		text += "\n\n<i>The app is running locally; open it in your browser.</i>"
	}
	a.tgText(ctx, m.Chat.ID, text, rows(a.appButton("Open BidSpot")))
}

// onHandshake: someone pressed Start on a sign-in or link deep link. They are asked to confirm,
// because the link may have been sent to them by someone else.
func (a *App) onHandshake(ctx context.Context, m *tgMessage, purpose, code string) {
	t, _ := a.Store.AuthTokenByCode(ctx, code)
	if t == nil || t.Purpose != purpose {
		a.tgText(ctx, m.Chat.ID, "That link has expired. Start again from the website.", nil)
		return
	}
	if ok, _ := a.Store.ResolveAuthToken(ctx, code, "pending", "waiting", 0, m.From.ID); !ok {
		a.tgText(ctx, m.Chat.ID, "That link was already used. Start again from the website.", nil)
		return
	}
	t.TgID = m.From.ID
	if err := a.sendConfirm(ctx, m.Chat.ID, *t); err != nil {
		log.Printf("bot: confirm prompt: %v", err)
	}
}

// sendConfirm asks "is this you?" with Approve / Deny buttons for a handshake.
func (a *App) sendConfirm(ctx context.Context, chat int64, t store.AuthToken) error {
	what := map[string]string{
		"login": "Sign in to BidSpot",
		"link":  "Link this Telegram account to your BidSpot account",
		"2fa":   "Approve a password sign-in to BidSpot",
	}[t.Purpose]
	text := fmt.Sprintf("🔐 <b>%s?</b>\n\nRequested from %s.\n\nOnly approve if you just did this yourself. Never approve a request someone sent you.",
		what, html.EscapeString(t.Client))
	return a.tgText(ctx, chat, text, [][]map[string]any{{
		{"text": "✅ Approve", "callback_data": "hs:ok:" + t.Code},
		{"text": "✖ Deny", "callback_data": "hs:no:" + t.Code},
	}})
}

func (a *App) onCallback(ctx context.Context, u tgUpdate) {
	q := u.CallbackQuery
	answer := func(text string) {
		a.tgCall(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": q.ID, "text": text})
	}
	edit := func(text string) {
		if q.Message != nil {
			a.tgCall(ctx, "editMessageText", map[string]any{"chat_id": q.Message.Chat.ID, "message_id": q.Message.MessageID, "text": text, "parse_mode": "HTML"})
		}
	}
	parts := strings.SplitN(q.Data, ":", 3)
	if len(parts) != 3 || parts[0] != "hs" {
		answer("")
		return
	}
	verdict, code := parts[1], parts[2]
	t, _ := a.Store.AuthTokenByCode(ctx, code)
	if t == nil || t.Status != "waiting" {
		answer("This request has expired.")
		edit("⌛ This request has expired.")
		return
	}
	if t.TgID != q.From.ID {
		answer("This request is not yours.")
		return
	}
	if verdict != "ok" {
		a.Store.ResolveAuthToken(ctx, code, "waiting", "denied", 0, 0)
		answer("Denied")
		edit("✖ Denied. Nobody was signed in.")
		return
	}
	var userID int64
	switch t.Purpose {
	case "login":
		acct, err := a.tgAccount(ctx, q.From.profile())
		if err != nil {
			answer("Something went wrong; try again.")
			return
		}
		userID = acct.ID
	case "link":
		if err := a.Store.SetUserTg(ctx, t.UserID, q.From.profile()); err != nil {
			a.Store.ResolveAuthToken(ctx, code, "waiting", "denied", 0, 0)
			answer("Already linked elsewhere")
			edit("✖ " + html.EscapeString(linkErr(err).Error()) + ".")
			return
		}
		userID = t.UserID
	case "2fa":
		userID = t.UserID
	}
	if ok, _ := a.Store.ResolveAuthToken(ctx, code, "waiting", "ok", userID, 0); !ok {
		answer("This request has expired.")
		return
	}
	answer("Approved")
	edit(map[string]string{
		"login": "✅ Signed in. You can go back to the browser.",
		"link":  "✅ Linked. Telegram now signs you in to this account.",
		"2fa":   "✅ Sign-in approved.",
	}[t.Purpose])
}

// --- Telegram Stars ---

func starsPayload(id int64) string { return "pay:" + strconv.FormatInt(id, 10) }

func parseStarsPayload(s string) int64 {
	id, _ := strconv.ParseInt(strings.TrimPrefix(s, "pay:"), 10, 64)
	return id
}

// preCheckout must be answered within 10 seconds: yes only if the payment is still open and the
// amount is the one we asked for.
func (a *App) preCheckout(ctx context.Context, u tgUpdate) {
	q := u.PreCheckoutQuery
	p, _ := a.Store.Payment(ctx, parseStarsPayload(q.InvoicePayload))
	ok := p != nil && p.Gateway == "stars" && p.Status == "pending" && q.Currency == "XTR" && q.TotalAmount == int(p.Amount)
	body := map[string]any{"pre_checkout_query_id": q.ID, "ok": ok}
	if !ok {
		body["error_message"] = "This checkout has expired. Start a new one in the app."
	}
	if _, err := a.tgCall(ctx, "answerPreCheckoutQuery", body); err != nil {
		log.Printf("bot: answerPreCheckoutQuery: %v", err)
	}
}

func (a *App) onStarsPaid(ctx context.Context, m *tgMessage) {
	sp := m.SuccessfulPayment
	id := parseStarsPayload(sp.InvoicePayload)
	paid, err := a.Store.CompletePayment(ctx, id, sp.ChargeID)
	if err != nil {
		log.Printf("bot: stars payment %d (%s): %v", id, sp.ChargeID, err)
		return
	}
	if paid {
		a.notifyPaid(ctx, id)
	}
}

// notifyPaid tells the payer in Telegram (when linked) that the money arrived.
func (a *App) notifyPaid(ctx context.Context, id int64) {
	p, _ := a.Store.Payment(ctx, id)
	if p == nil {
		return
	}
	log.Printf("payment %d paid: %g %s via %s (user %d)", p.ID, p.Amount, p.Currency, p.Gateway, p.UserID)
	u, _ := a.Store.UserByID(ctx, p.UserID)
	if u == nil || u.TgID == 0 || a.Cfg.TelegramBotToken == "" {
		return
	}
	var got []string
	if p.Credits > 0 {
		got = append(got, fmt.Sprintf("<b>%d</b> AI credits", p.Credits))
	}
	if p.ProDays > 0 {
		got = append(got, fmt.Sprintf("<b>%d days</b> of Pro", p.ProDays))
	}
	a.tgText(ctx, u.TgID, "💎 Payment received: "+strings.Join(got, " and ")+". Thank you!", nil)
}
