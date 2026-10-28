// Package auth holds the cryptography behind sign-in: password hashes, session tokens, and the
// two ways Telegram vouches for a person (Mini App initData and the Login Widget).
package auth

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// --- passwords: PBKDF2-SHA256, stored as "pbkdf2-sha256$<iterations>$<salt>$<hash>" ---

const (
	pbkdfIter = 600_000 // OWASP's 2023 figure for PBKDF2-HMAC-SHA256
	pbkdfLen  = 32
)

var ErrBadPassword = errors.New("wrong login or password")

func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pw, salt, pbkdfIter, pbkdfLen)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdfIter, enc(salt), enc(key)), nil
}

func CheckPassword(hash, pw string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	salt, err2 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err3 := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || err2 != nil || err3 != nil || iter < 1 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

// --- random tokens ---

// Token is n random bytes, URL-safe. Session tokens use 32; codes in bot links 12.
func Token(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken is how a session token is stored: a leaked database holds no usable tokens.
func HashToken(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

// --- Telegram ---

// TgUser is the person Telegram vouched for.
type TgUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	PhotoURL  string `json:"photo_url"`
	Language  string `json:"language_code"`
}

func (u TgUser) Name() string { return strings.TrimSpace(u.FirstName + " " + u.LastName) }

var (
	ErrBadSignature = errors.New("telegram data: bad signature")
	ErrExpired      = errors.New("telegram data: too old, open the app again")
)

func hmacSHA256(key, msg []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(msg)
	return m.Sum(nil)
}

// checkString is Telegram's data-check-string: every field but the excluded ones, sorted, as
// key=value lines.
func checkString(v url.Values, exclude ...string) string {
	skip := map[string]bool{}
	for _, k := range exclude {
		skip[k] = true
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		if !skip[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + v.Get(k)
	}
	return strings.Join(lines, "\n")
}

func fresh(v url.Values, maxAge time.Duration, now time.Time) error {
	ts, err := strconv.ParseInt(v.Get("auth_date"), 10, 64)
	if err != nil {
		return ErrBadSignature
	}
	if age := now.Sub(time.Unix(ts, 0)); age > maxAge || age < -5*time.Minute {
		return ErrExpired
	}
	return nil
}

// VerifyWebApp checks the initData string a Mini App receives (Telegram.WebApp.initData):
// HMAC-SHA256 keyed with HMAC-SHA256("WebAppData", bot token).
// https://core.telegram.org/bots/webapps#validating-data-received-via-the-mini-app
func VerifyWebApp(initData, botToken string, maxAge time.Duration, now time.Time) (TgUser, error) {
	var u TgUser
	v, err := url.ParseQuery(initData)
	if err != nil || v.Get("hash") == "" || botToken == "" {
		return u, ErrBadSignature
	}
	want, err := hex.DecodeString(v.Get("hash"))
	if err != nil {
		return u, ErrBadSignature
	}
	secret := hmacSHA256([]byte("WebAppData"), []byte(botToken))
	ok := false
	// "signature" (Bot API 8.0, for third-party Ed25519 checks) is part of the hashed fields; some
	// clients have been seen hashing without it, so either form is accepted.
	for _, ex := range [][]string{{"hash"}, {"hash", "signature"}} {
		if hmac.Equal(hmacSHA256(secret, []byte(checkString(v, ex...))), want) {
			ok = true
			break
		}
	}
	if !ok {
		return u, ErrBadSignature
	}
	if err := fresh(v, maxAge, now); err != nil {
		return u, err
	}
	if err := json.Unmarshal([]byte(v.Get("user")), &u); err != nil || u.ID == 0 {
		return u, errors.New("telegram data: no user")
	}
	return u, nil
}

// VerifyWidget checks the fields the Login Widget hands to its callback: HMAC-SHA256 keyed with
// SHA256(bot token). https://core.telegram.org/widgets/login#checking-authorization
func VerifyWidget(fields map[string]string, botToken string, maxAge time.Duration, now time.Time) (TgUser, error) {
	var u TgUser
	v := url.Values{}
	for k, val := range fields {
		if val != "" {
			v.Set(k, val)
		}
	}
	want, err := hex.DecodeString(v.Get("hash"))
	if err != nil || len(want) == 0 || botToken == "" {
		return u, ErrBadSignature
	}
	secret := sha256.Sum256([]byte(botToken))
	if !hmac.Equal(hmacSHA256(secret[:], []byte(checkString(v, "hash"))), want) {
		return u, ErrBadSignature
	}
	if err := fresh(v, maxAge, now); err != nil {
		return u, err
	}
	u.ID, _ = strconv.ParseInt(v.Get("id"), 10, 64)
	u.FirstName, u.LastName, u.Username, u.PhotoURL = v.Get("first_name"), v.Get("last_name"), v.Get("username"), v.Get("photo_url")
	if u.ID == 0 {
		return u, ErrBadSignature
	}
	return u, nil
}
