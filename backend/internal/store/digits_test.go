package store

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
)

func TestPermutations(t *testing.T) {
	if got := permutations("552"); !slices.Equal(got, []string{"255", "525", "552"}) {
		t.Fatalf("got %v", got)
	}
}

func TestDigitSearch(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	ctx := context.Background()
	for _, n := range []string{"ws552", "top525", "a255b", "x5y5z2", "five52", "plain", "user_55_2"} {
		if err := s.EnsureName(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	for mode, want := range map[string][]string{
		"row":       {"user_55_2", "ws552"},
		"any":       {"a255b", "top525", "user_55_2", "ws552"},
		"scattered": {"a255b", "top525", "user_55_2", "ws552", "x5y5z2"},
	} {
		items, _, err := s.Query(ctx, Filter{Digits: "552", DigitMode: mode, Sort: "name"})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, u := range items {
			got = append(got, u.Name)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", mode, got, want)
		}
	}
}
