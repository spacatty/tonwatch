package app

import (
	"context"
	"encoding/json"
	"net/http"

	"fragwatch/internal/i18n"
)

// Every account has a language (en | ru) for what the bot sends on its own: alerts, receipts,
// sign-in prompts. It starts as the Telegram app's language and follows the switch in the web app.

const kvLang = "lang"

func (a *App) userLang(ctx context.Context, userID int64) string {
	return i18n.Or(a.Store.GetUserKV(ctx, userID, kvLang))
}

// rememberLang keeps the Telegram language of an account that has none yet.
func (a *App) rememberLang(ctx context.Context, userID int64, code string) {
	if l := i18n.Norm(code); l != "" && a.Store.GetUserKV(ctx, userID, kvLang) == "" {
		a.Store.SetUserKV(ctx, userID, kvLang, l)
	}
}

// chatLang is the language to answer a Telegram message in: the account's, else the sender's app.
func (a *App) chatLang(ctx context.Context, from tgFrom) string {
	if u, _ := a.Store.UserByTg(ctx, from.ID); u != nil {
		if l := a.Store.GetUserKV(ctx, u.ID, kvLang); l != "" {
			return i18n.Or(l)
		}
	}
	return i18n.Or(from.LanguageCode)
}

// setLang: the web app's language switch, so the bot writes in the language picked there.
func (a *App) setLang(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if u == nil {
		httpErr(w, apiError("sign in first"), 401)
		return
	}
	var b struct {
		Lang string `json:"lang"`
	}
	json.NewDecoder(r.Body).Decode(&b)
	l := i18n.Norm(b.Lang)
	if l == "" {
		httpErr(w, apiError("unknown language"), 400)
		return
	}
	a.Store.SetUserKV(r.Context(), u.ID, kvLang, l)
	writeJSON(w, map[string]any{"lang": l})
}
