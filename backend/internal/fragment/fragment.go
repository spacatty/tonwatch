// Package fragment talks to fragment.com's internal web API (the one its own UI uses).
//
// Findings that shape this package:
//   - POST /api?hash=<page hash> method=searchAuctions returns HTML rows; no login needed.
//   - Every (filter, sort) combination is capped at 500 rows and offset_id paging is ignored,
//     so Sweep queries every sort order and merges the results.
//   - query=<text> returns only ~10 matches, so word search is a fan-out over a word list.
//   - /username/<name> shows status for any name, including ones hidden from the lists.
package fragment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"fragwatch/internal/httpx"
	"fragwatch/internal/store"
)

const base = "https://fragment.com"

type Client struct {
	http *httpx.Client
	mu   sync.Mutex
	hash string
}

func New(h *httpx.Client) *Client { return &Client{http: h} }

var hashRe = regexp.MustCompile(`api\?hash=([0-9a-f]+)`)

func (c *Client) apiHash(ctx context.Context, refresh bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hash != "" && !refresh {
		return c.hash, nil
	}
	body, err := c.http.Get(ctx, base+"/", nil)
	if err != nil {
		return "", err
	}
	m := hashRe.FindSubmatch(body)
	if m == nil {
		return "", errors.New("fragment: api hash not found on homepage")
	}
	c.hash = string(m[1])
	return c.hash, nil
}

type apiResp struct {
	OK    bool   `json:"ok"`
	HTML  string `json:"html"`
	Body  string `json:"body"`
	Error string `json:"error"`
}

func (c *Client) call(ctx context.Context, params url.Values) (*apiResp, error) {
	for attempt := 0; attempt < 2; attempt++ {
		hash, err := c.apiHash(ctx, attempt > 0)
		if err != nil {
			return nil, err
		}
		body, err := c.http.Do(ctx, func() (*http.Request, error) {
			req, err := http.NewRequest("POST", base+"/api?hash="+hash, strings.NewReader(params.Encode()))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
			req.Header.Set("X-Requested-With", "XMLHttpRequest")
			req.Header.Set("Origin", base)
			req.Header.Set("Referer", base+"/")
			return req, nil
		})
		if err != nil {
			return nil, err
		}
		var r apiResp
		if err := json.Unmarshal(body, &r); err != nil || (!r.OK && r.Error != "") {
			continue // likely stale hash; refetch once
		}
		return &r, nil
	}
	return nil, errors.New("fragment: api call failed")
}

// Search runs one searchAuctions query. filter: auction|sale|sold|"" ; sort: ending|listed|price_asc|price_desc.
func (c *Client) Search(ctx context.Context, filter, sort, query string) ([]store.FragRow, error) {
	r, err := c.call(ctx, url.Values{
		"type": {"usernames"}, "query": {query}, "filter": {filter}, "sort": {sort}, "method": {"searchAuctions"},
	})
	if err != nil {
		return nil, err
	}
	html := r.HTML
	if html == "" {
		html = r.Body
	}
	return parseList(html, filter)
}

var numRe = regexp.MustCompile(`[0-9][0-9,]*(\.[0-9]+)?`)

func parseNum(s string) *float64 {
	m := numRe.FindString(s)
	if m == "" {
		return nil
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(m, ",", ""), 64)
	if err != nil {
		return nil
	}
	return &f
}

func parseTime(s string) *int64 {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	u := t.Unix()
	return &u
}

func statusFromText(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "on auction":
		return "auction"
	case "for sale":
		return "sale"
	case "sold":
		return "sold"
	case "taken":
		return "taken"
	case "available":
		return "available"
	}
	return strings.ToLower(strings.TrimSpace(s))
}

func parseList(html, filter string) ([]store.FragRow, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<table>" + html + "</table>"))
	if err != nil {
		return nil, err
	}
	var out []store.FragRow
	doc.Find("tr.tm-row-selectable").Each(func(_ int, tr *goquery.Selection) {
		name := strings.TrimPrefix(strings.TrimSpace(tr.Find(".table-cell-value-row .tm-value").First().Text()), "@")
		if name == "" {
			href, _ := tr.Find("a.table-cell").First().Attr("href")
			name = strings.TrimPrefix(href, "/username/")
		}
		if name == "" || strings.Contains(name, " ") {
			return
		}
		row := store.FragRow{Name: strings.ToLower(name), FromSrc: store.SrcFragmentList}
		row.Price = parseNum(tr.Find(".icon-ton").First().Text())
		if usd := tr.Find(".table-cell-desc.wide-only").First().Text(); strings.Contains(usd, "$") {
			row.USD = parseNum(usd)
		}
		status := filter
		if status == "" {
			status = statusFromText(tr.Find(".table-cell-status-thin").First().Text())
		}
		row.Status = status
		if status == "auction" {
			if dt, ok := tr.Find("time[datetime]").First().Attr("datetime"); ok {
				row.EndsAt = parseTime(dt)
			}
		}
		out = append(out, row)
	})
	return out, nil
}

// Sweep pulls every listing Fragment exposes by combining sort orders (each capped at 500).
func (c *Client) Sweep(ctx context.Context, filters []string, onBatch func([]store.FragRow) error) (int, error) {
	sortsFor := map[string][]string{
		"auction": {"ending", "listed", "price_asc", "price_desc"},
		"sale":    {"listed", "price_asc", "price_desc", "ending"},
		"sold":    {"listed", "price_asc", "price_desc"},
	}
	seen := map[string]bool{}
	var errs []error
	for _, f := range filters {
		for _, s := range sortsFor[f] {
			rows, err := c.Search(ctx, f, s, "")
			if err != nil {
				errs = append(errs, fmt.Errorf("%s/%s: %w", f, s, err))
				continue
			}
			var fresh []store.FragRow
			for _, r := range rows {
				if !seen[f+r.Name] {
					seen[f+r.Name] = true
					fresh = append(fresh, r)
				}
			}
			if err := onBatch(fresh); err != nil {
				return len(seen), err
			}
		}
	}
	return len(seen), errors.Join(errs...)
}

// Detail scrapes one username page. Returns nil if Fragment doesn't know the name.
func (c *Client) Detail(ctx context.Context, name string) (*store.FragRow, error) {
	body, err := c.http.Get(ctx, base+"/username/"+url.PathEscape(name), nil)
	if err != nil {
		var se *httpx.StatusError
		if errors.As(err, &se) && se.Code == 404 {
			return nil, nil
		}
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	statusText := doc.Find(".tm-section-header-status").First().Text()
	if statusText == "" {
		return nil, nil
	}
	row := &store.FragRow{Name: strings.ToLower(name), Status: statusFromText(statusText), FromSrc: store.SrcProbe}
	bid := doc.Find(".tm-section-bid-info").First()
	row.Price = parseNum(bid.Find(".icon-ton").First().Text())
	if usd := bid.Find(".table-cell-desc").First().Text(); strings.Contains(usd, "$") {
		row.USD = parseNum(usd)
	}
	if href, ok := bid.Find("a.tm-wallet").First().Attr("href"); ok {
		row.Owner = href[strings.LastIndex(href, "/")+1:]
	}
	if row.Status == "auction" {
		if dt, ok := doc.Find("time.tm-countdown-timer").First().Attr("datetime"); ok {
			row.EndsAt = parseTime(dt)
		}
		if v, ok := doc.Find(".js-buy-now-btn").First().Attr("data-bid-amount"); ok {
			row.BuyNow = parseNum(v)
		}
	}
	return row, nil
}

// DetailMany fetches many detail pages in parallel and streams results in batches.
func (c *Client) DetailMany(ctx context.Context, names []string, workers int, onBatch func([]store.FragRow) error, progress func(done int)) error {
	in := make(chan string)
	out := make(chan *store.FragRow)
	var wg sync.WaitGroup
	for i := 0; i < max(1, workers); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range in {
				r, err := c.Detail(ctx, n)
				if err != nil {
					r = nil
				}
				select {
				case out <- r:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(in)
		for _, n := range names {
			select {
			case in <- n:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(out) }()

	var batch []store.FragRow
	done := 0
	var firstErr error
	for r := range out {
		done++
		if r != nil {
			batch = append(batch, *r)
		}
		if len(batch) >= 50 {
			if err := onBatch(batch); err != nil && firstErr == nil {
				firstErr = err
			}
			batch = nil
		}
		if progress != nil {
			progress(done)
		}
	}
	if len(batch) > 0 {
		if err := onBatch(batch); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = ctx.Err()
	}
	return firstErr
}
