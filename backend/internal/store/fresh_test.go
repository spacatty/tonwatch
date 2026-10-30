package store

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// The fresh feed: names first seen since a cursor, newest first, narrowed by where they are on offer.
func TestFreshFeedQuery(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	if err := s.UpsertFragment(ctx, []FragRow{
		{Name: "oldauction", Status: "auction", Price: fp(10)},
		{Name: "newauction", Status: "auction", Price: fp(10)},
		{Name: "newsale", Status: "sale", Price: fp(20)},
		{Name: "gemslisted", Status: "sold", Price: fp(30)},
		{Name: "appslisted", Status: "sold", Price: fp(30)},
		{Name: "otherlisted", Status: "sold", Price: fp(30)},
		{Name: "nomarket", Status: "sold", Price: fp(30)},
		{Name: "sleeping", Status: "sold", Price: fp(30)},
	}); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Unix() - 3600
	// first_seen in the order the collectors "found" them, a minute apart
	for i, n := range []string{"oldauction", "sleeping", "nomarket", "otherlisted", "appslisted", "gemslisted", "newsale", "newauction"} {
		s.DB.ExecContext(ctx, `UPDATE usernames SET first_seen=? WHERE name=?`, t0+int64(i)*60, n)
	}
	// status is derived: sold on Fragment and listed on-chain reads as market, not listed as owned
	for n, m := range map[string]any{"gemslisted": "Getgems", "appslisted": "Marketapp Marketplace", "otherlisted": "webdom.ton", "nomarket": nil} {
		s.DB.ExecContext(ctx, `UPDATE usernames SET chain_on_sale=1, chain_market=? WHERE name=?`, m, n)
	}

	names := func(f Filter) []string {
		f.Sort = "recent"
		items, total, err := s.Query(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		if f.NoCount && total != -1 {
			t.Errorf("NoCount still counted: %d", total)
		}
		var out []string
		for _, u := range items {
			out = append(out, u.Name)
		}
		return out
	}

	all := []string{"newauction", "newsale", "gemslisted", "appslisted", "otherlisted", "nomarket", "sleeping", "oldauction"}
	if got := names(Filter{NoCount: true}); !slices.Equal(got, all) {
		t.Errorf("newest first: %v", got)
	}
	// the cursor is inclusive: a name first seen in the cursor's own second still comes back
	if got := names(Filter{SeenSince: t0 + 6*60}); !slices.Equal(got, []string{"newauction", "newsale"}) {
		t.Errorf("since: %v", got)
	}
	for venues, want := range map[string][]string{
		"auction":         {"newauction", "oldauction"},
		"sale":            {"newsale"},
		"getgems":         {"gemslisted"},
		"marketapp":       {"appslisted"},
		"market":          {"otherlisted", "nomarket"},
		"owned":           {"sleeping"},
		"getgems,auction": {"newauction", "gemslisted", "oldauction"},
		"bogus":           nil,
	} {
		if got := names(Filter{Venues: strings.Split(venues, ",")}); !slices.Equal(got, want) {
			t.Errorf("venue %s: %v, want %v", venues, got, want)
		}
	}
	if got := names(Filter{Venues: []string{"sale", "market"}, SeenSince: t0 + 3*60}); !slices.Equal(got, []string{"newsale", "otherlisted"}) {
		t.Errorf("venue and since together: %v", got)
	}
}
