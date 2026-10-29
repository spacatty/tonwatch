// Package config loads settings from environment variables and an optional .env file.
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr string
	// DatabaseURL: postgres://user:pass@host:5432/db?sslmode=disable. Empty uses the PG* variables.
	DatabaseURL string

	OpenRouterKey   string
	OpenRouterModel string
	LLMBatchSize    int
	LLMWorkers      int
	LLMReasoning    bool
	// The premium pass re-records the facts of the names that matter most (top scores, every
	// four-letter name) with a stronger model. Manual only: job "llm-premium".
	LLMPremiumModel  string
	LLMPremiumEffort string // OpenRouter reasoning.effort: low | medium | high; empty = no reasoning
	LLMPremiumBatch  int
	// EmbedModel turns names into vectors for Ask AI's semantic search (job "embed").
	EmbedModel string
	EmbedDims  int

	TonapiKey string

	// TelegramBotToken: the bot (@BotFather) behind Telegram sign-in, the Mini App, Stars payments
	// and alerts. Empty = all of those are off; password accounts still work.
	TelegramBotToken string
	// BotPoll: this process reads the bot's updates (getUpdates). Only one process may.
	BotPoll bool
	// PublicURL is where people open the app (https://… in production). The bot's "Open app"
	// button and the Login Widget need it; the Mini App needs it to be https.
	PublicURL string
	// AdminTelegramIDs are made admins when they sign in. The first account is admin anyway.
	AdminTelegramIDs []int64
	// DailyCredits is the allowance every account that has paid once is topped up to each day;
	// FreeDailyCredits is the smaller one for accounts that never have.
	DailyCredits     int
	FreeDailyCredits int
	// The daily allowance is claimed by hand behind a Cloudflare Turnstile check when these are
	// set; without them the claim is a press-and-hold in the app, which stops nothing scripted.
	TurnstileSite   string
	TurnstileSecret string
	// TonWallet receives TON top-ups; each checkout gets its own comment to match the transfer.
	// Empty = the TON gateway is off.
	TonWallet string
	// StarsEnabled turns on Telegram Stars checkout (needs the bot).
	StarsEnabled bool

	// Proxies is a list of proxy URLs (http://, https://, socks5://) rotated per request.
	Proxies []string

	FragmentRPS   float64
	TonapiRPS     float64
	DetailWorkers int

	// Probe: Fragment detail-check owned names whose LLM score is at least this.
	ProbeMinScore int

	EveryAuctions time.Duration
	EverySweep    time.Duration
	EveryChain    time.Duration
	EveryProbe    time.Duration
	AutoJobs      bool
	// SkipChain stops the tonapi collectors (the full chain crawl and listing prices), e.g. on a
	// dev machine that should not spend the API quota. Fragment collection keeps running.
	SkipChain bool
}

// Load reads .env files (first found wins per key; real env vars always win) then env.
func Load() Config {
	for _, p := range []string{".env", "../.env"} {
		loadDotenv(p)
	}
	return Config{
		Addr:             env("ADDR", "127.0.0.1:8080"),
		DatabaseURL:      env("DATABASE_URL", ""),
		OpenRouterKey:    env("OPENROUTER_API_KEY", ""),
		OpenRouterModel:  env("OPENROUTER_MODEL", "deepseek/deepseek-v4.1-flash"),
		LLMBatchSize:     envInt("LLM_BATCH_SIZE", 150),
		LLMWorkers:       envInt("LLM_WORKERS", 4),
		LLMReasoning:     env("LLM_REASONING", "0") == "1",
		LLMPremiumModel:  env("LLM_PREMIUM_MODEL", "anthropic/claude-opus-5.5"),
		LLMPremiumEffort: env("LLM_PREMIUM_EFFORT", "low"),
		LLMPremiumBatch:  envInt("LLM_PREMIUM_BATCH", 40),
		EmbedModel:       env("EMBED_MODEL", "openai/text-embedding-3-small"),
		EmbedDims:        envInt("EMBED_DIMS", 256),
		TonapiKey:        env("TONAPI_KEY", ""),
		TelegramBotToken: env("TELEGRAM_BOT_TOKEN", ""),
		BotPoll:          env("BOT_POLL", "1") != "0",
		PublicURL:        strings.TrimSuffix(env("PUBLIC_URL", "http://localhost:3000"), "/"),
		AdminTelegramIDs: int64List(env("ADMIN_TELEGRAM_IDS", "")),
		DailyCredits:     envInt("DAILY_CREDITS", 100),
		FreeDailyCredits: envInt("FREE_DAILY_CREDITS", 20),
		TurnstileSite:    env("TURNSTILE_SITE_KEY", ""),
		TurnstileSecret:  env("TURNSTILE_SECRET_KEY", ""),
		TonWallet:        env("TON_WALLET", ""),
		StarsEnabled:     env("STARS", "1") != "0",
		Proxies:          splitList(env("PROXIES", "")),
		FragmentRPS:      envFloat("FRAGMENT_RPS", 4),
		TonapiRPS:        envFloat("TONAPI_RPS", 0.9),
		DetailWorkers:    envInt("DETAIL_WORKERS", 6),
		ProbeMinScore:    envInt("PROBE_MIN_SCORE", 7),
		EveryAuctions:    envDur("EVERY_AUCTIONS", 3*time.Minute),
		EverySweep:       envDur("EVERY_SWEEP", 30*time.Minute),
		EveryChain:       envDur("EVERY_CHAIN", 12*time.Hour),
		EveryProbe:       envDur("EVERY_PROBE", 6*time.Hour),
		AutoJobs:         env("AUTO_JOBS", "1") != "0",
		SkipChain:        truthy(env("SKIP_CHAIN", "")),
	}
}

func loadDotenv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
		}
	}
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if n, err := strconv.Atoi(env(k, "")); err == nil {
		return n
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if n, err := strconv.ParseFloat(env(k, ""), 64); err == nil {
		return n
	}
	return def
}

func envDur(k string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(env(k, "")); err == nil {
		return d
	}
	return def
}

func int64List(s string) []int64 {
	var out []int64
	for _, p := range splitList(s) {
		if n, err := strconv.ParseInt(p, 10, 64); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// truthy reads "1", "true", "yes" or "on" as true.
func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
