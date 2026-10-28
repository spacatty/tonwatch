// Package httpx is a small HTTP client with per-source rate limiting, retries
// and optional round-robin proxy rotation.
package httpx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

const UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"

type Client struct {
	clients []*http.Client // one per proxy (or a single direct client)
	next    atomic.Uint64
	limiter *rate.Limiter
	Retries int
}

// New builds a client. rps<=0 disables rate limiting. Each proxy gets its own
// transport; with N proxies the effective rate limit scales by N.
func New(rps float64, proxies []string) *Client {
	c := &Client{Retries: 4}
	mk := func(proxy *url.URL) *http.Client {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.MaxIdleConnsPerHost = 32
		if proxy != nil {
			tr.Proxy = http.ProxyURL(proxy)
		}
		jar, _ := cookiejar.New(nil)
		return &http.Client{Transport: tr, Timeout: 40 * time.Second, Jar: jar}
	}
	for _, p := range proxies {
		if u, err := url.Parse(p); err == nil {
			c.clients = append(c.clients, mk(u))
		}
	}
	if len(c.clients) == 0 {
		c.clients = []*http.Client{mk(nil)}
	}
	if rps > 0 {
		eff := rps * float64(len(c.clients))
		c.limiter = rate.NewLimiter(rate.Limit(eff), max(1, int(eff)))
	}
	return c
}

type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string { return fmt.Sprintf("http %d: %.200s", e.Code, e.Body) }

// Do sends the request built by mk (called again on every retry so bodies can be re-read)
// and returns the response body.
func (c *Client) Do(ctx context.Context, mk func() (*http.Request, error)) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<attempt) * 500 * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if c.limiter != nil {
			if err := c.limiter.Wait(ctx); err != nil {
				return nil, err
			}
		}
		req, err := mk()
		if err != nil {
			return nil, err
		}
		req = req.WithContext(ctx)
		if req.Header.Get("User-Agent") == "" {
			req.Header.Set("User-Agent", UserAgent)
		}
		hc := c.clients[c.next.Add(1)%uint64(len(c.clients))]
		resp, err := hc.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = &StatusError{resp.StatusCode, string(body)}
			continue
		}
		if resp.StatusCode >= 400 {
			return body, &StatusError{resp.StatusCode, string(body)}
		}
		return body, nil
	}
	return nil, lastErr
}

func (c *Client) Get(ctx context.Context, u string, hdr map[string]string) ([]byte, error) {
	return c.Do(ctx, func() (*http.Request, error) {
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return nil, err
		}
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		return req, nil
	})
}
