package web

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"engram-manager/internal/store"
	_ "modernc.org/sqlite"
)

func createTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}

	schema := `
	CREATE TABLE sessions (
		id TEXT PRIMARY KEY,
		project TEXT,
		directory TEXT,
		started_at TEXT NOT NULL DEFAULT (datetime('now')),
		ended_at TEXT,
		summary TEXT,
		ownership_mode TEXT,
		runtime_lease_expires_at TEXT,
		local_creation_project TEXT
	);

	CREATE TABLE observations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT REFERENCES sessions(id),
		type TEXT NOT NULL,
		title TEXT NOT NULL,
		content TEXT NOT NULL,
		tool_name TEXT,
		project TEXT,
		scope TEXT NOT NULL DEFAULT 'project',
		topic_key TEXT,
		revision_count INTEGER NOT NULL DEFAULT 1,
		duplicate_count INTEGER NOT NULL DEFAULT 1,
		last_seen_at TEXT,
		pinned INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now')),
		deleted_at TEXT,
		review_after TEXT,
		expires_at TEXT,
		sync_id TEXT,
		normalized_hash TEXT,
		embedding_model TEXT
	);

	CREATE VIRTUAL TABLE observations_fts USING fts5(
		title, content, tokenize = 'trigram'
	);

	CREATE TABLE user_prompts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT REFERENCES sessions(id),
		project TEXT,
		content TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);

	CREATE TABLE memory_relations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		sync_id TEXT NOT NULL UNIQUE,
		source_id TEXT,
		target_id TEXT,
		relation TEXT NOT NULL DEFAULT 'pending',
		reason TEXT,
		evidence TEXT,
		confidence REAL,
		judgment_status TEXT NOT NULL DEFAULT 'pending',
		marked_by_actor TEXT,
		marked_by_kind TEXT,
		marked_by_model TEXT,
		session_id TEXT,
		superseded_at TEXT,
		superseded_by_relation_id INTEGER,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("exec schema: %v", err)
	}

	return store.NewForTest(db)
}

func TestServerEndpoints(t *testing.T) {
	st := createTestStore(t)
	defer st.Close()

	// Seed one observation
	_, err := st.DB().Exec(`
		INSERT INTO sessions (id, project) VALUES ('s1', 'proj-alpha');
		INSERT INTO observations (id, session_id, type, title, content, project)
		VALUES (101, 's1', 'decision', 'Use SQLite', 'Architecture choice', 'proj-alpha');
	`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	srv, err := NewServer(st, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 1. GET /api/health
	res, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}

	// 2. GET /api/overview
	res, err = http.Get(ts.URL + "/api/overview")
	if err != nil {
		t.Fatalf("GET /api/overview: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		var errBody map[string]any
		json.NewDecoder(res.Body).Decode(&errBody)
		t.Fatalf("overview status = %d, want 200, errBody = %+v", res.StatusCode, errBody)
	}

	// 3. GET /api/observations
	res, err = http.Get(ts.URL + "/api/observations")
	if err != nil {
		t.Fatalf("GET /api/observations: %v", err)
	}
	var obsResp struct {
		Observations []store.Observation `json:"observations"`
		Count        int                 `json:"count"`
	}
	if err := json.NewDecoder(res.Body).Decode(&obsResp); err != nil {
		t.Fatalf("decode observations: %v", err)
	}
	if obsResp.Count != 1 || obsResp.Observations[0].Title != "Use SQLite" {
		t.Errorf("unexpected observations payload: %+v", obsResp)
	}

	// 4. GET /api/observations/101
	res, err = http.Get(ts.URL + "/api/observations/101")
	if err != nil {
		t.Fatalf("GET /api/observations/101: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		var errBody map[string]any
		json.NewDecoder(res.Body).Decode(&errBody)
		t.Fatalf("status = %d, want 200, errBody = %+v", res.StatusCode, errBody)
	}

	// 5. GET /api/relations
	res, err = http.Get(ts.URL + "/api/relations")
	if err != nil {
		t.Fatalf("GET /api/relations: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("relations status = %d, want 200", res.StatusCode)
	}

	// 6. GET /api/sessions
	res, err = http.Get(ts.URL + "/api/sessions")
	if err != nil {
		t.Fatalf("GET /api/sessions: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("sessions status = %d, want 200", res.StatusCode)
	}

	// 7. GET /api/sessions/s1/timeline
	res, err = http.Get(ts.URL + "/api/sessions/s1/timeline")
	if err != nil {
		t.Fatalf("GET /api/sessions/s1/timeline: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("session timeline status = %d, want 200", res.StatusCode)
	}

	// 8. GET /api/review-queue
	res, err = http.Get(ts.URL + "/api/review-queue")
	if err != nil {
		t.Fatalf("GET /api/review-queue: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("review queue status = %d, want 200", res.StatusCode)
	}

	// 9. GET / static index.html
	res, err = http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("static index status = %d, want 200", res.StatusCode)
	}
}
