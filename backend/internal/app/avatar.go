package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Avatars: signing in through the bot gives us the person's id and name but no photo (only the
// Login Widget and the Mini App send photo_url). For those accounts the photo is read through the
// Bot API and served from here, so the bot token never reaches the browser. Cached in memory.
const (
	avatarTTL     = 12 * time.Hour
	avatarMissTTL = time.Hour // nobody to show: try again later, not on every page view
	avatarMax     = 512 << 10
)

type avatarEntry struct {
	body []byte
	ct   string
	at   time.Time
}

var (
	avatarMu    sync.Mutex
	avatarCache = map[int64]avatarEntry{}
)

// avatarURL is what userJSON puts in tg_photo for a Telegram account without a stored photo.
func (a *App) avatarURL(userID, tgID int64) string {
	if tgID == 0 || a.Cfg.TelegramBotToken == "" {
		return ""
	}
	return "/api/avatar/" + strconv.FormatInt(userID, 10)
}

func (a *App) avatarRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/avatar/{id}", a.getAvatar)
}

func (a *App) getAvatar(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u, err := a.Store.UserByID(r.Context(), id)
	if err != nil || u == nil || u.TgID == 0 {
		http.NotFound(w, r)
		return
	}
	avatarMu.Lock()
	e, ok := avatarCache[u.TgID]
	avatarMu.Unlock()
	fresh := ok && (len(e.body) > 0 && time.Since(e.at) < avatarTTL || len(e.body) == 0 && time.Since(e.at) < avatarMissTTL)
	if !fresh {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		body, ct, err := a.fetchTgPhoto(ctx, u.TgID)
		cancel()
		if err != nil && ok && len(e.body) > 0 {
			body, ct = e.body, e.ct // Telegram hiccup: keep showing the old one
		}
		e = avatarEntry{body: body, ct: ct, at: time.Now()}
		avatarMu.Lock()
		avatarCache[u.TgID] = e
		avatarMu.Unlock()
	}
	if len(e.body) == 0 {
		http.NotFound(w, r) // the <img> falls back to the initial
		return
	}
	w.Header().Set("Content-Type", e.ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Write(e.body)
}

// fetchTgPhoto downloads the smallest square of the newest profile photo, or nothing if the
// person has none or hides it from bots.
func (a *App) fetchTgPhoto(ctx context.Context, tgID int64) ([]byte, string, error) {
	out, err := a.tgCall(ctx, "getUserProfilePhotos", map[string]any{"user_id": tgID, "limit": 1})
	if err != nil {
		return nil, "", err
	}
	var photos struct {
		Result struct {
			Photos [][]struct {
				FileID string `json:"file_id"`
				Width  int    `json:"width"`
			} `json:"photos"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &photos); err != nil {
		return nil, "", err
	}
	if len(photos.Result.Photos) == 0 || len(photos.Result.Photos[0]) == 0 {
		return nil, "", nil
	}
	// sizes come smallest first; take the first one at least 160px wide (crisp at 2x in a 36px circle)
	sizes := photos.Result.Photos[0]
	fileID := sizes[len(sizes)-1].FileID
	for _, s := range sizes {
		if s.Width >= 160 {
			fileID = s.FileID
			break
		}
	}
	out, err = a.tgCall(ctx, "getFile", map[string]any{"file_id": fileID})
	if err != nil {
		return nil, "", err
	}
	var f struct {
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &f); err != nil || f.Result.FilePath == "" {
		return nil, "", fmt.Errorf("telegram: no file path")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.telegram.org/file/bot"+a.Cfg.TelegramBotToken+"/"+f.Result.FilePath, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("telegram file: http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, avatarMax))
	if err != nil {
		return nil, "", err
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" || ct == "application/octet-stream" {
		ct = http.DetectContentType(body)
	}
	return body, ct, nil
}
