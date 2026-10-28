package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fragwatch/internal/llm"
)

// A short chat per concept: questions about how to build it, what goes in and out, who it is
// for. Every message is paid (Prices["chat"]); the thread is kept per concept so it can be
// reopened, and only the last few turns are sent back to the model to keep each message cheap.
const (
	chatKeep     = 40   // turns stored per concept
	chatContext  = 10   // turns sent to the model with each new question
	chatMaxQ     = 1200 // characters per question
	chatMaxReply = 900  // tokens per answer
)

const chatPrompt = `You are a pragmatic product and engineering advisor helping someone who wants to launch a project under a
Telegram username. You are given the concept. Answer their questions about it concretely, as someone who has
shipped Telegram bots, Mini Apps and channels:
- Be specific: name the APIs, data sources, libraries and Telegram features involved (Bot API, Mini Apps and
  initData, BotFather settings, deep links and start parameters, payments in Stars, channels, inline mode),
  and say what is and is not possible with public interfaces. If something needs data that is not public,
  say so plainly and offer the closest workable route.
- When asked how to build something, sketch the inputs, the processing and the outputs, with a small example
  of each where it helps (a sample input, a sample report line).
- Keep answers short and scannable: a few short paragraphs or a tight list, under 220 words unless they ask
  for more. Plain Markdown: **bold**, lists, inline code. No tables, no headings above level 3.
- Stay on this concept. If a question drifts, answer briefly and bring it back.
- If the concept as pitched depends on something Telegram or the data source does not allow, say so in the
  first line, then give the closest version that IS buildable (usually: users connect their own bot, account
  or data) and answer for that version.`

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	TS      int64  `json:"ts"`
}

func chatKey(id int64) string { return "chat:" + strconv.FormatInt(id, 10) }

func (a *App) loadChat(ctx context.Context, id int64) []chatMsg {
	msgs := []chatMsg{}
	if v := a.uget(ctx, chatKey(id)); v != "" {
		json.Unmarshal([]byte(v), &msgs)
	}
	return msgs
}

func (a *App) getConceptChat(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpErr(w, err, 400)
		return
	}
	writeJSON(w, map[string]any{"messages": a.loadChat(r.Context(), id), "price": Prices["chat"]})
}

// postConceptChat answers one question: {"question":"..."}.
func (a *App) postConceptChat(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpErr(w, err, 400)
		return
	}
	var b struct {
		Question string `json:"question"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || strings.TrimSpace(b.Question) == "" {
		httpErr(w, apiError("question is required"), 400)
		return
	}
	q := clip(b.Question, chatMaxQ)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	c, err := a.Store.Concept(ctx, uid(ctx), id)
	if err != nil || c == nil {
		httpErr(w, apiError("unknown concept"), 404)
		return
	}
	if a.paywall(w, r, "chat") {
		return
	}

	// the concept and the person go in the system message (cached across the thread)
	var sb strings.Builder
	sb.WriteString(chatPrompt)
	fmt.Fprintf(&sb, "\n\nThe concept: %s. %s", c.Project, c.Pitch)
	if c.FirstMove != "" {
		fmt.Fprintf(&sb, "\nFirst move suggested: %s", c.FirstMove)
	}
	if c.Form != "" {
		fmt.Fprintf(&sb, "\nForm in Telegram: %s", c.Form)
	}
	if c.BuiltWith != "" {
		fmt.Fprintf(&sb, "\nBuilt with: %s", c.BuiltWith)
	}
	if c.How != "" {
		fmt.Fprintf(&sb, "\nHow the handle is used: %s", c.How)
	}
	for _, n := range c.Names {
		fmt.Fprintf(&sb, "\nUsername: @%s (%s)", n.Name, n.Angle)
	}
	if s := a.settings(ctx); s.About != "" {
		fmt.Fprintf(&sb, "\n\nAbout the person asking: %s", s.About)
	}
	if v := a.settings(ctx).Avoid; v != "" {
		fmt.Fprintf(&sb, "\nThey are not interested in: %s. Do not steer the project there.", v)
	}

	msgs := a.loadChat(ctx, id)
	start := max(0, len(msgs)-chatContext)
	turns := make([]llm.Turn, 0, chatContext+1)
	for _, m := range msgs[start:] {
		turns = append(turns, llm.Turn{Role: m.Role, Content: m.Content})
	}
	turns = append(turns, llm.Turn{Role: "user", Content: q})

	answer, usage, err := a.LLM.Converse(ctx, sb.String(), turns, 0.5, chatMaxReply)
	a.addUsage(usage)
	if err != nil {
		httpErr(w, err, 502)
		return
	}
	a.charge(ctx, "chat", usage, c.Project)

	now := time.Now().Unix()
	msgs = append(msgs, chatMsg{Role: "user", Content: q, TS: now}, chatMsg{Role: "assistant", Content: answer, TS: now})
	if len(msgs) > chatKeep {
		msgs = msgs[len(msgs)-chatKeep:]
	}
	if js, err := json.Marshal(msgs); err == nil {
		a.uset(ctx, chatKey(id), string(js))
	}
	writeJSON(w, map[string]any{"messages": msgs, "usage": usage, "credits": a.balance(ctx)})
}
