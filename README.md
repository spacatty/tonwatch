# BidSpot

A local tracker for Telegram usernames on Fragment and the TON blockchain. It collects far more listings than fragment.com shows, rates every name with a cheap LLM to drop gibberish, and gives you a filterable dashboard. Each row has links to bid, buy or make an offer.

- **backend/**: Go (API on `127.0.0.1:8080`, SQLite, background jobs)
- **frontend/**: Next.js dashboard (`localhost:3000`, which proxies `/api` to the backend)

## Run

```bash
cp .env.example .env          # then paste OPENROUTER_API_KEY
cd backend && go run ./cmd/server
cd frontend && npm install && npm run dev
```

On Windows you can run `start.ps1` instead; it starts both.

## Accounts, Telegram and payments

Everyone has an account. Names, prices and AI ratings are shared; trackers, stars, hidden names, taste, ideas, the portfolio, settings and credits are per account. **The first account becomes admin and inherits everything made before accounts existed.**

Ways to sign in, all ending in the same account:

| Door | How | Needs |
|---|---|---|
| Telegram Mini App | opening the app from the bot signs in from Telegram's signed `initData` | bot + https `PUBLIC_URL` (menu button set automatically) |
| Continue with Telegram (web) | a `t.me/<bot>?start=login_…` link; the bot asks *Approve / Deny* | bot (works on localhost) |
| Telegram Login Widget (web) | Telegram's own button | bot + https domain set with `/setdomain` |
| Login + password | PBKDF2-SHA256 | nothing |
| Telegram 2FA | password sign-ins wait for *Approve* in the bot | a linked Telegram |

A Telegram-only account can add a login and password in **Account**; a password account can link Telegram there.

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
- **AI credits**: every AI action has a fixed price (ideas 10, grid read 5, Ask AI 5, rotate 3, taste profile 2, why-this-name 1, topic 1); the balance refills to `DAILY_CREDITS` each day, bought credits sit on top, and the ledger shows the real token cost per action. Cached answers are free. System prompts are marked for the provider's prompt cache.
- **Telegram alerts** (Pro): every Pro account with Telegram linked gets its own alerts from the bot: new names inside its settings, tracked auctions an hour from closing, and price drops on names it follows, every 5 minutes.
- **Ctrl/⌘ K**: jump to any username, or check a new one on Fragment and track it. `/` focuses search.
- Clicking a name opens its sheet: price, buy-now, countdown, Fragment, Getgems and Tonviewer links, refresh, track with a note, and the name's status and price history.

## Statuses

`auction` / `sale` are Fragment listings. `market` is a listing on another marketplace (link to Getgems). `owned` means not listed; **Offer** opens Fragment, where any owned username has a "Make an offer" button (it needs your TON wallet connected on Fragment).

## LLM filter

`backend/internal/llm/prompt.go` holds the system prompt (0–10 score, category, short meaning). Obvious junk is removed first by `store/junk.go` at no cost. To tune the prompt:

```bash
cd backend && go run ./cmd/rate pizza darkmoon vip8989d maria_art
```

After you edit the prompt, use **Jobs → Re-rate everything**. With deepseek-v4.1-flash, rating about 500k names costs a few dollars.

## API

`GET /api/usernames?status=auction,sale&max_price=50&min_score=7&category=word,name&sort=ending&q=^dark`
Every route needs a session (cookie on the web, `Authorization: Bearer` in the Mini App) except `/api/auth/*`, `/api/meta` and `/api/billing/packs`; `/api/jobs`, `/api/llm/*` and `/api/admin/*` need an admin. Also: `GET /api/usernames/{name}` (with history), `POST /api/usernames/{name}/refresh`, `POST /api/usernames/{name}/watch`, `GET /api/jobs`, `POST /api/jobs/{name}/run|stop`, `GET /api/stats`.
