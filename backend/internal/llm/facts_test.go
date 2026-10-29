package llm

import (
	"slices"
	"strings"
	"testing"
)

func TestParseFacts(t *testing.T) {
	content := `{"r":[
		["coffee","coffee","word","en",4,2,2,"hot drink; cafes, roasters, delivery"],
		["@GoodNews","good news","phrase","en",3,1,2,"good news"],
		["harbor","harbor","word","en",3,1,2,"sheltered port; shipping, logistics"],
		["lunexo","","brand","",1,0,2,"an invented word"],
		["stranger","x","word","en",9,9,9,""],
		["casa","casa","foreign","es",7,-1,1.6,"Spanish: house; real estate"],
		["short"]
	]}`
	rows, err := parseFacts(content, []string{"coffee", "goodnews", "harbor", "lunexo", "casa", "short"})
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]FactRow{}
	for _, r := range rows {
		by[r.Name] = r
	}
	if len(rows) != 5 || by["stranger"].Name != "" || by["short"].Name != "" {
		t.Fatalf("only asked-for, complete rows: %+v", rows)
	}
	if c := by["coffee"]; c.Facts.Fame != 4 || c.Facts.Pull != 2 || c.Gloss == "" || c.Facts.Lang != "en" {
		t.Errorf("coffee: %+v", c)
	}
	if g := by["goodnews"].Gloss; g != "" {
		t.Errorf("a gloss that restates the words is dropped: %q", g)
	}
	if g := by["lunexo"].Gloss; g != "" {
		t.Errorf("no gloss below fame 2: %q", g)
	}
	if c := by["casa"]; c.Facts.Fame != 5 || c.Facts.Pull != 0 || c.Facts.Clean != 2 {
		t.Errorf("values are clamped and rounded: %+v", c.Facts)
	}
}

func TestRestates(t *testing.T) {
	for _, c := range []struct {
		gloss, words, name string
		want               bool
	}{
		{"good news", "good news", "goodnews", true},
		{"The dark moon.", "dark moon", "darkmoon", true},
		{"goodnews", "good news", "goodnews", true},
		{"", "coffee", "coffee", true},
		{"Spanish: house; real estate", "casa", "casa", false},
		{"blue fox, a nickname for gamers", "blue fox", "bluefox", false},
	} {
		if got := restates(c.gloss, c.words, c.name); got != c.want {
			t.Errorf("restates(%q, %q) = %v", c.gloss, c.words, got)
		}
	}
}

func TestWithAnchors(t *testing.T) {
	names := make([]string, 40)
	for i := range names {
		names[i] = "name" + strings.Repeat("x", i%5)
	}
	names[7] = "travel" // an anchor that is also asked about is not sent twice
	out := withAnchors(names)
	if len(out) != len(names)+len(Anchors)-1 {
		t.Fatalf("len %d", len(out))
	}
	if n := slices.Index(out, "madrid"); n < 3 || n > len(out)-3 {
		t.Errorf("anchors are spread through the batch, not stacked at an end: madrid at %d", n)
	}
	count := 0
	for _, n := range out {
		if n == "travel" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("travel %d times", count)
	}
	if got := withAnchors(nil); len(got) != len(Anchors) {
		t.Errorf("anchors alone: %v", got)
	}
}

func TestDrift(t *testing.T) {
	var exact []FactRow
	for _, a := range Anchors {
		r := FactRow{Name: a.Name}
		r.Facts.Fame, r.Facts.Pull = a.Fame, a.Pull
		exact = append(exact, r)
	}
	if d := drift(exact); !d.OK() || d.Fame != 0 || d.Seen != len(Anchors) {
		t.Errorf("exact answers: %+v", d)
	}
	off := slices.Clone(exact)
	for i := range off {
		off[i].Facts.Fame += 2
	}
	if d := drift(off); d.OK() || d.Fame != 2 {
		t.Errorf("fame two off everywhere must fail: %+v", d)
	}
	if d := drift(exact[:2]); d.OK() {
		t.Errorf("too few anchors answered must fail: %+v", d)
	}
}
