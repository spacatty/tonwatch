// Package app wires sources, store and jobs together.
package app

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
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
	"fragwatch/internal/rating"
	"fragwatch/internal/store"
	"fragwatch/internal/vector"
)

//go:embed words.txt
var wordList string

type App struct {
	Cfg   config.Config
	Store *store.Store
	Frag  *fragment.Client
	Chain *chain.Client
	LLM   *llm.Client
	// Premium re-records facts for the names that matter most (job "llm-premium").
	Premium *llm.Client
	Jobs    *jobs.Manager

	llmMu    sync.Mutex
	llmUsage llm.Usage

	vecMu sync.Mutex
	vecs  *vector.Index // Ask AI's semantic index, loaded on first use

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
		return a.priceChainSales(ctx, j, -1)
	}, Desc: "Fetch marketplace + price (tonapi) for on-chain listings that don't have one yet"})
	a.Jobs.Add(&jobs.Job{Name: "llm", Every: time.Minute, Run: a.runLLM,
		Desc: "Record the facts the rating is computed from, for names that have none yet, with the OpenRouter model"})
	a.Jobs.Add(&jobs.Job{Name: "llm-premium", Run: a.runPremium,
		Desc: fmt.Sprintf("Re-record facts with %s for the names that matter most: score 8+, every 4-letter name, 5 letters with score 6+", c.LLMPremiumModel)})
	a.Jobs.Add(&jobs.Job{Name: "rescore", Run: a.runRescore,
		Desc: "Recompute every score from the stored facts with the current formula (no model calls)"})
	a.Jobs.Add(&jobs.Job{Name: "embed", Run: a.runEmbed,
		Desc: "Embed names rated 6+, or listed and rated 4+, for Ask AI's search by meaning"})
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
	return a.priceChainSales(ctx, j, items.Load())
}

// crawled is how many items the chain crawl just read, shown before the pricing count (-1: none).
func (a *App) priceChainSales(ctx context.Context, j *jobs.Job, crawled int64) error {
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
		if crawled >= 0 {
			j.Progress("crawled %d items; priced %d marketplace listings", crawled, priced)
		} else {
			j.Progress("priced %d marketplace listings", priced)
		}
	}
	return ctx.Err()
}

func (a *App) runLLM(ctx context.Context, j *jobs.Job) error {
	return a.runFacts(ctx, j, a.LLM, a.Store.FactsBatch, a.Cfg.LLMBatchSize, a.Cfg.LLMWorkers, false)
}

// runPremium re-records the facts of the names that matter most with the premium model.
func (a *App) runPremium(ctx context.Context, j *jobs.Job) error {
	return a.runFacts(ctx, j, a.Premium, a.Store.PremiumBatch, a.Cfg.LLMPremiumBatch, 2, true)
}

// runFacts has the model record facts for every name next() hands out, workers batches at a time,
// and saves the facts with the score they give. It stops when the queue is empty or after five
// failed batches.
func (a *App) runFacts(ctx context.Context, j *jobs.Job, c *llm.Client, next func(context.Context, int, map[string]bool) ([]string, error),
	size, workers int, premium bool) error {
	if c == nil || !c.Enabled() {
		j.Progress("OPENROUTER_API_KEY not set — skipping")
		return nil
	}
	var mu sync.Mutex
	inflight := map[string]bool{}
	var done atomic.Int64
	var lastErr atomic.Value
	failures := atomic.Int64{}
	var wg sync.WaitGroup
	for w := 0; w < max(1, workers); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil && failures.Load() < 5 {
				mu.Lock()
				batch, err := next(ctx, max(1, size), inflight)
				for _, n := range batch {
					inflight[n] = true
				}
				mu.Unlock()
				if err != nil || len(batch) == 0 {
					return
				}
				res, err := c.RecordFacts(ctx, batch)
				if err != nil {
					failures.Add(1)
					lastErr.Store(err.Error())
				} else if len(res.Missing) == len(batch) {
					failures.Add(1)
				}
				if err == nil && !res.Drift.OK() {
					// asked twice and still off the yardstick: kept, since a third try rarely does better
					log.Printf("%s: batch of %d drifted from the anchors after a retry (%s); saved anyway", j.Name, len(batch), res.Drift)
				}
				if len(res.Rows) > 0 {
					rows := make([]store.FactsRow, len(res.Rows))
					for i, r := range res.Rows {
						rows[i] = store.FactsRow{Name: r.Name, Facts: r.Facts, Gloss: r.Gloss}
					}
					if err := a.Store.SaveFacts(ctx, rows, c.Model, premium); err != nil {
						lastErr.Store(err.Error())
					}
				}
				if err == nil && len(res.Missing) < len(batch) {
					// the model answered but skipped some names: count it, so they aren't retried forever
					a.Store.MarkLLMSkipped(ctx, res.Missing)
				}
				a.addUsage(res.Usage)
				cost := a.LLMUsage().Cost
				mu.Lock()
				for _, n := range batch {
					// keep skipped names in-flight for this run so one stubborn name can't loop forever
					if !slices.Contains(res.Missing, n) {
						delete(inflight, n)
					}
				}
				mu.Unlock()
				j.Progress("rated %d names this run · session cost $%.4f", done.Add(int64(len(res.Rows))), cost)
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

// kvRatingVersion holds the rating.Version the stored scores were computed with.
const kvRatingVersion = "rating_version"

// RescoreIfStale starts the rescore when the formula changed since the scores were computed.
func (a *App) RescoreIfStale(ctx context.Context) {
	if a.Store.GetKV(ctx, kvRatingVersion) != strconv.Itoa(rating.Version) {
		a.Jobs.Trigger("rescore")
	}
}

// runRescore recomputes every score from the stored facts with the current formula. No model
// calls: a page of names at a time, and only changed scores are written.
func (a *App) runRescore(ctx context.Context, j *jobs.Job) error {
	after, read, changed := "", 0, 0
	for {
		last, n, err := a.Store.Rescore(ctx, after, 5000)
		if err != nil {
			return err
		}
		if last == "" {
			break
		}
		after, changed = last, changed+n
		read += 5000
		j.Progress("rescored %d names, %d changed", read, changed)
	}
	a.Store.SetKV(ctx, kvRatingVersion, strconv.Itoa(rating.Version))
	j.Progress("rescored with formula v%d, %d changed", rating.Version, changed)
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

// RefreshOne re-reads one name right now (and inserts it if new): its Fragment page, and its NFT
// on-chain when the address is known, since a sale on Getgems or another marketplace shows only there.
func (a *App) RefreshOne(ctx context.Context, name string) (*store.Username, error) {
	row, err := a.Frag.Detail(ctx, name)
	if err != nil {
		return nil, err
	}
	if row != nil {
		if err := a.Store.UpsertFragment(ctx, []store.FragRow{*row}); err != nil {
			return nil, err
		}
	}
	u, err := a.Store.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if u != nil && u.NFTAddress != nil {
		c, err := a.Chain.Item(ctx, *u.NFTAddress)
		if err != nil {
			return nil, err
		}
		if c != nil {
			if err := a.Store.UpsertChain(ctx, []store.ChainRow{*c}); err != nil {
				return nil, err
			}
			return a.Store.Get(ctx, name)
		}
	}
	if row == nil {
		return nil, fmt.Errorf("fragment has no page for @%s", name)
	}
	return u, nil
}
