package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"testing"
	"time"
)

const bot = "123456:TEST-token"

func TestPassword(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "correct horse") {
		t.Fatal("right password rejected")
	}
	if CheckPassword(h, "correct horse ") || CheckPassword(h, "") || CheckPassword("garbage", "x") {
		t.Fatal("wrong password accepted")
	}
	h2, _ := HashPassword("correct horse")
	if h == h2 {
		t.Fatal("hashes must be salted")
	}
}

// signWebApp builds initData the way Telegram does.
func signWebApp(v url.Values) string {
	secret := hmacSHA256([]byte("WebAppData"), []byte(bot))
	v.Set("hash", hex.EncodeToString(hmacSHA256(secret, []byte(checkString(v)))))
	return v.Encode()
}

func TestVerifyWebApp(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	v := url.Values{}
	v.Set("auth_date", strconv.FormatInt(now.Unix()-60, 10))
	v.Set("query_id", "AAH")
	v.Set("user", `{"id":42,"first_name":"Ann","username":"ann"}`)
	v.Set("signature", "sig")
	data := signWebApp(v)

	u, err := VerifyWebApp(data, bot, 24*time.Hour, now)
	if err != nil || u.ID != 42 || u.Username != "ann" {
		t.Fatalf("valid data: %v %+v", err, u)
	}
	if _, err := VerifyWebApp(data, "other:token", 24*time.Hour, now); err != ErrBadSignature {
		t.Fatalf("wrong bot token: %v", err)
	}
	tampered, _ := url.ParseQuery(data)
	tampered.Set("user", `{"id":43,"first_name":"Eve"}`)
	if _, err := VerifyWebApp(tampered.Encode(), bot, 24*time.Hour, now); err != ErrBadSignature {
		t.Fatalf("tampered user: %v", err)
	}
	if _, err := VerifyWebApp(data, bot, 24*time.Hour, now.Add(48*time.Hour)); err != ErrExpired {
		t.Fatalf("old data: %v", err)
	}
}

func TestVerifyWidget(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	f := map[string]string{"id": "7", "first_name": "Bo", "username": "bo", "auth_date": strconv.FormatInt(now.Unix(), 10)}
	v := url.Values{}
	for k, x := range f {
		v.Set(k, x)
	}
	secret := sha256.Sum256([]byte(bot))
	f["hash"] = hex.EncodeToString(hmacSHA256(secret[:], []byte(checkString(v))))

	u, err := VerifyWidget(f, bot, time.Hour, now)
	if err != nil || u.ID != 7 || u.Username != "bo" {
		t.Fatalf("valid widget data: %v %+v", err, u)
	}
	f["id"] = "8"
	if _, err := VerifyWidget(f, bot, time.Hour, now); err != ErrBadSignature {
		t.Fatalf("tampered id: %v", err)
	}
}
