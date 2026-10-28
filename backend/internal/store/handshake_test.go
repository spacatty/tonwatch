package store

import (
	"context"
	"testing"
	"time"
)

// Real Telegram ids no longer fit in 32 bits; the handshake must carry them.
func TestHandshakeBigTelegramID(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	const tg = 7_412_345_678
	if err := s.CreateAuthToken(ctx, AuthToken{Secret: "s1", Code: "c1", Purpose: "login"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	ok, err := s.ResolveAuthToken(ctx, "c1", "pending", "waiting", 0, tg)
	if err != nil || !ok {
		t.Fatalf("pending → waiting: ok=%v err=%v", ok, err)
	}
	uid, err := s.CreateUser(ctx, "", "", "", &TgProfile{ID: tg, Username: "big"}, "user")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ResolveAuthToken(ctx, "c1", "waiting", "ok", uid, 0); err != nil || !ok {
		t.Fatalf("waiting → ok: ok=%v err=%v", ok, err)
	}
	got, err := s.TakeAuthToken(ctx, "s1")
	if err != nil || got == nil || got.TgID != tg || got.UserID != uid {
		t.Fatalf("take: %+v %v", got, err)
	}
	if err := s.SetUserTg(ctx, uid, TgProfile{ID: tg, Username: "big2"}); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.UserByTg(ctx, tg); u == nil || u.TgUsername != "big2" {
		t.Fatalf("user by tg: %+v", u)
	}
}
