package store

import "context"

// Name embeddings for Ask AI's search by meaning. Only names worth suggesting get one (see
// embedWhere); src is the text that was embedded, so a name whose reading or meaning changed is
// embedded again.
const vectorSchema = `
CREATE TABLE IF NOT EXISTS name_vectors (
  name  TEXT PRIMARY KEY,
  model TEXT NOT NULL,
  src   TEXT NOT NULL,
  vec   BYTEA NOT NULL,  -- int8 per dimension
  scale REAL NOT NULL,   -- value = int8 * scale
  at    BIGINT NOT NULL
);
`

// embedText is what gets embedded: the name, how it reads, and what it means.
const embedText = `u.name || COALESCE(' | ' || u.words, '') || COALESCE(' | ' || u.meaning, '')`

// embedWhere: names rated 6+, or listed now and rated 4+.
const embedWhere = `(u.score >= 6 OR (u.status IN ('auction','sale','market') AND u.score >= 4))`

// VectorRow is one embedded name.
type VectorRow struct {
	Name  string
	Src   string
	Vec   []int8
	Scale float32
}

// ToEmbed lists names that need a vector from model, best rated first, with the text to embed.
func (s *Store) ToEmbed(ctx context.Context, model string, limit int) ([]VectorRow, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT u.name, `+embedText+` FROM usernames u
		LEFT JOIN name_vectors v ON v.name = u.name
		WHERE `+embedWhere+` AND (v.name IS NULL OR v.model != ? OR v.src != `+embedText+`)
		ORDER BY u.score DESC, u.name LIMIT ?`, model, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VectorRow
	for rows.Next() {
		var r VectorRow
		if err := rows.Scan(&r.Name, &r.Src); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountToEmbed is how many names would be embedded, and how many already are, for model.
func (s *Store) CountToEmbed(ctx context.Context, model string) (todo, done int, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FILTER (WHERE v.name IS NULL OR v.model != ? OR v.src != `+embedText+`),
			COUNT(v.name) FILTER (WHERE v.model = ?)
		FROM usernames u LEFT JOIN name_vectors v ON v.name = u.name WHERE `+embedWhere, model, model).Scan(&todo, &done)
	return
}

func (s *Store) SaveVectors(ctx context.Context, model string, rows []VectorRow) error {
	rows = lastBy(rows, func(r VectorRow) string { return r.Name })
	if len(rows) == 0 {
		return nil
	}
	names, srcs := make([]string, len(rows)), make([]string, len(rows))
	vecs := make([][]byte, len(rows))
	scales := make([]float32, len(rows))
	for i, r := range rows {
		names[i], srcs[i], scales[i] = r.Name, r.Src, r.Scale
		b := make([]byte, len(r.Vec))
		for j, v := range r.Vec {
			b[j] = byte(v)
		}
		vecs[i] = b
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO name_vectors(name, model, src, vec, scale, at)
		SELECT name, ?, src, vec, scale, ? FROM unnest(?::text[], ?::text[], ?::bytea[], ?::real[]) AS r(name, src, vec, scale)
		ON CONFLICT (name) DO UPDATE SET model = excluded.model, src = excluded.src, vec = excluded.vec, scale = excluded.scale, at = excluded.at`,
		model, now(), names, srcs, vecs, scales)
	return err
}

// EachVector calls fn for every stored vector of model with the given dimensions.
func (s *Store) EachVector(ctx context.Context, model string, dims int, fn func(name string, vec []int8, scale float32)) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, vec, scale FROM name_vectors WHERE model = ? AND length(vec) = ?`, model, dims)
	if err != nil {
		return err
	}
	defer rows.Close()
	var b []byte
	for rows.Next() {
		var name string
		var scale float32
		if err := rows.Scan(&name, &b, &scale); err != nil {
			return err
		}
		v := make([]int8, len(b))
		for i, x := range b {
			v[i] = int8(x)
		}
		fn(name, v, scale)
	}
	return rows.Err()
}
