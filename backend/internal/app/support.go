package app

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fragwatch/internal/i18n"
	"fragwatch/internal/store"
)

// Support tickets. People write from the bot (the Support button, /support, or just typing while a
// ticket is open) or from the web app; the team answers in the admin panel, or by replying to the
// bot's notice in Telegram. Every message is stored once and delivered to both sides: answers reach
// the person in Telegram (when linked) and in the app.

const supportMax = 4000 // Telegram's message limit, near enough

// kvSupportCompose: the person pressed Support in the bot; their next message opens a ticket.
const kvSupportCompose = "support_compose"

func (a *App) supportRoutes(mux *http.ServeMux) {
	// the account's own tickets
	mux.HandleFunc("GET /api/support", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ts, err := a.Store.UserTickets(ctx, uid(ctx), 50)
		if err != nil {
			httpErr(w, err, 500)
			return
		}
		writeJSON(w, map[string]any{"tickets": ts, "unread": a.Store.SupportUnread(ctx, uid(ctx)), "bot": a.botName()})
	})
	mux.HandleFunc("GET /api/support/unread", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]int{"unread": a.Store.SupportUnread(r.Context(), uid(r.Context()))})
	})
	// {"body": "…"}: a new ticket
	mux.HandleFunc("POST /api/support", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		body, ok := supportBody(w, r)
		if !ok {
			return
		}
		id, err := a.Store.CreateTicket(ctx, uid(ctx), subjectOf(body))
		if err == nil {
			_, err = a.supportFromUser(ctx, currentUser(ctx), id, body, "web")
		}
		if err != nil {
			httpErr(w, err, 500)
			return
		}
		a.writeUserTicket(w, r, id)
	})
	mux.HandleFunc("GET /api/support/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := a.ownTicket(w, r); ok {
			a.writeUserTicket(w, r, id)
		}
	})
	mux.HandleFunc("POST /api/support/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id, ok := a.ownTicket(w, r)
		if !ok {
			return
		}
		body, ok := supportBody(w, r)
		if !ok {
			return
		}
		if _, err := a.supportFromUser(ctx, currentUser(ctx), id, body, "web"); err != nil {
			httpErr(w, err, 500)
			return
		}
		a.writeUserTicket(w, r, id)
	})
	mux.HandleFunc("POST /api/support/{id}/close", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := a.ownTicket(w, r); ok {
			a.Store.SetTicketStatus(r.Context(), id, "closed")
			a.writeUserTicket(w, r, id)
		}
	})

	// --- the team's side ---

	// ?status=open|closed|all
	mux.HandleFunc("GET /api/admin/support", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		status := r.URL.Query().Get("status")
		if status == "all" {
			status = ""
		} else if status != "closed" {
			status = "open"
		}
		ts, err := a.Store.Tickets(ctx, status, 300)
		if err != nil {
			httpErr(w, err, 500)
			return
		}
		labels := map[int64]string{}
		for i := range ts {
			ts[i].User = a.labelOf(ctx, labels, ts[i].UserID)
		}
		writeJSON(w, map[string]any{"tickets": ts, "waiting": a.Store.SupportWaiting(ctx)})
	})
	mux.HandleFunc("GET /api/admin/support/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		a.writeStaffTicket(w, r, id)
	})
	// {"body": "…"}: the team answers
	mux.HandleFunc("POST /api/admin/support/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		body, ok := supportBody(w, r)
		if !ok {
			return
		}
		if err := a.supportFromStaff(ctx, currentUser(ctx), id, body, "web"); err != nil {
			httpErr(w, err, 500)
			return
		}
		a.writeStaffTicket(w, r, id)
	})
	// {"status": "open"|"closed"}
	mux.HandleFunc("POST /api/admin/support/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		var b struct {
			Status string `json:"status"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		if b.Status != "open" && b.Status != "closed" {
			httpErr(w, apiError("status must be open or closed"), 400)
			return
		}
		if t, _ := a.Store.Ticket(ctx, id, true); t == nil {
			httpErr(w, errNotFound, 404)
			return
		}
		a.Store.SetTicketStatus(ctx, id, b.Status)
		a.writeStaffTicket(w, r, id)
	})
	// {"body": "…"}: write to an account first, from the bot's name. Goes into their open ticket,
	// or opens one.
	mux.HandleFunc("POST /api/admin/users/{id}/message", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		u, _ := a.Store.UserByID(ctx, userID)
		if u == nil {
			httpErr(w, errNotFound, 404)
			return
		}
		body, ok := supportBody(w, r)
		if !ok {
			return
		}
		var id int64
		if t, err := a.Store.OpenTicket(ctx, userID); err != nil {
			httpErr(w, err, 500)
			return
		} else if t != nil {
			id = t.ID
		} else if id, err = a.Store.CreateTicket(ctx, userID, subjectOf(body)); err != nil {
			httpErr(w, err, 500)
			return
		}
		if err := a.supportFromStaff(ctx, currentUser(ctx), id, body, "web"); err != nil {
			httpErr(w, err, 500)
			return
		}
		a.writeStaffTicket(w, r, id)
	})
	// the account's open ticket, if any, so the panel can show the thread before writing
	mux.HandleFunc("GET /api/admin/users/{id}/ticket", func(w http.ResponseWriter, r *http.Request) {
		userID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		t, err := a.Store.OpenTicket(r.Context(), userID)
		if err != nil {
			httpErr(w, err, 500)
			return
		}
		if t == nil {
			writeJSON(w, map[string]any{"ticket": nil})
			return
		}
		a.writeStaffTicket(w, r, t.ID)
	})
}

func supportBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	var b struct {
		Body string `json:"body"`
	}
	json.NewDecoder(r.Body).Decode(&b)
	body := clip(b.Body, supportMax)
	if body == "" {
		httpErr(w, apiError("write a message first"), 400)
		return "", false
	}
	return body, true
}

// subjectOf: a ticket is named after the start of its first message.
func subjectOf(body string) string {
	line, _, _ := strings.Cut(body, "\n")
	if r := []rune(line); len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return line
}

// ownTicket: the {id} ticket, when it belongs to the signed-in account.
func (a *App) ownTicket(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	t, _ := a.Store.Ticket(r.Context(), id, false)
	if t == nil || t.UserID != uid(r.Context()) {
		httpErr(w, errNotFound, 404)
		return 0, false
	}
	return id, true
}

// writeUserTicket answers with the ticket and its thread as the account sees it, and marks it read.
func (a *App) writeUserTicket(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	a.Store.MarkTicketSeen(ctx, id, false)
	t, _ := a.Store.Ticket(ctx, id, false)
	ms, err := a.Store.SupportMessages(ctx, id)
	if err != nil || t == nil {
		httpErr(w, errNotFound, 404)
		return
	}
	writeJSON(w, map[string]any{"ticket": t, "messages": ms})
}

// writeStaffTicket is the same for the team, with who wrote each answer and whom the ticket is from.
func (a *App) writeStaffTicket(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	a.Store.MarkTicketSeen(ctx, id, true)
	t, _ := a.Store.Ticket(ctx, id, true)
	ms, err := a.Store.SupportMessages(ctx, id)
	if err != nil || t == nil {
		httpErr(w, errNotFound, 404)
		return
	}
	labels := map[int64]string{}
	t.User = a.labelOf(ctx, labels, t.UserID)
	for i := range ms {
		if ms[i].Staff {
			ms[i].Author = a.labelOf(ctx, labels, ms[i].AuthorID)
		}
	}
	var user map[string]any
	if u, _ := a.Store.UserByID(ctx, t.UserID); u != nil {
		user = a.userJSON(u)
	}
	writeJSON(w, map[string]any{"ticket": t, "messages": ms, "user": user})
}

func (a *App) labelOf(ctx context.Context, cache map[int64]string, id int64) string {
	if l, ok := cache[id]; ok {
		return l
	}
	l := "#" + strconv.FormatInt(id, 10)
	if u, _ := a.Store.UserByID(ctx, id); u != nil {
		l = u.Label()
	}
	cache[id] = l
	return l
}

// supportFromUser stores what the account wrote and tells the team in Telegram.
func (a *App) supportFromUser(ctx context.Context, u *store.User, ticketID int64, body, via string) (store.SupportMessage, error) {
	m, err := a.Store.AddSupportMessage(ctx, ticketID, u.ID, false, body, via)
	if err != nil {
		return m, err
	}
	a.Store.Log(ctx, u.ID, 0, "support.message", fmt.Sprintf("#%d via %s", ticketID, via))
	go a.notifyStaff(ticketID, u, body)
	return m, nil
}

// supportFromStaff stores the team's answer and delivers it to the account's Telegram.
func (a *App) supportFromStaff(ctx context.Context, me *store.User, ticketID int64, body, via string) error {
	t, _ := a.Store.Ticket(ctx, ticketID, true)
	if t == nil {
		return errNotFound
	}
	if _, err := a.Store.AddSupportMessage(ctx, ticketID, me.ID, true, body, via); err != nil {
		return err
	}
	a.Store.Log(ctx, t.UserID, me.ID, "support.reply", fmt.Sprintf("#%d via %s", ticketID, via))
	u, _ := a.Store.UserByID(ctx, t.UserID)
	if u == nil || u.TgID == 0 || a.Cfg.TelegramBotToken == "" {
		return nil // they read it in the app
	}
	l := a.userLang(ctx, u.ID)
	err := a.tgText(ctx, u.TgID, i18n.T(l, "support_reply", ticketID, html.EscapeString(body)), [][]map[string]any{{
		{"text": i18n.T(l, "btn_solved"), "callback_data": "sup:close:" + strconv.FormatInt(ticketID, 10)},
	}})
	if err != nil {
		// stored all the same: the app shows it, and Telegram may just be blocked by the person
		log.Printf("support: deliver #%d to tg %d: %v", ticketID, u.TgID, err)
	}
	return nil
}

// notifyStaff sends a new message to every admin with Telegram linked. Replying to that notice
// answers the ticket (see onStaffReply).
func (a *App) notifyStaff(ticketID int64, from *store.User, body string) {
	if a.Cfg.TelegramBotToken == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, chat := range a.Store.AdminTgIDs(ctx) {
		if chat == from.TgID {
			continue
		}
		var l string
		if u, _ := a.Store.UserByTg(ctx, chat); u != nil {
			l = a.userLang(ctx, u.ID)
		}
		if err := a.tgText(ctx, chat, i18n.T(l, "support_staff_new", ticketID, html.EscapeString(from.Label()), html.EscapeString(body)), nil); err != nil {
			log.Printf("support: notify admin %d: %v", chat, err)
		}
	}
}

// --- in the bot ---

var ticketRef = regexp.MustCompile(`#(\d+)`)

// onSupportText handles a plain message to the bot. It answers true when the message was support's:
// an admin replying to a ticket notice, a message into an open ticket, or the first message after
// pressing Support.
func (a *App) onSupportText(ctx context.Context, m *tgMessage, l string) bool {
	u, _ := a.Store.UserByTg(ctx, m.From.ID)
	// a reply to one of the bot's ticket messages ("#12" in its first line)
	var replied int64
	if m.ReplyTo != nil && m.ReplyTo.From != nil && m.ReplyTo.From.Username == a.botName() {
		if match := ticketRef.FindStringSubmatch(firstLine(m.ReplyTo.Text)); match != nil {
			replied, _ = strconv.ParseInt(match[1], 10, 64)
		}
	}
	var open *store.Ticket
	if u != nil && replied != 0 {
		if t, _ := a.Store.Ticket(ctx, replied, false); t != nil && t.UserID == u.ID {
			open = t // their own ticket, even a closed one: writing reopens it
		} else if u.Admin() {
			a.onStaffReply(ctx, m, u, replied, l)
			return true
		}
	}
	if u != nil && open == nil {
		open, _ = a.Store.OpenTicket(ctx, u.ID)
	}
	composing := false
	if u != nil {
		at, _ := strconv.ParseInt(a.Store.GetUserKV(ctx, u.ID, kvSupportCompose), 10, 64)
		composing = at > time.Now().Add(-time.Hour).Unix()
	}
	if open == nil && !composing {
		return false
	}
	body := clip(m.Text, supportMax)
	if body == "" {
		a.tgText(ctx, m.Chat.ID, i18n.T(l, "support_text_only"), nil)
		return true
	}
	id := int64(0)
	if open != nil {
		id = open.ID
	} else {
		var err error
		if id, err = a.Store.CreateTicket(ctx, u.ID, subjectOf(body)); err != nil {
			log.Printf("support: new ticket: %v", err)
			a.tgText(ctx, m.Chat.ID, i18n.T(l, "oops"), nil)
			return true
		}
	}
	if composing {
		a.Store.SetUserKV(ctx, u.ID, kvSupportCompose, "")
	}
	if _, err := a.supportFromUser(ctx, u, id, body, "bot"); err != nil {
		log.Printf("support: #%d from bot: %v", id, err)
		a.tgText(ctx, m.Chat.ID, i18n.T(l, "oops"), nil)
		return true
	}
	if open == nil {
		a.tgText(ctx, m.Chat.ID, i18n.T(l, "support_sent", id), nil)
	} else {
		// a follow-up: a quiet 👌 instead of another message
		a.tgCall(ctx, "setMessageReaction", map[string]any{"chat_id": m.Chat.ID, "message_id": m.MessageID,
			"reaction": []map[string]string{{"type": "emoji", "emoji": "👌"}}})
	}
	return true
}

func (a *App) onStaffReply(ctx context.Context, m *tgMessage, me *store.User, id int64, l string) {
	body := clip(m.Text, supportMax)
	if body == "" {
		a.tgText(ctx, m.Chat.ID, i18n.T(l, "support_text_only"), nil)
		return
	}
	if err := a.supportFromStaff(ctx, me, id, body, "bot"); err != nil {
		a.tgText(ctx, m.Chat.ID, i18n.T(l, "support_no_ticket", id), nil)
		return
	}
	a.tgCall(ctx, "setMessageReaction", map[string]any{"chat_id": m.Chat.ID, "message_id": m.MessageID,
		"reaction": []map[string]string{{"type": "emoji", "emoji": "👌"}}})
}

// startSupport: /support or the Support button. The next message opens a ticket (or, with one
// open already, goes into it).
func (a *App) startSupport(ctx context.Context, from tgFrom, chat int64, l string) {
	u, err := a.tgAccount(ctx, from.profile())
	if err != nil {
		a.tgText(ctx, chat, i18n.T(l, "oops"), nil)
		return
	}
	a.rememberLang(ctx, u.ID, from.LanguageCode)
	if t, _ := a.Store.OpenTicket(ctx, u.ID); t != nil {
		a.tgText(ctx, chat, i18n.T(l, "support_open", t.ID), nil)
		return
	}
	a.Store.SetUserKV(ctx, u.ID, kvSupportCompose, strconv.FormatInt(time.Now().Unix(), 10))
	a.tgText(ctx, chat, i18n.T(l, "support_prompt"), nil)
}

// onSupportCallback: sup:new (the Support button) and sup:close:<id> (Solved under an answer).
func (a *App) onSupportCallback(ctx context.Context, q tgCallback, l string, parts []string, answer, edit func(string)) {
	switch {
	case len(parts) == 2 && parts[1] == "new":
		answer("")
		chat := q.From.ID
		if q.Message != nil {
			chat = q.Message.Chat.ID
		}
		a.startSupport(ctx, q.From, chat, l)
	case len(parts) == 3 && parts[1] == "close":
		id, _ := strconv.ParseInt(parts[2], 10, 64)
		u, _ := a.Store.UserByTg(ctx, q.From.ID)
		t, _ := a.Store.Ticket(ctx, id, false)
		if u == nil || t == nil || t.UserID != u.ID {
			answer(i18n.T(l, "not_yours"))
			return
		}
		a.Store.SetTicketStatus(ctx, id, "closed")
		answer(i18n.T(l, "support_closed_toast"))
		if q.Message != nil {
			// keep the answer, drop the button
			a.tgCall(ctx, "editMessageReplyMarkup", map[string]any{"chat_id": q.Message.Chat.ID, "message_id": q.Message.MessageID})
		}
		a.tgText(ctx, q.From.ID, i18n.T(l, "support_closed", id), nil)
	default:
		answer("")
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
