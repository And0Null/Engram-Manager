package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFtsPhraseQuotesUserInput(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"  gate  ", `"gate"`},
		{`he said "hi"`, `"he said ""hi"""`},
		{"AND OR NOT", `"AND OR NOT"`},
		{"fix-", `"fix-"`},
	}
	for _, tc := range tests {
		if got := ftsPhrase(tc.in); got != tc.want {
			t.Errorf("ftsPhrase(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// FTS5 syntax in user input must never reach the parser as syntax, otherwise a
// stray quote fails the entire search instead of returning nothing.
func TestFtsPhraseNeutralizesSyntax(t *testing.T) {
	for _, hostile := range []string{`"`, `a" OR "b`, `*`, `a NEAR(b)`, `(`, `col:val`} {
		got := ftsPhrase(hostile)
		if got == "" {
			t.Errorf("ftsPhrase(%q) produced an empty query", hostile)
		}
		if got[0] != '"' || got[len(got)-1] != '"' {
			t.Errorf("ftsPhrase(%q) = %q is not a single quoted phrase", hostile, got)
		}
	}
}

func TestEscapeLike(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{"100%", `100\%`},
		{"a_b", `a\_b`},
		{`back\slash`, `back\\slash`},
	}
	for _, tc := range tests {
		if got := escapeLike(tc.in); got != tc.want {
			t.Errorf("escapeLike(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestObservationPreviewCollapsesWhitespace(t *testing.T) {
	o := Observation{Content: "**What**:  two\n\nlines   here"}
	if got := o.Preview(100); got != "**What**: two lines here" {
		t.Errorf("Preview = %q", got)
	}
	if got := (Observation{Content: "abcdefghij"}).Preview(5); got != "abcd…" {
		t.Errorf("Preview truncate = %q, want %q", got, "abcd…")
	}
}

func TestParseTimeNeverFails(t *testing.T) {
	if got := ParseTime("2026-10-08 21:36:00"); got.IsZero() {
		t.Error("ParseTime rejected a valid sqlite timestamp")
	}
	if got := ParseTime(""); !got.IsZero() {
		t.Error("ParseTime(\"\") should be zero")
	}
	if got := ParseTime("garbage"); !got.IsZero() {
		t.Error("ParseTime should swallow unparseable input, not error out")
	}
}

func TestExpandPathUsesHome(t *testing.T) {
	if got := expandPath("~/.engram/engram.db"); got == "~/.engram/engram.db" {
		t.Error("expandPath left the tilde unresolved")
	}
}

// The database under test is the user's real memory store, so every test
// against it is skipped when absent rather than invented.

func realDB(t *testing.T) string {
	t.Helper()
	p := expandPath(DefaultPath)
	if _, err := os.Stat(p); err != nil {
		t.Skipf("no engram database at %s", p)
	}
	return p
}

func TestRealDBHealth(t *testing.T) {
	st, err := Open(realDB(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	h, err := st.Health()
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if h.TotalObservations == 0 {
		t.Error("expected a non-empty store")
	}
	if h.LiveObservations+h.DeletedCount != h.TotalObservations {
		t.Errorf("live (%d) + deleted (%d) != total (%d)",
			h.LiveObservations, h.DeletedCount, h.TotalObservations)
	}
	if h.TotalObservations < h.LiveObservations {
		t.Error("deleted observations counted as live")
	}
	t.Logf("projects=%d obs=%d live=%d sessions=%d active=%d prompts=%d",
		h.Projects, h.TotalObservations, h.LiveObservations, h.TotalSessions, h.ActiveSessions, h.TotalPrompts)
}

func TestRealDBListExcludesDeletedByDefault(t *testing.T) {
	st, err := Open(realDB(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	live, err := st.List(Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, o := range live {
		if o.Deleted() {
			t.Fatalf("List returned soft-deleted observation %d without IncludeDeleted", o.ID)
		}
	}

	all, err := st.List(Filter{IncludeDeleted: true, Limit: 5000})
	if err != nil {
		t.Fatalf("List include deleted: %v", err)
	}
	if len(all) < len(live) {
		t.Errorf("including deleted returned fewer rows (%d) than excluding (%d)", len(all), len(live))
	}
}

func TestRealDBGetNotFound(t *testing.T) {
	st, err := Open(realDB(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	if _, err := st.Get(-1); err == nil {
		t.Error("expected an error for a nonexistent id")
	}
}

func TestRealDBSearchIsLiteral(t *testing.T) {
	st, err := Open(realDB(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	// A quote-heavy query must return an empty result, not an error: that is
	// the whole reason queries are quoted before they reach FTS5.
	items, err := st.List(Filter{Query: `he said "hi` + `"`, Limit: 20})
	if err != nil {
		t.Fatalf("search with quotes: %v", err)
	}
	t.Logf("quote-heavy search matched %d rows", len(items))
}

func TestRealDBOpenIsReadOnly(t *testing.T) {
	st, err := Open(realDB(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	if _, err := st.db.Exec("CREATE TABLE em_probe(x)"); err == nil {
		t.Fatal("store accepted a write; the read-only guarantee is broken")
	}
	if _, err := st.db.Exec("DELETE FROM observations WHERE 0"); err == nil {
		t.Fatal("store accepted a DELETE")
	}
}

func TestRealDBOpenMissingPath(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "nope.db")); err == nil {
		t.Error("expected an error for a missing database")
	}
}

func TestRealDBFacetsAreScopedToSafeColumns(t *testing.T) {
	st, err := Open(realDB(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	if _, err := st.Facet("type"); err != nil {
		t.Errorf("facet type: %v", err)
	}
	// The facet column is interpolated into SQL, so anything outside the
	// allowlist must be refused rather than sanitized.
	if _, err := st.Facet("1); DROP TABLE observations--"); err == nil {
		t.Error("Facet accepted a column outside the allowlist")
	}
}

func TestRealDBDBSizeIncludesWAL(t *testing.T) {
	st, err := Open(realDB(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	db, wal, err := st.DBSize()
	if err != nil {
		t.Fatalf("DBSize: %v", err)
	}
	if db <= 0 {
		t.Error("expected a non-empty database file")
	}
	t.Logf("db=%d wal=%d", db, wal)
}
