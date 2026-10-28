// Package store persists usernames in SQLite. Each data source writes its own
// columns; the dashboard-facing `status` and `price` are generated from them.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Source bits recorded in usernames.src.
const (
	SrcFragmentList   = 1
	SrcFragmentSearch = 2
	SrcChain          = 4
	SrcProbe          = 8
)

const schema = `
CREATE TABLE IF NOT EXISTS usernames (
  name          TEXT PRIMARY KEY,
  len           INTEGER NOT NULL,
  has_digit     INTEGER NOT NULL DEFAULT 0,
  has_under     INTEGER NOT NULL DEFAULT 0,
  junk          INTEGER NOT NULL DEFAULT 0,
  src           INTEGER NOT NULL DEFAULT 0,

  frag_status   TEXT NOT NULL DEFAULT '',
  frag_price    REAL,
  frag_buy_now  REAL,
  frag_usd      REAL,
  frag_ends_at  INTEGER,
  frag_owner    TEXT,
  frag_at       INTEGER,

  nft_address   TEXT,
  owner         TEXT,
  chain_on_sale INTEGER NOT NULL DEFAULT 0,
  sale_contract TEXT,
  chain_market  TEXT,
  chain_price   REAL,
  chain_at      INTEGER,

  score         INTEGER,
  category      TEXT,
  meaning       TEXT,
  llm_at        INTEGER,

  watched       INTEGER NOT NULL DEFAULT 0,
  note          TEXT,
  first_seen    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL,

  status TEXT GENERATED ALWAYS AS (CASE
    WHEN frag_status = 'auction' THEN 'auction'
    WHEN frag_status = 'sale' THEN 'sale'
    WHEN chain_on_sale = 1 THEN 'market'
    WHEN frag_status = 'available' THEN 'available'
    ELSE 'owned' END) VIRTUAL,
  price REAL GENERATED ALWAYS AS (CASE
    WHEN frag_status IN ('auction','sale') THEN frag_price
    WHEN chain_on_sale = 1 THEN chain_price
    ELSE NULL END) VIRTUAL
);
CREATE INDEX IF NOT EXISTS ix_status_price ON usernames(status, price);
CREATE INDEX IF NOT EXISTS ix_status_ends ON usernames(status, frag_ends_at);
CREATE INDEX IF NOT EXISTS ix_score ON usernames(score);
CREATE INDEX IF NOT EXISTS ix_len ON usernames(len);
CREATE INDEX IF NOT EXISTS ix_watched ON usernames(watched) WHERE watched = 1;
CREATE INDEX IF NOT EXISTS ix_llm_queue ON usernames(junk, score);
CREATE INDEX IF NOT EXISTS ix_nft ON usernames(nft_address);

CREATE TABLE IF NOT EXISTS events (
  id     INTEGER PRIMARY KEY AUTOINCREMENT,
  name   TEXT NOT NULL,
  ts     INTEGER NOT NULL,
  kind   TEXT NOT NULL,
  detail TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_events_name ON events(name, ts);
CREATE INDEX IF NOT EXISTS ix_events_ts ON events(ts);

CREATE TABLE IF NOT EXISTS kv (k TEXT PRIMARY KEY, v TEXT NOT NULL);

CREATE TABLE IF NOT EXISTS concepts (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL DEFAULT 0,
  ts         INTEGER NOT NULL,
  source     TEXT NOT NULL,              -- set | rotate | refine | imagine
  project    TEXT NOT NULL,
  pitch      TEXT NOT NULL DEFAULT '',
  first_move TEXT NOT NULL DEFAULT '',
  form       TEXT NOT NULL DEFAULT '',   -- mini app | web app | bot | channel | both …
  how        TEXT NOT NULL DEFAULT '',   -- how the name is used in the product
  note       TEXT NOT NULL DEFAULT ''    -- the user's own steer, when there was one
);
CREATE TABLE IF NOT EXISTS concept_names (
  concept_id INTEGER NOT NULL REFERENCES concepts(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  angle      TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (concept_id, name)
);
CREATE INDEX IF NOT EXISTS ix_concept_names_name ON concept_names(name);

CREATE TABLE IF NOT EXISTS portfolio (
  user_id   INTEGER NOT NULL DEFAULT 0,
  name      TEXT NOT NULL,
  bought    REAL,            -- what they paid, TON (NULL = unknown)
  bought_at INTEGER,
  concept   TEXT NOT NULL DEFAULT '',  -- the project they attached to it
  note      TEXT NOT NULL DEFAULT '',
  added_at  INTEGER NOT NULL,
  PRIMARY KEY (user_id, name)
);

CREATE TABLE IF NOT EXISTS trackers (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL DEFAULT 0,
  name       TEXT NOT NULL,
  view       TEXT NOT NULL,
  filters    TEXT NOT NULL, -- the dashboard's filter state, opaque to the backend
  params     TEXT NOT NULL, -- the same search as an /api/usernames query string
  seen_at    INTEGER NOT NULL,
  created_at INTEGER NOT NULL
);
`

// migrations run after schema; each must be safe to run on every start.
var migrations = []struct {
	column string
	sql    []string
	table  string // default "usernames"
}{
	{"liked", []string{
		// The user's verdict on a generated concept: 1 liked, -1 disliked, 0 none.
		`ALTER TABLE concepts ADD COLUMN liked INTEGER NOT NULL DEFAULT 0`,
	}, "concepts"},
	{"llm_tries", []string{
		// How many times the model was asked about a name and skipped it. Rated names are never
		// asked again; skipped ones get one more chance, then they stay unrated instead of costing tokens forever.
		`ALTER TABLE usernames ADD COLUMN llm_tries INTEGER NOT NULL DEFAULT 0`,
	}, ""},
	{"detail_at", []string{
		// When the name's own Fragment page was last read (the only source of buy-now prices).
		`ALTER TABLE usernames ADD COLUMN detail_at INTEGER`,
	}, ""},
	{"hidden", []string{
		// Names the user never wants to see again: left out of every list unless asked for.
		`ALTER TABLE usernames ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0`,
		`CREATE INDEX IF NOT EXISTS ix_hidden ON usernames(hidden) WHERE hidden = 1`,
	}, ""},
	{"buyout", []string{
		// What it costs to own the name right now: buy-now on an auction, the listing price otherwise.
		`ALTER TABLE usernames ADD COLUMN buyout REAL GENERATED ALWAYS AS (CASE
		   WHEN frag_status = 'auction' THEN frag_buy_now
		   WHEN frag_status = 'sale' THEN frag_price
		   WHEN chain_on_sale = 1 THEN chain_price
		   ELSE NULL END) VIRTUAL`,
		`CREATE INDEX IF NOT EXISTS ix_buyout ON usernames(buyout)`,
	}, ""},
	// Accounts: rows made before there were users get user_id 0 and go to the first account
	// (ClaimLegacy). usernames.watched/hidden/note stay only as that hand-over's source.
	{"user_id", []string{`ALTER TABLE trackers ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0`}, "trackers"},
	{"user_id", []string{`ALTER TABLE concepts ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0`}, "concepts"},
	// What a concept runs on (data, its source, the Telegram surface): proof it can be built.
	{"built_with", []string{`ALTER TABLE concepts ADD COLUMN built_with TEXT NOT NULL DEFAULT ''`}, "concepts"},
	{"user_id", []string{
		// the key becomes (user_id, name): two people can own notes on the same name
		`CREATE TABLE portfolio_new (user_id INTEGER NOT NULL DEFAULT 0, name TEXT NOT NULL, bought REAL, bought_at INTEGER,
		   concept TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', added_at INTEGER NOT NULL, PRIMARY KEY (user_id, name))`,
		`INSERT INTO portfolio_new(user_id, name, bought, bought_at, concept, note, added_at) SELECT 0, name, bought, bought_at, concept, note, added_at FROM portfolio`,
		`DROP TABLE portfolio`,
		`ALTER TABLE portfolio_new RENAME TO portfolio`,
	}, "portfolio"},
}

func migrate(db *sql.DB) error {
	for _, m := range migrations {
		var n int
		table := m.table
		if table == "" {
			table = "usernames"
		}
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_xinfo(?) WHERE name = ?`, table, m.column).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		for _, q := range m.sql {
			if _, err := db.Exec(q); err != nil {
				return fmt.Errorf("migrate %s: %w", m.column, err)
			}
		}
	}
	return nil
}

type Store struct {
	DB *sql.DB

	typMu sync.Mutex
	typ   map[int]float64 // typical price per AI score, for the "value" sort
	typAt time.Time
}

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		os.MkdirAll(dir, 0o755)
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(15000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(on)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	if err := migrate(db); err != nil {
		return nil, err
	}
	for _, q := range []string{usersSchema, postMigrate} {
		if _, err := db.Exec(q); err != nil {
			return nil, fmt.Errorf("schema: %w", err)
		}
	}
	return &Store{DB: db}, nil
}

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

func nullf(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func nulli(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func nulls(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func insertBase(ctx context.Context, tx *sql.Tx, name string, src int, ts int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO usernames(name,len,has_digit,has_under,junk,src,first_seen,updated_at)
		VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(name) DO UPDATE SET src = src | excluded.src`,
		name, len(name), b2i(strings.ContainsAny(name, "0123456789")), b2i(strings.Contains(name, "_")),
		b2i(IsJunk(name)), src, ts, ts)
	return err
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

// UpsertFragment writes Fragment data and records status/price changes as events.
func (s *Store) UpsertFragment(ctx context.Context, rows []FragRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ts := now()
	for _, r := range rows {
		var oldStatus string
		var oldPrice sql.NullFloat64
		err := tx.QueryRowContext(ctx, `SELECT frag_status, frag_price FROM usernames WHERE name=?`, r.Name).Scan(&oldStatus, &oldPrice)
		existed := err == nil
		if err := insertBase(ctx, tx, r.Name, r.FromSrc, ts); err != nil {
			return err
		}
		// Only a name's own page shows the buy-now price; list rows don't carry it. So list rows keep
		// the known buy-now while the same auction runs, and only a page read sets it (or clears it).
		detail := b2i(r.FromSrc == SrcProbe)
		_, err = tx.ExecContext(ctx, `UPDATE usernames SET frag_status=?, frag_price=?,
			frag_buy_now = CASE WHEN ? = 1 THEN ? WHEN frag_status = 'auction' AND ? = 'auction' THEN frag_buy_now ELSE NULL END,
			detail_at = CASE WHEN ? = 1 THEN ? ELSE detail_at END,
			frag_usd=?, frag_ends_at=?, frag_owner=COALESCE(?, frag_owner), frag_at=?, updated_at=? WHERE name=?`,
			r.Status, nullf(r.Price), detail, nullf(r.BuyNow), r.Status, detail, ts,
			nullf(r.USD), nulli(r.EndsAt), nulls(r.Owner), ts, ts, r.Name)
		if err != nil {
			return err
		}
		switch {
		case !existed || oldStatus == "":
			if r.Status == "auction" || r.Status == "sale" {
				addEvent(ctx, tx, r.Name, ts, "listed", fmt.Sprintf("%s at %s TON", r.Status, fmtPrice(r.Price)))
			}
		case oldStatus != r.Status:
			addEvent(ctx, tx, r.Name, ts, "status", fmt.Sprintf("%s → %s (%s TON)", oldStatus, r.Status, fmtPrice(r.Price)))
		case r.Price != nil && oldPrice.Valid && oldPrice.Float64 != *r.Price:
			addEvent(ctx, tx, r.Name, ts, "price", fmt.Sprintf("%g → %g TON", oldPrice.Float64, *r.Price))
		}
	}
	return tx.Commit()
}

func addEvent(ctx context.Context, tx *sql.Tx, name string, ts int64, kind, detail string) {
	tx.ExecContext(ctx, `INSERT INTO events(name,ts,kind,detail) VALUES(?,?,?,?)`, name, ts, kind, detail)
}

// UpsertChain writes on-chain ownership. Market/price are filled later by SetChainSale.
func (s *Store) UpsertChain(ctx context.Context, rows []ChainRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ts := now()
	for _, r := range rows {
		if err := insertBase(ctx, tx, r.Name, SrcChain, ts); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE usernames SET nft_address=?, owner=?, chain_on_sale=?, sale_contract=?,
			chain_market = CASE WHEN ? = 1 AND sale_contract IS ? THEN chain_market ELSE NULL END,
			chain_price  = CASE WHEN ? = 1 AND sale_contract IS ? THEN chain_price ELSE NULL END,
			chain_at=?, updated_at=? WHERE name=?`,
			r.NFTAddress, nulls(r.Owner), b2i(r.OnSale), nulls(r.SaleContract),
			b2i(r.OnSale), nulls(r.SaleContract), b2i(r.OnSale), nulls(r.SaleContract), ts, ts, r.Name)
		if err != nil {
			return err
		}
		if r.Market != "" {
			var oldPrice sql.NullFloat64
			tx.QueryRowContext(ctx, `SELECT chain_price FROM usernames WHERE name=?`, r.Name).Scan(&oldPrice)
			if _, err := tx.ExecContext(ctx, `UPDATE usernames SET chain_market=?, chain_price=? WHERE name=?`, r.Market, nullf(r.Price), r.Name); err != nil {
				return err
			}
			if r.Price != nil && (!oldPrice.Valid || oldPrice.Float64 != *r.Price) {
				addEvent(ctx, tx, r.Name, ts, "market", fmt.Sprintf("listed on %s at %g TON", r.Market, *r.Price))
			}
		}
	}
	return tx.Commit()
}

type ChainSale struct {
	NFTAddress string
	Market     string
	Price      *float64
}

func (s *Store) SetChainSales(ctx context.Context, sales []ChainSale) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ts := now()
	for _, c := range sales {
		var name string
		var oldPrice sql.NullFloat64
		if tx.QueryRowContext(ctx, `SELECT name, chain_price FROM usernames WHERE nft_address=?`, c.NFTAddress).Scan(&name, &oldPrice) != nil {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE usernames SET chain_market=?, chain_price=?, updated_at=? WHERE name=?`,
			nulls(c.Market), nullf(c.Price), ts, name); err != nil {
			return err
		}
		if c.Price != nil && (!oldPrice.Valid || oldPrice.Float64 != *c.Price) {
			addEvent(ctx, tx, name, ts, "market", fmt.Sprintf("listed on %s at %g TON", c.Market, *c.Price))
		}
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

// --- LLM scoring ---

type Score struct {
	Name     string
	Score    int
	Category string
	Meaning  string
}

// UnscoredBatch returns names waiting for LLM scoring, listed (buyable) ones first, then shortest.
func (s *Store) UnscoredBatch(ctx context.Context, limit int, exclude map[string]bool) ([]string, error) {
	rows, err := s.strings(ctx, `SELECT name FROM usernames WHERE score IS NULL AND junk = 0 AND llm_tries < 2
		ORDER BY CASE status WHEN 'auction' THEN 0 WHEN 'sale' THEN 0 WHEN 'market' THEN 1 ELSE 2 END, len, name LIMIT ?`, limit+len(exclude))
	if err != nil {
		return nil, err
	}
	out := rows[:0]
	for _, n := range rows {
		if !exclude[n] && len(out) < limit {
			out = append(out, n)
		}
	}
	return out, nil
}

// MarkLLMSkipped counts one more attempt for names the model left out of its answer.
func (s *Store) MarkLLMSkipped(ctx context.Context, names []string) error {
	if len(names) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, n := range names {
		if _, err := tx.ExecContext(ctx, `UPDATE usernames SET llm_tries = llm_tries + 1 WHERE name = ?`, n); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ScoreMedian is the typical price of names with one AI score: what such names actually sold for,
// and what sellers ask for them right now.
type ScoreMedian struct {
	Score     int     `json:"score"`
	Sold      float64 `json:"sold"`
	SoldCount int     `json:"sold_count"`
	Ask       float64 `json:"ask"`
	AskCount  int     `json:"ask_count"`
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	m := v[len(v)/2]
	if len(v)%2 == 0 {
		m = (v[len(v)/2-1] + v[len(v)/2]) / 2
	}
	return m
}

// ScoreMedians returns, per AI score, the median sale price of sold names and the median buyout of
// names listed now. Asking prices run far above real sales, so deals are judged against sales first.
func (s *Store) ScoreMedians(ctx context.Context) ([]ScoreMedian, error) {
	sold := map[int][]float64{}
	ask := map[int][]float64{}
	for _, q := range []struct {
		sql string
		m   map[int][]float64
	}{
		{`SELECT score, frag_price FROM usernames WHERE frag_status = 'sold' AND frag_price > 0 AND score IS NOT NULL ORDER BY score, frag_price`, sold},
		{`SELECT score, buyout FROM usernames WHERE buyout > 0 AND score IS NOT NULL ORDER BY score, buyout`, ask},
	} {
		rows, err := s.DB.QueryContext(ctx, q.sql)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var sc int
			var v float64
			if err := rows.Scan(&sc, &v); err != nil {
				rows.Close()
				return nil, err
			}
			q.m[sc] = append(q.m[sc], v)
		}
		rows.Close()
	}
	out := []ScoreMedian{}
	for sc := 0; sc <= 10; sc++ {
		if len(sold[sc])+len(ask[sc]) == 0 {
			continue
		}
		out = append(out, ScoreMedian{Score: sc, Sold: median(sold[sc]), SoldCount: len(sold[sc]), Ask: median(ask[sc]), AskCount: len(ask[sc])})
	}
	return out, nil
}

// Typical is the price a name with each AI score usually goes for: the median sale if there are
// 30+ sales, else the median asking price if there are 10+ listings. Cached for 10 minutes.
func (s *Store) Typical(ctx context.Context) map[int]float64 {
	s.typMu.Lock()
	defer s.typMu.Unlock()
	if s.typ != nil && time.Since(s.typAt) < 10*time.Minute {
		return s.typ
	}
	ms, err := s.ScoreMedians(ctx)
	if err != nil {
		return s.typ
	}
	t := map[int]float64{}
	for _, m := range ms {
		if m.SoldCount >= 30 && m.Sold > 0 {
			t[m.Score] = m.Sold
		} else if m.AskCount >= 10 && m.Ask > 0 {
			t[m.Score] = m.Ask
		}
	}
	s.typ, s.typAt = t, time.Now()
	return t
}

func (s *Store) SaveScores(ctx context.Context, scores []Score) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ts := now()
	for _, sc := range scores {
		if _, err := tx.ExecContext(ctx, `UPDATE usernames SET score=?, category=?, meaning=?, llm_at=? WHERE name=?`,
			sc.Score, nulls(sc.Category), nulls(sc.Meaning), ts, sc.Name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ResetScores clears LLM results so everything is rescored (e.g. after editing the prompt).
func (s *Store) ResetScores(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE usernames SET score=NULL, category=NULL, meaning=NULL, llm_at=NULL, llm_tries=0`)
	return err
}

// --- probe / refresh targets ---

// NamesForProbe: good-looking names that Fragment hasn't checked recently.
func (s *Store) NamesForProbe(ctx context.Context, minScore int, olderThan time.Duration, limit int) ([]string, error) {
	return s.strings(ctx, `SELECT name FROM usernames WHERE score >= ? AND (frag_at IS NULL OR frag_at < ?)
		ORDER BY score DESC, len LIMIT ?`, minScore, time.Now().Add(-olderThan).Unix(), limit)
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
	return s.strings(ctx, `SELECT name FROM usernames WHERE (frag_status='auction' AND frag_ends_at < ?)
		OR name IN (SELECT un_name FROM user_names WHERE un_watched=1)`, now())
}

// EnsureName inserts a bare username (e.g. typed by the user) so it can be refreshed and tracked.
func (s *Store) EnsureName(ctx context.Context, name string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertBase(ctx, tx, name, 0, now()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetKV(ctx context.Context, k string) string {
	var v string
	s.DB.QueryRowContext(ctx, `SELECT v FROM kv WHERE k=?`, k).Scan(&v)
	return v
}

func (s *Store) SetKV(ctx context.Context, k, v string) {
	s.DB.ExecContext(ctx, `INSERT INTO kv(k,v) VALUES(?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, k, v)
}
