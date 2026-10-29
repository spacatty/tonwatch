package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const expandPrompt = `You help someone search a database of Telegram usernames (a-z, 0-9, "_").
Given a topic, list the word stems that would appear inside usernames about it.

Return 15-30 lowercase stems, 2-10 letters each, no spaces: the topic word itself, synonyms,
closely related words, common abbreviations and short forms, and the same idea in other major
languages written in Latin letters when they are common. Prefer stems that are distinctive;
avoid stems so short or generic they match everything ("a", "go", "me").

Example topic "vpn": vpn, proxy, tunnel, secure, privacy, private, shield, guard, safe, hide,
anon, incognito, stealth, cloak, firewall, encrypt, net, socks, wireguard, tor

Output ONLY JSON: {"keywords":["...", "..."]}`

var stemRe = regexp.MustCompile(`^[a-z0-9_]{2,16}$`)

// Expand turns a topic ("vpn", "coffee shop") into word stems to search names and meanings for.
func (c *Client) Expand(ctx context.Context, topic string) ([]string, Usage, error) {
	content, usage, err := c.Chat(ctx, expandPrompt, "Topic: "+topic, 0.3, 800)
	if err != nil {
		return nil, usage, err
	}
	var doc struct {
		Keywords []string `json:"keywords"`
	}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return nil, usage, fmt.Errorf("bad model JSON: %w", err)
	}
	return cleanList(doc.Keywords, stemRe, 30), usage, nil
}

func cleanList(in []string, re *regexp.Regexp, limit int) []string {
	seen := map[string]bool{}
	var out []string
	for _, k := range in {
		k = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(k, "@")))
		if re.MatchString(k) && !seen[k] && len(out) < limit {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

const planPrompt = `You are a naming expert helping someone find a Telegram username (a-z, 0-9, "_", 4-32 chars) for their project.
Read their request and return:
- "keywords": 15-30 lowercase word stems (2-10 letters) that a fitting existing username would contain: the core concept,
  synonyms, related words, short forms, and words from other languages written in Latin letters when natural.
- "ideas": 15 fresh, brandable username ideas for this project, 4-15 characters, only a-z, 0-9 and "_".
  Mix short invented brand names, clean compounds and memorable words. No generic filler like "best" or "official".
- "summary": one short sentence (≤ 15 words) restating what they need.

Output ONLY JSON: {"summary":"...","keywords":[...],"ideas":[...]}`

var ideaRe = regexp.MustCompile(`^[a-z][a-z0-9_]{3,31}$`)

type Plan struct {
	Summary  string   `json:"summary"`
	Keywords []string `json:"keywords"`
	Ideas    []string `json:"ideas"`
}

// PlanSearch reads a free-text request and decides what to search for.
func (c *Client) PlanSearch(ctx context.Context, brief string) (Plan, Usage, error) {
	var p Plan
	content, usage, err := c.Chat(ctx, InLang(ctx, planPrompt), brief, 0.6, 1500)
	if err != nil {
		return p, usage, err
	}
	if err := json.Unmarshal([]byte(content), &p); err != nil {
		return p, usage, fmt.Errorf("bad model JSON: %w", err)
	}
	p.Keywords = cleanList(p.Keywords, stemRe, 30)
	p.Ideas = cleanList(p.Ideas, ideaRe, 20)
	return p, usage, nil
}

const rankPrompt = `You are a naming expert. Someone described their project, and here are Telegram usernames that exist
on the market, one per line as: name | meaning | availability.

Pick the ones that genuinely fit their project and rank them, best first. Judge relevance to the project first,
then how clean, memorable and brandable the name is. Skip names that only match by accident
(e.g. "vpn" hidden inside an unrelated word) and names that are spammy.

Return at most 25, each with fit 0-10 and a reason of at most 12 words saying why it fits.
Output ONLY JSON: {"r":[["name",fit,"reason"], ...]}`

type Pick struct {
	Name   string `json:"name"`
	Fit    int    `json:"fit"`
	Reason string `json:"reason"`
}

// Rank asks the model which candidates fit the brief. candidates are "name | meaning | availability" lines.
func (c *Client) Rank(ctx context.Context, brief string, candidates []string) ([]Pick, Usage, error) {
	user := "Project: " + brief + "\n\nCandidates:\n" + strings.Join(candidates, "\n")
	content, usage, err := c.Chat(ctx, InLang(ctx, rankPrompt), user, 0.2, 2500)
	if err != nil {
		return nil, usage, err
	}
	var doc struct {
		R [][]any `json:"r"`
	}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return nil, usage, fmt.Errorf("bad model JSON: %w", err)
	}
	var out []Pick
	for _, row := range doc.R {
		if len(row) < 2 {
			continue
		}
		name, _ := row[0].(string)
		fit, _ := row[1].(float64)
		var reason string
		if len(row) > 2 {
			reason, _ = row[2].(string)
		}
		out = append(out, Pick{Name: strings.ToLower(strings.TrimPrefix(name, "@")), Fit: int(fit + 0.5), Reason: reason})
	}
	return out, usage, nil
}
