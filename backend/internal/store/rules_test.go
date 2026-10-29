package store

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRuleValidation(t *testing.T) {
	for js, wantErr := range map[string]string{
		`{"op":"or","rules":[{"field":"score","cmp":"gte","value":7},{"field":"kind","cmp":"in","value":["crypto"]}]}`: "",
		`{"not":true,"field":"has_digits","cmp":"is","value":true}`:                                                    "",
		`{"field":"buyout","cmp":"between","value":[10,30]}`:                                                           "",
		`{"field":"keywords","cmp":"any","value":["bank","pay"]}`:                                                      "",
		`{"field":"name","cmp":"starts","value":"ton"}`:                                                                "",
		`{"field":"password","cmp":"eq","value":1}`:                                                                    "unknown field",
		`{"field":"score","cmp":"like","value":7}`:                                                                     "does not fit",
		`{"field":"score","cmp":"gte","value":"seven"}`:                                                                "does not fit",
		`{"field":"kind","cmp":"in","value":["dragons"]}`:                                                              "unknown value",
		`{"field":"status","cmp":"in","value":["auction'); DROP TABLE users;--"]}`:                                     "unknown value",
		`{"op":"xor","rules":[]}`:                                                                                      "unknown group",
		`{"rules":[{"rules":[{"rules":[{"rules":[{"rules":[{"rules":[{"field":"len","cmp":"lt","value":5}]}]}]}]}]}]}`: "too deep",
	} {
		_, err := ParseRules(js)
		switch {
		case wantErr == "" && err != nil:
			t.Errorf("%s: unexpected %v", js, err)
		case wantErr != "" && (err == nil || !strings.Contains(err.Error(), wantErr)):
			t.Errorf("%s: want error containing %q, got %v", js, wantErr, err)
		}
	}
}

func TestRuleQueries(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	fp := func(v float64) *float64 { return &v }
	rows := []FragRow{
		{Name: "moonbank", Status: "sale", Price: fp(20), FromSrc: SrcFragmentList},
		{Name: "cryptox9", Status: "sale", Price: fp(500), FromSrc: SrcFragmentList},
		{Name: "pretty", Status: "auction", Price: fp(40), FromSrc: SrcFragmentList},
		{Name: "plainone", Status: "sale", Price: fp(90), FromSrc: SrcFragmentList},
		{Name: "nopricex", Status: "taken", FromSrc: SrcFragmentList},
	}
	if err := s.UpsertFragment(ctx, rows); err != nil {
		t.Fatal(err)
	}
	s.SaveScores(ctx, []Score{
		{Name: "moonbank", Score: 6, Category: "brand", Meaning: "moon bank"},
		{Name: "cryptox9", Score: 5, Category: "crypto", Meaning: "crypto x"},
		{Name: "pretty", Score: 8, Category: "word", Meaning: "pretty"},
		{Name: "plainone", Score: 6, Category: "phrase", Meaning: "plain one"},
		{Name: "nopricex", Score: 4, Category: "word"},
	})
	run := func(js string) []string {
		t.Helper()
		r, err := ParseRules(js)
		if err != nil {
			t.Fatalf("%s: %v", js, err)
		}
		items, _, err := s.Query(ctx, Filter{Rules: r, Sort: "name", Limit: 50})
		if err != nil {
			t.Fatalf("%s: %v", js, err)
		}
		var out []string
		for _, u := range items {
			out = append(out, u.Name)
		}
		slices.Sort(out)
		return out
	}
	check := func(js string, want ...string) {
		t.Helper()
		if got := run(js); !slices.Equal(got, want) {
			t.Errorf("%s\n got %v\nwant %v", js, got, want)
		}
	}

	// beautiful OR crypto, AND priced under 600 TON (an auction without buy-now has a price but no buyout)
	check(`{"op":"and","rules":[
		{"op":"or","rules":[{"field":"score","cmp":"gte","value":7},{"field":"kind","cmp":"in","value":["crypto"]}]},
		{"field":"price","cmp":"lt","value":600}]}`, "cryptox9", "pretty")
	// NOT keeps names the inner rule cannot speak about: nopricex has no buyout at all
	check(`{"not":true,"field":"buyout","cmp":"lt","value":100}`, "cryptox9", "nopricex", "pretty")
	// nested NOT group: neither digits nor a phrase
	check(`{"not":true,"op":"or","rules":[{"field":"has_digits","cmp":"is","value":true},{"field":"kind","cmp":"in","value":["phrase"]}]}`,
		"moonbank", "nopricex", "pretty")
	check(`{"field":"keywords","cmp":"any","value":["bank","plain"]}`, "moonbank", "plainone")
	check(`{"field":"name","cmp":"ends","value":"x"}`, "nopricex")
	check(`{"field":"status","cmp":"nin","value":["sale","owned"]}`, "pretty")

	// relative: under half the typical price for the score (typical for 6 is 100 here)
	s.typ, s.typAt = Typical{Score: map[int]float64{5: 1000, 6: 100, 8: 50}}, time.Now()
	check(`{"field":"value_ratio","cmp":"lte","value":0.5}`, "cryptox9", "moonbank")

	// quiet: nothing moved for 3 days except plainone, which moved yesterday
	week := now() - 7*86400
	s.DB.ExecContext(ctx, `UPDATE usernames SET first_seen=?`, week)
	s.DB.ExecContext(ctx, `UPDATE events SET ts=?`, week)
	s.DB.ExecContext(ctx, `INSERT INTO events(name, ts, kind, detail) VALUES('plainone', ?, 'price', '100 → 90 TON')`, now()-86400)
	check(`{"op":"and","rules":[{"field":"quiet_days","cmp":"gte","value":3},{"field":"status","cmp":"in","value":["sale","auction"]}]}`,
		"cryptox9", "moonbank", "pretty")
}

func TestRuleSimplify(t *testing.T) {
	r, _ := ParseRules(`{"op":"and","rules":[{"op":"or","rules":[{"op":"and","rules":[{"op":"or","rules":[{"field":"score","cmp":"gte","value":7},{"field":"kind","cmp":"in","value":["crypto"]}]},{"field":"has_digits","cmp":"is","value":false}]},{"field":"value_ratio","cmp":"lt","value":0.5}]}]}`)
	got, _ := json.Marshal(r.Simplify())
	want := `{"op":"or","rules":[{"op":"and","rules":[{"op":"or","rules":[{"field":"score","cmp":"gte","value":7},{"field":"kind","cmp":"in","value":["crypto"]}]},{"field":"has_digits","cmp":"is","value":false}]},{"field":"value_ratio","cmp":"lt","value":0.5}]}`
	if string(got) != want {
		t.Fatalf("the single-child root should go:\n got %s\nwant %s", got, want)
	}
	n, _ := ParseRules(`{"not":true,"rules":[{"not":true,"field":"junk","cmp":"is","value":true}]}`)
	if got, _ := json.Marshal(n.Simplify()); string(got) != `{"field":"junk","cmp":"is","value":true}` {
		t.Fatalf("NOT of NOT cancels: %s", got)
	}
	m, _ := ParseRules(`{"op":"or","rules":[{"field":"len","cmp":"lt","value":5},{"op":"or","rules":[{"field":"junk","cmp":"is","value":false},{"field":"rated","cmp":"is","value":true}]}]}`)
	if s := m.Simplify(); len(s.Rules) != 3 {
		t.Fatalf("an OR inside an OR melts in: %+v", s)
	}
}
