package store

import (
	"context"
	"strings"
	"testing"
)

func TestAIChat(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	ann, _ := s.CreateUser(ctx, "ann", "", "h", nil, "user")
	bob, _ := s.CreateUser(ctx, "bob", "", "h", nil, "user")

	first, _ := s.AddAITurn(ctx, ann, "vpn app", map[string]any{"buyable_only": true})
	second, _ := s.AddAITurn(ctx, ann, "coffee shop", nil)
	s.AddAITurn(ctx, bob, "bob's own", nil)
	if n := s.PendingAITurns(ctx, ann); n != 2 {
		t.Fatalf("two questions in flight, got %d", n)
	}

	s.FinishAITurn(ctx, first, map[string]any{"summary": "short and safe"}, "")
	s.FinishAITurn(ctx, second, nil, "planning: timeout")

	turns, err := s.AITurns(ctx, ann, 30)
	if err != nil || len(turns) != 2 {
		t.Fatalf("ann sees her two turns, got %d (%v)", len(turns), err)
	}
	if turns[0].Brief != "vpn app" || turns[0].Status != "done" || !strings.Contains(string(turns[0].Result), "short and safe") {
		t.Fatalf("oldest first, answered: %+v", turns[0])
	}
	if !strings.Contains(string(turns[0].Opts), `"buyable_only":true`) {
		t.Fatalf("options kept: %s", turns[0].Opts)
	}
	if turns[1].Status != "error" || turns[1].Error != "planning: timeout" || turns[1].Result != nil {
		t.Fatalf("failure kept with its reason: %+v", turns[1])
	}

	// a question the server never finished (it restarted) turns into an error instead of spinning forever
	stuck, _ := s.AddAITurn(ctx, ann, "lost", nil)
	s.DB.ExecContext(ctx, `UPDATE ai_turns SET created_at = created_at - 3600 WHERE id=?`, stuck)
	turns, _ = s.AITurns(ctx, ann, 30)
	if last := turns[len(turns)-1]; last.Status != "error" || last.Error != "interrupted" {
		t.Fatalf("stale question should read as interrupted: %+v", last)
	}

	if turns, _ := s.AITurns(ctx, ann, 1); len(turns) != 1 || turns[0].Brief != "lost" {
		t.Fatalf("the limit keeps the newest turns, got %+v", turns)
	}
	s.ClearAITurns(ctx, ann)
	if turns, _ := s.AITurns(ctx, ann, 30); len(turns) != 0 {
		t.Fatalf("cleared, got %d", len(turns))
	}
	if turns, _ := s.AITurns(ctx, bob, 30); len(turns) != 1 {
		t.Fatalf("clearing ann's chat leaves bob's, got %d", len(turns))
	}
}
