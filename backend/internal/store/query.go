package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type Username struct {
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	FragStatus   string   `json:"frag_status"`
	Price        *float64 `json:"price"`
	BuyNow       *float64 `json:"buy_now"`
	Buyout       *float64 `json:"buyout"`
	USD          *float64 `json:"usd"`
	EndsAt       *int64   `json:"ends_at"`
	LastSale     *float64 `json:"last_sale"`
	Owner        *string  `json:"owner"`
	NFTAddress   *string  `json:"nft_address"`
	NFTFriendly  string   `json:"nft_friendly,omitempty"`
	ChainMarket  *string  `json:"chain_market"`
	SaleContract *string  `json:"sale_contract"`
	Len          int      `json:"len"`
	Junk         bool     `json:"junk"`
	Score        *int     `json:"score"`
	Category     *string  `json:"category"`
	Meaning      *string  `json:"meaning"`
	Watched      bool     `json:"watched"`
	Hidden       bool     `json:"hidden"`
	Note         *string  `json:"note"`
	Src          int      `json:"src"`
	FragAt       *int64   `json:"frag_at"`
	ChainAt      *int64   `json:"chain_at"`
	FirstSeen    int64    `json:"first_seen"`
}

const cols = `name, status, frag_status, price, frag_buy_now, buyout, frag_usd,
  CASE WHEN frag_status='auction' THEN frag_ends_at END,
  CASE WHEN frag_status='sold' THEN frag_price END,
  COALESCE(owner, frag_owner), nft_address, chain_market, sale_contract, len, junk, score, category, meaning,
  COALESCE(un_watched, 0), COALESCE(un_hidden, 0), un_note, src, frag_at, chain_at, first_seen`

// from joins one person's verdicts (star, hide, note) onto the shared names. Its single argument
// is the user id; 0 joins nothing, so every name reads as unstarred and visible.
const from = ` FROM usernames LEFT JOIN user_names ON user_id = ? AND un_name = name`

func scanUsername(sc interface{ Scan(...any) error }) (Username, error) {
	var u Username
	var junk, watched, hidden int
	err := sc.Scan(&u.Name, &u.Status, &u.FragStatus, &u.Price, &u.BuyNow, &u.Buyout, &u.USD, &u.EndsAt, &u.LastSale,
		&u.Owner, &u.NFTAddress, &u.ChainMarket, &u.SaleContract, &u.Len, &junk, &u.Score, &u.Category, &u.Meaning,
		&watched, &hidden, &u.Note, &u.Src, &u.FragAt, &u.ChainAt, &u.FirstSeen)
	u.Junk, u.Watched, u.Hidden = junk == 1, watched == 1, hidden == 1
	if u.NFTAddress != nil {
		u.NFTFriendly = FriendlyAddress(*u.NFTAddress)
	}
	return u, err
}

type Filter struct {
	UserID        int64    // whose stars and hidden names apply
	Q             string   // substring; "^abc" prefix, "abc$" suffix, "=abc" exact
	Statuses      []string // auction, sale, market, owned, available
	Categories    []string
	NotCategories []string // kinds to leave out; unrated names stay
	Keywords      []string // any of these in the name (underscores ignored) or its meaning
	Digits        string   // e.g. "552"; see DigitMode
	DigitMode     string   // "row" (552 together), "any" (any order, together: 552/525/255), "scattered" (anywhere in the name)
	MinPrice      *float64
	MaxPrice      *float64
	MinBuyout     *float64
	MaxBuyout     *float64
	BuyNow        bool  // only names that can be bought instantly
	ChangedSince  int64 // only names first seen, listed or repriced after this unix time
	MinLen        int
	MaxLen        int
	MinScore      *int
	Unscored      bool // include names without an LLM score even when MinScore is set
	HideJunk      bool
	NoDigits      bool
	NoUnder       bool
	EndingIn      int64 // seconds; only auctions ending within this window
	Live          bool  // hide auctions whose end time has passed (Fragment settles about one per second)
	Watched       bool
	Hidden        bool // only names the user archived; otherwise archived names are always left out
	Sort          string // a column from sortCols (legacy "price_asc"/"price_desc" still work)
	Dir           string // "asc" | "desc"; empty = the column's natural direction
	Limit         int
	Offset        int
}

// sortCols are the sortable columns: SQL expression and natural direction. Missing values
// (no price, not an auction) always sort last, whichever direction.
var sortCols = map[string]struct{ expr, dir string }{
	"ending":  {"CASE WHEN frag_status = 'auction' THEN frag_ends_at END", "asc"},
	"score":   {"score", "desc"},
	"price":   {"price", "asc"},
	"buyout":  {"buyout", "asc"},
	"len":     {"len", "asc"},
	"name":    {"name", "asc"},
	"status":  {"status", "asc"},
	"recent":  {"first_seen", "desc"},
	"updated": {"updated_at", "desc"},
	"value":   {"", "asc"}, // built by valueExpr
	"random":  {"random()", "asc"},
}

// valueExpr is a name's price as a share of the typical price for its AI score: 0.2 means it costs a
// fifth of what names rated the same usually go for. The price is the buy-now, else the current one.
func valueExpr(typical map[int]float64) string {
	if len(typical) == 0 {
		return "NULL"
	}
	var b strings.Builder
	b.WriteString("(COALESCE(buyout, price) / CASE score")
	for sc := 0; sc <= 10; sc++ {
		if v := typical[sc]; v > 0 {
			fmt.Fprintf(&b, " WHEN %d THEN %g", sc, v)
		}
	}
	b.WriteString(" END)")
	return b.String()
}

// orderBy builds ORDER BY from a sort spec. sort is one column ("score") or several in priority
// order with directions ("score:desc,buyout:asc"); dir applies to a single bare column.
func orderBy(sort, dir string, typical map[int]float64) string {
	var parts []string
	seen := map[string]bool{}
	for _, key := range strings.Split(sort, ",") {
		col, d, _ := strings.Cut(strings.TrimSpace(key), ":")
		switch col { // names used before columns had a direction
		case "price_asc":
			col, d = "price", "asc"
		case "price_desc":
			col, d = "price", "desc"
		}
		c, ok := sortCols[col]
		if !ok || seen[col] {
			continue
		}
		seen[col] = true
		if d == "" && !strings.Contains(sort, ",") {
			d = dir
		}
		if d != "asc" && d != "desc" {
			d = c.dir
		}
		if col == "value" {
			c.expr = valueExpr(typical)
		}
		parts = append(parts, c.expr+" IS NULL, "+c.expr+" "+strings.ToUpper(d))
	}
	if len(parts) == 0 {
		c := sortCols["score"]
		parts = append(parts, c.expr+" IS NULL, "+c.expr+" DESC")
	}
	return strings.Join(parts, ", ") + ", len ASC, name ASC"
}

func (s *Store) Query(ctx context.Context, f Filter) ([]Username, int, error) {
	var where []string
	var args []any
	add := func(cond string, a ...any) { where = append(where, cond); args = append(args, a...) }

	if q := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(f.Q, "@"))); q != "" {
		switch {
		case strings.HasPrefix(q, "="):
			add("name = ?", q[1:])
		case strings.HasPrefix(q, "^"):
			add("name LIKE ? ESCAPE '\\'", likeEsc(q[1:])+"%")
		case strings.HasSuffix(q, "$"):
			add("name LIKE ? ESCAPE '\\'", "%"+likeEsc(q[:len(q)-1]))
		default:
			// underscores don't matter: "dark_moon" matches "darkmoon" and the other way round
			bare := strings.ReplaceAll(q, "_", "")
			add("(name LIKE ? ESCAPE '\\' OR replace(name, '_', '') LIKE ? ESCAPE '\\' OR meaning LIKE ?)",
				"%"+likeEsc(q)+"%", "%"+likeEsc(bare)+"%", "%"+q+"%")
		}
	}
	if len(f.Keywords) > 0 {
		var ors []string
		for _, k := range f.Keywords {
			k = strings.ToLower(strings.TrimSpace(k))
			if k == "" {
				continue
			}
			ors = append(ors, "replace(name, '_', '') LIKE ? ESCAPE '\\' OR meaning LIKE ?")
			args = append(args, "%"+likeEsc(strings.ReplaceAll(k, "_", ""))+"%", "%"+k+"%")
		}
		if len(ors) > 0 {
			where = append(where, "("+strings.Join(ors, " OR ")+")")
		}
	}
	if cond, a := digitCond(f.Digits, f.DigitMode); cond != "" {
		add(cond, a...)
	}
	if len(f.Statuses) > 0 {
		add("status IN ("+placeholders(len(f.Statuses))+")", toAny(f.Statuses)...)
	}
	if len(f.Categories) > 0 {
		add("category IN ("+placeholders(len(f.Categories))+")", toAny(f.Categories)...)
	}
	if len(f.NotCategories) > 0 {
		add("(category IS NULL OR category NOT IN ("+placeholders(len(f.NotCategories))+"))", toAny(f.NotCategories)...)
	}
	if f.MinPrice != nil {
		add("price >= ?", *f.MinPrice)
	}
	if f.MaxPrice != nil {
		add("price <= ?", *f.MaxPrice)
	}
	if f.MinBuyout != nil {
		add("buyout >= ?", *f.MinBuyout)
	}
	if f.MaxBuyout != nil {
		add("buyout <= ?", *f.MaxBuyout)
	}
	if f.BuyNow {
		add("buyout IS NOT NULL")
	}
	if f.ChangedSince > 0 {
		add("(first_seen > ? OR EXISTS (SELECT 1 FROM events e WHERE e.name = usernames.name AND e.ts > ? AND e.kind IN ('listed','status','market','price')))",
			f.ChangedSince, f.ChangedSince)
	}
	if f.MinLen > 0 {
		add("len >= ?", f.MinLen)
	}
	if f.MaxLen > 0 {
		add("len <= ?", f.MaxLen)
	}
	if f.MinScore != nil {
		if f.Unscored {
			add("(score >= ? OR (score IS NULL AND junk = 0))", *f.MinScore)
		} else {
			add("score >= ?", *f.MinScore)
		}
	}
	if f.HideJunk {
		add("junk = 0")
	}
	if f.NoDigits {
		add("has_digit = 0")
	}
	if f.NoUnder {
		add("has_under = 0")
	}
	if f.EndingIn > 0 {
		add("frag_status = 'auction' AND frag_ends_at BETWEEN ? AND ?", now(), now()+f.EndingIn)
	}
	if f.Live {
		add("(frag_status != 'auction' OR frag_ends_at IS NULL OR frag_ends_at >= ?)", now())
	}
	if f.Watched {
		add("un_watched = 1")
	}
	if f.Hidden {
		add("un_hidden = 1")
	} else {
		add("COALESCE(un_hidden, 0) = 0")
	}
	w := ""
	if len(where) > 0 {
		w = " WHERE " + strings.Join(where, " AND ")
	}
	var typical map[int]float64
	if strings.Contains(f.Sort, "value") {
		typical = s.Typical(ctx)
	}
	order := orderBy(f.Sort, f.Dir, typical)
	if f.Limit <= 0 || f.Limit > 2000 {
		f.Limit = 100
	}

	args = append([]any{f.UserID}, args...)
	var total int
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*)"+from+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT "+cols+from+w+" ORDER BY "+order+" LIMIT ? OFFSET ?",
		append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Username{}
	for rows.Next() {
		u, err := scanUsername(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

// Get reads one name without anyone's verdicts; GetFor adds one person's.
func (s *Store) Get(ctx context.Context, name string) (*Username, error) { return s.GetFor(ctx, 0, name) }

func (s *Store) GetFor(ctx context.Context, userID int64, name string) (*Username, error) {
	u, err := scanUsername(s.DB.QueryRowContext(ctx, "SELECT "+cols+from+" WHERE name=?", userID, name))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

type Event struct {
	Name   string `json:"name"`
	TS     int64  `json:"ts"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

// Events lists one name's history, or (name "") the feed of names worth looking at for userID:
// well rated, or starred by them.
func (s *Store) Events(ctx context.Context, userID int64, name string, limit int) ([]Event, error) {
	q := `SELECT name, ts, kind, detail FROM events`
	var args []any
	if name != "" {
		q += ` WHERE name=?`
		args = append(args, name)
	} else {
		q += ` WHERE name IN (SELECT name FROM usernames WHERE score>=6) OR name IN (SELECT un_name FROM user_names WHERE user_id=? AND un_watched=1)`
		args = append(args, userID)
	}
	q += ` ORDER BY ts DESC, id DESC LIMIT ?`
	rows, err := s.DB.QueryContext(ctx, q, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Name, &e.TS, &e.Kind, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type Stats struct {
	Total    int            `json:"total"`
	ByStatus map[string]int `json:"by_status"`
	Scored   int            `json:"scored"`
	Queue    int            `json:"llm_queue"`
	Junk     int            `json:"junk"`
	Good     int            `json:"good"`
	Watched  int            `json:"watched"`
	Hidden   int            `json:"hidden"`
	Category map[string]int `json:"by_category"`
}

func (s *Store) Stats(ctx context.Context) (Stats, error) {
	st := Stats{ByStatus: map[string]int{}, Category: map[string]int{}}
	// Watched/Hidden are one person's counts: the caller fills them in (UserNameCounts)
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(score), SUM(score IS NULL AND junk=0), SUM(junk), SUM(score>=7) FROM usernames`).
		Scan(&st.Total, &st.Scored, nz(&st.Queue), nz(&st.Junk), nz(&st.Good))
	if err != nil {
		return st, err
	}
	for _, q := range []struct {
		sql string
		m   map[string]int
	}{
		{`SELECT status, COUNT(*) FROM usernames GROUP BY status`, st.ByStatus},
		{`SELECT category, COUNT(*) FROM usernames WHERE category IS NOT NULL GROUP BY category`, st.Category},
	} {
		rows, err := s.DB.QueryContext(ctx, q.sql)
		if err != nil {
			return st, err
		}
		for rows.Next() {
			var k string
			var n int
			rows.Scan(&k, &n)
			q.m[k] = n
		}
		rows.Close()
	}
	return st, nil
}

type nullInt struct{ p *int }

func (n nullInt) Scan(v any) error {
	if x, ok := v.(int64); ok {
		*n.p = int(x)
	}
	return nil
}

func nz(p *int) nullInt { return nullInt{p} }

func likeEsc(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func placeholders(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// Count returns how many names match f (Limit/Offset/Sort ignored).
func (s *Store) Count(ctx context.Context, f Filter) (int, error) {
	f.Limit = 1
	_, total, err := s.Query(ctx, f)
	return total, err
}
