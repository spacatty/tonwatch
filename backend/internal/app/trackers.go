package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"fragwatch/internal/store"
)

func (a *App) trackerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/trackers", a.listTrackers)
	mux.HandleFunc("POST /api/trackers", a.createTracker)
	mux.HandleFunc("PUT /api/trackers/{id}", a.updateTracker)
	mux.HandleFunc("DELETE /api/trackers/{id}", a.deleteTracker)
	mux.HandleFunc("POST /api/trackers/{id}/seen", a.seenTracker)
}

// listTrackers returns every tracker with its match count and how many matches are new
// (first seen, listed or repriced) since it was last opened.
func (a *App) listTrackers(w http.ResponseWriter, r *http.Request) {
	ts, err := a.Store.Trackers(r.Context(), uid(r.Context()))
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	for i := range ts {
		q, _ := url.ParseQuery(ts[i].Params)
		f := parseFilter(q)
		a.gate(r.Context(), &f)
		ts[i].Total, _ = a.count(r.Context(), f)
		f.ChangedSince = ts[i].SeenAt
		ts[i].New, _ = a.count(r.Context(), f)
	}
	writeJSON(w, ts)
}

type trackerBody struct {
	Name    *string `json:"name"`
	View    *string `json:"view"`
	Filters *string `json:"filters"`
	Params  *string `json:"params"`
}

func (a *App) createTracker(w http.ResponseWriter, r *http.Request) {
	var b trackerBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.Name == nil || strings.TrimSpace(*b.Name) == "" || b.Params == nil {
		httpErr(w, apiError("name and params are required"), 400)
		return
	}
	if badRules(w, *b.Params) {
		return
	}
	if !currentUser(r.Context()).Unlocked() {
		if ts, _ := a.Store.Trackers(r.Context(), uid(r.Context())); len(ts) >= FreeTrackers {
			httpErr(w, apiError("more trackers unlock after your first top-up"), 402)
			return
		}
	}
	t := store.Tracker{Name: clip(*b.Name, 80), View: deref(b.View, "all"), Filters: deref(b.Filters, "{}"), Params: *b.Params}
	id, err := a.Store.CreateTracker(r.Context(), uid(r.Context()), t)
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	a.Store.Log(r.Context(), uid(r.Context()), 0, "tracker.create", t.Name)
	writeJSON(w, map[string]int64{"id": id})
}

func deref(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}

func trackerID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (a *App) updateTracker(w http.ResponseWriter, r *http.Request) {
	id, err := trackerID(r)
	var b trackerBody
	if err == nil {
		err = json.NewDecoder(r.Body).Decode(&b)
	}
	if err != nil {
		httpErr(w, err, 400)
		return
	}
	if b.Params != nil && badRules(w, *b.Params) {
		return
	}
	if err := a.Store.UpdateTracker(r.Context(), uid(r.Context()), id, b.Name, b.View, b.Filters, b.Params); err != nil {
		httpErr(w, notFound(err), statusOf(err))
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *App) deleteTracker(w http.ResponseWriter, r *http.Request) {
	id, err := trackerID(r)
	if err != nil {
		httpErr(w, err, 400)
		return
	}
	if err := a.Store.DeleteTracker(r.Context(), uid(r.Context()), id); err != nil {
		httpErr(w, notFound(err), statusOf(err))
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *App) seenTracker(w http.ResponseWriter, r *http.Request) {
	id, err := trackerID(r)
	if err == nil {
		err = a.Store.TrackerSeen(r.Context(), uid(r.Context()), id)
	}
	if err != nil {
		httpErr(w, err, 400)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errNotFound
	}
	return err
}

func statusOf(err error) int {
	if errors.Is(err, sql.ErrNoRows) {
		return 404
	}
	return 500
}
