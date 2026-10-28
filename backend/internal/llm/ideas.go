package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const tastePrompt = `You write a short taste profile of someone who collects Telegram usernames, from the names they
liked and skipped. Describe what they go for in 60-90 words: length, sound and shape (real words,
invented brands, compounds, leet, underscores), themes and kinds of name, and what to avoid. Be
concrete and name a few of the liked names as examples. Write it as instructions to a naming expert
("They like …, avoid …"). If their note about themselves is given, fold it in.

Output ONLY JSON: {"profile":"..."}`

// TasteProfile distils liked and skipped names into a few lines that steer later suggestions.
// It is cached by the caller, so it costs one call per change of taste.
func (c *Client) TasteProfile(ctx context.Context, likes, skips []string, about, behaviour string) (string, Usage, error) {
	var b strings.Builder
	if len(likes) > 0 {
		fmt.Fprintf(&b, "Liked: %s\n", strings.Join(likes, ", "))
	}
	if len(skips) > 0 {
		fmt.Fprintf(&b, "Skipped: %s\n", strings.Join(skips, ", "))
	}
	if behaviour = strings.TrimSpace(behaviour); behaviour != "" {
		b.WriteString(behaviour + "\n")
	}
	if about = strings.TrimSpace(about); about != "" {
		fmt.Fprintf(&b, "About them: %s\n", about)
	}
	content, usage, err := c.Chat(ctx, tastePrompt, b.String(), 0.3)
	if err != nil {
		return "", usage, err
	}
	var doc struct {
		Profile string `json:"profile"`
	}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return "", usage, fmt.Errorf("bad model JSON: %w", err)
	}
	return strings.TrimSpace(doc.Profile), usage, nil
}

// feasible is appended to every prompt that invents a project: a concept that cannot be built with
// public interfaces is worse than no concept. The list names where Telegram ideas usually break.
const feasible = `

HARD RULE: every project must be buildable by one developer or a small team today, with public APIs, public
data or data the users themselves hand over. If it needs data nobody can get, it is not a concept: drop it.
What Telegram does NOT allow (never build on these):
- reading another bot's permissions, settings, admin rights, code or users;
- reading messages, members or activity of chats the bot is not in, or private chats between users;
- listing all bots, channels or users; seeing who viewed or forwarded something; a user's contacts or phone
  number unless they share it;
- messaging users who have not started the bot first; joining chats or acting as a user (userbots and mass
  scraping break the terms and get banned).
What it DOES allow: bots in chats where they are added (they see what admins let them), Mini Apps with the
user's own profile (initData) and whatever the user types, uploads or connects (OAuth, API keys, wallets),
channels and posting, payments in Stars, inline mode, deep links. Public web data, public APIs and the TON
blockchain (including Fragment listings and NFT ownership) are fair game.
A concept built around users connecting their OWN account, bot, wallet or data is fine.`

const ideasPrompt = `You are a creative director who invents projects around Telegram usernames that are for sale right now.
You get a list of names on the market, one per line as: name | meaning | price. Build project concepts that
could launch under one of these names.

What makes a concept good:
- It is a real service, tool, media format, community, game or studio that someone could start, with a clear
  thing it does: "parse this, make that, deliver this". Not a to-do list, not a crypto exchange, not a token,
  not a generic shop, not "a bot for X". The more specific and surprising the better.
- The name is read creatively, not literally. "betspot" can become Bid Spot, a place to bid on usernames;
  "frogvoice" can be a voice-note zine. Say how the name should be read.
- The name should be worth more inside the concept than it costs on the list.

Return exactly %d concepts. Each: "project" (a brand name, 1-3 words), "pitch" (one sentence, ≤ 25 words,
what it is and for whom), "built_with" (≤ 22 words: the data it runs on, where that data comes from, and the
Telegram surface, e.g. "Mini App + bot; users upload their own CSV; an LLM writes the summary"),
"first_move" (≤ 15 words: the first concrete thing to build or publish),
"names": 1-3 usernames from the list ONLY, each with "angle" (≤ 12 words: how the name reads in this concept).
Spend the concepts across different names; do not reuse a name in two concepts.

Output ONLY JSON: {"ideas":[{"project":"...","pitch":"...","built_with":"...","first_move":"...","names":[{"name":"...","angle":"..."}]}]}` + feasible

type IdeaName struct {
	Name  string `json:"name"`
	Angle string `json:"angle"`
}

type Idea struct {
	Project   string     `json:"project"`
	Pitch     string     `json:"pitch"`
	BuiltWith string     `json:"built_with"`
	FirstMove string     `json:"first_move"`
	Names     []IdeaName `json:"names"`
}

// Ideas invents project concepts over the candidate names. profile is the user's taste profile
// (may be empty); candidates are "name | meaning | price" lines, already filtered to a budget.
// direction, when given, is the person's own steer for this concept ("make it about audio").
func (c *Client) Ideas(ctx context.Context, profile string, candidates []string, n int, avoid []string, direction string) ([]Idea, Usage, error) {
	// the static prompt goes first so the provider's prompt cache can serve it; everything that
	// varies per person or per call follows
	system := fmt.Sprintf(ideasPrompt, n)
	if profile != "" {
		system += "\n\nTaste of the person you are working for:\n" + profile
	}
	if len(avoid) > 0 {
		system += "\n\nConcepts they already have; make something clearly different from these: " + strings.Join(avoid, "; ")
	}
	user := "Names on the market:\n" + strings.Join(candidates, "\n")
	if direction != "" {
		user += "\n\nTheir direction, which the concept must follow: " + direction
	}
	content, usage, err := c.Chat(ctx, system, user, 0.8)
	if err != nil {
		return nil, usage, err
	}
	var doc struct {
		Ideas []Idea `json:"ideas"`
	}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return nil, usage, fmt.Errorf("bad model JSON: %w", err)
	}
	for i := range doc.Ideas {
		for j := range doc.Ideas[i].Names {
			doc.Ideas[i].Names[j].Name = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(doc.Ideas[i].Names[j].Name), "@"))
		}
	}
	return doc.Ideas, usage, nil
}

const anglePrompt = `You are a creative director. Someone is looking at one Telegram username that is for sale and wants to
know whether it is worth owning. Read the name creatively, not literally (letters can split, words can be
re-heard: "betspot" is also Bid Spot). Answer in two short parts:
- "angle": ≤ 20 words, how the name reads and what it could stand for.
- "project": ≤ 18 words, one specific thing they could build or publish under it: a service, tool, media
  format, community or game, not a shop, token, exchange or "a bot for X".
- "fit": 0-10, how well it suits the person described (5 if no profile is given).

Output ONLY JSON: {"angle":"...","project":"...","fit":7}` + feasible

type Angle struct {
	Angle   string `json:"angle"`
	Project string `json:"project"`
	Fit     int    `json:"fit"`
}

const imaginePrompt = `You are a creative director. Someone owns, or is about to buy, ONE Telegram username and wants a project to
run under it. Read the name creatively, not literally (letters can split, words can be re-heard: "betspot" is
also Bid Spot). Invent one specific project: a real service, tool, media format, community, game or studio
with a clear thing it does ("parse this, make that, deliver this"). Not a to-do list, not a crypto exchange,
not a token, not a generic shop, not "a bot for X".

Answer with:
- "project": a brand name, 1-3 words, that reads out of the username.
- "pitch": one sentence, ≤ 25 words, what it is and for whom.
- "form": the shape it takes in Telegram, one of: "mini app", "bot", "channel", "mini app + bot",
  "channel + bot", "web app + bot", "web app". Pick what the project really needs.
- "how": ≤ 30 words, how the username is used day to day: the handle people type, the t.me link, the
  bot/channel/app it names, what the brand looks like.
- "built_with": ≤ 22 words, the data it runs on, where that data comes from, and the key APIs.
- "first_move": ≤ 15 words, the first concrete thing to build or publish.
- "fit": 0-10, how well it suits the person described (5 if no profile is given).

Output ONLY JSON: {"project":"...","pitch":"...","form":"...","how":"...","built_with":"...","first_move":"...","fit":7}` + feasible

type Imagined struct {
	Project   string `json:"project"`
	Pitch     string `json:"pitch"`
	Form      string `json:"form"`
	How       string `json:"how"`
	FirstMove string `json:"first_move"`
	Fit       int    `json:"fit"`
	BuiltWith string `json:"built_with"`
}

// Imagine makes one project for one name. prior are titles already made for it, so the new one
// goes elsewhere; direction is the person's own steer, if any.
func (c *Client) Imagine(ctx context.Context, profile, name, meaning, price string, prior []string, direction string) (Imagined, Usage, error) {
	system := imaginePrompt
	if profile != "" {
		system += "\n\nThe person:\n" + profile
	}
	user := fmt.Sprintf("Username: %s\nMeaning: %s\nPrice: %s", name, meaning, price)
	if len(prior) > 0 {
		user += "\nProjects already made for this name; make something clearly different: " + strings.Join(prior, "; ")
	}
	if direction != "" {
		user += "\nTheir direction, which the project must follow: " + direction
	}
	content, usage, err := c.Chat(ctx, system, user, 0.9)
	var out Imagined
	if err != nil {
		return out, usage, err
	}
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return out, usage, fmt.Errorf("bad model JSON: %w", err)
	}
	out.Fit = min(10, max(0, out.Fit))
	return out, usage, nil
}

const anglesPrompt = `You are a creative director. Someone is looking at a grid of Telegram usernames that are for sale, one per
line as: name | meaning | price. For EACH name, read it creatively, not literally (letters can split, words
can be re-heard: "betspot" is also Bid Spot) and answer:
- "angle": ≤ 20 words, how the name reads and what it could stand for.
- "project": ≤ 18 words, one specific thing they could build or publish under it: a service, tool, media
  format, community or game, not a shop, token, exchange or "a bot for X".
- "fit": 0-10, how well it suits the person described (5 if no profile is given).
Cover every name once; use the names exactly as given.

Output ONLY JSON: {"names":[{"name":"...","angle":"...","project":"...","fit":7}]}` + feasible

// NameAngles reads a grid of names in one call: ~10x cheaper per name than NameAngle.
func (c *Client) NameAngles(ctx context.Context, profile string, lines []string) (map[string]Angle, Usage, error) {
	system := anglesPrompt
	if profile != "" {
		system += "\n\nThe person:\n" + profile
	}
	content, usage, err := c.Chat(ctx, system, strings.Join(lines, "\n"), 0.7)
	if err != nil {
		return nil, usage, err
	}
	var doc struct {
		Names []struct {
			Name string `json:"name"`
			Angle
		} `json:"names"`
	}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return nil, usage, fmt.Errorf("bad model JSON: %w", err)
	}
	out := map[string]Angle{}
	for _, n := range doc.Names {
		name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(n.Name), "@"))
		n.Fit = min(10, max(0, n.Fit))
		out[name] = n.Angle
	}
	return out, usage, nil
}

// NameAngle reads one name for one person: a tiny call, cached by the caller per name and profile.
func (c *Client) NameAngle(ctx context.Context, profile, name, meaning, price string) (Angle, Usage, error) {
	system := anglePrompt
	if profile != "" {
		system += "\n\nThe person:\n" + profile
	}
	user := fmt.Sprintf("Name: %s\nMeaning: %s\nPrice: %s", name, meaning, price)
	content, usage, err := c.Chat(ctx, system, user, 0.7)
	var a Angle
	if err != nil {
		return a, usage, err
	}
	if err := json.Unmarshal([]byte(content), &a); err != nil {
		return a, usage, fmt.Errorf("bad model JSON: %w", err)
	}
	a.Fit = min(10, max(0, a.Fit))
	return a, usage, nil
}
