// Command import-sqlite copies a BidSpot SQLite database (data/fragwatch.db, from before the
// move to Postgres) into the Postgres database the server uses. It runs once, into an empty
// database, in one transaction: either everything arrives or nothing does.
//
//	DATABASE_URL=postgres://… import-sqlite -from data/fragwatch.db
//
// Stop the old server first so SQLite folds its WAL into the file.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	_ "modernc.org/sqlite"

	"fragwatch/internal/config"
	"fragwatch/internal/store"
)

// tables in an order that satisfies the foreign keys.
var tables = []string{
	"users", "sessions", "auth_tokens", "user_kv", "user_names", "ledger", "payments",
	"usernames", "events", "kv", "concepts", "concept_names", "portfolio", "trackers",
}

type column struct{ name, typ string }

func main() {
	from := flag.String("from", "data/fragwatch.db", "the SQLite database to import")
	flag.Parse()
	cfg := config.Load()
	ctx := context.Background()

	// creates the schema if this is a fresh database
	st, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	st.Close()

	src, err := sql.Open("sqlite", "file:"+*from+"?mode=ro&_pragma=busy_timeout(15000)")
	if err != nil {
		log.Fatal(err)
	}
	defer src.Close()
	if err := src.PingContext(ctx); err != nil {
		log.Fatalf("open %s: %v (stop the old server and try again)", *from, err)
	}

	pg, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pg.Close(ctx)
	for _, t := range tables {
		var n int64
		if err := pg.QueryRow(ctx, `SELECT COUNT(*) FROM `+t).Scan(&n); err != nil {
			log.Fatal(err)
		}
		if n > 0 {
			log.Fatalf("%s already has %d rows: import only runs into an empty database", t, n)
		}
	}

	tx, err := pg.Begin(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback(ctx)
	start := time.Now()
	for _, t := range tables {
		n, err := copyTable(ctx, src, tx, t)
		if err != nil {
			log.Fatalf("%s: %v", t, err)
		}
		fmt.Printf("%-14s %8d rows\n", t, n)
	}
	// identity columns continue after the imported ids
	for _, t := range []string{"users", "ledger", "payments", "events", "concepts", "trackers"} {
		if _, err := tx.Exec(ctx, `SELECT setval(pg_get_serial_sequence('`+t+`', 'id'), COALESCE(MAX(id), 0) + 1, false) FROM `+t); err != nil {
			log.Fatalf("%s: %v", t, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		log.Fatal(err)
	}
	if _, err := pg.Exec(ctx, `ANALYZE`); err != nil {
		log.Printf("analyze: %v", err)
	}
	fmt.Printf("imported in %s\n", time.Since(start).Round(time.Millisecond))
}

// copyTable streams one table with COPY. Only columns both sides have are copied (generated
// columns are recomputed by Postgres); values are converted to the Postgres column's type.
func copyTable(ctx context.Context, src *sql.DB, tx pgx.Tx, table string) (int64, error) {
	have := map[string]bool{}
	rows, err := src.QueryContext(ctx, `SELECT name FROM pragma_table_xinfo(?) WHERE hidden = 0`, table)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var c string
		rows.Scan(&c)
		have[c] = true
	}
	rows.Close()
	if len(have) == 0 {
		return 0, nil // the old database never had this table
	}

	prow, err := tx.Query(ctx, `SELECT column_name, data_type FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1 AND is_generated = 'NEVER' ORDER BY ordinal_position`, table)
	if err != nil {
		return 0, err
	}
	var cols []column
	for prow.Next() {
		var c column
		if err := prow.Scan(&c.name, &c.typ); err != nil {
			return 0, err
		}
		if have[c.name] {
			cols = append(cols, c)
		}
	}
	prow.Close()
	if err := prow.Err(); err != nil {
		return 0, err
	}

	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = `"` + c.name + `"`
	}
	data, err := src.QueryContext(ctx, `SELECT `+strings.Join(names, ", ")+` FROM `+table+` ORDER BY rowid`)
	if err != nil {
		return 0, err
	}
	defer data.Close()
	ident := make([]string, len(cols))
	for i, c := range cols {
		ident[i] = c.name
	}
	return tx.CopyFrom(ctx, pgx.Identifier{table}, ident, &source{rows: data, cols: cols})
}

// source feeds sql.Rows to COPY, one converted row at a time.
type source struct {
	rows *sql.Rows
	cols []column
}

func (s *source) Next() bool { return s.rows.Next() }

func (s *source) Values() ([]any, error) {
	raw := make([]any, len(s.cols))
	ptrs := make([]any, len(s.cols))
	for i := range raw {
		ptrs[i] = &raw[i]
	}
	if err := s.rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	for i, c := range s.cols {
		v, err := convert(raw[i], c.typ)
		if err != nil {
			return nil, fmt.Errorf("column %s: %w", c.name, err)
		}
		raw[i] = v
	}
	return raw, nil
}

func (s *source) Err() error { return s.rows.Err() }

// convert maps what SQLite stored (it does not enforce column types) onto the Postgres type.
func convert(v any, typ string) (any, error) {
	if b, ok := v.([]byte); ok {
		v = string(b)
	}
	if v == nil {
		return nil, nil
	}
	switch typ {
	case "bigint", "integer", "smallint":
		switch x := v.(type) {
		case int64:
			return x, nil
		case float64:
			return int64(x), nil
		case string:
			if x == "" {
				return nil, nil
			}
			var n int64
			_, err := fmt.Sscan(x, &n)
			return n, err
		}
	case "double precision", "real", "numeric":
		switch x := v.(type) {
		case float64:
			return x, nil
		case int64:
			return float64(x), nil
		case string:
			if x == "" {
				return nil, nil
			}
			var f float64
			_, err := fmt.Sscan(x, &f)
			return f, err
		}
	case "text":
		switch x := v.(type) {
		case string:
			return strings.ToValidUTF8(strings.ReplaceAll(x, "\x00", ""), "�"), nil
		default:
			return fmt.Sprint(x), nil
		}
	}
	return v, nil
}
