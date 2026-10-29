package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"fragwatch/internal/llm"
	"fragwatch/internal/rating"
	"fragwatch/internal/store"
)

// An eval set is a fixed sample of sold names with the price each sold for and the score stored
// when the sample was drawn (the old rating). It is drawn once and kept in a JSON file, so every
// run compares against the same names. The facts the model records for them are cached next to
// it: retuning the formula re-evaluates without a single model call.
//
// Two sets: the main one, stratified by length (60 names per bucket), checks every bucket; the
// four-letter one (300 names) is big enough to tune that bucket on. Its names alternate between a
// tuning half and a held-out half, and the numbers to report come from the held-out half.
//
// The command prints only aggregates, never the names themselves.

type evalName struct {
	Name     string  `json:"name"`
	Len      int     `json:"len"`
	Price    float64 `json:"price"`
	OldScore int     `json:"old_score"`
}

type evalFacts struct {
	Model string                 `json:"model"`
	Facts map[string]llm.FactRow `json:"facts"`
}

// evalSlice is one eval set: where it lives and how it is drawn.
type evalSlice struct {
	setFile, factsFile string
	draws              []evalDraw
}

// evalDraw draws up to limit sold names with a stored score and a length in [min, max], in md5
// order: a fixed, letter-blind sample.
type evalDraw struct{ min, max, limit int }

var mainSlice = func() evalSlice {
	s := evalSlice{setFile: "evalset.json", factsFile: "evalfacts.json"}
	for _, b := range rating.LenBuckets {
		s.draws = append(s.draws, evalDraw{b.Min, b.Max, 60})
	}
	return s
}()

var fourSlice = evalSlice{setFile: "evalset4.json", factsFile: "evalfacts4.json", draws: []evalDraw{{4, 4, 300}}}

func evalPath(file string) string {
	_, src, _, ok := runtime.Caller(0)
	if !ok {
		return file
	}
	return filepath.Join(filepath.Dir(src), file)
}

// evalRow is one eval name with its three scores: stored when drawn, the previous formula, the current one.
type evalRow struct {
	evalName
	facts     rating.Facts
	prev, new int
	logPrice  float64
}

func scored(set []evalName, facts map[string]llm.FactRow) []evalRow {
	var out []evalRow
	for _, n := range set {
		r, ok := facts[n.Name]
		if !ok {
			continue
		}
		sc, _ := rating.Score(n.Name, r.Facts)
		out = append(out, evalRow{evalName: n, facts: r.Facts, prev: prevScore(n.Name, r.Facts), new: sc, logPrice: math.Log(n.Price)})
	}
	return out
}

func runEval(ctx context.Context, st *store.Store, c *llm.Client, rebuild, refresh, explore bool) error {
	var sets [2][]evalRow
	for i, sl := range []evalSlice{mainSlice, fourSlice} {
		set, err := loadEvalSet(ctx, st, sl, rebuild)
		if err != nil {
			return err
		}
		facts, err := evalFactsFor(ctx, c, set, sl.factsFile, refresh || rebuild)
		if err != nil {
			return err
		}
		sets[i] = scored(set, facts)
	}
	all, four := sets[0], sets[1]
	tune, held := halves(four)
	if explore {
		exploreFour(tune)
		return nil
	}

	fmt.Printf("Spearman correlation of score with log sale price (formula v%d against v%d, facts by %s)\n", rating.Version, prevVersion, c.Model)
	fmt.Printf("%-18s %4s   %6s   %6s   %6s\n", "set", "n", "stored", fmt.Sprintf("v%d", prevVersion), fmt.Sprintf("v%d", rating.Version))
	report("all lengths", all)
	byBucket := map[string][]evalRow{}
	for _, r := range all {
		byBucket[rating.LenBucket(r.Len)] = append(byBucket[rating.LenBucket(r.Len)], r)
	}
	for _, b := range rating.LenBuckets {
		report("  "+b.Name, byBucket[b.Name])
	}
	report("4 letters, held", held)
	report("4 letters, tuning", tune)
	return nil
}

func report(label string, rows []evalRow) {
	n := len(rows)
	old, prev, nw, price := make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n)
	for i, r := range rows {
		old[i], prev[i], nw[i], price[i] = float64(r.OldScore), float64(r.prev), float64(r.new), r.logPrice
	}
	fmt.Printf("%-18s %4d   %+.3f   %+.3f   %+.3f\n", label, n, spearman(old, price), spearman(prev, price), spearman(nw, price))
}

// halves splits the four-letter set: even positions (in md5 order) to tune on, odd ones held out.
func halves(rows []evalRow) (tune, held []evalRow) {
	for i, r := range rows {
		if i%2 == 0 {
			tune = append(tune, r)
		} else {
			held = append(held, r)
		}
	}
	return tune, held
}

// exploreFour prints the median sale price of the tuning half by each fact, for tuning the
// four-letter rules. Only counts and prices, never names.
func exploreFour(rows []evalRow) {
	group := func(title string, key func(evalRow) string) {
		by := map[string][]float64{}
		for _, r := range rows {
			k := key(r)
			by[k] = append(by[k], r.Price)
		}
		keys := make([]string, 0, len(by))
		for k := range by {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s n=%d med=%.0f", k, len(by[k]), median(by[k])))
		}
		fmt.Printf("%-9s %s\n", title, strings.Join(parts, " | "))
	}
	prices := make([]float64, len(rows))
	for i, r := range rows {
		prices[i] = r.Price
	}
	fmt.Printf("tuning half: %d four-letter names, median %.0f TON\n", len(rows), median(prices))
	group("fame", func(r evalRow) string { return fmt.Sprint(r.facts.Fame) })
	group("pull", func(r evalRow) string { return fmt.Sprint(r.facts.Pull) })
	group("clean", func(r evalRow) string { return fmt.Sprint(r.facts.Clean) })
	group("pattern", func(r evalRow) string { return "[" + rating.Pattern(r.Name) + "]" })
	group("kind", func(r evalRow) string { return r.facts.Kind })
	group("digits", func(r evalRow) string { return fmt.Sprint(strings.ContainsAny(r.Name, "0123456789")) })
	group("vowels", func(r evalRow) string { return fmt.Sprint(vowels(r.Name)) })
	group("fame/pull", func(r evalRow) string { return fmt.Sprintf("%d/%d", r.facts.Fame, r.facts.Pull) })
	group("reading", func(r evalRow) string { return fourReading(r.facts) })
	group("read/fame", func(r evalRow) string { return fmt.Sprintf("%s/%d", fourReading(r.facts), r.facts.Fame) })
	group("rare", func(r evalRow) string { return fmt.Sprint(strings.ContainsAny(r.Name, "jqxz")) })
	group("prev", func(r evalRow) string { return fmt.Sprintf("%02d", r.prev) })
	group("new", func(r evalRow) string { return fmt.Sprintf("%02d", r.new) })
}

// fourReading sorts a four-letter name into a plain reading (a word or a name) or a letter combination.
func fourReading(f rating.Facts) string {
	switch f.Kind {
	case "word", "name", "geo", "slang", "foreign", "crypto", "phrase":
		if f.Fame >= 2 {
			return "reads"
		}
	}
	return "combo"
}

func vowels(name string) int {
	n := 0
	for _, r := range name {
		if strings.ContainsRune("aeiou", r) {
			n++
		}
	}
	return n
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// loadEvalSet reads a slice's set, drawing it from the database first if it is missing.
func loadEvalSet(ctx context.Context, st *store.Store, sl evalSlice, rebuild bool) ([]evalName, error) {
	path := evalPath(sl.setFile)
	if !rebuild {
		if b, err := os.ReadFile(path); err == nil {
			var set []evalName
			if err := json.Unmarshal(b, &set); err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			return set, nil
		}
	}
	var set []evalName
	for _, d := range sl.draws {
		rows, err := st.DB.QueryContext(ctx, `SELECT name, len, frag_price, score FROM usernames
			WHERE frag_status = 'sold' AND frag_price > 0 AND score IS NOT NULL AND len BETWEEN ? AND ?
			ORDER BY md5(name) LIMIT ?`, d.min, d.max, d.limit)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var n evalName
			if err := rows.Scan(&n.Name, &n.Len, &n.Price, &n.OldScore); err != nil {
				rows.Close()
				return nil, err
			}
			set = append(set, n)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	if len(set) == 0 {
		return nil, errors.New("no sold names with a score in the database")
	}
	b, _ := json.MarshalIndent(set, "", " ")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return nil, err
	}
	fmt.Printf("drew %d sold names into %s\n", len(set), path)
	return set, nil
}

// evalFactsFor returns the recorded facts of an eval set, asking the model only for names its
// facts file does not hold for this model.
func evalFactsFor(ctx context.Context, c *llm.Client, set []evalName, file string, refresh bool) (map[string]llm.FactRow, error) {
	path := evalPath(file)
	cache := evalFacts{Model: c.Model, Facts: map[string]llm.FactRow{}}
	if !refresh {
		if b, err := os.ReadFile(path); err == nil {
			var old evalFacts
			if json.Unmarshal(b, &old) == nil && old.Model == c.Model && old.Facts != nil {
				cache = old
			}
		}
	}
	var todo []string
	for _, n := range set {
		if _, ok := cache.Facts[n.Name]; !ok {
			todo = append(todo, n.Name)
		}
	}
	var cost float64
	for i := 0; i < len(todo); i += 150 {
		batch := todo[i:min(len(todo), i+150)]
		res, err := c.RecordFacts(ctx, batch)
		cost += res.Usage.Cost
		if err != nil {
			return nil, err
		}
		fmt.Printf("recorded facts for %d/%d names · anchors %s (retried %v) · missing %d\n", len(res.Rows), len(batch), res.Drift, res.Retried, len(res.Missing))
		for _, r := range res.Rows {
			cache.Facts[r.Name] = r
		}
	}
	if len(todo) > 0 {
		fmt.Printf("model cost $%.4f\n", cost)
		b, _ := json.MarshalIndent(cache, "", " ")
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return nil, err
		}
	}
	return cache.Facts, nil
}
