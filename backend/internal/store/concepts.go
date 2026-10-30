package store

import (
	"context"
)

// Concept is one generated project idea, kept forever: the history lets the user see every idea
// that was ever made for a name, and lets later runs steer clear of what was made before.
type Concept struct {
	ID        int64         `json:"id"`
	TS        int64         `json:"ts"`
	Source    string        `json:"source"`
	Project   string        `json:"project"`
	Pitch     string        `json:"pitch"`
	FirstMove string        `json:"first_move"`
	Form      string        `json:"form,omitempty"`
	How       string        `json:"how,omitempty"`
	BuiltWith string        `json:"built_with,omitempty"`
	Note      string        `json:"note,omitempty"`
	Names     []ConceptName `json:"names"`
	Chat      int           `json:"chat"`  // messages in its chat; filled by the app
	Liked     int           `json:"liked"` // 1 liked, -1 disliked, 0 none
}

type ConceptName struct {
	Name  string `json:"name"`
	Angle string `json:"angle"`
}

func (s *Store) AddConcept(ctx context.Context, userID int64, c Concept) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO concepts(user_id, ts, source, project, pitch, first_move, form, how, note, built_with) VALUES(?,?,?,?,?,?,?,?,?,?) RETURNING id`,
		userID, now(), c.Source, c.Project, c.Pitch, c.FirstMove, c.Form, c.How, c.Note, c.BuiltWith).Scan(&id)
	if err != nil {
		return 0, err
	}
	for _, n := range c.Names {
		if _, err := tx.ExecContext(ctx, `INSERT INTO concept_names(concept_id, name, angle) VALUES(?,?,?) ON CONFLICT DO NOTHING`, id, n.Name, n.Angle); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

// Concepts lists the history, newest first; name narrows it to concepts that use that name.
func (s *Store) Concepts(ctx context.Context, userID int64, name string, limit int) ([]Concept, error) {
	q := `SELECT id, ts, source, project, pitch, first_move, form, how, note, liked, built_with FROM concepts WHERE user_id = ?`
	args := []any{userID}
	if name != "" {
		q += ` AND id IN (SELECT concept_id FROM concept_names WHERE name = ?)`
		args = append(args, name)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Concept{}
	byID := map[int64]int{}
	for rows.Next() {
		var c Concept
		if err := rows.Scan(&c.ID, &c.TS, &c.Source, &c.Project, &c.Pitch, &c.FirstMove, &c.Form, &c.How, &c.Note, &c.Liked, &c.BuiltWith); err != nil {
			return nil, err
		}
		c.Names = []ConceptName{}
		byID[c.ID] = len(out)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}
	ids := make([]int64, 0, len(out))
	for _, c := range out {
		ids = append(ids, c.ID)
	}
	nrows, err := s.DB.QueryContext(ctx, `SELECT concept_id, name, angle FROM concept_names WHERE concept_id = ANY(?::int8[]) ORDER BY seq`, ids)
	if err != nil {
		return nil, err
	}
	defer nrows.Close()
	for nrows.Next() {
		var id int64
		var n ConceptName
		if err := nrows.Scan(&id, &n.Name, &n.Angle); err != nil {
			return nil, err
		}
		out[byID[id]].Names = append(out[byID[id]].Names, n)
	}
	return out, nrows.Err()
}

// ConceptTitles returns the project titles already made (optionally for one name), newest first.
func (s *Store) ConceptTitles(ctx context.Context, userID int64, name string, limit int) ([]string, error) {
	if name != "" {
		return s.strings(ctx, `SELECT project FROM concepts WHERE user_id = ? AND id IN (SELECT concept_id FROM concept_names WHERE name = ?) ORDER BY id DESC LIMIT ?`, userID, name, limit)
	}
	return s.strings(ctx, `SELECT project FROM concepts WHERE user_id = ? ORDER BY id DESC LIMIT ?`, userID, limit)
}

// RecentConceptNames returns the names used in the user's last `concepts` concepts.
func (s *Store) RecentConceptNames(ctx context.Context, userID int64, concepts int) ([]string, error) {
	return s.strings(ctx, `SELECT DISTINCT name FROM concept_names WHERE concept_id IN (SELECT id FROM concepts WHERE user_id = ? ORDER BY id DESC LIMIT ?)`, userID, concepts)
}

func (s *Store) DeleteConcept(ctx context.Context, userID, id int64) error {
	return oneRow(s.DB.ExecContext(ctx, `DELETE FROM concepts WHERE id=? AND user_id=?`, id, userID))
}

// Concept returns one of the user's concepts with its names; sql.ErrNoRows when it is not theirs.
func (s *Store) Concept(ctx context.Context, userID, id int64) (*Concept, error) {
	var c Concept
	err := s.DB.QueryRowContext(ctx, `SELECT id, ts, source, project, pitch, first_move, form, how, note, liked, built_with FROM concepts WHERE id=? AND user_id=?`, id, userID).
		Scan(&c.ID, &c.TS, &c.Source, &c.Project, &c.Pitch, &c.FirstMove, &c.Form, &c.How, &c.Note, &c.Liked, &c.BuiltWith)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT name, angle FROM concept_names WHERE concept_id=? ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c.Names = []ConceptName{}
	for rows.Next() {
		var n ConceptName
		if err := rows.Scan(&n.Name, &n.Angle); err != nil {
			return nil, err
		}
		c.Names = append(c.Names, n)
	}
	return &c, rows.Err()
}

// ConceptID finds the newest concept with this title (0 if none).
func (s *Store) ConceptID(ctx context.Context, userID int64, project string) int64 {
	var id int64
	s.DB.QueryRowContext(ctx, `SELECT id FROM concepts WHERE user_id=? AND project=? ORDER BY id DESC LIMIT 1`, userID, project).Scan(&id)
	return id
}

// SetConceptLiked records the user's verdict: 1, -1 or 0.
func (s *Store) SetConceptLiked(ctx context.Context, userID, id int64, v int) error {
	return oneRow(s.DB.ExecContext(ctx, `UPDATE concepts SET liked=? WHERE id=? AND user_id=?`, v, id, userID))
}

// LikedConcepts returns "Project: pitch" lines for the newest concepts with this verdict.
func (s *Store) LikedConcepts(ctx context.Context, userID int64, v, limit int) ([]string, error) {
	return s.strings(ctx, `SELECT project || ': ' || pitch FROM concepts WHERE user_id=? AND liked=? ORDER BY id DESC LIMIT ?`, userID, v, limit)
}
