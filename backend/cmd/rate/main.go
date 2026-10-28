// Command rate scores usernames with the configured LLM, for tuning the system prompt:
//
//	go run ./cmd/rate pizza darkmoon xqpppppp vip8989d maria_art
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"fragwatch/internal/config"
	"fragwatch/internal/httpx"
	"fragwatch/internal/llm"
	"fragwatch/internal/store"
)

func main() {
	names := os.Args[1:]
	if len(names) == 0 {
		names = []string{"pizza", "darkmoon", "xqpppppp", "vip8989d", "maria_art", "casa", "h4ck3r", "jinsanjiao138", "goodnews", "btc6z6", "elens", "tonwallet"}
	}
	cfg := config.Load()
	c := llm.New(httpx.New(0, nil), cfg.OpenRouterKey, cfg.OpenRouterModel)
	c.Reasoning = cfg.LLMReasoning
	scores, missing, usage, err := c.Rate(context.Background(), names)
	if err != nil {
		log.Fatal(err)
	}
	for _, s := range scores {
		fmt.Printf("%-18s %2d  %-8s %s  (rule-junk=%v)\n", s.Name, s.Score, s.Category, s.Meaning, store.IsJunk(s.Name))
	}
	fmt.Printf("missing: %v\ntokens in/out: %d/%d  cost $%.6f\n", missing, usage.PromptTokens, usage.CompletionTokens, usage.Cost)
}
