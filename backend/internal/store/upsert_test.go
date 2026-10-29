package store

import (
	"context"
	"testing"

	"fragwatch/internal/rating"
)


func kinds(t *testing.T, s *Store, name string) []string {
	t.Helper()
	ev, err := s.Events(context.Background(), 0, name, 50)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(ev))
	for i, e := range ev {
		out[len(ev)-1-i] = e.Kind + ": " + e.Detail // oldest first
	}
	return out
}

func TestUpsertFragmentBuyNowAndEvents(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	list := func(price float64, status string) {
		t.Helper()
		if err := s.UpsertFragment(ctx, []FragRow{{Name: "moon", Status: status, Price: fp(price), FromSrc: SrcFragmentList}}); err != nil {
			t.Fatal(err)
		}
	}
	list(10, "auction")
	// a page read brings the buy-now price; same price, so no event
	if err := s.UpsertFragment(ctx, []FragRow{{Name: "moon", Status: "auction", Price: fp(10), BuyNow: fp(50), Owner: "EQx", FromSrc: SrcProbe}}); err != nil {
		t.Fatal(err)
	}
	list(12, "auction") // list rows keep the known buy-now while the auction runs
	u, _ := s.Get(ctx, "moon")
	if u.BuyNow == nil || *u.BuyNow != 50 || u.Buyout == nil || *u.Buyout != 50 || u.Owner == nil || *u.Owner != "EQx" {
		t.Fatalf("buy-now/owner not kept: %+v", u)
	}
	list(12, "sold")
	u, _ = s.Get(ctx, "moon")
	if u.BuyNow != nil || u.Status != "owned" || u.LastSale == nil || *u.LastSale != 12 {
		t.Fatalf("after the sale: %+v", u)
	}
	want := []string{"listed: auction at 10 TON", "price: 10 → 12 TON", "status: auction → sold (12 TON)"}
	if got := kinds(t, s, "moon"); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("events %q, want %q", got, want)
	}
	// the same name twice in one batch: the last row wins, no "row affected twice" error
	if err := s.UpsertFragment(ctx, []FragRow{{Name: "sun", Status: "sale", Price: fp(1)}, {Name: "sun", Status: "sale", Price: fp(2)}}); err != nil {
		t.Fatal(err)
	}
	if u, _ = s.Get(ctx, "sun"); *u.Price != 2 {
		t.Fatalf("last row should win, price %v", *u.Price)
	}
}

func TestUpsertChainKeepsListing(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	up := func(r ChainRow) {
		t.Helper()
		if err := s.UpsertChain(ctx, []ChainRow{r}); err != nil {
			t.Fatal(err)
		}
	}
	up(ChainRow{Name: "star", NFTAddress: "0:aa", Owner: "o1", OnSale: true, SaleContract: "c1", Market: "getgems", Price: fp(5)})
	up(ChainRow{Name: "star", NFTAddress: "0:aa", Owner: "o1", OnSale: true, SaleContract: "c1"}) // same listing, no market info
	u, _ := s.Get(ctx, "star")
	if u.Status != "market" || u.Price == nil || *u.Price != 5 || u.ChainMarket == nil || *u.ChainMarket != "getgems" {
		t.Fatalf("same sale contract should keep the listing: %+v", u)
	}
	up(ChainRow{Name: "star", NFTAddress: "0:aa", Owner: "o1", OnSale: true, SaleContract: "c2"}) // relisted elsewhere
	if u, _ = s.Get(ctx, "star"); u.Price != nil || u.ChainMarket != nil {
		t.Fatalf("a new sale contract should clear the old price: %+v", u)
	}
	if missing, _ := s.ChainSalesMissingPrice(ctx, 10); len(missing) != 1 || missing[0] != "0:aa" {
		t.Fatalf("missing price %v", missing)
	}
	if err := s.SetChainSales(ctx, []ChainSale{{NFTAddress: "0:aa", Market: "fragment", Price: fp(7)}}); err != nil {
		t.Fatal(err)
	}
	if u, _ = s.Get(ctx, "star"); u.Price == nil || *u.Price != 7 {
		t.Fatalf("SetChainSales: %+v", u)
	}
	want := []string{"market: listed on getgems at 5 TON", "market: listed on fragment at 7 TON"}
	if got := kinds(t, s, "star"); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("events %q, want %q", got, want)
	}
}

func TestScoresAndSkips(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	for _, n := range []string{"alpha", "beta"} {
		s.EnsureName(ctx, n)
	}
	if err := s.SaveScores(ctx, []Score{{Name: "alpha", Score: 8, Category: "word", Meaning: "first"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkLLMSkipped(ctx, []string{"beta"}); err != nil {
		t.Fatal(err)
	}
	s.MarkLLMSkipped(ctx, []string{"beta"})
	s.SaveFacts(ctx, []FactsRow{{Name: "alpha", Facts: rating.Facts{Words: "alpha", Kind: "word", Fame: 4, Pull: 1, Clean: 2}}}, "m", false)
	if q, _ := s.FactsBatch(ctx, 10, nil); len(q) != 0 {
		t.Fatalf("alpha has facts and beta was skipped twice; queue %v", q)
	}
	u, _ := s.Get(ctx, "alpha")
	if u.Score == nil || *u.Score < 7 || *u.Category != "word" {
		t.Fatalf("score not saved: %+v", u)
	}
	st, err := s.Stats(ctx)
	if err != nil || st.Total != 2 || st.Scored != 1 || st.Good != 1 {
		t.Fatalf("stats %+v %v", st, err)
	}
}
