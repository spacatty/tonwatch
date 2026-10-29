package store

import (
	"context"
	"testing"
)

func TestSupport(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	ann, _ := s.CreateUser(ctx, "ann", "", "h", nil, "user")
	boss, _ := s.CreateUser(ctx, "boss", "", "h", nil, "admin")

	if open, err := s.OpenTicket(ctx, ann); err != nil || open != nil {
		t.Fatalf("no ticket yet: %v %v", open, err)
	}
	id, err := s.CreateTicket(ctx, ann, "cannot pay")
	if err != nil {
		t.Fatal(err)
	}
	s.AddSupportMessage(ctx, id, ann, false, "cannot pay with TON", "bot")
	s.AddSupportMessage(ctx, id, ann, false, "still cannot", "web")
	if n := s.SupportWaiting(ctx); n != 1 {
		t.Fatalf("one ticket waits on the team, got %d", n)
	}
	staff, _ := s.Ticket(ctx, id, true)
	if staff.Unread != 2 || staff.LastFrom != "user" || staff.Preview != "still cannot" {
		t.Fatalf("team sees two unread, latest first in preview: %+v", staff)
	}

	s.AddSupportMessage(ctx, id, boss, true, "fixed, try again", "web")
	if n := s.SupportUnread(ctx, ann); n != 1 {
		t.Fatalf("ann has one answer to read, got %d", n)
	}
	if n := s.SupportWaiting(ctx); n != 0 {
		t.Fatalf("answered: nothing waits, got %d", n)
	}
	if staff, _ = s.Ticket(ctx, id, true); staff.Unread != 0 {
		t.Fatalf("writing marks the team's side read: %+v", staff)
	}
	s.MarkTicketSeen(ctx, id, false)
	if n := s.SupportUnread(ctx, ann); n != 0 {
		t.Fatalf("read: %d", n)
	}

	s.SetTicketStatus(ctx, id, "closed")
	if open, _ := s.OpenTicket(ctx, ann); open != nil {
		t.Fatalf("closed tickets are not open: %+v", open)
	}
	if ts, _ := s.Tickets(ctx, "closed", 10); len(ts) != 1 {
		t.Fatalf("closed queue: %+v", ts)
	}
	// writing again reopens it
	s.AddSupportMessage(ctx, id, ann, false, "it broke again", "bot")
	if open, _ := s.OpenTicket(ctx, ann); open == nil || open.ID != id {
		t.Fatalf("reopened: %+v", open)
	}
	ms, _ := s.SupportMessages(ctx, id)
	if len(ms) != 4 || !ms[2].Staff || ms[2].AuthorID != boss || ms[0].Via != "bot" {
		t.Fatalf("thread in order: %+v", ms)
	}
	if mine, _ := s.UserTickets(ctx, boss, 10); len(mine) != 0 {
		t.Fatalf("answering is not owning: %+v", mine)
	}
}
