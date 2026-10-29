package store

import (
	"context"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := openPG(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	return s
}

func TestUsersAreSeparate(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	s.EnsureName(ctx, "alpha")
	s.EnsureName(ctx, "beta")
	ann, _ := s.CreateUser(ctx, "ann", "", "h", nil, "admin")
	bob, _ := s.CreateUser(ctx, "bob", "", "h", &TgProfile{ID: 42, Username: "bob"}, "user")

	if _, err := s.CreateUser(ctx, "ANN", "", "h", nil, "user"); err != ErrTaken {
		t.Fatalf("logins are case-insensitive and unique, got %v", err)
	}
	if _, err := s.CreateUser(ctx, "", "", "", &TgProfile{ID: 42}, "user"); err != ErrTaken {
		t.Fatalf("a Telegram id belongs to one account, got %v", err)
	}

	s.SetUserWatched(ctx, ann, "alpha", true, nil)
	s.SetUserHidden(ctx, bob, "alpha", true)

	_, n, _ := s.Query(ctx, Filter{UserID: ann, Watched: true})
	if n != 1 {
		t.Fatalf("ann starred one name, query says %d", n)
	}
	_, n, _ = s.Query(ctx, Filter{UserID: bob, Watched: true})
	if n != 0 {
		t.Fatalf("bob starred nothing, query says %d", n)
	}
	_, n, _ = s.Query(ctx, Filter{UserID: bob})
	if n != 1 {
		t.Fatalf("bob hid alpha: one name left, got %d", n)
	}
	_, n, _ = s.Query(ctx, Filter{UserID: ann})
	if n != 2 {
		t.Fatalf("bob's hiding must not touch ann's list, got %d", n)
	}
	if u, _ := s.GetFor(ctx, ann, "alpha"); u == nil || !u.Watched {
		t.Fatal("GetFor should carry ann's star")
	}

	tid, _ := s.CreateTracker(ctx, ann, Tracker{Name: "t", View: "all", Filters: "{}", Params: ""})
	if err := s.DeleteTracker(ctx, bob, tid); err == nil {
		t.Fatal("bob deleted ann's tracker")
	}
	if ts, _ := s.Trackers(ctx, bob); len(ts) != 0 {
		t.Fatal("bob sees ann's trackers")
	}
	s.SetUserKV(ctx, ann, "settings", `{"min_score":9}`)
	if s.GetUserKV(ctx, bob, "settings") != "" {
		t.Fatal("bob reads ann's settings")
	}
}

func TestClaimLegacy(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	s.EnsureName(ctx, "old")
	s.DB.Exec(`UPDATE usernames SET watched=1, note='mine' WHERE name='old'`)
	s.DB.Exec(`INSERT INTO trackers(name, view, filters, params, seen_at, created_at) VALUES('legacy','all','{}','',0,0)`)
	s.SetKV(ctx, "settings", `{"min_score":8}`)
	s.SetKV(ctx, "credits:balance", "345")
	s.SetKV(ctx, "chain_offset", "1000")

	id, _ := s.CreateUser(ctx, "owner", "", "h", nil, "admin")
	if err := s.ClaimLegacy(ctx, id); err != nil {
		t.Fatal(err)
	}
	if ts, _ := s.Trackers(ctx, id); len(ts) != 1 {
		t.Fatal("the legacy tracker should be the owner's")
	}
	if u, _ := s.GetFor(ctx, id, "old"); !u.Watched || u.Note == nil || *u.Note != "mine" {
		t.Fatalf("the star and note should carry over: %+v", u)
	}
	if s.GetUserKV(ctx, id, "settings") != `{"min_score":8}` || s.GetKV(ctx, "settings") != "" {
		t.Fatal("settings should move to the owner")
	}
	if s.GetKV(ctx, "chain_offset") != "1000" {
		t.Fatal("collector state is not personal and must stay")
	}
	if s.Credits(ctx, id) != 345 {
		t.Fatalf("the balance should carry over, got %d", s.Credits(ctx, id))
	}
}

func TestPaymentPaysOnce(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	id, _ := s.CreateUser(ctx, "payer", "", "h", nil, "user")
	pid, err := s.CreatePayment(ctx, Payment{UserID: id, Gateway: "ton", Pack: "c1000", Credits: 1000, ProDays: 30, Amount: 1, Currency: "TON", Memo: "fw-x"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		ok, err := s.CompletePayment(ctx, pid, "txhash")
		if err != nil {
			t.Fatal(err)
		}
		if ok != (i == 0) {
			t.Fatalf("attempt %d: paid=%v", i, ok)
		}
	}
	if s.Credits(ctx, id) != 1000 {
		t.Fatalf("credits paid out %d, want 1000", s.Credits(ctx, id))
	}
	u, _ := s.UserByID(ctx, id)
	if u.Plan() != "pro" {
		t.Fatal("30 Pro days should make the plan pro")
	}
	// a second checkout can't reuse the same transaction
	pid2, _ := s.CreatePayment(ctx, Payment{UserID: id, Gateway: "ton", Pack: "c1000", Credits: 1000, Amount: 1, Currency: "TON", Memo: "fw-y"}, time.Hour)
	if ok, _ := s.CompletePayment(ctx, pid2, "txhash"); ok {
		t.Fatal("one transaction paid two checkouts")
	}
	// spending never goes below zero
	s.Spend(ctx, id, LedgerEntry{Feature: "ideas", Credits: 5000})
	if s.Credits(ctx, id) != 0 {
		t.Fatalf("balance went to %d", s.Credits(ctx, id))
	}
	if led, _ := s.Ledger(ctx, id, 10); len(led) != 2 {
		t.Fatalf("ledger should hold the top-up and the spend, has %d", len(led))
	}
}

func TestProWithCredits(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	id, _ := s.CreateUser(ctx, "saver", "", "h", nil, "user")
	if u, _ := s.UserByID(ctx, id); u.Unlocked() {
		t.Fatal("an account that never paid should not be unlocked")
	}
	if err := s.BuyProWithCredits(ctx, id, 1500, 30); err != ErrShort {
		t.Fatalf("an empty balance bought Pro: %v", err)
	}
	pid, _ := s.CreatePayment(ctx, Payment{UserID: id, Gateway: "ton", Pack: "c5000", Credits: 2000, Amount: 4, Currency: "TON", Memo: "fw-p"}, time.Hour)
	s.CompletePayment(ctx, pid, "tx-p")
	u, _ := s.UserByID(ctx, id)
	if !u.Paid || !u.Unlocked() || u.Plan() != "free" {
		t.Fatalf("one payment should unlock without Pro: paid=%v plan=%s", u.Paid, u.Plan())
	}
	if err := s.BuyProWithCredits(ctx, id, 1500, 30); err != nil {
		t.Fatal(err)
	}
	if err := s.BuyProWithCredits(ctx, id, 1500, 30); err != ErrShort {
		t.Fatalf("500 left bought Pro again: %v", err)
	}
	u, _ = s.UserByID(ctx, id)
	if u.Credits != 500 || u.Plan() != "pro" || u.PlanUntil < time.Now().Unix()+29*86400 {
		t.Fatalf("credits=%d plan=%s until=%d", u.Credits, u.Plan(), u.PlanUntil)
	}
}
