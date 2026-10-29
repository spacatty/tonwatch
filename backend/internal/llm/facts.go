package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode"

	"fragwatch/internal/rating"
)

// FactRow is what the model recorded about one name.
type FactRow struct {
	Name  string
	Facts rating.Facts
	Gloss string // a note for buyers; only for fame >= 2, and never a restatement of the words
}

// Anchor is a name whose facts are settled. Anchors ride along in every batch, unseen by the
// rest of the pipeline: how far the model's answer for them drifts tells whether the batch was
// read with the same yardstick as every other batch.
type Anchor struct {
	Name       string
	Fame, Pull int
}

// Anchors are kept out of the prompt's own examples, so the model cannot copy them.
var Anchors = []Anchor{
	{"travel", 4, 2},
	{"madrid", 4, 2},
	{"anna", 4, 1},
	{"sunrise", 3, 1},
	{"although", 3, 0},
	{"greenowl", 2, 1},
	{"velmora", 1, 0},
	{"qxzt7kp", 0, 0},
}

// MaxDrift is the largest mean absolute error on fame or pull over the anchors a batch may have
// before it is asked again.
const MaxDrift = 0.8

// Drift is how far a batch's anchors landed from their settled facts.
type Drift struct {
	Fame, Pull float64
	Seen       int // anchors the model answered for
}

// OK says the batch kept the yardstick: most anchors answered, and neither error over MaxDrift.
func (d Drift) OK() bool {
	return d.Seen*2 >= len(Anchors) && d.Fame <= MaxDrift && d.Pull <= MaxDrift
}

func (d Drift) String() string {
	return fmt.Sprintf("fame %.2f, pull %.2f over %d anchors", d.Fame, d.Pull, d.Seen)
}

// FactsResult is one batch: the rows for the names asked about (anchors left out), the names the
// model skipped, and the anchors' drift. Retried says the first answer drifted and was asked again;
// Drift is then the second answer's, which is kept whatever it is.
type FactsResult struct {
	Rows    []FactRow
	Missing []string
	Drift   Drift
	Retried bool
	Usage   Usage
}

// RecordFacts asks the model for the facts of one batch of names, with the anchors mixed in.
// A batch whose anchors drift is asked once more; the second answer is kept either way and the
// caller logs it.
func (c *Client) RecordFacts(ctx context.Context, names []string) (FactsResult, error) {
	var res FactsResult
	for try := 0; try < 2; try++ {
		rows, drift, usage, err := c.factsOnce(ctx, names)
		res.Usage.add(usage)
		if err != nil {
			return res, err
		}
		res.Rows, res.Drift, res.Retried = rows, drift, try > 0
		if drift.OK() {
			break
		}
	}
	got := map[string]bool{}
	for _, r := range res.Rows {
		got[r.Name] = true
	}
	for _, n := range names {
		if !got[n] {
			res.Missing = append(res.Missing, n)
		}
	}
	return res, nil
}

func (u *Usage) add(o Usage) {
	u.PromptTokens += o.PromptTokens
	u.CompletionTokens += o.CompletionTokens
	u.CachedTokens += o.CachedTokens
	u.Cost += o.Cost
}

func (c *Client) factsOnce(ctx context.Context, names []string) ([]FactRow, Drift, Usage, error) {
	lines := withAnchors(names)
	maxTokens := 300 + 60*len(lines)
	if c.Effort != "" || c.Reasoning {
		maxTokens = 2*maxTokens + 4096 // room to think before answering
	}
	content, usage, err := c.Chat(ctx, FactsPrompt, strings.Join(lines, "\n"), 0.1, maxTokens)
	if err != nil {
		return nil, Drift{}, usage, err
	}
	rows, err := parseFacts(content, lines)
	if err != nil {
		return nil, Drift{}, usage, err
	}
	asked := map[string]bool{}
	for _, n := range names {
		asked[n] = true
	}
	out := rows[:0]
	var anchorRows []FactRow
	for _, r := range rows {
		if asked[r.Name] {
			out = append(out, r)
		} else {
			anchorRows = append(anchorRows, r)
		}
	}
	return out, drift(anchorRows), usage, nil
}

// withAnchors spreads the anchors evenly through the batch. An anchor that is itself in the batch
// is not added twice: its answer is then a real row, and drift is measured on the others.
func withAnchors(names []string) []string {
	in := map[string]bool{}
	for _, n := range names {
		in[n] = true
	}
	var anchors []string
	for _, a := range Anchors {
		if !in[a.Name] {
			anchors = append(anchors, a.Name)
		}
	}
	out := make([]string, 0, len(names)+len(anchors))
	step := float64(len(names)) / float64(len(anchors)+1)
	next, k := step, 0
	for i, n := range names {
		for k < len(anchors) && float64(i) >= next {
			out = append(out, anchors[k])
			k++
			next += step
		}
		out = append(out, n)
	}
	return append(out, anchors[k:]...)
}

// drift compares the anchors' answers with their settled facts.
func drift(rows []FactRow) Drift {
	want := map[string]Anchor{}
	for _, a := range Anchors {
		want[a.Name] = a
	}
	var d Drift
	for _, r := range rows {
		a, ok := want[r.Name]
		if !ok {
			continue
		}
		d.Fame += math.Abs(float64(r.Facts.Fame - a.Fame))
		d.Pull += math.Abs(float64(r.Facts.Pull - a.Pull))
		d.Seen++
	}
	if d.Seen > 0 {
		d.Fame /= float64(d.Seen)
		d.Pull /= float64(d.Seen)
	}
	return d
}

// parseFacts reads {"r":[["name","words","kind","lang",fame,pull,clean,"gloss"], ...]}, keeping
// only names that were asked about, each once.
func parseFacts(content string, names []string) ([]FactRow, error) {
	var doc struct {
		R [][]any `json:"r"`
	}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return nil, fmt.Errorf("bad model JSON: %w", err)
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	str := func(row []any, i int) string {
		if i < len(row) {
			s, _ := row[i].(string)
			return strings.TrimSpace(s)
		}
		return ""
	}
	num := func(row []any, i, hi int) int {
		if i < len(row) {
			if v, ok := row[i].(float64); ok {
				return min(hi, max(0, int(math.Round(v))))
			}
		}
		return 0
	}
	var out []FactRow
	for _, row := range doc.R {
		if len(row) < 7 {
			continue
		}
		name := strings.ToLower(strings.TrimPrefix(str(row, 0), "@"))
		if !want[name] {
			continue
		}
		delete(want, name)
		f := rating.Facts{
			Words: strings.Join(strings.Fields(strings.ToLower(str(row, 1))), " "),
			Kind:  strings.ToLower(str(row, 2)),
			Lang:  strings.ToLower(str(row, 3)),
			Fame:  num(row, 4, 5),
			Pull:  num(row, 5, 2),
			Clean: num(row, 6, 2),
		}
		if len(f.Lang) > 3 {
			f.Lang = ""
		}
		gloss := str(row, 7)
		if f.Fame < 2 || restates(gloss, f.Words, name) {
			gloss = ""
		}
		if len(gloss) > 120 {
			gloss = strings.TrimSpace(gloss[:120])
		}
		out = append(out, FactRow{Name: name, Facts: f, Gloss: gloss})
	}
	return out, nil
}

var glossFiller = map[string]bool{"a": true, "an": true, "the": true, "of": true, "and": true, "or": true, "in": true,
	"on": true, "for": true, "to": true, "with": true, "is": true, "name": true, "word": true, "username": true}

// restates says a gloss adds nothing to the words it reads as: "good news" for goodnews. Every
// word of the gloss, filler aside, already being in the reading (or the name itself) is a restatement.
func restates(gloss, words, name string) bool {
	tokens := strings.FieldsFunc(strings.ToLower(gloss), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	have := map[string]bool{name: true, strings.ReplaceAll(name, "_", ""): true}
	for _, w := range strings.Fields(words) {
		have[w] = true
	}
	var rest []string
	for _, t := range tokens {
		if !glossFiller[t] {
			rest = append(rest, t)
		}
	}
	if len(rest) == 0 || have[strings.Join(rest, "")] {
		return true
	}
	for _, t := range rest {
		if !have[t] {
			return false
		}
	}
	return true
}
