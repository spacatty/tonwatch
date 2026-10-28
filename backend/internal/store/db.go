package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// DB is *sql.DB speaking Postgres through pgx. Queries are written with ? placeholders (the
// dynamic filters build them piece by piece); every call rewrites them to $1, $2… on the way out.
type DB struct{ *sql.DB }

// Tx is a transaction with the same placeholder rewriting.
type Tx struct{ *sql.Tx }

func (d *DB) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return d.DB.ExecContext(ctx, rebind(q), args...)
}

func (d *DB) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return d.DB.QueryContext(ctx, rebind(q), args...)
}

func (d *DB) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return d.DB.QueryRowContext(ctx, rebind(q), args...)
}

func (d *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	tx, err := d.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{tx}, nil
}

func (t *Tx) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return t.Tx.ExecContext(ctx, rebind(q), args...)
}

func (t *Tx) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return t.Tx.QueryContext(ctx, rebind(q), args...)
}

func (t *Tx) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return t.Tx.QueryRowContext(ctx, rebind(q), args...)
}

// rebind turns ? into $1, $2… outside quoted literals.
func rebind(q string) string {
	if !strings.Contains(q, "?") {
		return q
	}
	var b strings.Builder
	b.Grow(len(q) + 16)
	n, quoted := 0, false
	for i := 0; i < len(q); i++ {
		c := q[i]
		switch {
		case c == '\'':
			quoted = !quoted
		case c == '?' && !quoted:
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// uniqueErr maps a unique-constraint violation to ErrTaken.
func uniqueErr(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return ErrTaken
	}
	return err
}
