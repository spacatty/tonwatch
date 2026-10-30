package app

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// Names from the last sets wait while there are others, and the top of the list is not always
// what the model sees first.
func TestPickCandidates(t *testing.T) {
	noShuffle := func(int, func(int, int)) {}
	all := []string{"a", "b", "c", "d", "e", "f"}

	if got := pickCandidates(all, []string{"a", "b", "c"}, 3, noShuffle); !slices.Equal(got, []string{"d", "e", "f"}) {
		t.Errorf("recent names should wait: %v", got)
	}
	if got := pickCandidates(all, []string{"a", "b", "c"}, 5, noShuffle); !slices.Equal(got, []string{"d", "e", "f", "a", "b"}) {
		t.Errorf("recent names fill up when there are too few others: %v", got)
	}
	if got := pickCandidates(all[:2], nil, 5, noShuffle); len(got) != 2 {
		t.Errorf("a small pool is returned whole: %v", got)
	}

	r := rand.New(rand.NewPCG(1, 2))
	firsts := map[string]bool{}
	for range 20 {
		firsts[pickCandidates(all, nil, 6, r.Shuffle)[0]] = true
	}
	if len(firsts) < 3 {
		t.Errorf("the order should change between sets, saw first names %v", firsts)
	}
}
