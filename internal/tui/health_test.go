package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"engram-manager/internal/store"
)

// A concern must appear exactly when its condition is true, and must never
// appear otherwise: a metrics screen that always shows warnings teaches the
// user to ignore it.
func TestConcernsFireOnlyWhenTrue(t *testing.T) {
	clean := store.Health{}
	if got := concerns(clean); len(got) != 0 {
		// Every entry must be inert on an empty store.
		for _, c := range got {
			if c.when {
				t.Errorf("a concern fired on an empty store: %s", c.label)
			}
		}
	}

	tests := []struct {
		name   string
		health store.Health
		want   string
	}{
		{"ambiguous", store.Health{AmbiguousSessions: 4}, "share a project"},
		{"no owning session", store.Health{ProjectsWithoutSession: 2}, "no owning session"},
		{"orphan", store.Health{OrphanSessions: 1}, "does not exist"},
		{"expiring", store.Health{ExpiringSoon: 3}, "expire within"},
		{"deleted", store.Health{DeletedCount: 9}, "soft-deleted"},
	}
	for _, tc := range tests {
		found := false
		for _, c := range concerns(tc.health) {
			if c.when && strings.Contains(c.label, tc.want) {
				found = true
				if c.why == "" {
					t.Errorf("%s: concern has no explanation", tc.name)
				}
			}
		}
		if !found {
			t.Errorf("%s: no concern reported for %+v", tc.name, tc.health)
		}
	}
}

func TestHumanSize(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{25100288, "23.9 MiB"},
		{23142072, "22.1 MiB"},
		{1 << 30, "1.0 GiB"},
	}
	for _, tc := range tests {
		if got := humanSize(tc.in); got != tc.want {
			t.Errorf("humanSize(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHealthScreenLoadsAndExits(t *testing.T) {
	st := realStore(t)
	m := New(st, nil)
	m.width, m.height = 100, 50

	next, cmd := m.loadHealthCmd()
	m = next.(Model)
	if m.view != viewHealth {
		t.Fatal("m did not open the metrics screen")
	}
	if m.health != nil {
		t.Error("metrics rendered before any data arrived")
	}
	if !strings.Contains(m.View(), "reading metrics") {
		t.Error("no loading state while metrics are pending")
	}

	m2, _ := m.Update(cmd())
	m = m2.(Model)
	if m.health == nil {
		t.Fatal("metrics never arrived")
	}
	out := m.View()
	if !strings.Contains(out, "memories") || !strings.Contains(out, "database") {
		t.Errorf("metrics screen is missing core rows:\n%s", out)
	}

	// esc returns to the list without any write.
	next, escCmd := press(m, "esc")
	m = next.(Model)
	if m.view != viewList {
		t.Error("esc did not leave the metrics screen")
	}
	if escCmd != nil {
		t.Error("leaving the metrics screen issued a command")
	}
}

func TestHealthScreenIsReadOnly(t *testing.T) {
	st := realStore(t)
	f := newFakeAPI(t)
	m := New(st, f.client())
	m.width, m.height = 100, 50
	next, cmd := m.loadHealthCmd()
	m = next.(Model)
	m.Update(cmd())

	// r is deliberately absent: it re-reads the metrics and writes nothing.
	for _, key := range []string{"e", "d", "D", "p", "y", "enter"} {
		got, kcmd := press(m, key)
		if got.view != viewHealth {
			t.Errorf("%q left the metrics screen (view=%v)", key, got.view)
		}
		if kcmd != nil {
			t.Errorf("%q issued a command from the metrics screen", key)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("the metrics screen called the API: %v", f.calls)
	}
}

func TestHealthRefreshReloads(t *testing.T) {
	st := realStore(t)
	m := New(st, nil)
	m.width, m.height = 100, 50
	next, cmd := m.loadHealthCmd()
	m = next.(Model)
	m.Update(cmd())

	got, refresh := press(m, "r")
	if refresh == nil {
		t.Fatal("r did not schedule a refresh")
	}
	if got.view != viewHealth {
		t.Error("refresh left the metrics screen")
	}
	msg := refresh()
	if _, ok := msg.(healthMsg); !ok {
		t.Errorf("refresh produced %T, want healthMsg", msg)
	}
}

func TestHealthErrorIsVisible(t *testing.T) {
	st := realStore(t)
	m := New(st, nil)
	m.width, m.height = 100, 50
	next, _ := m.loadHealthCmd()
	m = next.(Model)

	got := update(m, healthMsg{err: errString("database is locked")})
	if !strings.Contains(got.View(), "database is locked") {
		t.Errorf("a failed read must be visible:\n%s", got.View())
	}
}

// The metrics screen runs read-only SQL, so it must work with the API client
// absent entirely.
func TestHealthScreenWithoutAPI(t *testing.T) {
	st := realStore(t)
	m := New(st, nil)
	m.width, m.height = 100, 50
	next, cmd := m.loadHealthCmd()
	m = next.(Model)
	m = update(m, cmd())
	if m.health == nil {
		t.Fatal("metrics require no API client")
	}
}

// update applies a message and returns the concrete model.
func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

var _ tea.Model = Model{}
