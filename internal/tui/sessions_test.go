package tui

import (
	"strings"
	"testing"

	"engram-manager/internal/store"
)

func TestSessionConcernRules(t *testing.T) {
	tests := []struct {
		name    string
		session store.Session
		want    string
	}{
		{
			name:    "ended session is inert",
			session: store.Session{EndedAt: "2026-10-09 00:00:00", RuntimeLeaseExpiresAt: "2026-10-10"},
			want:    "",
		},
		{
			name:    "active with lease flagged",
			session: store.Session{RuntimeLeaseExpiresAt: "2026-10-10 12:00:00"},
			want:    "holds a runtime lease",
		},
		{
			name:    "active with obs but no ownership mode",
			session: store.Session{ObservationCount: 3, OwnershipMode: ""},
			want:    "no ownership metadata",
		},
		{
			name:    "active with explicit ownership is clean",
			session: store.Session{ObservationCount: 3, OwnershipMode: "exclusive"},
			want:    "",
		},
	}
	for _, tc := range tests {
		got := sessionConcern(tc.session)
		if got != tc.want {
			t.Errorf("%s: sessionConcern = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSessionsScreenLoadsAndNavigates(t *testing.T) {
	st := realStore(t)
	m := New(st, nil)
	m.width, m.height = 110, 24

	next, cmd := m.openSessions()
	m = next.(Model)
	if m.view != viewSessions {
		t.Fatal("openSessions did not switch view")
	}
	if !strings.Contains(m.View(), "loading sessions…") {
		t.Error("expected loading state before data arrives")
	}

	m = update(m, cmd())
	if len(m.sessions) == 0 {
		t.Skip("no sessions in test db")
	}

	view := m.View()
	if !strings.Contains(view, "active") || !strings.Contains(view, "obs") {
		t.Errorf("sessions view missing header/counts:\n%s", view)
	}

	// Navigation with j/k moves sessionCursor
	m, _ = press(m, "j")
	if m.sessionCursor != 1 {
		t.Errorf("cursor after 'j' = %d, want 1", m.sessionCursor)
	}
	m, _ = press(m, "k")
	if m.sessionCursor != 0 {
		t.Errorf("cursor after 'k' = %d, want 0", m.sessionCursor)
	}

	// esc returns to list
	m, _ = press(m, "esc")
	if m.view != viewList {
		t.Errorf("esc did not return to list, view = %v", m.view)
	}
}

func TestSessionsErrorIsVisible(t *testing.T) {
	st := realStore(t)
	m := New(st, nil)
	m.width, m.height = 100, 20
	next, _ := m.openSessions()
	m = next.(Model)

	m = update(m, sessionsMsg{err: errString("disk read error")})
	if !strings.Contains(m.View(), "disk read error") {
		t.Errorf("expected error in view:\n%s", m.View())
	}
}
