// Package chain reads the "Telegram Usernames" NFT collection from the tonapi indexer.
// This is the only complete list of minted usernames (Fragment's UI caps lists at 500).
//
// Note: Fragment's own auctions/sales live inside the NFT contract and are NOT reported
// as a sale here; a sale means a third-party marketplace (Getgems etc.) listing.
package chain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"fragwatch/internal/httpx"
	"fragwatch/internal/store"
)

const Collection = "EQCA14o1-VWhS2efqoh_9M1b_A9DtKTuoqfmkn83AbJzwnPi"

type Client struct {
	tonapi    *httpx.Client
	tonapiKey string
	toncenter *httpx.Client // payments only: its own quota (1 request/s without a key)
}

func New(tonapi *httpx.Client, tonapiKey string) *Client {
	return &Client{tonapi: tonapi, tonapiKey: tonapiKey, toncenter: httpx.New(0.9, nil)}
}

// tonapi's NFT shape (collection items and _bulk share it).
type taItem struct {
	Address string `json:"address"`
	DNS     string `json:"dns"`
	Owner   *struct {
		Address string `json:"address"`
	} `json:"owner"`
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Sale *struct {
		Address string `json:"address"`
		Market  struct {
			Name string `json:"name"`
		} `json:"market"`
		Owner *struct {
			Address string `json:"address"`
		} `json:"owner"`
		Price struct {
			CurrencyType string `json:"currency_type"`
			Value        string `json:"value"`
			Decimals     int    `json:"decimals"`
			TokenName    string `json:"token_name"`
		} `json:"price"`
	} `json:"sale"`
}

// market returns marketplace name and price (nil if unknown) for an item on sale.
func (it *taItem) market() (string, *float64) {
	if it.Sale == nil {
		return "", nil
	}
	m := strings.TrimSuffix(it.Sale.Market.Name, " Sales")
	if m == "" {
		m = "unknown"
	}
	if it.Sale.Price.CurrencyType != "native" && it.Sale.Price.TokenName != "" {
		m += " (" + it.Sale.Price.TokenName + ")"
	}
	if v, err := strconv.ParseFloat(it.Sale.Price.Value, 64); err == nil && v > 0 {
		p := v / math.Pow10(it.Sale.Price.Decimals)
		return m, &p
	}
	return m, nil
}

func (c *Client) tonapiDo(ctx context.Context, method, u string, payload []byte) ([]byte, error) {
	return c.tonapi.Do(ctx, func() (*http.Request, error) {
		var body io.Reader
		if payload != nil {
			body = bytes.NewReader(payload)
		}
		req, err := http.NewRequest(method, u, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if c.tonapiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.tonapiKey)
		}
		return req, nil
	})
}

// PageSize is 1000 with a tonapi key, 100 on the anonymous tier.
func (c *Client) PageSize() int {
	if c.tonapiKey != "" {
		return 1000
	}
	return 100
}

// page reads collection items from tonapi. (toncenter v3 was tried first, but it times out
// on offsets past ~66k, so it can't walk the whole collection.)
func (c *Client) page(ctx context.Context, offset int) ([]store.ChainRow, int, error) {
	q := url.Values{"limit": {strconv.Itoa(c.PageSize())}, "offset": {strconv.Itoa(offset)}}
	body, err := c.tonapiDo(ctx, "GET", "https://tonapi.io/v2/nfts/collections/"+Collection+"/items?"+q.Encode(), nil)
	if err != nil {
		return nil, 0, err
	}
	var resp struct {
		Items []taItem `json:"nft_items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, err
	}
	rows := make([]store.ChainRow, 0, len(resp.Items))
	for _, it := range resp.Items {
		name := strings.TrimSuffix(it.DNS, ".t.me")
		if name == "" {
			name = strings.TrimPrefix(it.Metadata.Name, "@")
		}
		if name == "" {
			continue
		}
		row := store.ChainRow{Name: strings.ToLower(name), NFTAddress: strings.ToLower(it.Address)}
		if it.Owner != nil {
			row.Owner = it.Owner.Address
		}
		if it.Sale != nil {
			row.OnSale = true
			row.SaleContract = strings.ToLower(it.Sale.Address)
			if it.Sale.Owner != nil {
				row.Owner = it.Sale.Owner.Address // the seller, not the sale contract
			}
			row.Market, row.Price = it.market()
		}
		row.Owner = strings.ToLower(row.Owner)
		rows = append(rows, row)
	}
	return rows, len(resp.Items), nil
}

// Crawl pages through the whole collection with parallel workers. startOffset lets a
// crashed crawl resume. onPage receives each page's rows and the offset it came from.
func (c *Client) Crawl(ctx context.Context, startOffset, workers int, onPage func(offset int, rows []store.ChainRow) error) error {
	var next atomic.Int64
	next.Store(int64(startOffset))
	var end atomic.Int64
	end.Store(math.MaxInt64)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for i := 0; i < max(1, workers); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				off := next.Add(int64(c.PageSize())) - int64(c.PageSize())
				if off >= end.Load() {
					return
				}
				rows, n, err := c.page(ctx, int(off))
				if err == nil && n > 0 {
					err = onPage(int(off), rows)
				}
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("offset %d: %w", off, err)
					}
					mu.Unlock()
					return
				}
				// Only an empty page ends the crawl: tonapi returns short pages mid-collection
				// (it drops some items), so a short page is not the end.
				if n == 0 {
					end.Store(min(end.Load(), off))
					return
				}
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

// SalePrices asks tonapi for marketplace + price of NFTs that are on sale (≤100 per call).
func (c *Client) SalePrices(ctx context.Context, nftAddrs []string) ([]store.ChainSale, error) {
	payload, _ := json.Marshal(map[string]any{"account_ids": nftAddrs})
	body, err := c.tonapiDo(ctx, "POST", "https://tonapi.io/v2/nfts/_bulk", payload)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Items []taItem `json:"nft_items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	out := make([]store.ChainSale, 0, len(resp.Items))
	for _, it := range resp.Items {
		m, p := it.market()
		if m == "" {
			m = "unknown" // indexer said on sale, tonapi sees no sale (yet)
		}
		out = append(out, store.ChainSale{NFTAddress: strings.ToLower(it.Address), Market: m, Price: p})
	}
	return out, nil
}

// Transfer is one incoming TON transfer to a wallet.
type Transfer struct {
	Hash    string
	Nano    int64  // amount in nanoTON
	Comment string // the text comment, if the transfer carried one
	Time    int64
}

// Incoming lists the latest incoming transfers to a wallet (newest first): how TON top-ups are
// seen, by the comment each checkout asks the payer to attach. It reads toncenter, not tonapi, so
// the chain crawler spending tonapi's daily quota never delays a payment.
func (c *Client) Incoming(ctx context.Context, wallet string, limit int) ([]Transfer, error) {
	u := fmt.Sprintf("https://toncenter.com/api/v2/getTransactions?address=%s&limit=%d&archival=true", url.QueryEscape(wallet), limit)
	body, err := c.toncenter.Get(ctx, u, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		OK     bool   `json:"ok"`
		Error  string `json:"error"`
		Result []struct {
			Utime int64 `json:"utime"`
			ID    struct {
				Hash string `json:"hash"`
			} `json:"transaction_id"`
			InMsg *struct {
				Value   string `json:"value"`
				Source  string `json:"source"`
				Message string `json:"message"`
				Data    struct {
					Type string `json:"@type"`
				} `json:"msg_data"`
			} `json:"in_msg"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("toncenter: %s", resp.Error)
	}
	var out []Transfer
	for _, t := range resp.Result {
		m := t.InMsg
		if m == nil || m.Source == "" { // external messages carry no value
			continue
		}
		nano, err := strconv.ParseInt(m.Value, 10, 64)
		if err != nil || nano <= 0 {
			continue
		}
		tr := Transfer{Hash: t.ID.Hash, Nano: nano, Time: t.Utime}
		if m.Data.Type == "msg.dataText" { // a plain text comment; raw payloads are not comments
			tr.Comment = strings.TrimSpace(m.Message)
		}
		out = append(out, tr)
	}
	return out, nil
}
