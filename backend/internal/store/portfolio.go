package store

import (
	"context"
	"database/sql"
)

// PortfolioRow is a name they own (or bought through the app), with what they paid and the
// concept it is meant for. The live Username row rides along so the view needs one request.
type PortfolioRow struct {
	Name     string   `json:"name"`
	Bought   *float64 `json:"bought"`
	BoughtAt *int64   `json:"bought_at"`
	Concept  string   `json:"concept"`
	Note     string   `json:"note"`
	AddedAt  int64    `json:"added_at"`
	Item     Username `json:"item"`
}

func (s *Store) Portfolio(ctx context.Context, userID int64) ([]PortfolioRow, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, bought, bought_at, concept, note, added_at FROM portfolio WHERE user_id=? ORDER BY added_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PortfolioRow{}
	for rows.Next() {
		var r PortfolioRow
		var bought sql.NullFloat64
		var at sql.NullInt64
		if err := rows.Scan(&r.Name, &bought, &at, &r.Concept, &r.Note, &r.AddedAt); err != nil {
			return nil, err
		}
		if bought.Valid {
			r.Bought = &bought.Float64
		}
		if at.Valid {
			r.BoughtAt = &at.Int64
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// a portfolio is a few dozen rows at most: one lookup each is fine
	for i := range out {
		if u, err := s.GetFor(ctx, userID, out[i].Name); err == nil && u != nil {
			out[i].Item = *u
		} else {
			out[i].Item = Username{Name: out[i].Name, Status: "owned"}
		}
	}
	return out, nil
}

// SetPortfolio adds or updates one row. The name must exist in usernames (EnsureName first).
func (s *Store) SetPortfolio(ctx context.Context, userID int64, r PortfolioRow) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO portfolio(user_id, name, bought, bought_at, concept, note, added_at) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(user_id, name) DO UPDATE SET bought=excluded.bought, bought_at=excluded.bought_at, concept=excluded.concept, note=excluded.note`,
		userID, r.Name, nf(r.Bought), ni(r.BoughtAt), r.Concept, r.Note, now())
	return err
}

func (s *Store) DeletePortfolio(ctx context.Context, userID int64, name string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM portfolio WHERE user_id=? AND name=?`, userID, name)
	return err
}

func nf(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func ni(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}
