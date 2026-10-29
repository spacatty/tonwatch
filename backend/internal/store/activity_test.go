package store

import (
	"context"
	"testing"
)

func TestActivityAndBalance(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	adm, _ := s.CreateUser(ctx, "root", "", "h", nil, "admin")
	ann, _ := s.CreateUser(ctx, "ann", "", "h", &TgProfile{ID: 7, Username: "ann"}, "user")

	s.AddCredits(ctx, ann, LedgerEntry{Feature: "grant", Credits: 120})
	d, err := s.SetCredits(ctx, ann, 50, "by root")
	if err != nil || d != -70 || s.Credits(ctx, ann) != 50 {
		t.Fatalf("set to 50 from 120: diff %d, balance %d, err %v", d, s.Credits(ctx, ann), err)
	}
	if d, _ := s.SetCredits(ctx, ann, 50, ""); d != 0 {
		t.Fatalf("setting the same balance should move nothing, moved %d", d)
	}
	led, _ := s.Ledger(ctx, ann, 10)
	if len(led) != 2 || led[0].Credits != -70 {
		t.Fatalf("ledger should hold the grant and the -70 correction, got %+v", led)
	}
	s.Spend(ctx, ann, LedgerEntry{Feature: "ideas", Credits: 10, Cost: 0.01})

	s.Log(ctx, ann, 0, "login", "web")
	s.Log(ctx, ann, 0, "ai.ideas", "")
	s.Log(ctx, ann, adm, "admin.credits", "set to 50")
	s.Log(ctx, 0, 0, "login", "") // no account: dropped

	all, _ := s.Activity(ctx, ActivityFilter{})
	if len(all) != 3 || all[0].Kind != "admin.credits" || all[0].User != "ann" || all[0].Actor != "root" {
		t.Fatalf("feed is newest first with labels, got %+v", all)
	}
	if ai, _ := s.Activity(ctx, ActivityFilter{Kind: "ai."}); len(ai) != 1 {
		t.Fatalf(`"ai." matches the family, got %d`, len(ai))
	}
	if page, _ := s.Activity(ctx, ActivityFilter{Before: all[0].ID, Limit: 1}); len(page) != 1 || page[0].ID != all[1].ID {
		t.Fatalf("paging by id, got %+v", page)
	}

	days, err := s.Daily(ctx, 7)
	if err != nil || len(days) != 7 {
		t.Fatalf("seven days, got %d (%v)", len(days), err)
	}
	today := days[6]
	if today.Signups != 2 || today.Active != 1 || today.Events != 3 || today.Spent != 10 {
		t.Fatalf("today: %+v", today)
	}
	if f, _ := s.FeatureUsage(ctx, 0); len(f) != 1 || f[0].Feature != "ideas" || f[0].Users != 1 {
		t.Fatalf("feature usage leaves grants out, got %+v", f)
	}
	if tot := s.UserTotals(ctx); tot.Total != 2 || tot.Admins != 1 || tot.Telegram != 1 || tot.Credits != 40 {
		t.Fatalf("totals: %+v", tot)
	}
	if fp := s.UserFootprint(ctx, ann); fp.Spent != 10 || fp.Granted != 50 {
		t.Fatalf("footprint: %+v", fp)
	}
}
