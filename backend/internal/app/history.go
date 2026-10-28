package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fragwatch/internal/store"
)

// Concept history: every idea ever generated is kept, browsable by name, and fed back as
// "already made" so later runs stay different. Uniqueness is per installation: the names are
// what is scarce, a project idea is not, so two users getting a similar concept for different
// names costs nobody anything. A hosted version could add a shared fingerprint of titles per name.
const (
	historyLimit = 300
	avoidRecent  = 40 // titles from history the next set is told to steer clear of
)

func (a *App) historyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/ideas/history", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > historyLimit {
			limit = historyLimit
		}
		cs, err := a.Store.Concepts(r.Context(), uid(r.Context()), cleanName(r.URL.Query().Get("name")), limit)
		if err != nil {
			httpErr(w, err, 500)
			return
		}
		for i := range cs {
			cs[i].Chat = len(a.loadChat(r.Context(), cs[i].ID))
		}
		writeJSON(w, map[string]any{"concepts": cs})
	})
	mux.HandleFunc("DELETE /api/ideas/history/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			httpErr(w, err, 400)
			return
		}
		if err := a.Store.DeleteConcept(r.Context(), uid(r.Context()), id); err != nil {
			httpErr(w, notFound(err), statusOf(err))
			return
		}
		a.uset(r.Context(), chatKey(id), "")
		writeJSON(w, map[string]bool{"ok": true})
	})
	// {"verdict":"like"|"dislike"|"clear"}: liked concepts steer later runs, disliked ones are avoided
	mux.HandleFunc("POST /api/ideas/history/{id}/verdict", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			httpErr(w, err, 400)
			return
		}
		var b struct {
			Verdict string `json:"verdict"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			httpErr(w, err, 400)
			return
		}
		v := map[string]int{"like": 1, "dislike": -1}[b.Verdict]
		if err := a.Store.SetConceptLiked(r.Context(), uid(r.Context()), id, v); err != nil {
			httpErr(w, notFound(err), statusOf(err))
			return
		}
		writeJSON(w, map[string]int{"liked": v})
	})
	mux.HandleFunc("POST /api/ideas/imagine", a.imagine)
	mux.HandleFunc("GET /api/ideas/chat/{id}", a.getConceptChat)
	mux.HandleFunc("POST /api/ideas/chat/{id}", a.postConceptChat)
}

// remember writes one generated concept to the history.
func (a *App) remember(ctx context.Context, source, note string, o *ideaOut) {
	c := store.Concept{Source: source, Project: o.Project, Pitch: o.Pitch, FirstMove: o.FirstMove, BuiltWith: o.BuiltWith, Note: note}
	for _, it := range o.Items {
		c.Names = append(c.Names, store.ConceptName{Name: it.Item.Name, Angle: it.Angle})
	}
	o.ID, _ = a.Store.AddConcept(ctx, uid(ctx), c)
}

// imagine: one full project for one name, {"name":"x","note":"optional steer"}. Prior projects for
// the name are passed as already made, so each press gives a different one.
func (a *App) imagine(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name string `json:"name"`
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || strings.TrimSpace(b.Name) == "" {
		httpErr(w, apiError("name is required"), 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	name := cleanName(b.Name)
	u, _ := a.Store.Get(ctx, name)
	if u == nil {
		httpErr(w, apiError("unknown name"), 404)
		return
	}
	if a.paywall(w, r, "imagine") {
		return
	}
	t := a.taste(ctx)
	s := a.settings(ctx)
	profile := t.Profile
	if profile == "" && s.About != "" {
		profile = "About them: " + s.About
	}
	profile = withConceptTaste(ctx, a, profile)
	prior, _ := a.Store.ConceptTitles(ctx, uid(ctx), name, 12)
	note := clip(b.Note, 300)
	im, usage, err := a.LLM.Imagine(ctx, profile, name, deref(u.Meaning, "-"), priceLine(u), prior, note)
	a.addUsage(usage)
	if err != nil {
		httpErr(w, err, 502)
		return
	}
	a.charge(ctx, "imagine", usage, name)
	c := store.Concept{Source: "imagine", Project: im.Project, Pitch: im.Pitch, FirstMove: im.FirstMove, Form: im.Form, How: im.How, BuiltWith: im.BuiltWith, Note: note,
		Names: []store.ConceptName{{Name: name, Angle: fmt.Sprintf("fit %d/10", im.Fit)}}}
	c.ID, _ = a.Store.AddConcept(ctx, uid(ctx), c)
	c.TS = time.Now().Unix()
	// the "why" cache for this name is now stale in the best way: the concept is richer
	writeJSON(w, map[string]any{"concept": c, "fit": im.Fit, "usage": usage, "credits": a.balance(ctx)})
}
