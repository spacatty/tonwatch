// Command rate records the facts of usernames with the configured LLM and shows the score the
// formula gives them, for tuning the facts prompt and the rating formula:
//
//	go run ./cmd/rate coffee goodnews xinqun lunexo qxzt7kp   # facts and scores for these names
//	go run ./cmd/rate -premium coffee travel                   # the same with the premium model
//	go run ./cmd/rate -eval                                    # old vs new score against real sales
//	go run ./cmd/rate -eval -explore                           # four-letter prices by fact, to tune on
//	go run ./cmd/rate -embed 300                               # embed 300 pending names (tests the embed job)
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"fragwatch/internal/app"
	"fragwatch/internal/config"
	"fragwatch/internal/httpx"
	"fragwatch/internal/llm"
	"fragwatch/internal/rating"
	"fragwatch/internal/store"
)

func main() {
	premium := flag.Bool("premium", false, "use LLM_PREMIUM_MODEL and LLM_PREMIUM_EFFORT")
	eval := flag.Bool("eval", false, "compare the stored and the new score with sale prices on a fixed set of sold names")
	refresh := flag.Bool("refresh", false, "with -eval: record the eval set's facts again instead of reusing evalfacts.json")
	rebuild := flag.Bool("rebuild", false, "with -eval: draw a new evalset.json from the database")
	explore := flag.Bool("explore", false, "with -eval: print median prices by fact for the four-letter tuning half")
	embed := flag.Int("embed", 0, "embed this many pending names into name_vectors")
	flag.Parse()

	cfg := config.Load()
	c := llm.New(httpx.New(0, nil), cfg.OpenRouterKey, cfg.OpenRouterModel)
	c.Reasoning = cfg.LLMReasoning
	if *premium {
		c = llm.New(httpx.New(0, nil), cfg.OpenRouterKey, cfg.LLMPremiumModel)
		c.Effort = cfg.LLMPremiumEffort
	}
	ctx := context.Background()

	switch {
	case *eval:
		st, err := store.Open(cfg.DatabaseURL)
		if err != nil {
			log.Fatal(err)
		}
		defer st.Close()
		if err := runEval(ctx, st, c, *rebuild, *refresh, *explore); err != nil {
			log.Fatal(err)
		}
	case *embed > 0:
		st, err := store.Open(cfg.DatabaseURL)
		if err != nil {
			log.Fatal(err)
		}
		defer st.Close()
		a := &app.App{Cfg: cfg, Store: st, LLM: c}
		start := time.Now()
		n, err := a.EmbedPending(ctx, *embed, nil)
		if err != nil {
			log.Fatal(err)
		}
		todo, done, _ := st.CountToEmbed(ctx, cfg.EmbedModel)
		fmt.Printf("embedded %d names in %s with %s; %d stored, %d still to embed; cost $%.6f\n",
			n, time.Since(start).Round(time.Millisecond), cfg.EmbedModel, done, todo, a.LLMUsage().Cost)
	default:
		names := flag.Args()
		if len(names) == 0 {
			names = []string{"coffee", "goodnews", "harbor", "xinqun", "lunexo", "maria_art", "alex1987", "qxzt7kp"}
		}
		res, err := c.RecordFacts(ctx, names)
		if err != nil {
			log.Fatal(err)
		}
		for _, r := range res.Rows {
			sc, kind := rating.Score(r.Name, r.Facts)
			f := r.Facts
			fmt.Printf("%-14s %2d %-8s | %-16q %-7s %-3s fame %d pull %d clean %d | %s\n",
				r.Name, sc, kind, f.Words, f.Kind, f.Lang, f.Fame, f.Pull, f.Clean, r.Gloss)
		}
		fmt.Printf("model %s · missing %v · anchors: %s (retried %v)\n", c.Model, res.Missing, res.Drift, res.Retried)
		fmt.Printf("tokens in/out: %d/%d (cached %d) · cost $%.6f\n", res.Usage.PromptTokens, res.Usage.CompletionTokens, res.Usage.CachedTokens, res.Usage.Cost)
	}
}
