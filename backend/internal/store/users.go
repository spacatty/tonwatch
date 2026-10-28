package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Accounts. One person can arrive from the web (password or Telegram login) or from the Telegram
// Mini App; either way it is one row here, and every personal thing (trackers, taste, credits,
// stars, hidden names, concepts, portfolio) hangs off users.id. The usernames themselves, their
// prices and AI ratings are shared by everyone.
const usersSchema = `
CREATE TABLE IF NOT EXISTS users (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  login       TEXT UNIQUE COLLATE NOCASE,  -- handle for password sign-in; NULL until they pick one
  email       TEXT UNIQUE COLLATE NOCASE,
  pass_hash   TEXT,                        -- NULL: no password, Telegram only
  tg_id       INTEGER UNIQUE,
  tg_username TEXT NOT NULL DEFAULT '',
  tg_name     TEXT NOT NULL DEFAULT '',
  tg_photo    TEXT NOT NULL DEFAULT '',
  tg_2fa      INTEGER NOT NULL DEFAULT 0,  -- password sign-ins must be approved in the bot
  role        TEXT NOT NULL DEFAULT 'user', -- user | admin
  plan        TEXT NOT NULL DEFAULT 'free', -- free | pro (pro here = granted for good by an admin)
  plan_until  INTEGER NOT NULL DEFAULT 0,  -- Pro bought for a period: active while now < plan_until
  credits     INTEGER NOT NULL DEFAULT 0,
  refill_day  TEXT NOT NULL DEFAULT '',    -- last day the free allowance was topped up
  created_at  INTEGER NOT NULL,
  seen_at     INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT PRIMARY KEY,             -- sha256 of the bearer token; the token itself is never stored
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  client     TEXT NOT NULL DEFAULT 'web',  -- web | miniapp
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  seen_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_sessions_user ON sessions(user_id);

-- Short-lived handshakes with the bot: sign in through a deep link, link Telegram to an account,
-- approve a password sign-in (2FA). The browser polls by secret; the bot only ever sees code.
CREATE TABLE IF NOT EXISTS auth_tokens (
  secret     TEXT PRIMARY KEY,
  code       TEXT NOT NULL UNIQUE,
  purpose    TEXT NOT NULL,                -- login | link | 2fa
  user_id    INTEGER NOT NULL DEFAULT 0,   -- link/2fa: the account; login: filled when approved
  tg_id      INTEGER NOT NULL DEFAULT 0,   -- 2fa: who must approve; login: who approved
  status     TEXT NOT NULL DEFAULT 'pending', -- pending | waiting (bot asked) | ok | denied
  client     TEXT NOT NULL DEFAULT '',     -- the browser that asked, shown in the bot's prompt
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS user_kv (
  user_id INTEGER NOT NULL,
  k       TEXT NOT NULL,
  v       TEXT NOT NULL,
  PRIMARY KEY (user_id, k)
);

-- A person's verdicts on shared names: star (watch), hide, note. Columns are prefixed so a join
-- with usernames never makes "name" or "watched" ambiguous.
CREATE TABLE IF NOT EXISTS user_names (
  user_id    INTEGER NOT NULL,
  un_name    TEXT NOT NULL,
  un_watched INTEGER NOT NULL DEFAULT 0,
  un_hidden  INTEGER NOT NULL DEFAULT 0,
  un_note    TEXT,
  PRIMARY KEY (user_id, un_name)
);
CREATE INDEX IF NOT EXISTS ix_user_names_watched ON user_names(un_name) WHERE un_watched = 1;

CREATE TABLE IF NOT EXISTS ledger (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  ts      INTEGER NOT NULL,
  feature TEXT NOT NULL,                  -- an AI feature, or topup | grant
  credits INTEGER NOT NULL,               -- spent (positive) on features; added on topup/grant
  tokens  INTEGER NOT NULL DEFAULT 0,
  cached  INTEGER NOT NULL DEFAULT 0,
  cost    REAL NOT NULL DEFAULT 0,
  note    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS ix_ledger_user ON ledger(user_id, id);

CREATE TABLE IF NOT EXISTS payments (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL,
  gateway    TEXT NOT NULL,               -- ton | stars
  pack       TEXT NOT NULL,
  credits    INTEGER NOT NULL DEFAULT 0,
  pro_days   INTEGER NOT NULL DEFAULT 0,
  amount     REAL NOT NULL,               -- in currency units (TON, or Stars)
  currency   TEXT NOT NULL,               -- TON | XTR
  memo       TEXT NOT NULL UNIQUE,        -- TON: the comment the transfer must carry
  status     TEXT NOT NULL DEFAULT 'pending', -- pending | paid | expired
  ext_id     TEXT,                        -- tx hash / telegram_payment_charge_id
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  paid_at    INTEGER
);
CREATE INDEX IF NOT EXISTS ix_payments_pending ON payments(gateway, status);
CREATE UNIQUE INDEX IF NOT EXISTS ix_payments_ext ON payments(gateway, ext_id) WHERE ext_id IS NOT NULL;
`

// postMigrate runs once the migrations have added user_id everywhere.
const postMigrate = `
CREATE INDEX IF NOT EXISTS ix_trackers_user ON trackers(user_id);
CREATE INDEX IF NOT EXISTS ix_concepts_user ON concepts(user_id);
`

var ErrTaken = errors.New("already taken")

type User struct {
	ID          int64  `json:"id"`
	Login       string `json:"login"`
	Email       string `json:"email"`
	HasPassword bool   `json:"has_password"`
	TgID        int64  `json:"tg_id"`
	TgUsername  string `json:"tg_username"`
	TgName      string `json:"tg_name"`
	TgPhoto     string `json:"tg_photo"`
	TG2FA       bool   `json:"tg_2fa"`
	Role        string `json:"role"`
	PlanRaw     string `json:"-"`
	PlanUntil   int64  `json:"plan_until"`
	Credits     int    `json:"credits"`
	RefillDay   string `json:"-"`
	CreatedAt   int64  `json:"created_at"`
	SeenAt      int64  `json:"seen_at"`

	PassHash string `json:"-"`
}

func (u *User) Admin() bool { return u != nil && u.Role == "admin" }

// Plan is what they have right now: Pro granted for good, or bought and not yet run out.
func (u *User) Plan() string {
	if u.PlanRaw == "pro" || u.PlanUntil > time.Now().Unix() {
		return "pro"
	}
	return "free"
}

// Label is how the account is shown to admins and in messages.
func (u *User) Label() string {
	switch {
	case u.Login != "":
		return u.Login
	case u.TgUsername != "":
		return "@" + u.TgUsername
	case u.TgName != "":
		return u.TgName
	case u.Email != "":
		return u.Email
	}
	return "user " + itoa64(u.ID)
}

const userCols = `id, COALESCE(login,''), COALESCE(email,''), COALESCE(pass_hash,''), COALESCE(tg_id,0), tg_username, tg_name, tg_photo,
  tg_2fa, role, plan, plan_until, credits, refill_day, created_at, seen_at`

func scanUser(sc interface{ Scan(...any) error }) (*User, error) {
	var u User
	var tfa int
	err := sc.Scan(&u.ID, &u.Login, &u.Email, &u.PassHash, &u.TgID, &u.TgUsername, &u.TgName, &u.TgPhoto,
		&tfa, &u.Role, &u.PlanRaw, &u.PlanUntil, &u.Credits, &u.RefillDay, &u.CreatedAt, &u.SeenAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.TG2FA, u.HasPassword = tfa == 1, u.PassHash != ""
	return &u, nil
}

func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id=?`, id))
}

// UserByLogin finds an account by its handle or its email.
func (s *Store) UserByLogin(ctx context.Context, login string) (*User, error) {
	login = strings.TrimSpace(login)
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE login=? OR email=? LIMIT 1`, login, login))
}

func (s *Store) UserByTg(ctx context.Context, tgID int64) (*User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE tg_id=?`, tgID))
}

func (s *Store) CountUsers(ctx context.Context) int {
	var n int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n
}

// TgProfile is what Telegram tells us about a person (Mini App, login widget or a bot message).
type TgProfile struct {
	ID       int64
	Username string
	Name     string
	Photo    string
}

// CreateUser makes an account. Login/email may be empty; a taken one returns ErrTaken.
func (s *Store) CreateUser(ctx context.Context, login, email, passHash string, tg *TgProfile, role string) (int64, error) {
	ts := now()
	var tgID any
	var tu, tn, tp string
	if tg != nil {
		tgID, tu, tn, tp = tg.ID, tg.Username, tg.Name, tg.Photo
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO users(login, email, pass_hash, tg_id, tg_username, tg_name, tg_photo, role, created_at, seen_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, nulls(login), nulls(email), nulls(passHash), tgID, tu, tn, tp, role, ts, ts)
	if err != nil {
		return 0, uniqueErr(err)
	}
	return res.LastInsertId()
}

func uniqueErr(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return ErrTaken
	}
	return err
}

// SetUserTg attaches (or refreshes) the Telegram identity. A tg_id that belongs to another
// account returns ErrTaken.
func (s *Store) SetUserTg(ctx context.Context, id int64, tg TgProfile) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET tg_id=?, tg_username=?, tg_name=?,
		tg_photo=CASE WHEN ? != '' THEN ? ELSE tg_photo END WHERE id=?`, tg.ID, tg.Username, tg.Name, tg.Photo, tg.Photo, id)
	return uniqueErr(err)
}

func (s *Store) UnlinkTg(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET tg_id=NULL, tg_username='', tg_name='', tg_photo='', tg_2fa=0 WHERE id=?`, id)
	return err
}

// UpdateUser changes any of login/email/pass_hash/tg_2fa that are non-nil. Empty login/email clear them.
func (s *Store) UpdateUser(ctx context.Context, id int64, login, email, passHash *string, tfa *bool) error {
	var sets []string
	var args []any
	if login != nil {
		sets, args = append(sets, "login=?"), append(args, nulls(*login))
	}
	if email != nil {
		sets, args = append(sets, "email=?"), append(args, nulls(*email))
	}
	if passHash != nil {
		sets, args = append(sets, "pass_hash=?"), append(args, nulls(*passHash))
	}
	if tfa != nil {
		sets, args = append(sets, "tg_2fa=?"), append(args, b2i(*tfa))
	}
	if len(sets) == 0 {
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id=?`, append(args, id)...)
	return uniqueErr(err)
}

func (s *Store) SetRole(ctx context.Context, id int64, role string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET role=? WHERE id=?`, role, id)
	return err
}

// SetPlan: "pro" grants Pro for good, "free" takes the grant back (time bought stays).
func (s *Store) SetPlan(ctx context.Context, id int64, plan string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET plan=? WHERE id=?`, plan, id)
	return err
}

// UsersWithPro lists accounts that have Pro now and a Telegram chat to write to: the alerts audience.
func (s *Store) UsersWithPro(ctx context.Context) ([]*User, error) {
	return s.users(ctx, `SELECT `+userCols+` FROM users WHERE tg_id IS NOT NULL AND (plan='pro' OR plan_until > ?) ORDER BY id`, now())
}

// ListUsers is the admin list, newest first; q matches login, email, Telegram handle or name.
func (s *Store) ListUsers(ctx context.Context, q string, limit int) ([]*User, error) {
	if q = strings.TrimSpace(strings.TrimPrefix(q, "@")); q != "" {
		like := "%" + likeEsc(q) + "%"
		return s.users(ctx, `SELECT `+userCols+` FROM users WHERE login LIKE ? ESCAPE '\' OR email LIKE ? ESCAPE '\'
			OR tg_username LIKE ? ESCAPE '\' OR tg_name LIKE ? ESCAPE '\' OR CAST(tg_id AS TEXT) = ? ORDER BY id DESC LIMIT ?`, like, like, like, like, q, limit)
	}
	return s.users(ctx, `SELECT `+userCols+` FROM users ORDER BY id DESC LIMIT ?`, limit)
}

func (s *Store) users(ctx context.Context, q string, args ...any) ([]*User, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// --- sessions ---

func (s *Store) CreateSession(ctx context.Context, tokenHash string, userID int64, client string, ttl time.Duration) error {
	ts := now()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO sessions(token_hash, user_id, client, created_at, expires_at, seen_at) VALUES(?,?,?,?,?,?)`,
		tokenHash, userID, client, ts, ts+int64(ttl.Seconds()), ts)
	return err
}

// SessionUser resolves a session to its account and marks both as seen (at most once a minute).
func (s *Store) SessionUser(ctx context.Context, tokenHash string) (*User, error) {
	var uid, seen int64
	err := s.DB.QueryRowContext(ctx, `SELECT user_id, seen_at FROM sessions WHERE token_hash=? AND expires_at > ?`, tokenHash, now()).Scan(&uid, &seen)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if ts := now(); ts-seen > 60 {
		s.DB.ExecContext(ctx, `UPDATE sessions SET seen_at=? WHERE token_hash=?`, ts, tokenHash)
		s.DB.ExecContext(ctx, `UPDATE users SET seen_at=? WHERE id=?`, ts, uid)
	}
	return s.UserByID(ctx, uid)
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, tokenHash)
	return err
}

// DeleteSessions signs an account out everywhere except keep (a token hash, or "").
func (s *Store) DeleteSessions(ctx context.Context, userID int64, keep string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=? AND token_hash != ?`, userID, keep)
	return err
}

// --- bot handshakes ---

type AuthToken struct {
	Secret    string
	Code      string
	Purpose   string
	UserID    int64
	TgID      int64
	Status    string
	Client    string
	ExpiresAt int64
}

func (s *Store) CreateAuthToken(ctx context.Context, t AuthToken, ttl time.Duration) error {
	ts := now()
	s.DB.ExecContext(ctx, `DELETE FROM auth_tokens WHERE expires_at < ?`, ts)
	_, err := s.DB.ExecContext(ctx, `INSERT INTO auth_tokens(secret, code, purpose, user_id, tg_id, status, client, created_at, expires_at)
		VALUES(?,?,?,?,?,?,?,?,?)`, t.Secret, t.Code, t.Purpose, t.UserID, t.TgID, "pending", t.Client, ts, ts+int64(ttl.Seconds()))
	return err
}

func (s *Store) authToken(ctx context.Context, col, v string) (*AuthToken, error) {
	var t AuthToken
	err := s.DB.QueryRowContext(ctx, `SELECT secret, code, purpose, user_id, tg_id, status, client, expires_at FROM auth_tokens
		WHERE `+col+`=? AND expires_at > ?`, v, now()).Scan(&t.Secret, &t.Code, &t.Purpose, &t.UserID, &t.TgID, &t.Status, &t.Client, &t.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &t, err
}

func (s *Store) AuthTokenBySecret(ctx context.Context, secret string) (*AuthToken, error) {
	return s.authToken(ctx, "secret", secret)
}

func (s *Store) AuthTokenByCode(ctx context.Context, code string) (*AuthToken, error) {
	return s.authToken(ctx, "code", code)
}

// ResolveAuthToken moves a token on from one status to the next; false if it was not in `from`
// any more (already answered, or expired).
func (s *Store) ResolveAuthToken(ctx context.Context, code, from, to string, userID, tgID int64) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE auth_tokens SET status=?, user_id=CASE WHEN ? > 0 THEN ? ELSE user_id END,
		tg_id=CASE WHEN ? > 0 THEN ? ELSE tg_id END WHERE code=? AND status=? AND expires_at > ?`,
		to, userID, userID, tgID, tgID, code, from, now())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// TakeAuthToken deletes a token that reached "ok" and returns it: an approval is used once.
func (s *Store) TakeAuthToken(ctx context.Context, secret string) (*AuthToken, error) {
	t, err := s.AuthTokenBySecret(ctx, secret)
	if err != nil || t == nil || t.Status != "ok" {
		return t, err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM auth_tokens WHERE secret=? AND status='ok'`, secret)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, nil
	}
	return t, nil
}

// --- per-user key/value ---

func (s *Store) GetUserKV(ctx context.Context, userID int64, k string) string {
	var v string
	s.DB.QueryRowContext(ctx, `SELECT v FROM user_kv WHERE user_id=? AND k=?`, userID, k).Scan(&v)
	return v
}

func (s *Store) SetUserKV(ctx context.Context, userID int64, k, v string) {
	if v == "" {
		s.DB.ExecContext(ctx, `DELETE FROM user_kv WHERE user_id=? AND k=?`, userID, k)
		return
	}
	s.DB.ExecContext(ctx, `INSERT INTO user_kv(user_id,k,v) VALUES(?,?,?) ON CONFLICT(user_id,k) DO UPDATE SET v=excluded.v`, userID, k, v)
}

// --- a person's verdicts on names ---

func (s *Store) SetUserWatched(ctx context.Context, userID int64, name string, on bool, note *string) error {
	if err := s.EnsureName(ctx, name); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO user_names(user_id, un_name, un_watched, un_note) VALUES(?,?,?,?)
		ON CONFLICT(user_id, un_name) DO UPDATE SET un_watched=excluded.un_watched, un_note=COALESCE(?, un_note)`,
		userID, name, b2i(on), ns(note), ns(note))
	return err
}

func (s *Store) SetUserHidden(ctx context.Context, userID int64, name string, on bool) error {
	if err := s.EnsureName(ctx, name); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO user_names(user_id, un_name, un_hidden) VALUES(?,?,?)
		ON CONFLICT(user_id, un_name) DO UPDATE SET un_hidden=excluded.un_hidden`, userID, name, b2i(on))
	return err
}

// UserNameCounts: how many names they starred and hid, for the sidebar.
func (s *Store) UserNameCounts(ctx context.Context, userID int64) (watched, hidden int) {
	s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(un_watched),0), COALESCE(SUM(un_hidden),0) FROM user_names WHERE user_id=?`, userID).Scan(&watched, &hidden)
	return
}

// --- credits ---

// Refill tops the balance up to the daily allowance once a day: the free tier. Bought credits
// sit on top, so a refill never takes anything away.
func (s *Store) Refill(ctx context.Context, userID int64, daily int, today string) {
	s.DB.ExecContext(ctx, `UPDATE users SET credits=MAX(credits, ?), refill_day=? WHERE id=? AND refill_day != ?`, daily, today, userID, today)
}

func (s *Store) Credits(ctx context.Context, userID int64) int {
	var n int
	s.DB.QueryRowContext(ctx, `SELECT credits FROM users WHERE id=?`, userID).Scan(&n)
	return n
}

type LedgerEntry struct {
	TS      int64   `json:"ts"`
	Feature string  `json:"feature"`
	Credits int     `json:"credits"`
	Tokens  int     `json:"tokens"`
	Cached  int     `json:"cached,omitempty"` // of those, prompt tokens the provider served from cache
	Cost    float64 `json:"cost"`
	Note    string  `json:"note,omitempty"`
}

// Spend takes credits (never below zero) and writes the ledger line in one transaction.
func (s *Store) Spend(ctx context.Context, userID int64, e LedgerEntry) error {
	return s.moveCredits(ctx, userID, -e.Credits, e)
}

// AddCredits puts credits on the balance (top-up, grant) with a ledger line.
func (s *Store) AddCredits(ctx context.Context, userID int64, e LedgerEntry) error {
	return s.moveCredits(ctx, userID, e.Credits, e)
}

func (s *Store) moveCredits(ctx context.Context, userID int64, delta int, e LedgerEntry) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := addCreditsTx(ctx, tx, userID, delta, e); err != nil {
		return err
	}
	return tx.Commit()
}

func addCreditsTx(ctx context.Context, tx *sql.Tx, userID int64, delta int, e LedgerEntry) error {
	if _, err := tx.ExecContext(ctx, `UPDATE users SET credits=MAX(0, credits + ?) WHERE id=?`, delta, userID); err != nil {
		return err
	}
	if e.TS == 0 {
		e.TS = now()
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO ledger(user_id, ts, feature, credits, tokens, cached, cost, note) VALUES(?,?,?,?,?,?,?,?)`,
		userID, e.TS, e.Feature, e.Credits, e.Tokens, e.Cached, e.Cost, e.Note)
	return err
}

func (s *Store) Ledger(ctx context.Context, userID int64, limit int) ([]LedgerEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT ts, feature, credits, tokens, cached, cost, note FROM ledger WHERE user_id=? ORDER BY id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LedgerEntry{}
	for rows.Next() {
		var e LedgerEntry
		if err := rows.Scan(&e.TS, &e.Feature, &e.Credits, &e.Tokens, &e.Cached, &e.Cost, &e.Note); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- payments ---

type Payment struct {
	ID        int64   `json:"id"`
	UserID    int64   `json:"user_id"`
	Gateway   string  `json:"gateway"`
	Pack      string  `json:"pack"`
	Credits   int     `json:"credits"`
	ProDays   int     `json:"pro_days"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	Memo      string  `json:"memo"`
	Status    string  `json:"status"`
	ExtID     string  `json:"ext_id,omitempty"`
	CreatedAt int64   `json:"created_at"`
	ExpiresAt int64   `json:"expires_at"`
	PaidAt    int64   `json:"paid_at,omitempty"`
	User      string  `json:"user,omitempty"` // admin list: who paid
}

const payCols = `id, user_id, gateway, pack, credits, pro_days, amount, currency, memo, status, COALESCE(ext_id,''), created_at, expires_at, COALESCE(paid_at,0)`

func scanPayment(sc interface{ Scan(...any) error }) (*Payment, error) {
	var p Payment
	err := sc.Scan(&p.ID, &p.UserID, &p.Gateway, &p.Pack, &p.Credits, &p.ProDays, &p.Amount, &p.Currency, &p.Memo, &p.Status, &p.ExtID, &p.CreatedAt, &p.ExpiresAt, &p.PaidAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &p, err
}

func (s *Store) CreatePayment(ctx context.Context, p Payment, ttl time.Duration) (int64, error) {
	ts := now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO payments(user_id, gateway, pack, credits, pro_days, amount, currency, memo, created_at, expires_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, p.UserID, p.Gateway, p.Pack, p.Credits, p.ProDays, p.Amount, p.Currency, p.Memo, ts, ts+int64(ttl.Seconds()))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) Payment(ctx context.Context, id int64) (*Payment, error) {
	return scanPayment(s.DB.QueryRowContext(ctx, `SELECT `+payCols+` FROM payments WHERE id=?`, id))
}

// PendingPayments: unexpired checkouts of one gateway that are still waiting for money.
func (s *Store) PendingPayments(ctx context.Context, gateway string) ([]*Payment, error) {
	s.DB.ExecContext(ctx, `UPDATE payments SET status='expired' WHERE status='pending' AND expires_at < ?`, now())
	return s.payments(ctx, `SELECT `+payCols+` FROM payments WHERE gateway=? AND status='pending' ORDER BY id`, gateway)
}

// Payments lists one account's payments (userID > 0) or everyone's, newest first.
func (s *Store) Payments(ctx context.Context, userID int64, limit int) ([]*Payment, error) {
	if userID > 0 {
		return s.payments(ctx, `SELECT `+payCols+` FROM payments WHERE user_id=? ORDER BY id DESC LIMIT ?`, userID, limit)
	}
	return s.payments(ctx, `SELECT `+payCols+` FROM payments ORDER BY id DESC LIMIT ?`, limit)
}

func (s *Store) payments(ctx context.Context, q string, args ...any) ([]*Payment, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Payment{}
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CompletePayment marks a pending (or just-expired: money that arrives late still counts)
// payment paid and delivers it: credits on the balance, Pro days on the plan. Idempotent: false
// when it was already paid, so a webhook or poll seen twice never pays out twice.
func (s *Store) CompletePayment(ctx context.Context, id int64, extID string) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	p, err := scanPayment(tx.QueryRowContext(ctx, `SELECT `+payCols+` FROM payments WHERE id=?`, id))
	if err != nil || p == nil {
		return false, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE payments SET status='paid', ext_id=?, paid_at=? WHERE id=? AND status IN ('pending','expired')`, nulls(extID), now(), id)
	if err != nil {
		return false, uniqueErr(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return false, nil
	}
	if p.Credits > 0 {
		if err := addCreditsTx(ctx, tx, p.UserID, p.Credits, LedgerEntry{Feature: "topup", Credits: p.Credits,
			Note: strings.TrimSpace(formatAmount(p.Amount) + " " + p.Currency + " via " + p.Gateway)}); err != nil {
			return false, err
		}
	}
	if p.ProDays > 0 {
		// time bought stacks: it starts when the current Pro period ends
		if _, err := tx.ExecContext(ctx, `UPDATE users SET plan_until = MAX(plan_until, ?) + ? WHERE id=?`, now(), p.ProDays*86400, p.UserID); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

// PaidTotals sums paid amounts per currency, for the admin overview.
func (s *Store) PaidTotals(ctx context.Context) map[string]float64 {
	out := map[string]float64{}
	rows, err := s.DB.QueryContext(ctx, `SELECT currency, SUM(amount) FROM payments WHERE status='paid' GROUP BY currency`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		var v float64
		if rows.Scan(&c, &v) == nil {
			out[c] = v
		}
	}
	return out
}

// --- the first account inherits the single-user install ---

// legacyKV are the global keys that held one person's state before there were accounts.
var legacyKV = []string{"settings", "taste", "taste:signals", "ideas:last", "ideas:last:lines", "alerts:last_run", "alerts:sent"}

// ClaimLegacy gives everything made before accounts existed to userID: trackers, concepts,
// portfolio, stars and hidden names, settings, taste, AI caches and the credit balance.
func (s *Store) ClaimLegacy(ctx context.Context, userID int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range []string{"trackers", "concepts", "portfolio"} {
		if _, err := tx.ExecContext(ctx, `UPDATE `+t+` SET user_id=? WHERE user_id=0`, userID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO user_names(user_id, un_name, un_watched, un_hidden, un_note)
		SELECT ?, name, watched, hidden, note FROM usernames WHERE watched=1 OR hidden=1 OR COALESCE(note,'') != ''`, userID); err != nil {
		return err
	}
	where := `k IN ('` + strings.Join(legacyKV, `','`) + `') OR k LIKE 'angle:%' OR k LIKE 'chat:%' OR k LIKE 'alerts:ending:%' OR k LIKE 'alerts:price:%'`
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO user_kv(user_id, k, v) SELECT ?, k, v FROM kv WHERE `+where, userID); err != nil {
		return err
	}
	var bal string
	tx.QueryRowContext(ctx, `SELECT v FROM kv WHERE k='credits:balance'`).Scan(&bal)
	if n := atoiSafe(bal); n > 0 {
		tx.ExecContext(ctx, `UPDATE users SET credits=MAX(credits, ?) WHERE id=?`, n, userID)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM kv WHERE `+where+` OR k LIKE 'credits:%'`); err != nil {
		return err
	}
	return tx.Commit()
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func formatAmount(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
