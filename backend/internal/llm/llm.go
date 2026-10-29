// Package llm talks to OpenRouter: facts about usernames, the AI filter, Ask AI, ideas, embeddings.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"fragwatch/internal/httpx"
)

type Client struct {
	http  *httpx.Client
	key   string
	Model string
	// Reasoning lets the model think before answering. Off by default: scores barely change,
	// but thinking multiplies output tokens (and cost) by ~4.
	Reasoning bool
	// Effort, when set, is sent as reasoning.effort ("low", "medium", "high") instead of Reasoning.
	// Reasoning models may refuse a temperature, so none is sent then.
	Effort string
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
//
// maxTokens caps the reply (0 = 4000): a JSON answer cut off at the cap is an error, never parsed.
func (c *Client) Chat(ctx context.Context, system, user string, temperature float64, maxTokens int) (string, Usage, error) {
	if maxTokens <= 0 {
		maxTokens = 4000
	}
	content, usage, err := c.complete(ctx, system, []Turn{{Role: "user", Content: user}}, temperature, true, maxTokens)
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
		"model":    c.Model,
		"messages": msgs,
		"usage":    map[string]bool{"include": true},
	}
	if c.Effort != "" {
		req["reasoning"] = map[string]any{"effort": c.Effort, "exclude": true}
	} else {
		req["temperature"] = temperature
		req["reasoning"] = map[string]bool{"enabled": c.Reasoning, "exclude": true}
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
		req.Header.Set("X-Title", "TonWatch")
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
			FinishReason string `json:"finish_reason"`
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
	if jsonMode && resp.Choices[0].FinishReason == "length" {
		return "", usage, fmt.Errorf("reply cut off at %d tokens", maxTokens)
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
