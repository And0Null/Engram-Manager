package store

import (
	"database/sql"
	"testing"
)

func createTestDB(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite memory: %v", err)
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

	CREATE TABLE observation_embeddings (
		observation_id INTEGER PRIMARY KEY,
		embedding BLOB NOT NULL,
		dimensions INTEGER NOT NULL,
		model TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	return &Store{db: db, path: ":memory:"}
}

func TestRelationsAndSummary(t *testing.T) {
	st := createTestDB(t)
	defer st.Close()

	// Seed observations
	_, err := st.db.Exec(`
		INSERT INTO observations (id, session_id, type, title, content, project)
		VALUES
			(1, 'ses-1', 'decision', 'Decision 1', 'Content 1', 'proj-a'),
			(2, 'ses-1', 'bugfix', 'Bugfix 2', 'Content 2', 'proj-a'),
			(3, 'ses-2', 'architecture', 'Arch 3', 'Content 3', 'proj-b');
	`)
	if err != nil {
		t.Fatalf("seed observations: %v", err)
	}

	// Seed relations
	_, err = st.db.Exec(`
		INSERT INTO memory_relations (sync_id, source_id, target_id, relation, judgment_status, reason, confidence)
		VALUES
			('rel-1', '1', '2', 'conflicts_with', 'judged', 'Direct contradiction', 0.95),
			('rel-2', '2', '3', 'related', 'judged', 'Shared domain logic', 0.8),
			('rel-3', '1', '3', 'pending', 'pending', 'Needs review', 0.5);
	`)
	if err != nil {
		t.Fatalf("seed relations: %v", err)
	}

	// Test Relations listing with joins
	rels, err := st.Relations(RelationFilter{})
	if err != nil {
		t.Fatalf("Relations(): %v", err)
	}
	if len(rels) != 3 {
		t.Fatalf("expected 3 relations, got %d", len(rels))
	}

	// Verify enriched join values
	r0 := rels[2] // rel-1
	if r0.SourceTitle != "Decision 1" || r0.TargetTitle != "Bugfix 2" {
		t.Errorf("joined titles unexpected: src=%q tgt=%q", r0.SourceTitle, r0.TargetTitle)
	}

	// Test RelationsSummary
	summary, err := st.RelationsSummary()
	if err != nil {
		t.Fatalf("RelationsSummary(): %v", err)
	}
	if summary.Total != 3 {
		t.Errorf("expected 3 total relations, got %d", summary.Total)
	}
	if summary.Conflicts != 1 {
		t.Errorf("expected 1 conflict, got %d", summary.Conflicts)
	}
	if summary.Pending != 1 {
		t.Errorf("expected 1 pending status, got %d", summary.Pending)
	}
	if summary.ByKind["conflicts_with"] != 1 || summary.ByKind["related"] != 1 {
		t.Errorf("ByKind map unexpected: %+v", summary.ByKind)
	}
}

func TestSessionTimelineAndPrompts(t *testing.T) {
	st := createTestDB(t)
	defer st.Close()

	_, err := st.db.Exec(`
		INSERT INTO sessions (id, project) VALUES ('ses-10', 'alpha');
		INSERT INTO user_prompts (session_id, content, created_at)
		VALUES ('ses-10', 'How do we fix the cache?', '2026-04-01 10:00:00');
		INSERT INTO observations (session_id, type, title, content, created_at)
		VALUES
			('ses-10', 'bugfix', 'Cache invalidation bug', 'Fixed invalidation', '2026-04-01 10:05:00'),
			('ses-10', 'learning', 'Redis TTL nuance', 'TTL key expiry nuance', '2026-04-01 10:10:00');
	`)
	if err != nil {
		t.Fatalf("seed session data: %v", err)
	}

	timeline, err := st.SessionTimeline("ses-10")
	if err != nil {
		t.Fatalf("SessionTimeline: %v", err)
	}
	if len(timeline) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(timeline))
	}
	if timeline[0].Title != "Cache invalidation bug" || timeline[1].Title != "Redis TTL nuance" {
		t.Errorf("order or titles mismatch in timeline: %+v", timeline)
	}

	prompts, err := st.SessionPrompts("ses-10")
	if err != nil {
		t.Fatalf("SessionPrompts: %v", err)
	}
	if len(prompts) != 1 || prompts[0].Content != "How do we fix the cache?" {
		t.Errorf("prompts mismatch: %+v", prompts)
	}
}

func TestReviewQueueAndActivity(t *testing.T) {
	st := createTestDB(t)
	defer st.Close()

	_, err := st.db.Exec(`
		INSERT INTO observations (type, title, content, review_after, created_at, scope)
		VALUES
			('pattern', 'Old Pattern', 'content', '2026-01-01 00:00:00', '2026-01-01 00:00:00', 'project'),
			('learning', 'Recent Learning', 'content', '2026-05-01 00:00:00', datetime('now'), 'global');
	`)
	if err != nil {
		t.Fatalf("seed review data: %v", err)
	}

	queue, err := st.ReviewQueue(10)
	if err != nil {
		t.Fatalf("ReviewQueue: %v", err)
	}
	if len(queue) != 2 {
		t.Errorf("expected 2 items in review queue, got %d", len(queue))
	}
	if queue[0].Title != "Old Pattern" {
		t.Errorf("earliest review item should be Old Pattern, got %s", queue[0].Title)
	}

	act, err := st.ActivitySummary()
	if err != nil {
		t.Fatalf("ActivitySummary: %v", err)
	}
	if act.Last24Hours != 1 {
		t.Errorf("expected 1 recent creation in 24h, got %d", act.Last24Hours)
	}
	if act.ByType["learning"] != 1 || act.ByType["pattern"] != 1 {
		t.Errorf("activity by type unexpected: %+v", act.ByType)
	}
	if act.ByScope["global"] != 1 || act.ByScope["project"] != 1 {
		t.Errorf("activity by scope unexpected: %+v", act.ByScope)
	}
}

func TestEmbeddingStats(t *testing.T) {
	st := createTestDB(t)
	defer st.Close()

	// Seed 2 observations
	_, err := st.db.Exec(`
		INSERT INTO observations (id, type, title, content, sync_id)
		VALUES
			(1, 'decision', 'Obs 1', 'Content 1', 'obs-1'),
			(2, 'bugfix', 'Obs 2', 'Content 2', 'obs-2');
		INSERT INTO observation_embeddings (observation_id, embedding, dimensions, model)
		VALUES (1, X'01020304', 384, 'test-minilm');
	`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	stats, err := st.EmbeddingStats()
	if err != nil {
		t.Fatalf("EmbeddingStats: %v", err)
	}
	if !stats.Available {
		t.Fatal("expected Available = true")
	}
	if stats.TotalEmbedded != 1 {
		t.Errorf("TotalEmbedded = %d, want 1", stats.TotalEmbedded)
	}
	if stats.PendingCount != 1 {
		t.Errorf("PendingCount = %d, want 1", stats.PendingCount)
	}
	if stats.Dimensions != 384 {
		t.Errorf("Dimensions = %d, want 384", stats.Dimensions)
	}
	if stats.Model != "test-minilm" {
		t.Errorf("Model = %q, want test-minilm", stats.Model)
	}
	if stats.CoveragePct != 50.0 {
		t.Errorf("CoveragePct = %v, want 50.0", stats.CoveragePct)
	}

	// Test enrichVector on Get and GetByRef
	obs1, err := st.Get(1)
	if err != nil || !obs1.HasVector || obs1.VectorDims != 384 {
		t.Errorf("obs1 vector enrichment failed: %+v, err: %v", obs1, err)
	}
	obs2, err := st.GetByRef("obs-2")
	if err != nil || obs2.HasVector {
		t.Errorf("obs2 should not have vector: %+v, err: %v", obs2, err)
	}
}
