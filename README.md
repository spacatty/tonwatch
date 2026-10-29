# TON Watch

A local tracker for Telegram usernames on Fragment and the TON blockchain. It collects far more listings than fragment.com shows, rates every name from facts a cheap LLM records about it, and gives you a filterable dashboard. Each row has links to bid, buy or make an offer.

- **backend/**: Go (API on `127.0.0.1:8080`, Postgres, background jobs)
- **frontend/**: Next.js dashboard (`localhost:3000`, which proxies `/api` to the backend)

## Production (Docker)

```bash
scripts/setup-env.sh          # writes .env: asks for the secrets, generates the database password
docker compose up -d --build
```

The stack is three containers configured by the one root `.env`:

| Service | Reachable from | |
|---|---|---|
| `frontend` | `BIND_IP:WEB_PORT` (default `127.0.0.1:3000`) | the only published port |
| `backend` | the frontend, over the `web` network | no host port; it reaches the internet (Fragment, Telegram, OpenRouter) |
| `db` (Postgres 18) | the backend, over the `db` network | no host port; that network is `internal` and has no way out |

The domain lives outside. Point your reverse proxy at `http://127.0.0.1:WEB_PORT`, which serves both the pages and `/api`, and set `PUBLIC_URL` to the proxy's https address. Containers restart on failure, wait for each other's health checks, cap their logs at 50 MB each, and stop cleanly on `docker compose down`.

- **Secrets:** run `scripts/setup-env.sh` again any time to change them. It backs `.env` up to `.env.bak.<time>` first, offers the current values as defaults (Enter keeps a value, `-` clears it), and keeps any keys the template doesn't know. Use `--yes` to fill in what's missing without asking. `.env` and its backups are git-ignored.
- **`PUBLIC_URL`** is built into the frontend, so after changing it run `docker compose up -d --build`. Other `.env` changes need only `docker compose up -d`.
- **`POSTGRES_PASSWORD`** is used only when the `pgdata` volume is first created, so the script never changes it once set.
- **Backups:** `scripts/backup-db.sh` writes a `pg_dump` to `backups/` and keeps the newest 14 (`KEEP=n`). Run it from cron; the restore command is at the top of the script.
- **Moving from SQLite:** stop the old server (so SQLite folds its WAL into the file), start the stack, then import the old database. It goes into the empty Postgres database in one transaction:

  ```bash
  docker compose run --rm --no-deps -v "$PWD/backend/data:/import" --entrypoint /app/import-sqlite backend -from /import/fragwatch.db
  ```

- **Moving data to another machine (dev → prod):** back up on one machine, copy the file over, and restore on the other. See [Moving data to production](#moving-data-to-production).
- **One bot reader:** only one process may poll the bot, so don't run `go run` with the same `TELEGRAM_BOT_TOKEN` and `BOT_POLL=1` alongside the stack.
- **Memory:** Postgres is tuned for a small server. Raise `PG_SHARED_BUFFERS` (~25% of RAM) and `PG_EFFECTIVE_CACHE_SIZE` (~75%) on bigger hosts.

## Moving data to production

The database travels as a `pg_dump` file. On the dev machine:

```bash
scripts/backup-db.sh                                   # writes backups/tonwatch-<time>.dump
scp backups/tonwatch-<time>.dump you@server:tonwatch/backups/
```

On Windows, use the PowerShell twin. It makes the same file, and Windows ships `scp`:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\backup-db.ps1
scp backups\tonwatch-<time>.dump you@server:tonwatch/backups/
```

On the server (first time):

```bash
git clone … tonwatch && cd tonwatch
scripts/setup-env.sh                                   # PUBLIC_URL=https://your.domain, bot token, ADMIN_TELEGRAM_IDS
docker compose up -d --build
scripts/restore-db.sh backups/tonwatch-<time>.dump      # stops the backend, restores in one transaction, starts it again
```

Then point the reverse proxy at `127.0.0.1:WEB_PORT`, run `/setdomain` in @BotFather for the Login Widget, and add `scripts/backup-db.sh` to cron.

- `restore-db.sh` refuses to overwrite a database that already has accounts unless you add `--force`, and it takes a safety dump first whenever the target holds data.
- Accounts are Telegram ids, so whoever signed in on dev signs in on prod as the same account, with the same trackers and credits.
- Each machine keeps its own `.env`: the dump carries data, not settings or secrets.
- **One bot, one reader.** If dev and prod share a bot token, stop the dev backend (or set `BOT_POLL=0` there) before prod starts, or the two fight over the bot's updates. A separate bot for dev avoids this.
- From an old SQLite file instead (a machine that never ran Postgres): use `import-sqlite` as described above. It only loads into an empty database.

## Development

```bash
scripts/setup-env.sh --yes                                              # .env with a database password
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d db # Postgres on 127.0.0.1:5432
cd backend && go run ./cmd/server                                       # uses DATABASE_URL from .env
cd frontend && npm install && npm run dev
```

On Windows you can run `start.ps1` instead; it starts all three.

The store tests run against a real Postgres, each in a throwaway schema, and are skipped without one:

```bash
docker run -d --rm --name pgtest -p 127.0.0.1:55432:5432 -e POSTGRES_PASSWORD=test postgres:18-alpine
cd backend && TEST_DATABASE_URL="postgres://postgres:test@127.0.0.1:55432/postgres?sslmode=disable" go test ./...
```

## Accounts, Telegram and payments

Everyone has an account, and **Telegram is the only way to sign in**: there are no passwords. Names, prices and AI ratings are shared; trackers, stars, hidden names, taste, ideas, the portfolio, settings and credits are per account.

**Admins** are the Telegram ids in `ADMIN_TELEGRAM_IDS`. Set it on a public server, or whoever signs in first becomes admin: when the list is empty, the first account ever is. The first admin also inherits everything made before accounts existed.

Three ways to sign in, all ending in the same account (one per Telegram id):

| Door | How | Needs |
|---|---|---|
| Telegram Mini App | opening the app from the bot signs in from Telegram's signed `initData` | bot + https `PUBLIC_URL` (menu button set automatically) |
| Continue with Telegram (web) | a `t.me/<bot>?start=login_…` link; the bot asks *Approve / Deny* | bot (works on localhost too) |
| Telegram Login Widget (web) | Telegram's own button, shown under the first one | bot + https domain set with `/setdomain` in @BotFather |

Without `TELEGRAM_BOT_TOKEN` nobody can sign in, and the sign-in screen says so.

**Credits and Pro** are bought as packs (`backend/internal/app/billing.go`) through payment gateways:
- **TON**: the payer sends to `TON_WALLET` with the checkout's comment; the `payments` job matches it through toncenter (separate from the crawler's tonapi quota) within seconds. No third party holds the money.
- **Telegram Stars**: an invoice link from the bot; the bot confirms `pre_checkout_query` and credits on `successful_payment`.

Payments are idempotent (one transaction pays one checkout once). Adding a gateway means implementing the `gateway` interface.

**Admin** (sidebar → Admin): accounts, give credits, grant Pro or admin, every payment, and the collectors. Only admins can see or run jobs, re-rate names, or see crawler internals.

## Where the data comes from

| Source | What it gives | Why |
|---|---|---|
| Fragment `searchAuctions` (the site's own JSON API) | auctions, fixed-price sales, sold | Each filter+sort combination is **capped at 500 rows** and paging is ignored, so the `sweep` job runs every sort order (ending/listed/price↑/price↓) and merges the results, giving about 2,000 per category |
| Fragment text search fan-out (`words` job) | listings matching words in `backend/internal/app/words.txt` | Search returns about 10 hits per query, so it is run over a word list to reach names past the 500 cap |
| TON chain via tonapi (`chain` job) | **every minted username** (500k+), owner, and marketplace + price when it's listed on Getgems etc. | The only complete list. Fragment's own auctions are *not* visible here (they live inside the NFT contract). toncenter v3 was tested and times out past offset ~66k |
| tonapi bulk (`prices` job) | prices for any on-chain listing still missing one | catch-up pass |
| Fragment username pages (`probe`, `auctions`, ↻ button) | exact status, min bid, buy-now, end time, owner | `probe` checks every name the LLM scored ≥ 7 to find auctions and sales that are hidden from Fragment's lists |

## Dashboard

Next.js 16 with shadcn/ui (Base UI), lucide icons, motion, sonner and cmdk. Light and dark themes follow the system and can be toggled in the sidebar.

- **Filter panel**: search (underscores ignored), an AI **Topic** filter (type "vpn" and the AI turns it into words to look for), buyout and price ranges, an "instant buy only" switch, AI score, length, digit combinations, kind of name, and character rules. It's a column on wide screens and a sheet on smaller ones.
- **Buyout**: what owning a name costs right now (buy-now on an auction, the listing price otherwise). You can filter and sort by it. Fragment's lists don't include buy-now prices, so the "Buy-now prices" collector reads auction pages, soonest-ending first.
- **Sortable table**: click any column header; click again to flip the direction. The Status header filters by status. More rows load as you scroll.
- **Worth a look**: the best-rated names closing next, or for sale now cheapest first, above the table.
- **Trackers**: save any view and filter set from the sidebar. Each shows its match count and how many names are new (first seen, listed or repriced) since you last opened it. A few starter trackers are created on first run.
- **Ask AI**: describe a project ("I'm building a VPN app"). The AI plans search words, ranks matching names from the whole database with a reason for each, and checks fresh ideas live on Fragment. Results can be browsed in full or saved as a tracker.
- **Snipes** (home): only offers open right now, inside your sniper settings (budget, minimum score, kinds). Each card has a Buy/Bid button, a star, a lightbulb ("why this name", through your taste) and an × ("not for me"). "Why these 12" reads a whole grid in one call.
- **Ideas**: like or skip names to teach a taste; stars, buys, long looks and dismissed cards teach it too, for free. Then the model invents projects on names you can buy today. Each concept can be rotated, or bent your way with "I'd say…" (free).
- **Table rows**: the buy/bid button, then your verdicts: star (track), heart (like, for the taste), lightbulb (imagine a project for this one name, 2 credits: what it is, mini app / bot / channel, how the handle is used, first move), eye-off (hide: it leaves every list for good; a **Hidden** view brings names back), and a ⋯ menu (re-check, copy, I own it, Getgems).
- **Everything generated**: every concept ever made is kept on the Ideas page, filterable by name, and new runs are told to steer clear of past titles.
- **Portfolio**: names you own, what you paid, what names of that score typically sell for now, and the concept attached to each.
- **AI credits**: every AI action has a fixed price (ideas 10, grid read 5, Ask AI 5, rotate 3, taste profile 2, why-this-name 1, topic 1); once a day they can claim a top-up to `DAILY_CREDITS` (`FREE_DAILY_CREDITS` until the account's first payment) behind a Cloudflare Turnstile check (`TURNSTILE_SITE_KEY`/`TURNSTILE_SECRET_KEY`; without keys, press and hold), and nothing arrives unless they come and claim it, bought credits sit on top, and the ledger shows the real token cost per action. Cached answers are free. System prompts are marked for the provider's prompt cache.
- **Free tier**: an account that has never paid sees every name, but only the first 300 rows of any search, without the rule builder, quiet-days or keyword filters, and with one tracker. Any payment unlocks all of it for good (`App.gate` in `api.go`). Pro (30 days) is 250 Stars, 2 TON, or 1,500 credits from the balance.
- **Telegram alerts** (Pro): every Pro account with Telegram linked gets its own alerts from the bot: new names inside its settings, tracked auctions an hour from closing, and price drops on names it follows, every 5 minutes.
- **Ctrl/⌘ K**: jump to any username, or check a new one on Fragment and track it. `/` focuses search.
- Clicking a name opens its sheet: price, buy-now, countdown, Fragment, Getgems and Tonviewer links, refresh, track with a note, and the name's status and price history.

## Statuses

`auction` / `sale` are Fragment listings. `market` is a listing on another marketplace (link to Getgems). `owned` means not listed; **Offer** opens Fragment, where any owned username has a "Make an offer" button (it needs your TON wallet connected on Fragment).

## AI rating

The rating has two layers, so it stays stable and can be retuned without asking the model again:

1. **Facts** (`backend/internal/llm/prompt.go`, job `llm`). The model reads each name once and records facts that do not drift with fashion: how it reads (`words`), its `kind` and language, `fame` 0–5 (how widely known the reading is), `pull` 0–2 (how much a business would want it), `clean` 0–2 (how naturally it is written), and for fame 2+ a short `gloss` shown as the meaning. A gloss that only splits the name into words is dropped.
2. **Score** (`backend/internal/rating/rating.go`). Arithmetic over the facts and the name's shape (length, repeated letters), calibrated on real Fragment sales. To change how names are judged, edit `Score` and bump `rating.Version`: on start the server sees the new version and recomputes every score from the stored facts in seconds (job `rescore`, also `POST /api/llm/rescore`). No model calls.

Every batch mixes lengths and letters (the queue is ordered by status, short names first, then `md5(name)`), and 8 anchor names with settled facts ride along in each one. If the model's fame or pull for the anchors is off by more than 0.8 on average, the batch is asked once more, then saved anyway and logged. Obvious junk is removed first by `store/junk.go` at no cost.

To try the prompt and formula on a few names, or to measure them against real sales:

```bash
cd backend && go run ./cmd/rate coffee goodnews harbor lunexo
cd backend && go run ./cmd/rate -eval      # Spearman of old vs new score against log sale price, by length
```

`-eval` draws two fixed samples of sold names once: about 300 stratified by length (`cmd/rate/evalset.json`) and 300 four-letter names (`evalset4.json`), each with the score stored at that moment as the "old" score. The facts it records are cached next to them (`evalfacts*.json`), so retuning the formula re-evaluates for free. It reports the stored score, the previous formula (frozen in `cmd/rate/prev.go`) and the current one. The four-letter sample alternates between a tuning half and a held-out half: tune with `-eval -explore` (median prices by fact on the tuning half) and judge on the held-out line. Add `-refresh` after editing the prompt, `-rebuild` for new samples. It prints only aggregates, never names.

**Jobs → Re-read all names** (`POST /api/llm/reset`) clears the recorded facts so the model reads every name again. It is non-destructive: scores, kinds and meanings stay visible until each name's new facts land. With deepseek-v4.1-flash, about 650k names cost a few dollars.

**Premium pass** (job `llm-premium`, manual only): a stronger model re-records the facts of the names that matter most, about 16k: score 8+, every 4-letter name, 5 letters with score 6+.

**Typical prices** (the "value" sort, deal badges, the portfolio, `GET /api/market/medians`) are medians per score and length bucket (4, 5, 6–7, 8–10, 11+), falling back to the score alone where a bucket has fewer than 30 sales and 10 listings.

**Search by meaning** (job `embed`, manual): names rated 6+, or listed and rated 4+ (about 128k), are embedded and kept in memory as int8 vectors. Ask AI adds the closest names by meaning to its keyword matches (at most 220 candidates go to ranking). Filters, the Topic chip and trackers stay keyword rules. Embedding all of them costs a few cents.

**AI filter**: answers are cached for everyone by prompt version, language and the normalised request, so a repeated request is free. Anything the rules cannot express comes back as `could_not` and is shown in the filter bar.

| Setting | Default | |
|---|---|---|
| `OPENROUTER_MODEL` | `deepseek/deepseek-v4.1-flash` | the facts model |
| `LLM_BATCH_SIZE` / `LLM_WORKERS` | `150` / `4` | names per request, requests in parallel |
| `LLM_REASONING` | `0` | let the facts model think first (≈4× the output tokens) |
| `LLM_PREMIUM_MODEL` | `anthropic/claude-opus-5.5` | the premium pass model |
| `LLM_PREMIUM_EFFORT` | `low` | sent as OpenRouter `reasoning.effort`; no temperature is sent then |
| `LLM_PREMIUM_BATCH` | `40` | names per premium request |
| `EMBED_MODEL` / `EMBED_DIMS` | `openai/text-embedding-3-small` / `256` | the search-by-meaning vectors |

## API

`GET /api/usernames?status=auction,sale&max_price=50&min_score=7&category=word,name&sort=ending&q=^dark`
Every route needs a session (cookie on the web, `Authorization: Bearer` in the Mini App) except `/api/auth/*`, `/api/meta` and `/api/billing/packs`; `/api/jobs`, `/api/llm/*` and `/api/admin/*` need an admin. Also: `GET /api/usernames/{name}` (with history), `POST /api/usernames/{name}/refresh`, `POST /api/usernames/{name}/watch`, `GET /api/jobs`, `POST /api/jobs/{name}/run|stop`, `GET /api/stats`.
