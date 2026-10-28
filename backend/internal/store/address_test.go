package store

import "testing"

func TestFriendlyAddress(t *testing.T) {
	// The collection address from tonapi: raw 0:80d78a… == EQCA14o1-VWhS2efqoh_9M1b_A9DtKTuoqfmkn83AbJzwnPi
	got := FriendlyAddress("0:80d78a35f955a14b679faa887ff4cd5bfc0f43b4a4eea2a7e6927f3701b273c2")
	if got != "EQCA14o1-VWhS2efqoh_9M1b_A9DtKTuoqfmkn83AbJzwnPi" {
		t.Fatalf("got %s", got)
	}
}

func TestIsJunk(t *testing.T) {
	for _, n := range []string{"vip8989d", "aaattttttttttttt", "xqpppppp", "c8cccccccccccccc", "n_nnnnn_nnn_nn"} {
		if !IsJunk(n) {
			t.Errorf("%s should be junk", n)
		}
	}
	for _, n := range []string{"pizza", "darkmoon", "crypto90", "agent007", "aaaaa", "8888", "maria_art", "hotelchelsea"} {
		if IsJunk(n) {
			t.Errorf("%s should not be junk", n)
		}
	}
}
