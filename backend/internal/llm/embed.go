package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Embed turns texts into vectors with an OpenRouter embedding model (e.g.
// openai/text-embedding-3-small at 256 dimensions), in the order given.
func (c *Client) Embed(ctx context.Context, model string, dims int, texts []string) ([][]float32, Usage, error) {
	var usage Usage
	if !c.Enabled() {
		return nil, usage, errors.New("OPENROUTER_API_KEY not set")
	}
	if len(texts) == 0 {
		return nil, usage, nil
	}
	req := map[string]any{"model": model, "input": texts}
	if dims > 0 {
		req["dimensions"] = dims
	}
	payload, _ := json.Marshal(req)
	body, err := c.http.Do(ctx, func() (*http.Request, error) {
		req, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/embeddings", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Title", "TonWatch")
		return req, nil
	})
	if err != nil {
		return nil, usage, err
	}
	var resp struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Usage Usage `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, usage, err
	}
	usage = resp.Usage
	if resp.Error != nil {
		return nil, usage, errors.New(resp.Error.Message)
	}
	out := make([][]float32, len(texts))
	for _, d := range resp.Data {
		if d.Index >= 0 && d.Index < len(out) {
			out[d.Index] = d.Embedding
		}
	}
	for i, v := range out {
		if len(v) == 0 || (dims > 0 && len(v) != dims) {
			return nil, usage, fmt.Errorf("embedding %d: got %d dimensions", i, len(v))
		}
	}
	return out, usage, nil
}
