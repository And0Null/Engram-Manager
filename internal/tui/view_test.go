package tui

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"engram-manager/internal/store"
)

// realStore opens the user's actual memory store, or skips. The UI is only
// ever exercised against real data: a fake would not catch the NULL columns,
// the odd timestamps, or the multi-megabyte content that actually show up.
func realStore(t *testing.T) *store.Store {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	path := home + "/.engram/engram.db"
	if _, err := os.Stat(path); err != nil {
		t.Skip("no engram database")
	}
	st, err := store.Open(path)
	if err != nil {
		t.Skipf("cannot open engram database: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestViewRendersList(t *testing.T) {
	st := realStore(t)
	items, err := st.List(store.Filter{Limit: 50})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) == 0 {
		t.Skip("no memories to render")
	}

	m := New(st, nil)
	m.width, m.height = 120, 24

	// Drive the real state transition rather than poking fields: the loaded
	// state is whatever Update produces, including the cleared status line.
	next, _ := m.Update(loadedMsg{items: items, total: len(items)})
	m = next.(Model)

	out := m.View()
	if strings.Contains(out, "loading memories…") {
		t.Error("View still shows the initial loading status after items arrived")
	}
	if !strings.Contains(out, trunc(oneLine(items[0].Title), clamp(m.width/4, 16, 52))) {
		t.Errorf("first title missing from the view:\n%s", out)
	}
	// Rows must fit the terminal; a long memory title or content must not wrap
	// the layout into unreadable garbage.
	for i, line := range strings.Split(out, "\n") {
		if w := len([]rune(stripANSI(line))); w > m.width+4 {
			t.Errorf("line %d is %d runes wide, over the %d column terminal:\n%s", i, w, m.width, line)
		}
	}
}

func TestViewRendersDetailWithEngramShape(t *testing.T) {
	st := realStore(t)
	items, err := st.List(store.Filter{Limit: 200})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Prefer a memory that actually has history, so the revisions and
	// relations sections are exercised rather than skipped.
	var o store.Observation
	for _, c := range items {
		if c.RevisionCount > 1 {
			o = c
			break
		}
	}
	if o.ID == 0 {
		o = items[0]
	}

	m := New(st, nil)
	m.width, m.height = 120, 40
	m.view = viewDetail
	m.detail, _ = st.Get(o.ID)
	m.versions, _ = st.Versions(o.ID)
	m.relations, _ = st.RelationsFor(o.ID)
	m.timeline, _ = st.Timeline(o.ID, 3, 3)

	out := m.View()
	if !strings.Contains(out, "#"+strconv.FormatInt(o.ID, 10)) {
		t.Errorf("detail is missing the observation id:\n%s", out)
	}
	if !strings.Contains(out, o.Type) {
		t.Errorf("detail is missing the type:\n%s", out)
	}
}

func TestViewEmptyState(t *testing.T) {
	st := realStore(t)
	m := New(st, nil)
	m.width, m.height = 80, 24
	m.status = ""
	if !strings.Contains(m.View(), "no memories match") {
		t.Error("expected an explicit empty state instead of a blank list")
	}
}

func TestViewErrorState(t *testing.T) {
	st := realStore(t)
	m := New(st, nil)
	m.width, m.height = 80, 24
	m.view = viewError
	m.err = os.ErrPermission
	if !strings.Contains(m.View(), "error:") {
		t.Error("expected the error to be visible")
	}
}

func TestWrapRespectsWidth(t *testing.T) {
	long := strings.Repeat("word ", 200)
	for _, line := range wrap(long, 40) {
		if len([]rune(line)) > 40 {
			t.Fatalf("wrapped line is %d runes: %q", len([]rune(line)), line)
		}
	}
	if got := wrap("a\n\nb", 40); len(got) != 3 {
		t.Errorf("blank-line paragraphs should be preserved, got %d lines", len(got))
	}
}

func TestWrapNarrowTerminalDoesNotBreak(t *testing.T) {
	for _, w := range []int{0, 1, 5, 19} {
		_ = wrap("some content here", w)
	}
}

func TestTruncAndPad(t *testing.T) {
	if got := trunc("abcdefghij", 5); got != "abcd…" {
		t.Errorf("trunc = %q", got)
	}
	if got := trunc("abc", 10); got != "abc" {
		t.Errorf("trunc shortened a short string: %q", got)
	}
	if got := pad("ab", 5); len([]rune(got)) != 5 {
		t.Errorf("pad = %q, width %d", got, len([]rune(got)))
	}
	if got := pad("abcdef", 3); got != "abcdef" {
		t.Errorf("pad must not truncate: %q", got)
	}
}

func TestOneLine(t *testing.T) {
	if got := oneLine("a\n\nb   c\t d"); got != "a b c d" {
		t.Errorf("oneLine = %q", got)
	}
}

func TestClamp(t *testing.T) {
	tests := []struct{ v, lo, hi, want int }{
		{5, 0, 10, 5}, {-1, 0, 10, 0}, {50, 0, 10, 10},
		{0, 0, -1, 0}, {3, 3, 3, 3},
	}
	for _, tc := range tests {
		if got := clamp(tc.v, tc.lo, tc.hi); got != tc.want {
			t.Errorf("clamp(%d,%d,%d) = %d, want %d", tc.v, tc.lo, tc.hi, got, tc.want)
		}
	}
}

func TestModelSurvivesNilAPI(t *testing.T) {
	// The UI must render with no API client: a stopped Engram server should
	// degrade to read-only, not crash the manager.
	st := realStore(t)
	m := New(st, nil)
	m.width, m.height = 100, 20
	_ = m.View()
}

func stripANSI(s string) string {
	var out strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case inEsc:
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
		case r == 0x1b:
			inEsc = true
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}
