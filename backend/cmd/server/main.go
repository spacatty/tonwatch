package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fragwatch/internal/app"
	"fragwatch/internal/chain"
	"fragwatch/internal/config"
	"fragwatch/internal/fragment"
	"fragwatch/internal/httpx"
	"fragwatch/internal/jobs"
	"fragwatch/internal/llm"
	"fragwatch/internal/store"
)

func main() {
	cfg := config.Load()
	st, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tonapiRPS := cfg.TonapiRPS
	if cfg.TonapiKey != "" && tonapiRPS < 5 {
		tonapiRPS = 9
	}
	llmHTTP := httpx.New(0, nil)
	llmHTTP.Retries = 2

	a := &app.App{
		Cfg:   cfg,
		Store: st,
		Frag:  fragment.New(httpx.New(cfg.FragmentRPS, cfg.Proxies)),
		Chain: chain.New(httpx.New(tonapiRPS, cfg.Proxies), cfg.TonapiKey),
		LLM:   llm.New(llmHTTP, cfg.OpenRouterKey, cfg.OpenRouterModel),
		Jobs:  jobs.NewManager(ctx),
	}
	a.LLM.Reasoning = cfg.LLMReasoning
	a.Premium = llm.New(llmHTTP, cfg.OpenRouterKey, cfg.LLMPremiumModel)
	a.Premium.Effort = cfg.LLMPremiumEffort
	a.RegisterJobs()
	// a new rating formula applies to every stored fact at once; no model calls, so AUTO_JOBS doesn't matter
	a.RescoreIfStale(ctx)
	auto := map[string]time.Duration{}
	if cfg.AutoJobs {
		auto = map[string]time.Duration{
			"auctions": 2 * time.Second, "sweep": 20 * time.Second, "chain": time.Minute, "prices": 90 * time.Second, "buynow": 15 * time.Second,
			"llm": 30 * time.Second, "probe": 30 * time.Minute, "words": 2 * time.Hour, "alerts": 45 * time.Second,
		}
	}
	if cfg.SkipChain {
		// no tonapi crawling: names keep coming from Fragment, on-chain listings stay as they are
		delete(auto, "chain")
		delete(auto, "prices")
	}
	if cfg.TonWallet != "" {
		// payments are watched whatever AUTO_JOBS says: people are waiting for their credits
		auto["payments"] = 10 * time.Second
	}
	a.Jobs.Schedule(auto)
	go a.RunBot(ctx)

	srv := &http.Server{Addr: cfg.Addr, Handler: a.Routes()}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	log.Printf("fragwatch API on http://%s (llm=%v model=%s proxies=%d bot=%v ton=%v)", cfg.Addr, a.LLM.Enabled(), cfg.OpenRouterModel,
		len(cfg.Proxies), cfg.TelegramBotToken != "", cfg.TonWallet != "")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
