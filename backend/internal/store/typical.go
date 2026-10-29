package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fragwatch/internal/rating"
)

// ScoreMedian is the typical price of names with one AI score: what such names actually sold for,
// and what sellers ask for them right now. Len is a length bucket (rating.LenBucket), or "" for
// every length together.
type ScoreMedian struct {
	Score     int     `json:"score"`
	Len       string  `json:"len,omitempty"`
	Sold      float64 `json:"sold"`
	SoldCount int     `json:"sold_count"`
	Ask       float64 `json:"ask"`
	AskCount  int     `json:"ask_count"`
}

// lenBucketSQL is rating.LenBucket as an SQL expression.
func lenBucketSQL() string {
	var b strings.Builder
	b.WriteString("CASE")
	for _, lb := range rating.LenBuckets[:len(rating.LenBuckets)-1] {
		fmt.Fprintf(&b, " WHEN len <= %d THEN '%s'", lb.Max, lb.Name)
	}
	fmt.Fprintf(&b, " ELSE '%s' END", rating.LenBuckets[len(rating.LenBuckets)-1].Name)
	return b.String()
}

// ScoreMedians returns, per AI score and per score and length bucket, the median sale price of sold
// names and the median buyout of names listed now. Asking prices run far above real sales, so
// deals are judged against sales first.
func (s *Store) ScoreMedians(ctx context.Context) ([]ScoreMedian, error) {
	bucket := lenBucketSQL()
	rows, err := s.DB.QueryContext(ctx, `
		SELECT score, COALESCE(b, ''), 's', percentile_cont(0.5) WITHIN GROUP (ORDER BY frag_price), COUNT(*)
		  FROM (SELECT score, frag_price, `+bucket+` AS b FROM usernames
		        WHERE frag_status = 'sold' AND frag_price > 0 AND score IS NOT NULL) x
		  GROUP BY GROUPING SETS ((score), (score, b))
		UNION ALL
		SELECT score, COALESCE(b, ''), 'a', percentile_cont(0.5) WITHIN GROUP (ORDER BY buyout), COUNT(*)
		  FROM (SELECT score, buyout, `+bucket+` AS b FROM usernames WHERE buyout > 0 AND score IS NOT NULL) x
		  GROUP BY GROUPING SETS ((score), (score, b))`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type key struct {
		score int
		len   string
	}
	by := map[key]*ScoreMedian{}
	for rows.Next() {
		var sc, n int
		var lb, kind string
		var med float64
		if err := rows.Scan(&sc, &lb, &kind, &med, &n); err != nil {
			return nil, err
		}
		k := key{sc, lb}
		m := by[k]
		if m == nil {
			m = &ScoreMedian{Score: sc, Len: lb}
			by[k] = m
		}
		if kind == "s" {
			m.Sold, m.SoldCount = med, n
		} else {
			m.Ask, m.AskCount = med, n
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []ScoreMedian{}
	for sc := 0; sc <= 10; sc++ {
		if m := by[key{sc, ""}]; m != nil {
			out = append(out, *m)
		}
		for _, lb := range rating.LenBuckets {
			if m := by[key{sc, lb.Name}]; m != nil {
				out = append(out, *m)
			}
		}
	}
	return out, nil
}

// Typical is the price names usually go for, by AI score and length bucket, falling back to the
// score alone where a bucket has too few sales and listings.
type Typical struct {
	Score map[int]float64            `json:"score"`
	ByLen map[string]map[int]float64 `json:"by_len"`
}

// For is the typical price for a name of this score and length.
func (t Typical) For(score, length int) (float64, bool) {
	if v, ok := t.ByLen[rating.LenBucket(length)][score]; ok {
		return v, true
	}
	v, ok := t.Score[score]
	return v, ok
}

func (t Typical) Empty() bool { return len(t.Score) == 0 && len(t.ByLen) == 0 }

// typicalOf is the price a median stands for: the median sale if there are 30+ sales, else the
// median asking price if there are 10+ listings.
func typicalOf(m ScoreMedian) (float64, bool) {
	if m.SoldCount >= 30 && m.Sold > 0 {
		return m.Sold, true
	}
	if m.AskCount >= 10 && m.Ask > 0 {
		return m.Ask, true
	}
	return 0, false
}

// Typical returns the typical prices, cached for 10 minutes.
func (s *Store) Typical(ctx context.Context) Typical {
	s.typMu.Lock()
	defer s.typMu.Unlock()
	if !s.typ.Empty() && time.Since(s.typAt) < 10*time.Minute {
		return s.typ
	}
	ms, err := s.ScoreMedians(ctx)
	if err != nil {
		return s.typ
	}
	t := Typical{Score: map[int]float64{}, ByLen: map[string]map[int]float64{}}
	for _, m := range ms {
		v, ok := typicalOf(m)
		switch {
		case !ok:
		case m.Len == "":
			t.Score[m.Score] = v
		default:
			if t.ByLen[m.Len] == nil {
				t.ByLen[m.Len] = map[int]float64{}
			}
			t.ByLen[m.Len][m.Score] = v
		}
	}
	s.typ, s.typAt = t, time.Now()
	return t
}

// valueExpr is a name's price as a share of the typical price for its AI score and length: 0.2
// means it costs a fifth of what names rated the same, of a similar length, usually go for. The
// price is the buy-now, else the current one.
func valueExpr(t Typical) string {
	if t.Empty() {
		return "NULL"
	}
	var b strings.Builder
	b.WriteString("(COALESCE(buyout, price) / CASE")
	for _, lb := range rating.LenBuckets {
		for sc := 0; sc <= 10; sc++ {
			if v := t.ByLen[lb.Name][sc]; v > 0 {
				fmt.Fprintf(&b, " WHEN score = %d AND len BETWEEN %d AND %d THEN %g", sc, lb.Min, lb.Max, v)
			}
		}
	}
	for sc := 0; sc <= 10; sc++ {
		if v := t.Score[sc]; v > 0 {
			fmt.Fprintf(&b, " WHEN score = %d THEN %g", sc, v)
		}
	}
	b.WriteString(" END)")
	return b.String()
}
