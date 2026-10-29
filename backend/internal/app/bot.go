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

	"fragwatch/internal/i18n"
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
	// LanguageCode is the language of their Telegram app (IETF tag, "ru", "en-US")
	LanguageCode string `json:"language_code"`
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
	a.setBotProfile(ctx)

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

// setBotProfile writes the commands, About and Description in every language. English is the
// default Telegram shows; Russian goes to people whose Telegram app is in Russian.
func (a *App) setBotProfile(ctx context.Context) {
	for _, l := range i18n.Langs {
		scope := func(body map[string]any) map[string]any {
			if l != i18n.EN {
				body["language_code"] = l
			}
			return body
		}
		a.tgCall(ctx, "setMyCommands", scope(map[string]any{"commands": []map[string]string{
			{"command": "start", "description": i18n.T(l, "cmd_start")},
			{"command": "balance", "description": i18n.T(l, "cmd_balance")},
		}}))
		if _, err := a.tgCall(ctx, "setMyShortDescription", scope(map[string]any{"short_description": i18n.T(l, "about")})); err != nil {
			log.Printf("bot: setMyShortDescription %s: %v", l, err)
		}
		if _, err := a.tgCall(ctx, "setMyDescription", scope(map[string]any{"description": i18n.T(l, "description")})); err != nil {
			log.Printf("bot: setMyDescription %s: %v", l, err)
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
	l := a.chatLang(ctx, *m.From)
	switch cmd {
	case "/start":
		if purpose, code, ok := strings.Cut(arg, "_"); ok && purpose == "login" {
			a.onHandshake(ctx, m, l, purpose, code)
			return
		}
		a.welcome(ctx, m, l)
	case "/balance":
		u, _ := a.Store.UserByTg(ctx, m.From.ID)
		if u == nil {
			a.tgText(ctx, m.Chat.ID, i18n.T(l, "no_account"), rows(a.appButton(i18n.T(l, "open_app"))))
			return
		}
		ctx = withUser(ctx, u)
		plan := "Free"
		if u.Plan() == "pro" {
			plan = "Pro"
			if u.PlanRaw != "pro" {
				plan = i18n.T(l, "plan_until", plan, time.Unix(u.PlanUntil, 0).UTC().Format(i18n.T(l, "date")))
			}
		}
		a.tgText(ctx, m.Chat.ID, i18n.T(l, "balance", a.balance(ctx), plan), rows(a.appButton(i18n.T(l, "top_up"))))
	default:
		a.welcome(ctx, m, l)
	}
}

func rows(btn []map[string]any) [][]map[string]any {
	if btn == nil {
		return nil
	}
	return [][]map[string]any{btn}
}

func (a *App) welcome(ctx context.Context, m *tgMessage, l string) {
	text := i18n.T(l, "welcome")
	if a.appButton("x") == nil {
		text += i18n.T(l, "welcome_local")
	}
	a.tgText(ctx, m.Chat.ID, text, rows(a.appButton(i18n.T(l, "open_app"))))
}

// onHandshake: someone pressed Start on a sign-in deep link. They are asked to confirm,
// because the link may have been sent to them by someone else.
func (a *App) onHandshake(ctx context.Context, m *tgMessage, l, purpose, code string) {
	t, _ := a.Store.AuthTokenByCode(ctx, code)
	if t == nil || t.Purpose != purpose {
		a.tgText(ctx, m.Chat.ID, i18n.T(l, "link_expired"), nil)
		return
	}
	ok, err := a.Store.ResolveAuthToken(ctx, code, "pending", "waiting", 0, m.From.ID)
	if err != nil {
		log.Printf("bot: handshake %s: %v", purpose, err)
		a.tgText(ctx, m.Chat.ID, i18n.T(l, "link_error"), nil)
		return
	}
	if !ok {
		a.tgText(ctx, m.Chat.ID, i18n.T(l, "link_used"), nil)
		return
	}
	t.TgID = m.From.ID
	if err := a.sendConfirm(ctx, m.Chat.ID, l, *t); err != nil {
		log.Printf("bot: confirm prompt: %v", err)
	}
}

// sendConfirm asks "is this you?" with Approve / Deny buttons for a handshake.
func (a *App) sendConfirm(ctx context.Context, chat int64, l string, t store.AuthToken) error {
	return a.tgText(ctx, chat, i18n.T(l, "confirm", html.EscapeString(t.Client)), [][]map[string]any{{
		{"text": i18n.T(l, "approve"), "callback_data": "hs:ok:" + t.Code},
		{"text": i18n.T(l, "deny"), "callback_data": "hs:no:" + t.Code},
	}})
}

func (a *App) onCallback(ctx context.Context, u tgUpdate) {
	q := u.CallbackQuery
	l := a.chatLang(ctx, q.From)
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
		answer(i18n.T(l, "req_expired"))
		edit(i18n.T(l, "req_gone"))
		return
	}
	if t.TgID != q.From.ID {
		answer(i18n.T(l, "not_yours"))
		return
	}
	if verdict != "ok" {
		a.Store.ResolveAuthToken(ctx, code, "waiting", "denied", 0, 0)
		answer(i18n.T(l, "denied_toast"))
		edit(i18n.T(l, "denied"))
		return
	}
	acct, err := a.tgAccount(ctx, q.From.profile())
	if err != nil {
		answer(i18n.T(l, "oops"))
		return
	}
	a.rememberLang(ctx, acct.ID, q.From.LanguageCode)
	if ok, _ := a.Store.ResolveAuthToken(ctx, code, "waiting", "ok", acct.ID, 0); !ok {
		answer(i18n.T(l, "req_expired"))
		return
	}
	answer(i18n.T(l, "ok_toast"))
	edit(i18n.T(l, "signed_in"))
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
		body["error_message"] = i18n.T(a.chatLang(ctx, q.From), "checkout_gone")
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
	a.Store.Log(ctx, p.UserID, 0, "pay.paid", fmt.Sprintf("%s: %g %s via %s", p.Pack, p.Amount, p.Currency, p.Gateway))
	u, _ := a.Store.UserByID(ctx, p.UserID)
	if u == nil || u.TgID == 0 || a.Cfg.TelegramBotToken == "" {
		return
	}
	l := a.userLang(ctx, u.ID)
	var got []string
	if p.Credits > 0 {
		got = append(got, i18n.T(l, "paid_credits", i18n.Num(l, p.Credits), i18n.N(l, "w_credits", p.Credits)))
	}
	if p.ProDays > 0 {
		got = append(got, i18n.T(l, "paid_pro", p.ProDays, i18n.N(l, "w_days", p.ProDays)))
	}
	a.tgText(ctx, u.TgID, i18n.T(l, "paid", strings.Join(got, i18n.T(l, "and"))), nil)
}
