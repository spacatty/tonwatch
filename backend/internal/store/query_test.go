package store

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func fp(v float64) *float64 { return &v }

func TestBuyoutKeywordsAndSort(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	ctx := context.Background()
	err = s.UpsertFragment(ctx, []FragRow{
		{Name: "vpnfast", Status: "auction", Price: fp(10), BuyNow: fp(20), FromSrc: SrcProbe}, // page read: has buy-now
		{Name: "safe_net", Status: "sale", Price: fp(15)},
		{Name: "tunnelx", Status: "auction", Price: fp(5)}, // no buy-now: not instantly buyable
		{Name: "coffee", Status: "sold", Price: fp(100)},
	})
	if err != nil {
		t.Fatal(err)
	}
	// a later list sweep (no buy-now in list rows) must not wipe the known buy-now price
	if err := s.UpsertFragment(ctx, []FragRow{{Name: "vpnfast", Status: "auction", Price: fp(12), FromSrc: SrcFragmentList}}); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.Get(ctx, "vpnfast"); u.Buyout == nil || *u.Buyout != 20 {
		t.Fatalf("buy-now lost after list sweep: %+v", u.Buyout)
	}
	names := func(f Filter) []string {
		items, _, err := s.Query(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, u := range items {
			out = append(out, u.Name)
		}
		return out
	}

	if got := names(Filter{BuyNow: true, Sort: "buyout"}); !slices.Equal(got, []string{"safe_net", "vpnfast"}) {
		t.Errorf("buy now by buyout asc: %v", got)
	}
	if got := names(Filter{BuyNow: true, Sort: "buyout", Dir: "desc"}); !slices.Equal(got, []string{"vpnfast", "safe_net"}) {
		t.Errorf("buyout desc: %v", got)
	}
	if got := names(Filter{MaxBuyout: fp(16)}); !slices.Equal(got, []string{"safe_net"}) {
		t.Errorf("max buyout: %v", got)
	}
	// keywords ignore underscores in names: "safenet" finds safe_net
	if got := names(Filter{Keywords: []string{"vpn", "safenet"}, Sort: "name"}); !slices.Equal(got, []string{"safe_net", "vpnfast"}) {
		t.Errorf("keywords: %v", got)
	}
	// price sort puts names without a price last in both directions
	if got := names(Filter{Sort: "price", Dir: "desc"}); got[len(got)-1] != "coffee" {
		t.Errorf("nulls last: %v", got)
	}
}

func TestMultiSort(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	ctx := context.Background()
	if err := s.UpsertFragment(ctx, []FragRow{
		{Name: "aaaa", Status: "sale", Price: fp(30)},
		{Name: "bbbb", Status: "sale", Price: fp(10)},
		{Name: "cccc", Status: "sale", Price: fp(5)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveScores(ctx, []Score{{Name: "aaaa", Score: 9}, {Name: "bbbb", Score: 9}, {Name: "cccc", Score: 6}}); err != nil {
		t.Fatal(err)
	}
	items, _, err := s.Query(ctx, Filter{Sort: "score:desc,buyout:asc"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range items {
		got = append(got, u.Name)
	}
	// best score first; among equal scores the cheaper buyout wins
	if !slices.Equal(got, []string{"bbbb", "aaaa", "cccc"}) {
		t.Errorf("got %v", got)
	}
}

func TestLLMQueueAndMedians(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	ctx := context.Background()
	if err := s.UpsertFragment(ctx, []FragRow{
		{Name: "rated", Status: "sale", Price: fp(10)},
		{Name: "skippy", Status: "sale", Price: fp(30)},
		{Name: "other", Status: "sale", Price: fp(20)},
	}); err != nil {
		t.Fatal(err)
	}
	s.SaveScores(ctx, []Score{{Name: "rated", Score: 8}, {Name: "other", Score: 8}})
	queue := func() []string { q, _ := s.UnscoredBatch(ctx, 10, nil); return q }
	if got := queue(); !slices.Equal(got, []string{"skippy"}) {
		t.Fatalf("rated names must never be queued again: %v", got)
	}
	s.MarkLLMSkipped(ctx, []string{"skippy"})
	if got := queue(); len(got) != 1 {
		t.Fatalf("one retry allowed: %v", got)
	}
	s.MarkLLMSkipped(ctx, []string{"skippy"})
	if got := queue(); len(got) != 0 {
		t.Fatalf("no third try: %v", got)
	}
	m, err := s.ScoreMedians(ctx)
	if err != nil || len(m) != 1 || m[0].Score != 8 || m[0].Ask != 15 || m[0].AskCount != 2 || m[0].SoldCount != 0 {
		t.Fatalf("medians: %+v %v", m, err)
	}
}

func TestValueSort(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	ctx := context.Background()
	if err := s.UpsertFragment(ctx, []FragRow{
		{Name: "pricey9", Status: "sale", Price: fp(5000)},
		{Name: "cheap8", Status: "sale", Price: fp(40)},
		{Name: "bid8", Status: "auction", Price: fp(400)},
		{Name: "unrated", Status: "sale", Price: fp(1)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveScores(ctx, []Score{{Name: "pricey9", Score: 9}, {Name: "cheap8", Score: 8}, {Name: "bid8", Score: 8}}); err != nil {
		t.Fatal(err)
	}
	// typical: score 9 → 10000, score 8 → 1000; so 5000/10000=0.5, 40/1000=0.04, 400/1000=0.4
	s.typ, s.typAt = map[int]float64{8: 1000, 9: 10000}, time.Now()
	items, _, err := s.Query(ctx, Filter{Sort: "value"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range items {
		got = append(got, u.Name)
	}
	if want := []string{"cheap8", "bid8", "pricey9", "unrated"}; !slices.Equal(got, want) {
		t.Errorf("value sort: %v, want %v", got, want)
	}
	// excluding a kind keeps the other kinds and the unrated names
	s.SaveScores(ctx, []Score{{Name: "bid8", Score: 8, Category: "junk"}})
	items, _, _ = s.Query(ctx, Filter{NotCategories: []string{"junk"}, Sort: "name"})
	got = got[:0]
	for _, u := range items {
		got = append(got, u.Name)
	}
	if want := []string{"cheap8", "pricey9", "unrated"}; !slices.Equal(got, want) {
		t.Errorf("not category: %v, want %v", got, want)
	}
}
