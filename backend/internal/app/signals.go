package app

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Behaviour signals teach the taste for free: a star, a buy click or a long look is a soft like,
// a dismissed card a soft skip. They are folded into the profile next to the explicit picks, and
// the profile is only rebuilt once the soft set has drifted enough to matter.
const (
	kvSignals   = "taste:signals"
	softLikeMin = 2 // weight a name needs to count as a soft like
	softLimit   = 20
	softDrift   = 3 // names that have to come or go before the profile is rebuilt
)

var signalWeight = map[string]int{"buy": 3, "star": 2, "open": 1, "dismiss": -3}

type signal struct {
	W  int   `json:"w"`
	TS int64 `json:"ts"`
}

func (a *App) loadSignals(ctx context.Context) map[string]signal {
	m := map[string]signal{}
	if v := a.uget(ctx, kvSignals); v != "" {
		json.Unmarshal([]byte(v), &m)
	}
	return m
}

// softTaste splits the signals into soft likes (strongest first) and soft skips, leaving out
// names with an explicit verdict.
func softTaste(sig map[string]signal, t Taste) (likes, skips []string) {
	explicit := map[string]bool{}
	for _, n := range append(append([]string{}, t.Likes...), t.Skips...) {
		explicit[n] = true
	}
	type kv struct {
		n string
		s signal
	}
	var all []kv
	for n, s := range sig {
		if !explicit[n] {
			all = append(all, kv{n, s})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].s.W != all[j].s.W {
			return all[i].s.W > all[j].s.W
		}
		return all[i].s.TS > all[j].s.TS
	})
	for _, x := range all {
		if x.s.W >= softLikeMin && len(likes) < softLimit {
			likes = append(likes, x.n)
		} else if x.s.W < 0 && len(skips) < softLimit {
			skips = append(skips, x.n)
		}
	}
	return likes, skips
}

// drift counts names that are in one soft set and not the other.
func drift(a, b []string) int {
	in := map[string]bool{}
	for _, n := range a {
		in[n] = true
	}
	d := 0
	for _, n := range b {
		if !in[n] {
			d++
		} else {
			delete(in, n)
		}
	}
	return d + len(in)
}

// postSignal records {"name":"x","kind":"star"|"buy"|"open"|"dismiss"}.
func (a *App) postSignal(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpErr(w, err, 400)
		return
	}
	name := cleanName(b.Name)
	if _, ok := signalWeight[b.Kind]; name == "" || !ok {
		httpErr(w, apiError("name and a known kind are required"), 400)
		return
	}
	likes, skips := a.recordSignal(r.Context(), name, b.Kind)
	if likes == nil {
		likes = []string{}
	}
	if skips == nil {
		skips = []string{}
	}
	writeJSON(w, map[string]any{"soft_likes": likes, "soft_skips": skips})
}

var signalsMu sync.Mutex

// recordSignal adds one signal's weight and returns the soft taste after it.
func (a *App) recordSignal(ctx context.Context, name, kind string) (likes, skips []string) {
	signalsMu.Lock()
	defer signalsMu.Unlock()
	m := a.loadSignals(ctx)
	s := m[name]
	s.W = min(9, max(-9, s.W+signalWeight[kind]))
	s.TS = time.Now().Unix()
	m[name] = s
	// keep the map bounded: drop the oldest zero-weight entries past 400
	if len(m) > 400 {
		type kv struct {
			n string
			s signal
		}
		var all []kv
		for n, s := range m {
			all = append(all, kv{n, s})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].s.TS < all[j].s.TS })
		for _, x := range all[:len(all)-400] {
			delete(m, x.n)
		}
	}
	js, _ := json.Marshal(m)
	a.uset(ctx, kvSignals, string(js))
	return softTaste(m, a.loadTaste(ctx))
}

// softLine is what the profile prompt gets from behaviour.
func softLine(likes, skips []string) string {
	var parts []string
	if len(likes) > 0 {
		parts = append(parts, "Starred, bought or lingered on (weaker signal than a like): "+strings.Join(likes, ", "))
	}
	if len(skips) > 0 {
		parts = append(parts, "Dismissed from the home screen: "+strings.Join(skips, ", "))
	}
	return strings.Join(parts, "\n")
}
