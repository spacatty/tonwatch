package app

import (
	"net/http"
	"strconv"
	"time"
)

// FreshLimit is the most names one read of the fresh feed returns.
const FreshLimit = 100

// listFresh is the live feed: names the collectors saw for the first time, newest first, with
// the table's filters plus venue (where it is on offer). The page polls it with since, the
// first_seen to read back to, and merges what comes back; free accounts get the same gate as the
// table. No total: counting the whole match on every poll would cost more than the page itself.
func (a *App) listFresh(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := parseFilter(q)
	f.Rules, f.QuietDays, f.Keywords = nil, 0, nil // the feed has no power filters of its own
	f.Venues = list(q.Get("venue"))
	f.SeenSince, _ = strconv.ParseInt(q.Get("since"), 10, 64)
	f.Sort, f.Dir, f.Offset, f.NoCount = "recent", "", 0, true
	if f.Limit <= 0 || f.Limit > FreshLimit {
		f.Limit = FreshLimit
	}
	locked := a.gate(r.Context(), &f)
	items, _, err := a.query(r.Context(), f)
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	out := map[string]any{"items": items, "now": time.Now().Unix()}
	if locked != nil {
		out["locked"] = locked
	}
	writeJSON(w, out)
}
