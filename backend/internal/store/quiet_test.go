package store

import (
	"context"
	"testing"
)

// "No bids for N days": a name counts once it has been tracked that long and its price has not
// moved in that time.
func TestQuietDays(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	fp := func(v float64) *float64 { return &v }
	for _, n := range []string{"sleepy", "busy", "fresh"} {
		if err := s.UpsertFragment(ctx, []FragRow{{Name: n, Status: "auction", Price: fp(10), FromSrc: SrcFragmentList}}); err != nil {
			t.Fatal(err)
		}
	}
	week := now() - 7*86400
	// sleepy and busy have been around for a week; fresh only just appeared
	s.DB.ExecContext(ctx, `UPDATE usernames SET first_seen=? WHERE name IN ('sleepy','busy')`, week)
	s.DB.ExecContext(ctx, `UPDATE events SET ts=? WHERE name IN ('sleepy','busy')`, week)
	// busy got a bid yesterday
	s.DB.ExecContext(ctx, `INSERT INTO events(name, ts, kind, detail) VALUES('busy', ?, 'price', '10 → 12 TON')`, now()-86400)

	items, _, err := s.Query(ctx, Filter{QuietDays: 5, Sort: "name"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "sleepy" {
		names := []string{}
		for _, u := range items {
			names = append(names, u.Name)
		}
		t.Fatalf("only sleepy went 5 days without a bid, got %v", names)
	}
}
