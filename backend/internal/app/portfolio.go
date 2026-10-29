package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"fragwatch/internal/store"
)

// Portfolio: the names they own, what they paid, what names of that score and length go for now, and the
// concept attached to each. Where the ideas land after the purchase.
type portfolioResp struct {
	Items     []portfolioItem `json:"items"`
	Paid      float64         `json:"paid"`      // sum of known purchase prices
	Now       float64         `json:"now"`       // sum of typical prices for the same names' scores
	Typical   map[int]float64 `json:"typical"`   // typical price per score alone (median sold, else median ask)
	Unpriced  int             `json:"unpriced"`  // names whose score has no typical price yet
	ListedNow int             `json:"listed_now"` // names in the portfolio that are on sale right now
}

type portfolioItem struct {
	store.PortfolioRow
	Typical *float64 `json:"typical"` // what names with this score and length typically sell for
}

func (a *App) portfolioRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/portfolio", a.getPortfolio)
	mux.HandleFunc("POST /api/portfolio", a.postPortfolio)
	mux.HandleFunc("DELETE /api/portfolio/{name}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Store.DeletePortfolio(r.Context(), uid(r.Context()), cleanName(r.PathValue("name"))); err != nil {
			httpErr(w, err, 500)
			return
		}
		a.getPortfolio(w, r)
	})
}

func (a *App) getPortfolio(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := a.Store.Portfolio(ctx, uid(ctx))
	if err != nil {
		httpErr(w, err, 500)
		return
	}
	typ := a.Store.Typical(ctx)
	res := portfolioResp{Items: []portfolioItem{}, Typical: typ.Score}
	for _, row := range rows {
		it := portfolioItem{PortfolioRow: row}
		if row.Item.Score != nil {
			if v, ok := typ.For(*row.Item.Score, row.Item.Len); ok {
				it.Typical = &v
				res.Now += v
			} else {
				res.Unpriced++
			}
		} else {
			res.Unpriced++
		}
		if row.Bought != nil {
			res.Paid += *row.Bought
		}
		if row.Item.Status == "auction" || row.Item.Status == "sale" || row.Item.Status == "market" {
			res.ListedNow++
		}
		res.Items = append(res.Items, it)
	}
	writeJSON(w, res)
}

// postPortfolio adds or edits: {"name","bought":12.5,"bought_at":unix,"concept":"...","note":"..."}.
func (a *App) postPortfolio(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name     string   `json:"name"`
		Bought   *float64 `json:"bought"`
		BoughtAt *int64   `json:"bought_at"`
		Concept  string   `json:"concept"`
		Note     string   `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		httpErr(w, err, 400)
		return
	}
	name := cleanName(b.Name)
	if name == "" {
		httpErr(w, apiError("name is required"), 400)
		return
	}
	ctx := r.Context()
	if err := a.Store.EnsureName(ctx, name); err != nil {
		httpErr(w, err, 500)
		return
	}
	if b.Bought != nil && *b.Bought < 0 {
		b.Bought = nil
	}
	row := store.PortfolioRow{Name: name, Bought: b.Bought, BoughtAt: b.BoughtAt, Concept: clip(b.Concept, 200), Note: clip(b.Note, 400)}
	if err := a.Store.SetPortfolio(ctx, uid(ctx), row); err != nil {
		httpErr(w, err, 500)
		return
	}
	// owning it is the strongest taste signal there is
	a.recordSignal(ctx, name, "buy")
	a.getPortfolio(w, r)
}

// clip trims s to at most n bytes, never splitting a character.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		for n > 0 && !utf8.RuneStart(s[n]) {
			n--
		}
		return s[:n]
	}
	return s
}
