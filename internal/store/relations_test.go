package store

import (
	"database/sql"
	"testing"
	"time"
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

func TestTypeCountsWindowAndPercentages(t *testing.T) {
	st := createTestDB(t)
	defer st.Close()

	old := time.Now().UTC().AddDate(0, 0, -40).Format("2006-01-02 15:04:05")
	_, err := st.db.Exec(`
		INSERT INTO observations (type, title, content, created_at)
		VALUES
			('decision', 'a', 'x', ?),
			('decision', 'b', 'x', ?),
			('bugfix', 'c', 'x', datetime('now'));
	`, old, old)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	all, err := st.TypeCounts(0)
	if err != nil {
		t.Fatalf("TypeCounts(0): %v", err)
	}
	if len(all) != 2 || all[0].Type != "decision" || all[0].Count != 2 {
		t.Fatalf("all-time counts = %+v", all)
	}
	if diff := all[0].Pct - 66.66; diff > 0.01 || diff < -0.01 {
		t.Errorf("decision pct = %v, want ~66.67", all[0].Pct)
	}

	week, err := st.TypeCounts(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("TypeCounts(7d): %v", err)
	}
	if len(week) != 1 || week[0].Type != "bugfix" {
		t.Fatalf("7-day window should only see the fresh row, got %+v", week)
	}
	if week[0].Pct != 100 {
		t.Errorf("single-type window pct = %v, want 100", week[0].Pct)
	}
}

func TestDailyActivityIsGapFreeAndSummarizes(t *testing.T) {
	st := createTestDB(t)
	defer st.Close()

	now := time.Now().UTC()
	today := now.Format("2006-01-02 15:04:05")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02 15:04:05")
	longAgo := now.AddDate(0, 0, -40).Format("2006-01-02 15:04:05")

	_, err := st.db.Exec(`
		INSERT INTO observations (type, title, content, created_at)
		VALUES
			('learning', 'today-1', 'x', ?),
			('learning', 'today-2', 'x', ?),
			('learning', 'today-3', 'x', ?),
			('learning', 'yest', 'x', ?),
			('learning', 'ancient', 'x', ?);
	`, today, today, today, yesterday, longAgo)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	series, err := st.DailyActivity(30)
	if err != nil {
		t.Fatalf("DailyActivity: %v", err)
	}
	if len(series.Days) != 30 {
		t.Fatalf("len(Days) = %d, want 30 (gap-free)", len(series.Days))
	}
	if series.Total != 4 {
		t.Errorf("Total = %d, want 4 (the 40-day-old row is out of window)", series.Total)
	}
	if series.ActiveDays != 2 {
		t.Errorf("ActiveDays = %d, want 2", series.ActiveDays)
	}
	if series.PeakCount != 3 {
		t.Errorf("PeakCount = %d, want 3", series.PeakCount)
	}
	if series.PeakDate != now.Format("2006-01-02") {
		t.Errorf("PeakDate = %q, want today", series.PeakDate)
	}
	if diff := series.AvgPerDay - 4.0/30.0; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("AvgPerDay = %v, want %v", series.AvgPerDay, 4.0/30.0)
	}

	// The last bucket must be today, so the strip always ends at "now".
	last := series.Days[len(series.Days)-1]
	if last.Date != now.Format("2006-01-02") {
		t.Errorf("last day = %q, want today", last.Date)
	}
	if last.Count != 3 {
		t.Errorf("today count = %d, want 3", last.Count)
	}
}

func TestDailyActivityClampsLongRequests(t *testing.T) {
	st := createTestDB(t)
	defer st.Close()

	series, err := st.DailyActivity(365)
	if err != nil {
		t.Fatalf("DailyActivity: %v", err)
	}
	if len(series.Days) != 90 {
		t.Errorf("len(Days) = %d, want the 90-day clamp", len(series.Days))
	}
}

// The list projection reports vector presence. A list that silently says
// "not embedded" for every row is worse than no badge at all.
func TestListCarriesVectorPresence(t *testing.T) {
	st := createTestDB(t)
	defer st.Close()

	_, err := st.db.Exec(`
		INSERT INTO observations (id, type, title, content)
		VALUES (1, 'decision', 'embedded', 'x'), (2, 'decision', 'bare', 'x');
		INSERT INTO observation_embeddings (observation_id, embedding, dimensions, model)
		VALUES (1, X'0102', 384, 'test-minilm');
	`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	list, err := st.List(Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	byID := map[int64]Observation{}
	for _, o := range list {
		byID[o.ID] = o
	}
	if !byID[1].HasVector {
		t.Error("memory 1 has a vector but List reported HasVector=false")
	}
	if byID[2].HasVector {
		t.Error("memory 2 has no vector but List reported HasVector=true")
	}
}

// A store with no vector store at all must still list, and must report every
// row as un-embedded rather than failing the query.
func TestListWithoutVectorTableDegrades(t *testing.T) {
	st := createTestDB(t)
	defer st.Close()

	if _, err := st.db.Exec(`DROP TABLE observation_embeddings`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := st.db.Exec(`
		INSERT INTO observations (type, title, content) VALUES ('decision','a','x')`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	list, err := st.List(Filter{})
	if err != nil {
		t.Fatalf("List without a vector table must degrade, got: %v", err)
	}
	if len(list) != 1 || list[0].HasVector {
		t.Errorf("list = %+v, want one row with HasVector=false", list)
	}
}
