package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"
)

// openPG opens a Store in a fresh schema of the database at TEST_DATABASE_URL, dropped when the
// test ends. Without that variable the store tests are skipped:
//
//	docker run -d --rm --name pgtest -p 127.0.0.1:55432:5432 -e POSTGRES_PASSWORD=test postgres:18-alpine
//	TEST_DATABASE_URL=postgres://postgres:test@127.0.0.1:55432/postgres?sslmode=disable go test ./internal/store
func openPG(t *testing.T) (*Store, error) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	schema := fmt.Sprintf("t_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		admin.Close()
		return nil, err
	}
	t.Cleanup(func() {
		admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		admin.Close()
	})
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	s, err := Open(u.String())
	if err == nil {
		t.Cleanup(func() { s.Close() })
	}
	return s, err
}

func TestRebind(t *testing.T) {
	for in, want := range map[string]string{
		`SELECT 1`:        `SELECT 1`,
		`a = ? AND b = ?`: `a = $1 AND b = $2`,
		`name LIKE ? ESCAPE '\' OR x = '?' OR y = ?`: `name LIKE $1 ESCAPE '\' OR x = '?' OR y = $2`,
		`v = 'it''s ?' AND w = ?`:                    `v = 'it''s ?' AND w = $1`,
	} {
		if got := rebind(in); got != want {
			t.Errorf("rebind(%q) = %q, want %q", in, got, want)
		}
	}
}
