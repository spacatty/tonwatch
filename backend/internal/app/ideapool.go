package app

import (
	"context"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"fragwatch/internal/store"
)

// Where the names for a set of ideas come from. The person picks one above the Generate button:
//   - taste: liked names on sale, the top of each tracker, the preferred kinds, then the best value
//     within the sniper settings (the default, and what it always was);
//   - favorites: liked, starred and soft-liked names that are on sale, at any price;
//   - snipes: names within the sniper settings only;
//   - tracker:<id>: one tracker's matches.
//
// However big the pool, the model sees `candidates` names. Names used in the last few concepts
// are held back and the rest is shuffled, so pressing "New set" again brings other names instead
// of the same best-value top.
const (
	recentConcepts = 25 // ~5 sets: their names wait while there are others
	poolDepth      = 3  // read this many times `candidates` from a big source before picking
)

type ideaSourceSpec struct {
	id      string // as stored and sent: taste | favorites | snipes | tracker:12
	kind    string // taste | favorites | snipes | tracker
	tracker int64
}

var tooFew = map[string]string{
	"taste":     "too few names within your settings; raise the budget or lower the minimum score",
	"snipes":    "too few names within your settings; raise the budget or lower the minimum score",
	"favorites": "too few of your liked or starred names are on sale; pick another source",
	"tracker":   "this tracker has too few names on sale; pick another source",
}

// ideaSource reads the requested source, falling back to the last one picked, and remembers it.
func (a *App) ideaSource(ctx context.Context, want string) ideaSourceSpec {
	stored := a.uget(ctx, kvIdeasSrc)
	if want == "" {
		want = stored
	}
	spec := parseIdeaSource(want)
	if spec.kind == "tracker" {
		if tr, _ := a.Store.Trackers(ctx, uid(ctx)); !hasTracker(tr, spec.tracker) {
			spec = parseIdeaSource("")
		}
	}
	if spec.id != stored {
		a.uset(ctx, kvIdeasSrc, spec.id)
	}
	return spec
}

func parseIdeaSource(v string) ideaSourceSpec {
	switch v {
	case "favorites", "snipes":
		return ideaSourceSpec{id: v, kind: v}
	}
	if rest, ok := strings.CutPrefix(v, "tracker:"); ok {
		if id, err := strconv.ParseInt(rest, 10, 64); err == nil && id > 0 {
			return ideaSourceSpec{id: v, kind: "tracker", tracker: id}
		}
	}
	return ideaSourceSpec{id: "taste", kind: "taste"}
}

func hasTracker(ts []store.Tracker, id int64) bool {
	for _, t := range ts {
		if t.ID == id {
			return true
		}
	}
	return false
}

type ideaPoolOut struct {
	byName map[string]store.Username
	order  []string // what the model sees, at most `candidates`
	total  int      // how many names the source holds on sale
}

// ideaPool gathers the names for one set from the chosen source.
func (a *App) ideaPool(ctx context.Context, s Settings, t Taste, src ideaSourceSpec) ideaPoolOut {
	skip := map[string]bool{}
	for _, n := range append(append([]string{}, t.Skips...), t.SoftSkips...) {
		skip[n] = true
	}
	out := ideaPoolOut{byName: map[string]store.Username{}}
	var all []string
	add := func(items []store.Username, limit int) {
		n := 0
		for _, u := range items {
			if skip[u.Name] || u.Buyout == nil {
				continue
			}
			if _, ok := out.byName[u.Name]; ok {
				continue
			}
			out.byName[u.Name] = u
			all = append(all, u.Name)
			if n++; n == limit {
				return
			}
		}
	}
	onSale := func(f store.Filter) store.Filter {
		f.Statuses, f.Live, f.BuyNow, f.Sort, f.Offset = []string{"auction", "sale", "market"}, true, true, "value", 0
		return f
	}
	minScore := s.MinScore
	base := onSale(store.Filter{MinScore: &minScore, HideJunk: true, Limit: candidates * poolDepth})
	if s.MaxBuyout > 0 {
		base.MaxBuyout = &s.MaxBuyout
	}

	switch src.kind {
	case "favorites":
		for _, n := range a.followedNames(ctx) {
			if u, _ := a.Store.Get(ctx, n); u != nil && u.Buyout != nil {
				add([]store.Username{*u}, 1)
			}
		}
		out.total = len(all)
	case "snipes":
		if items, total, err := a.query(ctx, base); err == nil {
			add(items, len(items))
			out.total = total
		}
	case "tracker":
		trackers, _ := a.Store.Trackers(ctx, uid(ctx))
		for _, tr := range trackers {
			if tr.ID != src.tracker {
				continue
			}
			q, _ := url.ParseQuery(tr.Params)
			f := onSale(parseFilter(q))
			f.Limit = candidates * poolDepth
			a.gate(ctx, &f) // a locked account reads its tracker as it does on the dashboard
			if items, total, err := a.query(ctx, f); err == nil {
				add(items, len(items))
				out.total = total
			}
		}
	default: // taste
		// liked names still on sale come first: they are the taste made concrete
		for _, n := range append(append([]string{}, t.Likes...), t.SoftLikes...) {
			if u, _ := a.Store.Get(ctx, n); u != nil && u.Buyout != nil && (s.MaxBuyout == 0 || *u.Buyout <= s.MaxBuyout) {
				add([]store.Username{*u}, 1)
			}
		}
		// the top of each tracker
		if trackers, err := a.Store.Trackers(ctx, uid(ctx)); err == nil {
			for _, tr := range trackers {
				q, _ := url.ParseQuery(tr.Params)
				f := onSale(parseFilter(q))
				f.Limit = 12
				if s.MaxBuyout > 0 && f.MaxBuyout == nil {
					f.MaxBuyout = &s.MaxBuyout
				}
				if items, _, err := a.query(ctx, f); err == nil {
					add(items, 8)
				}
			}
		}
		if len(s.Kinds) > 0 {
			k := base
			k.Categories = s.Kinds
			if items, _, err := a.query(ctx, k); err == nil {
				add(items, candidates)
			}
		}
		if items, total, err := a.query(ctx, base); err == nil {
			add(items, len(items))
			out.total = max(total, len(all))
		}
	}

	recent, _ := a.Store.RecentConceptNames(ctx, uid(ctx), recentConcepts)
	out.order = pickCandidates(all, recent, candidates, rand.Shuffle)
	return out
}

// pickCandidates takes up to n names: the ones not used lately first, the rest only to fill up,
// each group shuffled so the model is not drawn to whatever sits at the top of the list.
func pickCandidates(all, recent []string, n int, shuffle func(int, func(int, int))) []string {
	used := map[string]bool{}
	for _, r := range recent {
		used[r] = true
	}
	var fresh, again []string
	for _, name := range all {
		if used[name] {
			again = append(again, name)
		} else {
			fresh = append(fresh, name)
		}
	}
	shuffle(len(fresh), func(i, j int) { fresh[i], fresh[j] = fresh[j], fresh[i] })
	shuffle(len(again), func(i, j int) { again[i], again[j] = again[j], again[i] })
	out := append(fresh, again...)
	return out[:min(len(out), n)]
}

// getIdeaPool: how many names a source holds and how many the model would see, for the picker.
func (a *App) getIdeaPool(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	src := parseIdeaSource(r.URL.Query().Get("source"))
	pool := a.ideaPool(ctx, a.settings(ctx), a.loadTaste(ctx), src)
	writeJSON(w, map[string]any{"source": src.id, "total": pool.total, "sees": len(pool.order)})
}
