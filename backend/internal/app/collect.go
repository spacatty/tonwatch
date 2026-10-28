// Package app wires sources, store and jobs together.
package app

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fragwatch/internal/chain"
	"fragwatch/internal/config"
	"fragwatch/internal/fragment"
	"fragwatch/internal/jobs"
	"fragwatch/internal/llm"
	"fragwatch/internal/store"
)

//go:embed words.txt
var wordList string

type App struct {
	Cfg   config.Config
	Store *store.Store
	Frag  *fragment.Client
	Chain *chain.Client
	LLM   *llm.Client
	Jobs  *jobs.Manager

	llmMu    sync.Mutex
	llmUsage llm.Usage

	botMu   sync.RWMutex
	botUser string // the bot's @username, learned at start (getMe)
}

func (a *App) RegisterJobs() {
	c := a.Cfg
	a.Jobs.Add(&jobs.Job{Name: "auctions", Every: c.EveryAuctions, Run: a.runAuctions,
		Desc: "Ending-soon auctions + re-check ended auctions and watchlist"})
	a.Jobs.Add(&jobs.Job{Name: "sweep", Every: c.EverySweep, Run: a.runSweep,
		Desc: "All Fragment lists (auction/sale/sold) under every sort order, then re-check vanished listings"})
	a.Jobs.Add(&jobs.Job{Name: "words", Every: 24 * time.Hour, Run: a.runWords,
		Desc: "Fan-out Fragment text search over a word list to surface listings beyond the 500 cap"})
	a.Jobs.Add(&jobs.Job{Name: "chain", Every: c.EveryChain, Run: a.runChain,
		Desc: "Crawl every minted username + marketplace listings from the TON blockchain (tonapi)"})
	a.Jobs.Add(&jobs.Job{Name: "buynow", Every: 20 * time.Minute, Run: a.runBuyNow,
		Desc: "Read auction pages for buy-now prices (Fragment's lists don't include them), soonest-ending first"})
	a.Jobs.Add(&jobs.Job{Name: "prices", Every: 10 * time.Minute, Run: func(ctx context.Context, j *jobs.Job) error {
		return a.priceChainSales(ctx, j, "")
	}, Desc: "Fetch marketplace + price (tonapi) for on-chain listings that don't have one yet"})
	a.Jobs.Add(&jobs.Job{Name: "llm", Every: time.Minute, Run: a.runLLM,
		Desc: "Rate unscored usernames with the OpenRouter model"})
	a.Jobs.Add(&jobs.Job{Name: "probe", Every: c.EveryProbe, Run: a.runProbe,
		Desc: fmt.Sprintf("Check Fragment pages of names scored ≥ %d to find hidden auctions/sales", c.ProbeMinScore)})
	a.Jobs.Add(&jobs.Job{Name: "alerts", Every: alertEvery, Run: a.runAlerts,
		Desc: "Telegram alerts for every Pro account: new names inside their settings, tracked auctions closing, price drops on names they follow"})
	a.Jobs.Add(&jobs.Job{Name: "payments", Every: 20 * time.Second, Run: a.runPayments,
		Desc: "Watch the TON wallet for transfers that pay open checkouts"})
}

func (a *App) saveFrag(ctx context.Context) func([]store.FragRow) error {
	return func(rows []store.FragRow) error { return a.Store.UpsertFragment(ctx, rows) }
}

func (a *App) runAuctions(ctx context.Context, j *jobs.Job) error {
	rows, err := a.Frag.Search(ctx, "auction", "ending", "")
	if err != nil {
		return err
	}
	if err := a.Store.UpsertFragment(ctx, rows); err != nil {
		return err
	}
	names, err := a.Store.EndedAuctionsAndWatched(ctx)
	if err != nil {
		return err
	}
	j.Progress("%d ending auctions; re-checking %d ended/watched", len(rows), len(names))
	return a.Frag.DetailMany(ctx, names, a.Cfg.DetailWorkers, a.saveFrag(ctx), nil)
}

func (a *App) runSweep(ctx context.Context, j *jobs.Job) error {
	start := time.Now().Unix()
	total := 0
	n, err := a.Frag.Sweep(ctx, []string{"auction", "sale", "sold"}, func(rows []store.FragRow) error {
		total += len(rows)
		j.Progress("swept %d rows", total)
		return a.Store.UpsertFragment(ctx, rows)
	})
	if err != nil && ctx.Err() != nil {
		return err
	}
	stale, err2 := a.Store.StaleListings(ctx, start)
	if err2 != nil {
		return err2
	}
	j.Progress("swept %d unique; re-checking %d listings not seen in lists", n, len(stale))
	err3 := a.Frag.DetailMany(ctx, stale, a.Cfg.DetailWorkers, a.saveFrag(ctx), func(d int) {
		if d%25 == 0 {
			j.Progress("swept %d unique; re-checked %d/%d hidden listings", n, d, len(stale))
		}
	})
	return errors.Join(err, err3)
}

func (a *App) runWords(ctx context.Context, j *jobs.Job) error {
	words := strings.Fields(wordList)
	found := 0
	for i, w := range words {
		for _, f := range []string{"auction", "sale"} {
			rows, err := a.Frag.Search(ctx, f, "price_asc", w)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				continue
			}
			for k := range rows {
				rows[k].FromSrc = store.SrcFragmentSearch
			}
			found += len(rows)
			if err := a.Store.UpsertFragment(ctx, rows); err != nil {
				return err
			}
		}
		j.Progress("word %d/%d (%s): %d hits", i+1, len(words), w, found)
	}
	return nil
}

func (a *App) runChain(ctx context.Context, j *jobs.Job) error {
	start, _ := strconv.Atoi(a.Store.GetKV(ctx, "chain_offset"))
	var items atomic.Int64
	var mu sync.Mutex
	maxOff := start
	workers := 1
	if a.Cfg.TonapiKey != "" || len(a.Cfg.Proxies) > 0 {
		workers = 6
	}
	err := a.Chain.Crawl(ctx, start, workers, func(off int, rows []store.ChainRow) error {
		if err := a.Store.UpsertChain(ctx, rows); err != nil {
			return err
		}
		n := items.Add(int64(len(rows)))
		mu.Lock()
		if off > maxOff {
			maxOff = off
			// Resume point: with parallel workers earlier pages may still be in flight, so step back.
			a.Store.SetKV(ctx, "chain_offset", strconv.Itoa(max(0, off-workers*a.Chain.PageSize())))
		}
		mu.Unlock()
		j.Progress("crawled %d items (offset %d)", n, off)
		return nil
	})
	if err != nil {
		return err
	}
	a.Store.SetKV(ctx, "chain_offset", "0")
	a.Store.SetKV(ctx, "chain_done_at", strconv.FormatInt(time.Now().Unix(), 10))
	return a.priceChainSales(ctx, j, fmt.Sprintf("crawled %d items; ", items.Load()))
}

func (a *App) priceChainSales(ctx context.Context, j *jobs.Job, prefix string) error {
	priced := 0
	for ctx.Err() == nil {
		addrs, err := a.Store.ChainSalesMissingPrice(ctx, 100)
		if err != nil || len(addrs) == 0 {
			return err
		}
		sales, err := a.Chain.SalePrices(ctx, addrs)
		if err != nil {
			return err
		}
		// Anything tonapi didn't return still needs a marker so we don't ask forever.
		seen := map[string]bool{}
		for _, s := range sales {
			seen[s.NFTAddress] = true
		}
		for _, ad := range addrs {
			if !seen[ad] {
				sales = append(sales, store.ChainSale{NFTAddress: ad, Market: "unknown"})
			}
		}
		if err := a.Store.SetChainSales(ctx, sales); err != nil {
			return err
		}
		priced += len(addrs)
		j.Progress("%spriced %d marketplace listings", prefix, priced)
	}
	return ctx.Err()
}

func (a *App) runLLM(ctx context.Context, j *jobs.Job) error {
	if !a.LLM.Enabled() {
		j.Progress("OPENROUTER_API_KEY not set — skipping")
		return nil
	}
	var mu sync.Mutex
	inflight := map[string]bool{}
	var done atomic.Int64
	var lastErr atomic.Value
	failures := atomic.Int64{}
	var wg sync.WaitGroup
	for w := 0; w < max(1, a.Cfg.LLMWorkers); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil && failures.Load() < 5 {
				mu.Lock()
				batch, err := a.Store.UnscoredBatch(ctx, a.Cfg.LLMBatchSize, inflight)
				for _, n := range batch {
					inflight[n] = true
				}
				mu.Unlock()
				if err != nil || len(batch) == 0 {
					return
				}
				scores, missing, usage, err := a.LLM.Rate(ctx, batch)
				if err != nil {
					failures.Add(1)
					lastErr.Store(err.Error())
				} else if len(missing) > 0 && len(missing) == len(batch) {
					failures.Add(1)
				}
				if len(scores) > 0 {
					if err := a.Store.SaveScores(ctx, scores); err != nil {
						lastErr.Store(err.Error())
					}
				}
				if err == nil && len(missing) < len(batch) {
					// the model answered but skipped some names: count it, so they aren't retried forever
					a.Store.MarkLLMSkipped(ctx, missing)
				}
				a.llmMu.Lock()
				a.llmUsage.PromptTokens += usage.PromptTokens
				a.llmUsage.CompletionTokens += usage.CompletionTokens
				a.llmUsage.Cost += usage.Cost
				cost := a.llmUsage.Cost
				a.llmMu.Unlock()
				mu.Lock()
				for _, n := range batch {
					// keep skipped names in-flight for this run so one stubborn name can't loop forever
					if !slices.Contains(missing, n) {
						delete(inflight, n)
					}
				}
				mu.Unlock()
				j.Progress("rated %d names this run · session cost $%.4f", done.Add(int64(len(scores))), cost)
			}
		}()
	}
	wg.Wait()
	if failures.Load() >= 5 {
		if e, ok := lastErr.Load().(string); ok {
			return errors.New("stopped after repeated failures: " + e)
		}
		return errors.New("stopped after repeated failures")
	}
	return nil
}

func (a *App) LLMUsage() llm.Usage {
	a.llmMu.Lock()
	defer a.llmMu.Unlock()
	return a.llmUsage
}

func (a *App) runBuyNow(ctx context.Context, j *jobs.Job) error {
	names, err := a.Store.AuctionsNeedingDetail(ctx, 2*time.Hour, 6000)
	if err != nil {
		return err
	}
	j.Progress("reading %d auction pages", len(names))
	return a.Frag.DetailMany(ctx, names, a.Cfg.DetailWorkers, a.saveFrag(ctx), func(d int) {
		if d%25 == 0 {
			j.Progress("read %d/%d auction pages", d, len(names))
		}
	})
}

func (a *App) runProbe(ctx context.Context, j *jobs.Job) error {
	names, err := a.Store.NamesForProbe(ctx, a.Cfg.ProbeMinScore, 24*time.Hour, 20000)
	if err != nil {
		return err
	}
	j.Progress("probing %d names", len(names))
	return a.Frag.DetailMany(ctx, names, a.Cfg.DetailWorkers, a.saveFrag(ctx), func(d int) {
		if d%20 == 0 {
			j.Progress("probed %d/%d", d, len(names))
		}
	})
}

// RefreshOne re-scrapes one name from Fragment right now (and inserts it if new).
func (a *App) RefreshOne(ctx context.Context, name string) (*store.Username, error) {
	row, err := a.Frag.Detail(ctx, name)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("fragment has no page for @%s", name)
	}
	if err := a.Store.UpsertFragment(ctx, []store.FragRow{*row}); err != nil {
		return nil, err
	}
	return a.Store.Get(ctx, name)
}
