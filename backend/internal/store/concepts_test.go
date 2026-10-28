package store

import (
	"context"
	"path/filepath"
	"testing"
)

// A concept keeps its "built with" line, its names and the user's verdict, and stays private to its user.
func TestConceptRoundTrip(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	ctx := context.Background()

	id, err := s.AddConcept(ctx, 7, Concept{
		Source: "set", Project: "Perm Scan", Pitch: "Audits the bots in your own group.",
		BuiltWith: "Bot added to the user's group; getChatAdministrators; report in a Mini App",
		Names:     []ConceptName{{Name: "permscan", Angle: "scans permissions"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetConceptLiked(ctx, 7, id, 1); err != nil {
		t.Fatal(err)
	}

	c, err := s.Concept(ctx, 7, id)
	if err != nil || c == nil {
		t.Fatalf("concept: %v %v", c, err)
	}
	if c.BuiltWith == "" || c.Liked != 1 || len(c.Names) != 1 || c.Names[0].Name != "permscan" {
		t.Fatalf("round trip lost data: %+v", c)
	}
	list, err := s.Concepts(ctx, 7, "permscan", 10)
	if err != nil || len(list) != 1 || list[0].BuiltWith != c.BuiltWith {
		t.Fatalf("list by name: %+v %v", list, err)
	}
	if other, _ := s.Concepts(ctx, 8, "", 10); len(other) != 0 {
		t.Fatalf("another user sees the concept: %+v", other)
	}
}
