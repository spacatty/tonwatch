package store

import (
	"context"
	"database/sql"
)

// Tracker is a saved search. The backend only needs Params (an /api/usernames query string)
// to count matches; Filters is the dashboard's own state, stored as-is.
type Tracker struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	View      string `json:"view"`
	Filters   string `json:"filters"`
	Params    string `json:"params"`
	SeenAt    int64  `json:"seen_at"`
	CreatedAt int64  `json:"created_at"`
	Total     int    `json:"total"`
	New       int    `json:"new"`
}

func (s *Store) Trackers(ctx context.Context, userID int64) ([]Tracker, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, view, filters, params, seen_at, created_at FROM trackers WHERE user_id=? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tracker{}
	for rows.Next() {
		var t Tracker
		if err := rows.Scan(&t.ID, &t.Name, &t.View, &t.Filters, &t.Params, &t.SeenAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) CreateTracker(ctx context.Context, userID int64, t Tracker) (int64, error) {
	ts := now()
	var id int64
	err := s.DB.QueryRowContext(ctx, `INSERT INTO trackers(user_id, name, view, filters, params, seen_at, created_at) VALUES(?,?,?,?,?,?,?) RETURNING id`,
		userID, t.Name, t.View, t.Filters, t.Params, ts, ts).Scan(&id)
	return id, err
}

// UpdateTracker changes any of name/view/filters/params that are non-nil.
func (s *Store) UpdateTracker(ctx context.Context, userID, id int64, name, view, filters, params *string) error {
	return oneRow(s.DB.ExecContext(ctx, `UPDATE trackers SET name=COALESCE(?, name), view=COALESCE(?, view),
		filters=COALESCE(?, filters), params=COALESCE(?, params) WHERE id=? AND user_id=?`, ns(name), ns(view), ns(filters), ns(params), id, userID))
}

// oneRow turns "no row matched" (absent, or someone else's) into sql.ErrNoRows.
func oneRow(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func ns(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func (s *Store) DeleteTracker(ctx context.Context, userID, id int64) error {
	return oneRow(s.DB.ExecContext(ctx, `DELETE FROM trackers WHERE id=? AND user_id=?`, id, userID))
}

func (s *Store) TrackerSeen(ctx context.Context, userID, id int64) error {
	return oneRow(s.DB.ExecContext(ctx, `UPDATE trackers SET seen_at=? WHERE id=? AND user_id=?`, now(), id, userID))
}
