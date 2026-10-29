package store

import (
	"context"

	"fragwatch/internal/rating"
)

// --- AI rating: the facts the model records, and the score computed from them ---

// factsTier orders the facts queue: listed (buyable) names first, and within each status the
// short names (the expensive ones) first. Inside a tier md5(name) shuffles the rest, so every
// batch mixes lengths and letters instead of holding fifty names that all start with "aa".
const factsTier = `(CASE status WHEN 'auction' THEN 0 WHEN 'sale' THEN 0 WHEN 'market' THEN 1 ELSE 2 END) * 2 + (CASE WHEN len <= 5 THEN 0 ELSE 1 END)`

// premiumWhere picks the names the premium model re-reads: the top scores and every scarce length.
const premiumWhere = `premium_at IS NULL AND junk = 0 AND (score >= 8 OR len = 4 OR (len = 5 AND score >= 6))`

// Score sets a name's rating directly. Only tests use it; the app saves facts (SaveFacts).
type Score struct {
	Name     string
	Score    int
	Category string
	Meaning  string
}

func (s *Store) SaveScores(ctx context.Context, scores []Score) error {
	scores = lastBy(scores, func(sc Score) string { return sc.Name })
	if len(scores) == 0 {
		return nil
	}
	names, cats, meanings := make([]string, len(scores)), make([]string, len(scores)), make([]string, len(scores))
	vals := make([]int, len(scores))
	for i, sc := range scores {
		names[i], vals[i], cats[i], meanings[i] = sc.Name, sc.Score, sc.Category, sc.Meaning
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE usernames AS u SET score = r.score, category = NULLIF(r.category, ''),
			meaning = NULLIF(r.meaning, ''), llm_at = ?
		FROM unnest(?::text[], ?::int[], ?::text[], ?::text[]) AS r(name, score, category, meaning)
		WHERE u.name = r.name`, now(), names, vals, cats, meanings)
	return err
}

// FactsBatch returns names whose facts are not recorded yet, in the ix_facts_queue order.
func (s *Store) FactsBatch(ctx context.Context, limit int, exclude map[string]bool) ([]string, error) {
	return s.batch(ctx, `SELECT name FROM usernames WHERE facts_at IS NULL AND junk = 0 AND llm_tries < 2
		ORDER BY `+factsTier+`, md5(name) LIMIT ?`, limit, exclude)
}

// PremiumBatch returns names the premium model has not re-read yet, best first.
func (s *Store) PremiumBatch(ctx context.Context, limit int, exclude map[string]bool) ([]string, error) {
	return s.batch(ctx, `SELECT name FROM usernames WHERE `+premiumWhere+` AND llm_tries < 2
		ORDER BY score DESC NULLS LAST, len, md5(name) LIMIT ?`, limit, exclude)
}

func (s *Store) batch(ctx context.Context, q string, limit int, exclude map[string]bool) ([]string, error) {
	rows, err := s.strings(ctx, q, limit+len(exclude))
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
	_, err := s.DB.ExecContext(ctx, `UPDATE usernames SET llm_tries = llm_tries + 1 WHERE name = ANY(?::text[])`, names)
	return err
}

// FactsRow is one name's recorded facts and the gloss shown as its meaning.
type FactsRow struct {
	Name  string
	Facts rating.Facts
	Gloss string
}

// SaveFacts stores what the model recorded and the score, kind and meaning that follow from it.
// premium marks names the premium model read, so its pass does not read them again.
func (s *Store) SaveFacts(ctx context.Context, rows []FactsRow, model string, premium bool) error {
	rows = lastBy(rows, func(r FactsRow) string { return r.Name })
	if len(rows) == 0 {
		return nil
	}
	n := len(rows)
	names, words, langs, kinds, glosses, cats := make([]string, n), make([]string, n), make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	fame, pull, clean, scores := make([]int, n), make([]int, n), make([]int, n), make([]int, n)
	for i, r := range rows {
		names[i], words[i], langs[i], kinds[i], glosses[i] = r.Name, r.Facts.Words, r.Facts.Lang, r.Facts.Kind, r.Gloss
		fame[i], pull[i], clean[i] = r.Facts.Fame, r.Facts.Pull, r.Facts.Clean
		scores[i], cats[i] = rating.Score(r.Name, r.Facts)
	}
	ts := now()
	_, err := s.DB.ExecContext(ctx, `UPDATE usernames AS u SET words = NULLIF(r.words, ''), lang = NULLIF(r.lang, ''),
			fame = r.fame, pull = r.pull, clean = r.clean, facts_kind = NULLIF(r.kind, ''), facts_at = ?, facts_model = ?,
			premium_at = CASE WHEN ? THEN ?::bigint ELSE u.premium_at END,
			score = r.score, category = r.category, meaning = NULLIF(r.gloss, ''), llm_at = ?
		FROM unnest(?::text[], ?::text[], ?::text[], ?::text[], ?::text[], ?::int[], ?::int[], ?::int[], ?::int[], ?::text[])
			AS r(name, words, lang, kind, gloss, fame, pull, clean, score, category)
		WHERE u.name = r.name`, ts, model, premium, ts, ts, names, words, langs, kinds, glosses, fame, pull, clean, scores, cats)
	return err
}

// ResetFacts forgets the recorded facts so every name is read again (after editing the prompt).
// Scores, kinds and meanings stay as they are until the new facts land.
func (s *Store) ResetFacts(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE usernames SET words = NULL, lang = NULL, fame = NULL, pull = NULL, clean = NULL,
			facts_kind = NULL, facts_at = NULL, facts_model = NULL, premium_at = NULL, llm_tries = 0
		WHERE facts_at IS NOT NULL OR premium_at IS NOT NULL OR llm_tries != 0`)
	return err
}

// Rescore recomputes the score and kind of up to limit names with facts, those after the name
// `after` in name order, and returns the last name read ("" when there are no more) and how many
// scores changed. Only changed rows are written.
func (s *Store) Rescore(ctx context.Context, after string, limit int) (string, int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, COALESCE(words, ''), COALESCE(facts_kind, ''), COALESCE(lang, ''),
			COALESCE(fame, 0), COALESCE(pull, 0), COALESCE(clean, 0), score, COALESCE(category, '')
		FROM usernames WHERE facts_at IS NOT NULL AND name > ? ORDER BY name LIMIT ?`, after, limit)
	if err != nil {
		return "", 0, err
	}
	var names, cats []string
	var scores []int
	last := ""
	for rows.Next() {
		var name, cat string
		var f rating.Facts
		var old *int
		if err := rows.Scan(&name, &f.Words, &f.Kind, &f.Lang, &f.Fame, &f.Pull, &f.Clean, &old, &cat); err != nil {
			rows.Close()
			return "", 0, err
		}
		last = name
		sc, kind := rating.Score(name, f)
		if old == nil || *old != sc || cat != kind {
			names, scores, cats = append(names, name), append(scores, sc), append(cats, kind)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", 0, err
	}
	if len(names) > 0 {
		if _, err := s.DB.ExecContext(ctx, `UPDATE usernames AS u SET score = r.score, category = r.category
			FROM unnest(?::text[], ?::int[], ?::text[]) AS r(name, score, category) WHERE u.name = r.name`, names, scores, cats); err != nil {
			return "", 0, err
		}
	}
	return last, len(names), nil
}
