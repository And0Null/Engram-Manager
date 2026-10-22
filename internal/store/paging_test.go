package store

import "testing"

// A page is only honest if the total it is quoted against counts the same rows
// with the same filters. These tests pin that agreement.

func TestSessionsPageAndCount(t *testing.T) {
	st := createTestDB(t)
	defer st.Close()

	_, err := st.db.Exec(`
		INSERT INTO sessions (id, project, started_at, ended_at) VALUES
			('s1', 'p1', '2026-10-01 10:00:00', NULL),
			('s2', 'p1', '2026-10-02 10:00:00', NULL),
			('s3', 'p1', '2026-10-03 10:00:00', '2026-10-03 11:00:00'),
			('s4', 'p2', '2026-10-04 10:00:00', '2026-10-04 11:00:00'),
			('s5', 'p2', '2026-10-05 10:00:00', NULL);
	`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	cases := []struct {
		name         string
		project      string
		includeEnded bool
		want         int
	}{
		{"all", "", true, 5},
		{"open only", "", false, 3},
		{"one project", "p1", true, 3},
		{"project and open", "p1", false, 2},
	}
	for _, c := range cases {
		got, err := st.SessionsCount(c.project, c.includeEnded)
		if err != nil {
			t.Fatalf("SessionsCount(%s): %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("SessionsCount(%s) = %d, want %d", c.name, got, c.want)
		}
	}

	seen := map[string]bool{}
	for offset := 0; offset < 6; offset += 2 {
		page, err := st.SessionsPage("", true, 2, offset)
		if err != nil {
			t.Fatalf("SessionsPage(offset %d): %v", offset, err)
		}
		for _, s := range page {
			if seen[s.ID] {
				t.Errorf("session %s appears in two pages", s.ID)
			}
			seen[s.ID] = true
		}
	}
	if len(seen) != 5 {
		t.Errorf("pages covered %d sessions, want 5", len(seen))
	}

	// Open sessions sort before ended ones, so the first page is all open.
	first, err := st.SessionsPage("", true, 2, 0)
	if err != nil {
		t.Fatalf("SessionsPage: %v", err)
	}
	for _, s := range first {
		if s.EndedAt != "" {
			t.Errorf("first page contains ended session %s", s.ID)
		}
	}
}

func TestCountHonoursSessionFilter(t *testing.T) {
	st := createTestDB(t)
	defer st.Close()

	_, err := st.db.Exec(`
		INSERT INTO sessions (id) VALUES ('s1'), ('s2');
		INSERT INTO observations (session_id, type, title, content, created_at) VALUES
			('s1', 'decision', 'a', 'x', '2026-10-01 10:00:00'),
			('s1', 'decision', 'b', 'x', '2026-10-01 11:00:00'),
			('s2', 'bugfix', 'c', 'x', '2026-10-01 12:00:00');
	`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	all, err := st.Count(Filter{})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if all != 3 {
		t.Errorf("Count(all) = %d, want 3", all)
	}

	// The total a paged view quotes must describe the page it is showing.
	one, err := st.Count(Filter{SessionID: "s1"})
	if err != nil {
		t.Fatalf("Count(session): %v", err)
	}
	list, err := st.List(Filter{SessionID: "s1"})
	if err != nil {
		t.Fatalf("List(session): %v", err)
	}
	if one != len(list) {
		t.Errorf("Count(session) = %d but the page has %d rows", one, len(list))
	}
}

func TestReviewPendingCountMatchesTheQueue(t *testing.T) {
	st := createTestDB(t)
	defer st.Close()

	_, err := st.db.Exec(`
		INSERT INTO observations (type, title, content, review_after, deleted_at) VALUES
			('decision', 'due', 'x', '2026-01-01 00:00:00', NULL),
			('decision', 'due later', 'x', '2026-02-01 00:00:00', NULL),
			('decision', 'none', 'x', NULL, NULL),
			('decision', 'deleted', 'x', '2026-01-01 00:00:00', '2026-01-02 00:00:00');
	`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	n, err := st.ReviewPendingCount()
	if err != nil {
		t.Fatalf("ReviewPendingCount: %v", err)
	}
	queue, err := st.ReviewQueue(50)
	if err != nil {
		t.Fatalf("ReviewQueue: %v", err)
	}
	if n != 2 || len(queue) != n {
		t.Errorf("count = %d, queue = %d, want 2 and equal: the badge and the view must agree", n, len(queue))
	}
}
