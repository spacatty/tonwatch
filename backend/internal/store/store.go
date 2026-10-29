// Package store persists usernames in Postgres. Each data source writes its own columns; the
// dashboard-facing `status`, `price` and `buyout` are generated from them.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Source bits recorded in usernames.src.
const (
	SrcFragmentList   = 1
	SrcFragmentSearch = 2
	SrcChain          = 4
	SrcProbe          = 8
)

// schema is idempotent: it runs on every start. Add columns with ADD COLUMN IF NOT EXISTS below
// the table, never by editing a CREATE TABLE that already exists in someone's database.
//
// Trigram indexes serve the substring searches (name, name without underscores, meaning); the
// opclass is schema-qualified so the indexes work whatever the search_path.
const schema = `
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

CREATE TABLE IF NOT EXISTS usernames (
  name          TEXT PRIMARY KEY,
  len           INTEGER NOT NULL,
  has_digit     INTEGER NOT NULL DEFAULT 0,
  has_under     INTEGER NOT NULL DEFAULT 0,
  junk          INTEGER NOT NULL DEFAULT 0,
  src           INTEGER NOT NULL DEFAULT 0,

  frag_status   TEXT NOT NULL DEFAULT '',
  frag_price    DOUBLE PRECISION,
  frag_buy_now  DOUBLE PRECISION,   -- only a name's own page shows it
  frag_usd      DOUBLE PRECISION,
  frag_ends_at  BIGINT,
  frag_owner    TEXT,
  frag_at       BIGINT,
  detail_at     BIGINT,             -- when the name's own Fragment page was last read

  nft_address   TEXT,
  owner         TEXT,
  chain_on_sale INTEGER NOT NULL DEFAULT 0,
  sale_contract TEXT,
  chain_market  TEXT,
  chain_price   DOUBLE PRECISION,
  chain_at      BIGINT,

  score         INTEGER,
  category      TEXT,
  meaning       TEXT,
  llm_at        BIGINT,
  llm_tries     INTEGER NOT NULL DEFAULT 0, -- skipped by the model this many times; 2 = give up

  -- single-user leftovers: only ClaimLegacy reads them (verdicts live in user_names)
  watched       INTEGER NOT NULL DEFAULT 0,
  hidden        INTEGER NOT NULL DEFAULT 0,
  note          TEXT,

  first_seen    BIGINT NOT NULL,
  updated_at    BIGINT NOT NULL,

  status TEXT GENERATED ALWAYS AS (CASE
    WHEN frag_status = 'auction' THEN 'auction'
    WHEN frag_status = 'sale' THEN 'sale'
    WHEN chain_on_sale = 1 THEN 'market'
    WHEN frag_status = 'available' THEN 'available'
    ELSE 'owned' END) STORED,
  price DOUBLE PRECISION GENERATED ALWAYS AS (CASE
    WHEN frag_status IN ('auction','sale') THEN frag_price
    WHEN chain_on_sale = 1 THEN chain_price
    END) STORED,
  -- what it costs to own the name right now: buy-now on an auction, the listing price otherwise
  buyout DOUBLE PRECISION GENERATED ALWAYS AS (CASE
    WHEN frag_status = 'auction' THEN frag_buy_now
    WHEN frag_status = 'sale' THEN frag_price
    WHEN chain_on_sale = 1 THEN chain_price
    END) STORED
);
CREATE INDEX IF NOT EXISTS ix_status_price ON usernames(status, price);
CREATE INDEX IF NOT EXISTS ix_score ON usernames(score DESC NULLS LAST, len, name);
CREATE INDEX IF NOT EXISTS ix_price ON usernames(price) WHERE price IS NOT NULL;
CREATE INDEX IF NOT EXISTS ix_buyout ON usernames(buyout) WHERE buyout IS NOT NULL;
CREATE INDEX IF NOT EXISTS ix_len ON usernames(len);
CREATE INDEX IF NOT EXISTS ix_first_seen ON usernames(first_seen DESC);
CREATE INDEX IF NOT EXISTS ix_updated ON usernames(updated_at DESC);
CREATE INDEX IF NOT EXISTS ix_category ON usernames(category) WHERE category IS NOT NULL;
CREATE INDEX IF NOT EXISTS ix_auction_ends ON usernames(frag_ends_at) WHERE frag_status = 'auction';
CREATE INDEX IF NOT EXISTS ix_listed ON usernames(frag_at) WHERE frag_status IN ('auction','sale');
CREATE INDEX IF NOT EXISTS ix_nft ON usernames(nft_address) WHERE nft_address IS NOT NULL;
CREATE INDEX IF NOT EXISTS ix_chain_unpriced ON usernames(nft_address) WHERE chain_on_sale = 1 AND chain_market IS NULL;
-- facts the model recorded (see the rating package); score, category and meaning are computed from them
ALTER TABLE usernames ADD COLUMN IF NOT EXISTS words TEXT;
ALTER TABLE usernames ADD COLUMN IF NOT EXISTS lang TEXT;
ALTER TABLE usernames ADD COLUMN IF NOT EXISTS fame SMALLINT;
ALTER TABLE usernames ADD COLUMN IF NOT EXISTS pull SMALLINT;
ALTER TABLE usernames ADD COLUMN IF NOT EXISTS clean SMALLINT;
ALTER TABLE usernames ADD COLUMN IF NOT EXISTS facts_kind TEXT;   -- the kind as the model gave it; category is the final one
ALTER TABLE usernames ADD COLUMN IF NOT EXISTS facts_at BIGINT;
ALTER TABLE usernames ADD COLUMN IF NOT EXISTS facts_model TEXT;
ALTER TABLE usernames ADD COLUMN IF NOT EXISTS premium_at BIGINT;  -- facts re-recorded by the premium model
-- the facts queue in the order FactsBatch reads it (the expressions must match the query's)
DROP INDEX IF EXISTS ix_llm_queue;
CREATE INDEX IF NOT EXISTS ix_facts_queue ON usernames((`+factsTier+`), md5(name))
  WHERE facts_at IS NULL AND junk = 0 AND llm_tries < 2;
CREATE INDEX IF NOT EXISTS ix_name_trgm ON usernames USING gin (name public.gin_trgm_ops);
CREATE INDEX IF NOT EXISTS ix_bare_trgm ON usernames USING gin (replace(name, '_', '') public.gin_trgm_ops);
CREATE INDEX IF NOT EXISTS ix_meaning_trgm ON usernames USING gin (meaning public.gin_trgm_ops);

CREATE TABLE IF NOT EXISTS events (
  id     BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
  name   TEXT NOT NULL,
  ts     BIGINT NOT NULL,
  kind   TEXT NOT NULL,
  detail TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_events_name ON events(name, ts);
CREATE INDEX IF NOT EXISTS ix_events_ts ON events(ts DESC, id DESC);

CREATE TABLE IF NOT EXISTS kv (k TEXT PRIMARY KEY, v TEXT NOT NULL);

CREATE TABLE IF NOT EXISTS concepts (
  id         BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
  user_id    BIGINT NOT NULL DEFAULT 0,
  ts         BIGINT NOT NULL,
  source     TEXT NOT NULL,              -- set | rotate | refine | imagine
  project    TEXT NOT NULL,
  pitch      TEXT NOT NULL DEFAULT '',
  first_move TEXT NOT NULL DEFAULT '',
  form       TEXT NOT NULL DEFAULT '',   -- mini app | web app | bot | channel | both …
  how        TEXT NOT NULL DEFAULT '',   -- how the name is used in the product
  note       TEXT NOT NULL DEFAULT '',   -- the user's own steer, when there was one
  liked      INTEGER NOT NULL DEFAULT 0, -- 1 liked, -1 disliked, 0 none
  built_with TEXT NOT NULL DEFAULT ''    -- what it runs on: proof it can be built
);
CREATE INDEX IF NOT EXISTS ix_concepts_user ON concepts(user_id, id DESC);
CREATE TABLE IF NOT EXISTS concept_names (
  concept_id BIGINT NOT NULL REFERENCES concepts(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  angle      TEXT NOT NULL DEFAULT '',
  seq        BIGINT GENERATED BY DEFAULT AS IDENTITY, -- keeps the names in the order they were given
  PRIMARY KEY (concept_id, name)
);
CREATE INDEX IF NOT EXISTS ix_concept_names_name ON concept_names(name);

CREATE TABLE IF NOT EXISTS portfolio (
  user_id   BIGINT NOT NULL DEFAULT 0,
  name      TEXT NOT NULL,
  bought    DOUBLE PRECISION,          -- what they paid, TON (NULL = unknown)
  bought_at BIGINT,
  concept   TEXT NOT NULL DEFAULT '',  -- the project they attached to it
  note      TEXT NOT NULL DEFAULT '',
  added_at  BIGINT NOT NULL,
  PRIMARY KEY (user_id, name)
);

CREATE TABLE IF NOT EXISTS trackers (
  id         BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
  user_id    BIGINT NOT NULL DEFAULT 0,
  name       TEXT NOT NULL,
  view       TEXT NOT NULL,
  filters    TEXT NOT NULL, -- the dashboard's filter state, opaque to the backend
  params     TEXT NOT NULL, -- the same search as an /api/usernames query string
  seen_at    BIGINT NOT NULL,
  created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_trackers_user ON trackers(user_id, id);
`

type Store struct {
	DB *DB

	typMu sync.Mutex
	typ   Typical // typical prices per AI score and length, for the "value" sort
	typAt time.Time

	statMu sync.Mutex
	stat   *Stats // whole-table counts, cached briefly: they scan every row
	statAt time.Time
}

// Open connects to Postgres and brings the schema up to date. dsn is a postgres:// URL or
// key=value string; empty uses the PG* environment variables (PGHOST, PGUSER, PGPASSWORD…).
// The database may still be starting (docker compose), so the first connection is retried.
func Open(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(time.Hour)
	ctx := context.Background()
	for i := 0; ; i++ {
		if err = db.PingContext(ctx); err == nil {
			break
		}
		if i == 30 {
			db.Close()
			return nil, fmt.Errorf("connect to postgres: %w", err)
		}
		time.Sleep(time.Second)
	}
	// one schema change at a time, should two instances start together
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(7262016)`); err != nil {
		return nil, err
	}
	defer conn.ExecContext(ctx, `SELECT pg_advisory_unlock(7262016)`)
	for _, q := range []string{schema, usersSchema, activitySchema, aiChatSchema, vectorSchema} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			db.Close()
			return nil, fmt.Errorf("schema: %w", err)
		}
	}
	return &Store{DB: &DB{db}}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func now() int64 { return time.Now().Unix() }

// FragRow is what Fragment (list or detail page) tells us about a username.
type FragRow struct {
	Name    string
	Status  string // auction | sale | sold | taken | available
	Price   *float64
	BuyNow  *float64
	USD     *float64
	EndsAt  *int64
	Owner   string // detail pages only
	FromSrc int
}

// ChainRow is one NFT item from the TON indexer.
type ChainRow struct {
	Name         string
	NFTAddress   string
	Owner        string
	OnSale       bool
	SaleContract string
	Market       string // set when the indexer already knows the listing
	Price        *float64
}

func nulls(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func fmtPrice(p *float64) string {
	if p == nil {
		return "-"
	}
	return fmt.Sprintf("%g", *p)
}

// lastBy keeps the last row for each key, in first-seen order: one statement may not touch a row
// twice, and the latest data wins.
func lastBy[T any](rows []T, key func(T) string) []T {
	at := make(map[string]int, len(rows))
	out := make([]T, 0, len(rows))
	for _, r := range rows {
		k := key(r)
		if i, ok := at[k]; ok {
			out[i] = r
			continue
		}
		at[k] = len(out)
		out = append(out, r)
	}
	return out
}

// base holds the columns every new name gets, computed once per batch.
type base struct {
	len, digit, under, junk []int
}

func newBase(names []string) base {
	b := base{make([]int, len(names)), make([]int, len(names)), make([]int, len(names)), make([]int, len(names))}
	for i, n := range names {
		b.len[i], b.digit[i], b.under[i], b.junk[i] = len(n), b2i(strings.ContainsAny(n, "0123456789")), b2i(strings.Contains(n, "_")), b2i(IsJunk(n))
	}
	return b
}

type event struct {
	name, kind, detail string
}

func addEvents(ctx context.Context, tx *Tx, ts int64, evs []event) error {
	if len(evs) == 0 {
		return nil
	}
	names, kinds, details := make([]string, len(evs)), make([]string, len(evs)), make([]string, len(evs))
	for i, e := range evs {
		names[i], kinds[i], details[i] = e.name, e.kind, e.detail
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO events(name, ts, kind, detail)
		SELECT name, ?::bigint, kind, detail FROM unnest(?::text[], ?::text[], ?::text[]) AS e(name, kind, detail)`, ts, names, kinds, details)
	return err
}

// UpsertFragment writes Fragment data and records status/price changes as events. The whole
// batch is one statement; RETURNING old.* hands back what each name looked like before.
func (s *Store) UpsertFragment(ctx context.Context, rows []FragRow) error {
	rows = lastBy(rows, func(r FragRow) string { return r.Name })
	if len(rows) == 0 {
		return nil
	}
	n := len(rows)
	names, statuses, owners := make([]string, n), make([]string, n), make([]string, n)
	srcs, detail := make([]int, n), make([]int, n)
	prices, buyNow, usd := make([]*float64, n), make([]*float64, n), make([]*float64, n)
	ends := make([]*int64, n)
	for i, r := range rows {
		names[i], statuses[i], owners[i], srcs[i] = r.Name, r.Status, r.Owner, r.FromSrc
		prices[i], buyNow[i], usd[i], ends[i] = r.Price, r.BuyNow, r.USD, r.EndsAt
		detail[i] = b2i(r.FromSrc == SrcProbe)
	}
	b := newBase(names)
	ts := now()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Only a name's own page shows the buy-now price; list rows don't carry it. So list rows keep
	// the known buy-now while the same auction runs, and only a page read sets it (or clears it).
	// A page read is the one that sets detail_at, which is how the update below tells them apart.
	res, err := tx.QueryContext(ctx, `INSERT INTO usernames AS u (name, len, has_digit, has_under, junk, src, first_seen, updated_at,
			frag_status, frag_price, frag_buy_now, frag_usd, frag_ends_at, frag_owner, frag_at, detail_at)
		SELECT name, len, has_digit, has_under, junk, src, ?::bigint, ?::bigint, status, price,
			CASE WHEN detail = 1 THEN buy_now END, usd, ends_at, NULLIF(owner, ''), ?::bigint, CASE WHEN detail = 1 THEN ?::bigint END
		FROM unnest(?::text[], ?::int[], ?::int[], ?::int[], ?::int[], ?::int[], ?::int[], ?::text[], ?::float8[], ?::float8[], ?::float8[], ?::int8[], ?::text[])
			AS r(name, len, has_digit, has_under, junk, src, detail, status, price, buy_now, usd, ends_at, owner)
		ON CONFLICT (name) DO UPDATE SET src = u.src | excluded.src,
			frag_status = excluded.frag_status, frag_price = excluded.frag_price,
			frag_buy_now = CASE WHEN excluded.detail_at IS NOT NULL THEN excluded.frag_buy_now
				WHEN u.frag_status = 'auction' AND excluded.frag_status = 'auction' THEN u.frag_buy_now END,
			detail_at = COALESCE(excluded.detail_at, u.detail_at),
			frag_usd = excluded.frag_usd, frag_ends_at = excluded.frag_ends_at,
			frag_owner = COALESCE(excluded.frag_owner, u.frag_owner), frag_at = excluded.frag_at, updated_at = excluded.updated_at
		RETURNING new.name, COALESCE(old.frag_status, ''), old.frag_price`,
		ts, ts, ts, ts, names, b.len, b.digit, b.under, b.junk, srcs, detail, statuses, prices, buyNow, usd, ends, owners)
	if err != nil {
		return err
	}
	type before struct {
		status string
		price  sql.NullFloat64
	}
	old := make(map[string]before, n)
	for res.Next() {
		var name string
		var o before
		if err := res.Scan(&name, &o.status, &o.price); err != nil {
			res.Close()
			return err
		}
		old[name] = o
	}
	res.Close()
	if err := res.Err(); err != nil {
		return err
	}
	var evs []event
	for _, r := range rows {
		o := old[r.Name]
		switch {
		case o.status == "":
			if r.Status == "auction" || r.Status == "sale" {
				evs = append(evs, event{r.Name, "listed", fmt.Sprintf("%s at %s TON", r.Status, fmtPrice(r.Price))})
			}
		case o.status != r.Status:
			evs = append(evs, event{r.Name, "status", fmt.Sprintf("%s → %s (%s TON)", o.status, r.Status, fmtPrice(r.Price))})
		case r.Price != nil && o.price.Valid && o.price.Float64 != *r.Price:
			evs = append(evs, event{r.Name, "price", fmt.Sprintf("%g → %g TON", o.price.Float64, *r.Price)})
		}
	}
	if err := addEvents(ctx, tx, ts, evs); err != nil {
		return err
	}
	return tx.Commit()
}

// UpsertChain writes on-chain ownership. A listing the indexer already knows (Market) is written
// too; otherwise market/price survive only while the same sale contract is still up, and
// SetChainSales fills them in later.
func (s *Store) UpsertChain(ctx context.Context, rows []ChainRow) error {
	rows = lastBy(rows, func(r ChainRow) string { return r.Name })
	if len(rows) == 0 {
		return nil
	}
	n := len(rows)
	names, nfts, owners, contracts, markets := make([]string, n), make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	onSale, srcs := make([]int, n), make([]int, n)
	prices := make([]*float64, n)
	for i, r := range rows {
		names[i], nfts[i], owners[i], contracts[i], markets[i] = r.Name, r.NFTAddress, r.Owner, r.SaleContract, r.Market
		onSale[i], srcs[i], prices[i] = b2i(r.OnSale), SrcChain, r.Price
	}
	b := newBase(names)
	ts := now()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.QueryContext(ctx, `INSERT INTO usernames AS u (name, len, has_digit, has_under, junk, src, first_seen, updated_at,
			nft_address, owner, chain_on_sale, sale_contract, chain_market, chain_price, chain_at)
		SELECT name, len, has_digit, has_under, junk, src, ?::bigint, ?::bigint, nft, NULLIF(owner, ''), on_sale, NULLIF(contract, ''),
			NULLIF(market, ''), CASE WHEN market != '' THEN price END, ?::bigint
		FROM unnest(?::text[], ?::int[], ?::int[], ?::int[], ?::int[], ?::int[], ?::text[], ?::text[], ?::int[], ?::text[], ?::text[], ?::float8[])
			AS r(name, len, has_digit, has_under, junk, src, nft, owner, on_sale, contract, market, price)
		ON CONFLICT (name) DO UPDATE SET src = u.src | excluded.src,
			nft_address = excluded.nft_address, owner = excluded.owner, chain_on_sale = excluded.chain_on_sale,
			sale_contract = excluded.sale_contract,
			chain_market = CASE WHEN excluded.chain_market IS NOT NULL THEN excluded.chain_market
				WHEN excluded.chain_on_sale = 1 AND u.sale_contract IS NOT DISTINCT FROM excluded.sale_contract THEN u.chain_market END,
			chain_price = CASE WHEN excluded.chain_market IS NOT NULL THEN excluded.chain_price
				WHEN excluded.chain_on_sale = 1 AND u.sale_contract IS NOT DISTINCT FROM excluded.sale_contract THEN u.chain_price END,
			chain_at = excluded.chain_at, updated_at = excluded.updated_at
		RETURNING new.name, CASE WHEN new.chain_on_sale = 1 AND old.sale_contract IS NOT DISTINCT FROM new.sale_contract THEN old.chain_price END`,
		ts, ts, ts, names, b.len, b.digit, b.under, b.junk, srcs, nfts, owners, onSale, contracts, markets, prices)
	if err != nil {
		return err
	}
	// the price the listing had before, if it is the same listing
	old := make(map[string]sql.NullFloat64, n)
	for res.Next() {
		var name string
		var p sql.NullFloat64
		if err := res.Scan(&name, &p); err != nil {
			res.Close()
			return err
		}
		old[name] = p
	}
	res.Close()
	if err := res.Err(); err != nil {
		return err
	}
	var evs []event
	for _, r := range rows {
		if p := old[r.Name]; r.Market != "" && r.Price != nil && (!p.Valid || p.Float64 != *r.Price) {
			evs = append(evs, event{r.Name, "market", fmt.Sprintf("listed on %s at %g TON", r.Market, *r.Price)})
		}
	}
	if err := addEvents(ctx, tx, ts, evs); err != nil {
		return err
	}
	return tx.Commit()
}

type ChainSale struct {
	NFTAddress string
	Market     string
	Price      *float64
}

func (s *Store) SetChainSales(ctx context.Context, sales []ChainSale) error {
	sales = lastBy(sales, func(c ChainSale) string { return c.NFTAddress })
	if len(sales) == 0 {
		return nil
	}
	nfts, markets := make([]string, len(sales)), make([]string, len(sales))
	prices := make([]*float64, len(sales))
	for i, c := range sales {
		nfts[i], markets[i], prices[i] = c.NFTAddress, c.Market, c.Price
	}
	ts := now()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.QueryContext(ctx, `UPDATE usernames AS u SET chain_market = NULLIF(r.market, ''), chain_price = r.price, updated_at = ?
		FROM unnest(?::text[], ?::text[], ?::float8[]) AS r(nft, market, price)
		WHERE u.nft_address = r.nft
		RETURNING u.name, r.market, r.price, old.chain_price`, ts, nfts, markets, prices)
	if err != nil {
		return err
	}
	var evs []event
	for res.Next() {
		var name, market string
		var price, oldPrice sql.NullFloat64
		if err := res.Scan(&name, &market, &price, &oldPrice); err != nil {
			res.Close()
			return err
		}
		if price.Valid && (!oldPrice.Valid || oldPrice.Float64 != price.Float64) {
			evs = append(evs, event{name, "market", fmt.Sprintf("listed on %s at %g TON", market, price.Float64)})
		}
	}
	res.Close()
	if err := res.Err(); err != nil {
		return err
	}
	if err := addEvents(ctx, tx, ts, evs); err != nil {
		return err
	}
	return tx.Commit()
}

// ChainSalesMissingPrice lists NFT addresses that are on sale on-chain but not priced yet.
func (s *Store) ChainSalesMissingPrice(ctx context.Context, limit int) ([]string, error) {
	return s.strings(ctx, `SELECT nft_address FROM usernames WHERE chain_on_sale=1 AND chain_market IS NULL AND nft_address IS NOT NULL LIMIT ?`, limit)
}

func (s *Store) strings(ctx context.Context, q string, args ...any) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// --- probe / refresh targets ---

// NamesForProbe: good-looking names that Fragment hasn't checked recently.
func (s *Store) NamesForProbe(ctx context.Context, minScore int, olderThan time.Duration, limit int) ([]string, error) {
	return s.strings(ctx, `SELECT name FROM usernames WHERE score >= ? AND (frag_at IS NULL OR frag_at < ?)
		ORDER BY score DESC NULLS LAST, len, name LIMIT ?`, minScore, time.Now().Add(-olderThan).Unix(), limit)
}

// AuctionsNeedingDetail: live auctions whose page (and so buy-now price) wasn't read recently,
// soonest-ending first, since those are the ones you can still act on.
func (s *Store) AuctionsNeedingDetail(ctx context.Context, olderThan time.Duration, limit int) ([]string, error) {
	return s.strings(ctx, `SELECT name FROM usernames WHERE frag_status = 'auction' AND frag_ends_at > ?
		AND (detail_at IS NULL OR detail_at < ?) ORDER BY frag_ends_at LIMIT ?`, now(), time.Now().Add(-olderThan).Unix(), limit)
}

// StaleListings: names we think are listed on Fragment but weren't seen in the last sweep.
func (s *Store) StaleListings(ctx context.Context, before int64) ([]string, error) {
	return s.strings(ctx, `SELECT name FROM usernames WHERE frag_status IN ('auction','sale') AND (frag_at IS NULL OR frag_at < ?)`, before)
}

// EndedAuctionsAndWatched: auctions whose end time passed, plus names anyone has starred.
func (s *Store) EndedAuctionsAndWatched(ctx context.Context) ([]string, error) {
	return s.strings(ctx, `SELECT name FROM usernames WHERE frag_status = 'auction' AND frag_ends_at < ?
		UNION SELECT DISTINCT un_name FROM user_names WHERE un_watched = 1`, now())
}

// EnsureName inserts a bare username (e.g. typed by the user) so it can be refreshed and tracked.
func (s *Store) EnsureName(ctx context.Context, name string) error {
	ts := now()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO usernames(name, len, has_digit, has_under, junk, first_seen, updated_at)
		VALUES(?,?,?,?,?,?,?) ON CONFLICT (name) DO NOTHING`,
		name, len(name), b2i(strings.ContainsAny(name, "0123456789")), b2i(strings.Contains(name, "_")), b2i(IsJunk(name)), ts, ts)
	return err
}

func (s *Store) GetKV(ctx context.Context, k string) string {
	var v string
	s.DB.QueryRowContext(ctx, `SELECT v FROM kv WHERE k=?`, k).Scan(&v)
	return v
}

func (s *Store) SetKV(ctx context.Context, k, v string) {
	s.DB.ExecContext(ctx, `INSERT INTO kv(k,v) VALUES(?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, k, v)
}
