package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"fragwatch/internal/i18n"
	"fragwatch/internal/llm"
	"fragwatch/internal/store"
)

// Settings is the sniper configuration: what the home screen and the AI treat as "worth it".
type Settings struct {
	MaxBuyout    float64  `json:"max_buyout"`    // 0 = no cap
	MinScore     int      `json:"min_score"`     // names rated below this are ignored
	Kinds        []string `json:"kinds"`         // preferred kinds; empty = any
	DailyCredits int      `json:"daily_credits"` // free credits per day (read-only: the install's DAILY_CREDITS)
	About        string   `json:"about"`         // who they are / what they build, for the AI
	Avoid        string   `json:"avoid"`         // negative prompt: kinds of project and name they never want
	Plan         string   `json:"plan"`          // "free" | "pro" (read-only: bought, or granted by an admin)
	TelegramChat string   `json:"telegram_chat"` // where alerts go (read-only: their linked Telegram)
}

var defaultSettings = Settings{MaxBuyout: 100, MinScore: 7, Kinds: []string{}, Plan: "free"}

// Taste is what they liked and skipped in the picker, plus the cached profile built from it.
// SoftLikes/SoftSkips come from behaviour (see signals.go) and are read-only here.
type Taste struct {
	Likes       []string `json:"likes"`
	Skips       []string `json:"skips"`
	SoftLikes   []string `json:"soft_likes"`
	SoftSkips   []string `json:"soft_skips"`
	Profile     string   `json:"profile"`
	ProfileKey  string   `json:"-"`
	ProfileSoft []string `json:"-"` // the soft likes the profile was built from
}

const (
	kvSettings = "settings"
	kvTaste    = "taste"
	kvIdeas    = "ideas:last"
	maxLikes   = 60
	maxSkips   = 120
	ideaCount  = 5
	candidates = 110 // names the model sees per run: ~1.5k tokens
)

func (a *App) studioRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.settings(r.Context())) })
	mux.HandleFunc("PUT /api/settings", a.putSettings)
	mux.HandleFunc("GET /api/taste", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.taste(r.Context())) })
	mux.HandleFunc("POST /api/taste", a.postTaste)
	mux.HandleFunc("GET /api/taste/sample", a.tasteSample)
	mux.HandleFunc("GET /api/ideas", a.getIdeas)
	mux.HandleFunc("POST /api/ideas", a.postIdeas)
	mux.HandleFunc("POST /api/ai/angle", a.postAngle)
	mux.HandleFunc("POST /api/ideas/rotate", a.rotateIdea)
	mux.HandleFunc("POST /api/ai/angles", a.postAngles)
	mux.HandleFunc("POST /api/taste/signal", a.postSignal)
	a.creditsRoutes(mux)
	a.historyRoutes(mux)
	a.alertRoutes(mux)
	a.portfolioRoutes(mux)
}

func (a *App) settings(ctx context.Context) Settings {
	s := defaultSettings
	if v := a.uget(ctx, kvSettings); v != "" {
		json.Unmarshal([]byte(v), &s)
	}
	if s.Kinds == nil {
		s.Kinds = []string{}
	}
	// read-only here: the allowance is the install's, the plan is bought, the chat is their Telegram
	s.DailyCredits, s.Plan, s.TelegramChat = a.Cfg.DailyCredits, "free", ""
	if u := currentUser(ctx); u != nil {
		s.Plan = u.Plan()
		if u.TgID != 0 {
			s.TelegramChat = strconv.FormatInt(u.TgID, 10)
		}
	}
	return s
}

func (a *App) putSettings(w http.ResponseWriter, r *http.Request) {
	s := a.settings(r.Context())
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		httpErr(w, err, 400)
		return
	}
	s.MaxBuyout = max(0, s.MaxBuyout)
	s.MinScore = min(10, max(0, s.MinScore))
	s.About = strings.TrimSpace(s.About)
	if len(s.About) > 400 {
		s.About = s.About[:400]
	}
	s.Avoid = clip(s.Avoid, 400)
	if s.Kinds == nil {
		s.Kinds = []string{}
	}
	js, _ := json.Marshal(s)
	a.uset(r.Context(), kvSettings, string(js))
	writeJSON(w, a.settings(r.Context()))
}

func (a *App) taste(ctx context.Context) Taste {
	t := a.loadTaste(ctx)
	// the profile only counts while it was built from the current picks
	if !a.profileFresh(t, a.settings(ctx).About) {
		t.Profile = ""
	}
	return t
}

// profileFresh: the explicit picks are unchanged and the soft likes have not drifted by more
// than a few names since the profile was written.
func (a *App) profileFresh(t Taste, about string) bool {
	return t.Profile != "" && t.ProfileKey == tasteKey(t, about) && drift(t.ProfileSoft, t.SoftLikes) < softDrift
}

func tasteKey(t Taste, about string) string {
	h := sha1.Sum([]byte(strings.Join(t.Likes, ",") + "|" + strings.Join(t.Skips, ",") + "|" + about))
	return hex.EncodeToString(h[:8])
}

func (a *App) saveTaste(ctx context.Context, t Taste) {
	// ProfileKey is not exported in JSON; store it alongside
	js, _ := json.Marshal(struct {
		Taste
		Key  string   `json:"profile_key"`
		Soft []string `json:"profile_soft"`
	}{t, t.ProfileKey, t.ProfileSoft})
	a.uset(ctx, kvTaste, string(js))
}

func (a *App) loadTaste(ctx context.Context) Taste {
	var raw struct {
		Taste
		Key  string   `json:"profile_key"`
		Soft []string `json:"profile_soft"`
	}
	if v := a.uget(ctx, kvTaste); v != "" {
		json.Unmarshal([]byte(v), &raw)
	}
	t := raw.Taste
	t.ProfileKey, t.ProfileSoft = raw.Key, raw.Soft
	if t.Likes == nil {
		t.Likes = []string{}
	}
	if t.Skips == nil {
		t.Skips = []string{}
	}
	t.SoftLikes, t.SoftSkips = softTaste(a.loadSignals(ctx), t)
	if t.SoftLikes == nil {
		t.SoftLikes = []string{}
	}
	if t.SoftSkips == nil {
		t.SoftSkips = []string{}
	}
	return t
}

// buildProfile refreshes the cached taste profile when it is stale and there is enough to go on
// (three explicit likes, or six soft ones). Returns the profile to use, possibly empty.
func (a *App) buildProfile(ctx context.Context, t *Taste, s Settings) (string, error) {
	enough := len(t.Likes) >= 3 || len(t.SoftLikes) >= 6
	if enough && !a.profileFresh(*t, s.About) {
		p, usage, err := a.LLM.TasteProfile(ctx, t.Likes, t.Skips, s.About, softLine(t.SoftLikes, t.SoftSkips))
		a.addUsage(usage)
		if err != nil {
			return "", fmt.Errorf("taste profile: %w", err)
		}
		t.Profile, t.ProfileKey, t.ProfileSoft = p, tasteKey(*t, s.About), append([]string{}, t.SoftLikes...)
		a.saveTaste(ctx, *t)
		a.charge(ctx, "profile", usage, "")
	}
	profile := t.Profile
	if !a.profileFresh(*t, s.About) {
		profile = ""
	}
	if profile == "" && s.About != "" {
		profile = "About them: " + s.About
	}
	return profile, nil
}

// postTaste records one verdict: {"name":"x","verdict":"like"|"skip"|"clear"}, or {"reset":true}.
func (a *App) postTaste(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name    string `json:"name"`
		Verdict string `json:"verdict"`
		Reset   bool   `json:"reset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpErr(w, err, 400)
		return
	}
	ctx := r.Context()
	t := a.loadTaste(ctx)
	if b.Reset {
		t = Taste{Likes: []string{}, Skips: []string{}}
	} else {
		name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(b.Name), "@"))
		if name == "" {
			httpErr(w, apiError("name is required"), 400)
			return
		}
		t.Likes = slices.DeleteFunc(t.Likes, func(n string) bool { return n == name })
		t.Skips = slices.DeleteFunc(t.Skips, func(n string) bool { return n == name })
		switch b.Verdict {
		case "like":
			t.Likes = append(t.Likes, name)
		case "skip":
			t.Skips = append(t.Skips, name)
		}
		if len(t.Likes) > maxLikes {
			t.Likes = t.Likes[len(t.Likes)-maxLikes:]
		}
		if len(t.Skips) > maxSkips {
			t.Skips = t.Skips[len(t.Skips)-maxSkips:]
		}
	}
	a.saveTaste(ctx, t)
	writeJSON(w, a.taste(ctx))
}

// tasteSample serves names to judge: available, within budget, well rated, not judged yet.
func (a *App) tasteSample(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s := a.settings(ctx)
	t := a.loadTaste(ctx)
	seen := map[string]bool{}
	for _, n := range append(append([]string{}, t.Likes...), t.Skips...) {
		seen[n] = true
	}
	minScore := max(6, s.MinScore)
	f := store.Filter{Statuses: []string{"auction", "sale", "market"}, Live: true, MinScore: &minScore, HideJunk: true, Sort: "random", Limit: 80}
	if s.MaxBuyout > 0 {
		f.MaxBuyout = &s.MaxBuyout
	}
	items, _, err := a.query(ctx, f)
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	out := []store.Username{}
	for _, u := range items {
		if !seen[u.Name] {
			out = append(out, u)
		}
		if len(out) == 24 {
			break
		}
	}
	writeJSON(w, map[string]any{"items": out})
}

// IdeasResult is the last generated set, kept so a reload costs nothing.
type IdeasResult struct {
	GeneratedAt int64     `json:"generated_at"`
	Profile     string    `json:"profile"`
	Ideas       []ideaOut `json:"ideas"`
	Searched    int       `json:"searched"`
	Usage       llm.Usage `json:"usage"`
	Credits     int       `json:"credits"` // balance after this run
	Settings    Settings  `json:"settings"`
}

type ideaOut struct {
	llm.Idea
	ID    int64      `json:"id"` // the concept's row in the history, for its chat
	Items []ideaItem `json:"items"`
}

type ideaItem struct {
	Angle string         `json:"angle"`
	Item  store.Username `json:"item"`
}

func (a *App) getIdeas(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s := a.settings(ctx)
	res := IdeasResult{Ideas: []ideaOut{}}
	if v := a.uget(ctx, kvIdeas); v != "" {
		json.Unmarshal([]byte(v), &res)
	}
	// sets saved before concepts had ids: find them in the history by title
	for i := range res.Ideas {
		if res.Ideas[i].ID == 0 {
			res.Ideas[i].ID = a.Store.ConceptID(ctx, uid(ctx), res.Ideas[i].Project)
		}
	}
	res.Credits = a.balance(ctx)
	res.Settings = s
	writeJSON(w, res)
}

// postIdeas: one model call over the best-value names within the sniper settings (plus liked names
// still on sale and the top of each tracker), steered by the cached taste profile.
func (a *App) postIdeas(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	s := a.settings(ctx)
	t := a.loadTaste(ctx)
	if a.paywall(w, r, "ideas") {
		return
	}

	// taste profile: one call, reused until the picks change
	profile, err := a.buildProfile(ctx, &t, s)
	if err != nil {
		httpErr(w, err, 502)
		return
	}

	skip := map[string]bool{}
	for _, n := range append(append([]string{}, t.Skips...), t.SoftSkips...) {
		skip[n] = true
	}
	byName := map[string]store.Username{}
	var order []string
	add := func(items []store.Username, limit int) {
		n := 0
		for _, u := range items {
			if skip[u.Name] || u.Buyout == nil {
				continue
			}
			if _, ok := byName[u.Name]; ok {
				continue
			}
			byName[u.Name] = u
			order = append(order, u.Name)
			if n++; n == limit {
				return
			}
		}
	}
	minScore := s.MinScore
	base := store.Filter{Statuses: []string{"auction", "sale", "market"}, Live: true, BuyNow: true, MinScore: &minScore, HideJunk: true, Sort: "value", Limit: candidates}
	if s.MaxBuyout > 0 {
		base.MaxBuyout = &s.MaxBuyout
	}
	// liked names still on sale come first: they are the taste made concrete
	if len(t.Likes)+len(t.SoftLikes) > 0 {
		for _, n := range append(append([]string{}, t.Likes...), t.SoftLikes...) {
			if u, _ := a.Store.Get(ctx, n); u != nil && u.Buyout != nil && (s.MaxBuyout == 0 || *u.Buyout <= s.MaxBuyout) {
				add([]store.Username{*u}, 1)
			}
		}
	}
	// the top of each tracker
	if trackers, err := a.Store.Trackers(ctx, uid(ctx)); err == nil {
		for _, tr := range trackers {
			q, _ := url.ParseQuery(tr.Params)
			f := parseFilter(q)
			f.Statuses, f.Live, f.BuyNow, f.Sort, f.Limit, f.Offset = []string{"auction", "sale", "market"}, true, true, "value", 6, 0
			if s.MaxBuyout > 0 && f.MaxBuyout == nil {
				f.MaxBuyout = &s.MaxBuyout
			}
			if items, _, err := a.query(ctx, f); err == nil {
				add(items, 4)
			}
		}
	}
	if len(s.Kinds) > 0 {
		k := base
		k.Categories = s.Kinds
		k.Limit = candidates / 2
		if items, _, err := a.query(ctx, k); err == nil {
			add(items, candidates/2)
		}
	}
	if items, _, err := a.query(ctx, base); err == nil {
		add(items, candidates)
	}
	if len(order) > candidates {
		order = order[:candidates]
	}
	if len(order) < 5 {
		httpErr(w, apiError("too few names within your settings; raise the budget or lower the minimum score"), 400)
		return
	}
	lines := make([]string, 0, len(order))
	for _, n := range order {
		u := byName[n]
		lines = append(lines, fmt.Sprintf("%s | %s | %g TON", n, deref(u.Meaning, "-"), *u.Buyout))
	}

	// what was made before stays in the history; the new set is told to go elsewhere
	avoid, _ := a.Store.ConceptTitles(ctx, uid(ctx), "", avoidRecent)
	profile = withConceptTaste(ctx, a, profile)
	ideas, usage, err := a.LLM.Ideas(ctx, profile, lines, ideaCount, avoid, "")
	a.addUsage(usage)
	if err != nil {
		httpErr(w, fmt.Errorf("ideas: %w", err), 502)
		return
	}
	a.charge(ctx, "ideas", usage, "") // the tokens are spent even if the model misbehaved

	out := attachIdeas(ideas, byName, nil)
	for i := range out {
		a.remember(ctx, "set", "", &out[i])
	}
	res := IdeasResult{GeneratedAt: time.Now().Unix(), Profile: profile, Ideas: out, Searched: len(order), Usage: usage}
	if js, err := json.Marshal(res); err == nil {
		a.uset(ctx, kvIdeas, string(js))
	}
	// the candidate lines, so one concept can be swapped later without re-searching
	if js, err := json.Marshal(lines); err == nil {
		a.uset(ctx, kvIdeas+":lines", string(js))
	}
	res.Credits = a.balance(ctx)
	res.Settings = s
	writeJSON(w, res)
}

// attachIdeas keeps concepts whose names are real candidates, never reusing a name that `used` holds.
func attachIdeas(ideas []llm.Idea, byName map[string]store.Username, used map[string]bool) []ideaOut {
	if used == nil {
		used = map[string]bool{}
	}
	out := []ideaOut{}
	for _, id := range ideas {
		o := ideaOut{Idea: id, Items: []ideaItem{}}
		for _, nm := range id.Names {
			if u, ok := byName[nm.Name]; ok && !used[nm.Name] {
				used[nm.Name] = true
				o.Items = append(o.Items, ideaItem{Angle: nm.Angle, Item: u})
			}
		}
		if len(o.Items) > 0 && strings.TrimSpace(o.Project) != "" {
			out = append(out, o)
		}
	}
	return out
}

// rotateIdea replaces concept {index} with a new one over the names the other concepts left
// unused: a small call, since the search is already done. With a "note" ("I'd say: make it about
// audio, I'm in Lisbon") the new concept follows that direction, and it is free: it makes the
// idea theirs, which is the point.
func (a *App) rotateIdea(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Index int    `json:"index"`
		Note  string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpErr(w, err, 400)
		return
	}
	b.Note = strings.TrimSpace(b.Note)
	if len(b.Note) > 300 {
		b.Note = b.Note[:300]
	}
	feature := "rotate"
	if b.Note != "" {
		feature = "refine"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	var res IdeasResult
	var lines []string
	json.Unmarshal([]byte(a.uget(ctx, kvIdeas)), &res)
	json.Unmarshal([]byte(a.uget(ctx, kvIdeas+":lines")), &lines)
	if b.Index < 0 || b.Index >= len(res.Ideas) || len(lines) == 0 {
		httpErr(w, apiError("nothing to rotate; generate ideas first"), 400)
		return
	}
	if a.paywall(w, r, feature) {
		return
	}
	// names and titles the kept concepts hold, so the new one goes elsewhere. A refinement keeps
	// the concept's own names on the table: it is the same idea, bent their way.
	used := map[string]bool{}
	var avoid []string
	for i, id := range res.Ideas {
		if i == b.Index {
			if b.Note == "" {
				avoid = append(avoid, id.Project)
			}
			continue
		}
		avoid = append(avoid, id.Project)
		for _, it := range id.Items {
			used[it.Item.Name] = true
		}
	}
	if hist, _ := a.Store.ConceptTitles(ctx, uid(ctx), "", avoidRecent); len(hist) > 0 {
		avoid = append(avoid, hist...)
	}
	byName := map[string]store.Username{}
	var free []string
	for _, ln := range lines {
		name, _, _ := strings.Cut(ln, " | ")
		if used[name] {
			continue
		}
		if u, _ := a.Store.Get(ctx, name); u != nil && u.Buyout != nil {
			byName[name] = *u
			free = append(free, ln)
		}
	}
	if len(free) < 3 {
		httpErr(w, apiError("too few unused names left; generate a new set"), 400)
		return
	}
	note := b.Note
	if note != "" {
		note = fmt.Sprintf("Rework the concept %q (%s) the way they ask: %s", res.Ideas[b.Index].Project, res.Ideas[b.Index].Pitch, b.Note)
	}
	ideas, usage, err := a.LLM.Ideas(ctx, withConceptTaste(ctx, a, res.Profile), free, 1, avoid, note)
	a.addUsage(usage)
	if err != nil {
		httpErr(w, fmt.Errorf("rotate: %w", err), 502)
		return
	}
	a.charge(ctx, feature, usage, res.Ideas[b.Index].Project)
	fresh := attachIdeas(ideas, byName, nil)
	if len(fresh) == 0 {
		httpErr(w, apiError("the model returned nothing usable; try again"), 502)
		return
	}
	a.remember(ctx, feature, b.Note, &fresh[0])
	res.Ideas[b.Index] = fresh[0]
	res.Usage.PromptTokens += usage.PromptTokens
	res.Usage.CompletionTokens += usage.CompletionTokens
	res.Usage.Cost += usage.Cost
	if js, err := json.Marshal(res); err == nil {
		a.uset(ctx, kvIdeas, string(js))
	}
	res.Credits = a.balance(ctx)
	res.Settings = a.settings(ctx)
	writeJSON(w, res)
}

// postAngle: "why this name", for one name, through the taste profile. Cached per name and
// profile, so a card is paid for once.
func (a *App) postAngle(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || strings.TrimSpace(b.Name) == "" {
		httpErr(w, apiError("name is required"), 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(b.Name), "@"))
	u, _ := a.Store.Get(ctx, name)
	if u == nil {
		httpErr(w, apiError("unknown name"), 404)
		return
	}
	t := a.taste(ctx)
	s := a.settings(ctx)
	profile := t.Profile
	if profile == "" && s.About != "" {
		profile = "About them: " + s.About
	}
	key := angleKey(ctx, t, s, name)
	if v := a.uget(ctx, key); v != "" {
		var ang llm.Angle
		if json.Unmarshal([]byte(v), &ang) == nil {
			writeJSON(w, map[string]any{"angle": ang, "cached": true})
			return
		}
	}
	if a.paywall(w, r, "angle") {
		return
	}
	ang, usage, err := a.LLM.NameAngle(ctx, withAvoid(ctx, a, profile), name, deref(u.Meaning, "-"), priceLine(u))
	a.addUsage(usage)
	if err != nil {
		httpErr(w, err, 502)
		return
	}
	a.charge(ctx, "angle", usage, name)
	if js, err := json.Marshal(ang); err == nil {
		a.uset(ctx, key, string(js))
	}
	writeJSON(w, map[string]any{"angle": ang, "usage": usage, "credits": a.balance(ctx)})
}

// angleKey caches a read per name, taste and language (English keys stay as they were).
func angleKey(ctx context.Context, t Taste, s Settings, name string) string {
	lang := ""
	if l := i18n.From(ctx); l != i18n.EN {
		lang = l + ":"
	}
	return "angle:" + lang + tasteKey(t, s.About+"|"+s.Avoid) + ":" + name
}

func priceLine(u *store.Username) string {
	if u.Buyout != nil {
		return fmt.Sprintf("%g TON buy now", *u.Buyout)
	}
	if u.Price != nil {
		return fmt.Sprintf("%g TON", *u.Price)
	}
	return "not listed"
}

// postAngles reads a whole grid in one call: {"names":[...]} up to 12. Names already read are
// served from the cache; if nothing is left to read, the call is free.
func (a *App) postAngles(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Names []string `json:"names"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || len(b.Names) == 0 {
		httpErr(w, apiError("names are required"), 400)
		return
	}
	if len(b.Names) > 12 {
		b.Names = b.Names[:12]
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	t := a.taste(ctx)
	s := a.settings(ctx)
	profile := t.Profile
	if profile == "" && s.About != "" {
		profile = "About them: " + s.About
	}
	out := map[string]llm.Angle{}
	var todo []string
	byName := map[string]*store.Username{}
	for _, raw := range b.Names {
		name := cleanName(raw)
		if _, dup := out[name]; dup || name == "" {
			continue
		}
		if v := a.uget(ctx, angleKey(ctx, t, s, name)); v != "" {
			var ang llm.Angle
			if json.Unmarshal([]byte(v), &ang) == nil {
				out[name] = ang
				continue
			}
		}
		if u, _ := a.Store.Get(ctx, name); u != nil {
			byName[name] = u
			todo = append(todo, name)
		}
	}
	if len(todo) == 0 {
		writeJSON(w, map[string]any{"angles": out, "cached": true})
		return
	}
	if a.paywall(w, r, "scan") {
		return
	}
	lines := make([]string, 0, len(todo))
	for _, n := range todo {
		u := byName[n]
		lines = append(lines, fmt.Sprintf("%s | %s | %s", n, deref(u.Meaning, "-"), priceLine(u)))
	}
	got, usage, err := a.LLM.NameAngles(ctx, withAvoid(ctx, a, profile), lines)
	a.addUsage(usage)
	if err != nil {
		httpErr(w, err, 502)
		return
	}
	a.charge(ctx, "scan", usage, fmt.Sprintf("%d names", len(todo)))
	for n, ang := range got {
		if _, ok := byName[n]; !ok {
			continue
		}
		out[n] = ang
		if js, err := json.Marshal(ang); err == nil {
			a.uset(ctx, angleKey(ctx, t, s, n), string(js))
		}
	}
	writeJSON(w, map[string]any{"angles": out, "usage": usage, "credits": a.balance(ctx)})
}

// withAvoid appends the negative prompt from settings: what they never want suggested.
func withAvoid(ctx context.Context, a *App, profile string) string {
	if v := a.settings(ctx).Avoid; v != "" {
		profile += "\n\nNEVER suggest projects or readings in these areas, even loosely: " + v + ". Pick other names or other angles instead."
	}
	return strings.TrimSpace(profile)
}

// withConceptTaste adds the concepts they liked and disliked to the profile: the kind of project
// they want, which steers runs as much as the kind of name. The negative prompt rides along.
func withConceptTaste(ctx context.Context, a *App, profile string) string {
	profile = withAvoid(ctx, a, profile)
	liked, _ := a.Store.LikedConcepts(ctx, uid(ctx), 1, 8)
	disliked, _ := a.Store.LikedConcepts(ctx, uid(ctx), -1, 8)
	if len(liked) > 0 {
		profile += "\n\nProject concepts they liked (more in this spirit, not copies):\n- " + strings.Join(liked, "\n- ")
	}
	if len(disliked) > 0 {
		profile += "\n\nProject concepts they disliked (avoid this kind):\n- " + strings.Join(disliked, "\n- ")
	}
	return strings.TrimSpace(profile)
}
