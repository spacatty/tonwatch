package app

import (
	"reflect"
	"testing"

	"fragwatch/internal/llm"
)

func TestFilterFromSpec(t *testing.T) {
	i := func(v int) *int { return &v }
	f := func(v float64) *float64 { return &v }
	i64 := func(v int64) *int64 { return &v }

	// "auctions with no bids for 5 days, under 30 TON, beautiful or crypto"
	view, got := filterFromSpec(llm.FilterSpec{View: "auction", MinScore: i(7), Kinds: []string{"crypto", "Crypto", "unicorns"},
		MaxBuyout: f(30), QuietDays: i(5), Sort: "value", Summary: "quiet auctions under 30 TON"})
	want := map[string]any{"minScore": 7, "unscored": false, "categories": []string{"crypto"}, "maxBuyout": "30", "quietDays": "5",
		"sorts": []map[string]string{{"col": "value", "dir": "asc"}}}
	if view != "auction" || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %s %#v", view, got)
	}

	// nonsense from the model is dropped, not passed on
	view, got = filterFromSpec(llm.FilterSpec{View: "everything!", MinScore: i(42), MaxBuyout: f(-3), MinLen: i(9), MaxLen: i(5),
		Query: "a b; drop", Sort: "random", QuietDays: i(0)})
	if view != "buyable" || !reflect.DeepEqual(got, map[string]any{"minScore": 10, "unscored": false}) {
		t.Fatalf("bad values should fall away: %s %#v", view, got)
	}

	// "ending today" picks the nearest window the chip offers and moves to auctions
	view, got = filterFromSpec(llm.FilterSpec{EndingIn: i64(5000), MaxLen: i(6), NoDigits: true, Query: "^bank"})
	if view != "auction" || got["endingIn"] != "21600" || !reflect.DeepEqual(got["len"], []int{4, 6}) || got["noDigits"] != true || got["q"] != "^bank" {
		t.Fatalf("got %s %#v", view, got)
	}
}

func TestFilterFromSpecRules(t *testing.T) {
	_, got := filterFromSpec(llm.FilterSpec{Rules: []byte(`{"op":"or","rules":[{"field":"score","cmp":"gte","value":7},{"field":"kind","cmp":"in","value":["crypto"]}]}`)})
	if got["rules"] == nil {
		t.Fatalf("a valid tree is kept: %#v", got)
	}
	_, got = filterFromSpec(llm.FilterSpec{Rules: []byte(`{"field":"secret","cmp":"eq","value":1}`)})
	if _, ok := got["rules"]; ok {
		t.Fatalf("an invalid tree is dropped: %#v", got)
	}
}

func TestAIFilterCacheKey(t *testing.T) {
	a := aiFilterCacheKey("en", "Cheap  crypto names under 30 TON!")
	if b := aiFilterCacheKey("en", "cheap crypto names under 30 ton"); a != b {
		t.Errorf("case, spacing and end punctuation must not matter: %s vs %s", a, b)
	}
	if b := aiFilterCacheKey("ru", "cheap crypto names under 30 ton"); a == b {
		t.Error("the summary is in the reader's language: languages must not share a key")
	}
	if b := aiFilterCacheKey("en", "cheap crypto names under 40 ton"); a == b {
		t.Error("different requests share a key")
	}
}
