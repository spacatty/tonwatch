// Package llm rates usernames with a cheap OpenRouter model.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"fragwatch/internal/httpx"
	"fragwatch/internal/store"
)

type Client struct {
	http  *httpx.Client
	key   string
	Model string
	// Reasoning lets the model think before answering. Off by default: scores barely change,
	// but thinking multiplies output tokens (and cost) by ~4.
	Reasoning bool
}

func New(h *httpx.Client, key, model string) *Client { return &Client{http: h, key: key, Model: model} }

func (c *Client) Enabled() bool { return c.key != "" }

type Usage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	CachedTokens     int     `json:"cached_tokens,omitempty"` // prompt tokens served from the provider's cache
	Cost             float64 `json:"cost"`
}

// Chat sends one system+user exchange and returns the reply, asking for a JSON object.
//
// The system prompt is marked as a prompt-cache breakpoint. Every feature keeps its static
// instructions (and the taste profile) in the system message and the per-call data in the user
// message, so providers that cache (Anthropic, Gemini, and DeepSeek/OpenAI automatically) serve
// the repeated part at a fraction of the input price. Providers without caching ignore the mark.
func (c *Client) Chat(ctx context.Context, system, user string, temperature float64) (string, Usage, error) {
	content, usage, err := c.complete(ctx, system, []Turn{{Role: "user", Content: user}}, temperature, true, 0)
	if err != nil {
		return "", usage, err
	}
	return jsonBody(content), usage, nil
}

// Turn is one message of a conversation.
type Turn struct {
	Role    string `json:"role"` // user | assistant
	Content string `json:"content"`
}

// Converse continues a free-text conversation: the system prompt (cached) and the turns so far.
func (c *Client) Converse(ctx context.Context, system string, turns []Turn, temperature float64, maxTokens int) (string, Usage, error) {
	content, usage, err := c.complete(ctx, system, turns, temperature, false, maxTokens)
	return strings.TrimSpace(content), usage, err
}

func (c *Client) complete(ctx context.Context, system string, turns []Turn, temperature float64, jsonMode bool, maxTokens int) (string, Usage, error) {
	var usage Usage
	if !c.Enabled() {
		return "", usage, errors.New("OPENROUTER_API_KEY not set")
	}
	msgs := []map[string]any{
		{"role": "system", "content": []map[string]any{
			{"type": "text", "text": system, "cache_control": map[string]string{"type": "ephemeral"}},
		}},
	}
	for _, t := range turns {
		msgs = append(msgs, map[string]any{"role": t.Role, "content": t.Content})
	}
	req := map[string]any{
		"model":       c.Model,
		"messages":    msgs,
		"temperature": temperature,
		"usage":       map[string]bool{"include": true},
		"reasoning":   map[string]bool{"enabled": c.Reasoning, "exclude": true},
	}
	if jsonMode {
		req["response_format"] = map[string]string{"type": "json_object"}
	}
	if maxTokens > 0 {
		req["max_tokens"] = maxTokens
	}
	payload, _ := json.Marshal(req)
	body, err := c.http.Do(ctx, func() (*http.Request, error) {
		req, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Title", "BidSpot")
		return req, nil
	})
	if err != nil {
		return "", usage, err
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			Usage
			Details struct {
				Cached int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", usage, err
	}
	usage = resp.Usage.Usage
	usage.CachedTokens = resp.Usage.Details.Cached
	if resp.Error != nil {
		return "", usage, errors.New(resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return "", usage, errors.New("empty completion")
	}
	return resp.Choices[0].Message.Content, usage, nil
}

// jsonBody strips code fences and leading chatter around a JSON object.
func jsonBody(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.Trim(content, "` \n")
	if i := strings.Index(content, "{"); i > 0 {
		content = content[i:]
	}
	return content
}

// Rate scores one batch of names. Names the model skips are returned in missing.
func (c *Client) Rate(ctx context.Context, names []string) (scores []store.Score, missing []string, usage Usage, err error) {
	content, usage, err := c.Chat(ctx, SystemPrompt, strings.Join(names, "\n"), 0.1)
	if err != nil {
		return nil, names, usage, err
	}
	scores, err = parse(content, names)
	if err != nil {
		return nil, names, usage, err
	}
	got := map[string]bool{}
	for _, s := range scores {
		got[s.Name] = true
	}
	for _, n := range names {
		if !got[n] {
			missing = append(missing, n)
		}
	}
	return scores, missing, usage, nil
}

func parse(content string, names []string) ([]store.Score, error) {
	var doc struct {
		R [][]any `json:"r"`
	}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return nil, fmt.Errorf("bad model JSON: %w", err)
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	var out []store.Score
	for _, row := range doc.R {
		if len(row) < 3 {
			continue
		}
		name, _ := row[0].(string)
		name = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(name), "@"))
		if !want[name] {
			continue
		}
		score, _ := row[1].(float64)
		cat, _ := row[2].(string)
		cat = strings.ToLower(strings.TrimSpace(cat))
		if !slices.Contains(Categories, cat) {
			cat = "brand"
		}
		var meaning string
		if len(row) > 3 {
			meaning, _ = row[3].(string)
		}
		out = append(out, store.Score{Name: name, Score: max(0, min(10, int(score+0.5))), Category: cat, Meaning: meaning})
		delete(want, name)
	}
	return out, nil
}
