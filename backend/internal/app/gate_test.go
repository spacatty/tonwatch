package app

import (
	"context"
	"slices"
	"testing"
	"time"

	"fragwatch/internal/store"
)

func TestGate(t *testing.T) {
	a := &App{}
	rules, err := store.ParseRules(`{"op":"and","rules":[{"field":"len","cmp":"lt","value":5}]}`)
	if err != nil || rules == nil {
		t.Fatal("test rules did not parse:", err)
	}
	full := func() store.Filter {
		return store.Filter{Rules: rules, QuietDays: 7, Keywords: []string{"ai"}, Limit: 60, Offset: 270}
	}

	free := withUser(context.Background(), &store.User{ID: 1, Role: "user", PlanRaw: "free"})
	f := full()
	locked := a.gate(free, &f)
	for _, k := range []string{"rules", "quiet_days", "keywords", "depth"} {
		if !slices.Contains(locked, k) {
			t.Errorf("free account: %q should be locked, got %v", k, locked)
		}
	}
	if f.Rules != nil || f.QuietDays != 0 || f.Keywords != nil || f.Offset+f.Limit != FreeRows {
		t.Errorf("free account kept too much: %+v", f)
	}
	f = store.Filter{Offset: 600, Limit: 60}
	if a.gate(free, &f); f.Limit != 0 {
		t.Errorf("past the cap should return no rows, limit=%d", f.Limit)
	}

	for name, u := range map[string]*store.User{
		"paid":  {ID: 2, Role: "user", PlanRaw: "free", Paid: true},
		"pro":   {ID: 3, Role: "user", PlanRaw: "free", PlanUntil: time.Now().Unix() + 3600},
		"admin": {ID: 4, Role: "admin", PlanRaw: "free"},
	} {
		f := full()
		if locked := a.gate(withUser(context.Background(), u), &f); locked != nil || f.Rules == nil || f.Limit != 60 {
			t.Errorf("%s account should run the filter as asked: locked=%v %+v", name, locked, f)
		}
	}
}
