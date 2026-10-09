package store

import (
	"fmt"
	"sort"
	"time"
)

// ExpiringWindow is how soon a memory counts as "expiring soon" on the
// metrics screen.
const ExpiringWindow = 30 * 24 * time.Hour

// Health aggregates a store-wide snapshot in a handful of passes. Every count
// here is cheap local SQL; it deliberately does not reproduce `engram doctor`,
// which is a diagnostic verdict owned by Engram.
func (s *Store) Health() (Health, error) {
	var h Health
	cutoff := time.Now().UTC().Add(ExpiringWindow).Format("2006-01-02 15:04:05")
	row := s.db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM observations),
			(SELECT COUNT(*) FROM observations WHERE deleted_at IS NULL),
			(SELECT COUNT(*) FROM observations WHERE deleted_at IS NOT NULL),
			(SELECT COUNT(*) FROM sessions),
			(SELECT COUNT(*) FROM sessions WHERE ended_at IS NULL),
			(SELECT COUNT(*) FROM user_prompts),
			(SELECT COUNT(DISTINCT project) FROM sessions),
			(SELECT COUNT(*) FROM observations o
			   WHERE o.deleted_at IS NULL AND NOT EXISTS (SELECT 1 FROM sessions s WHERE s.id = o.session_id)),
			(SELECT COUNT(*) FROM sessions a
			   WHERE a.ended_at IS NULL AND EXISTS (SELECT 1 FROM sessions b
			     WHERE b.ended_at IS NULL AND b.id <> a.id AND b.project = a.project)),
			(SELECT COALESCE(SUM(duplicate_count - 1), 0) FROM observations WHERE deleted_at IS NULL),
			(SELECT COUNT(*) FROM observations WHERE pinned = 1 AND deleted_at IS NULL)`)
	if err := row.Scan(&h.TotalObservations, &h.LiveObservations, &h.DeletedCount,
		&h.TotalSessions, &h.ActiveSessions, &h.TotalPrompts, &h.Projects,
		&h.OrphanSessions, &h.AmbiguousSessions, &h.DuplicateCount,
		&h.PinnedCount); err != nil {
		return Health{}, fmt.Errorf("aggregate health: %w", err)
	}

	// ExpiringSoon is counted on its own rather than as another subquery: the
	// cutoff placeholder inside the aggregate above sat in an easy-to-misread
	// argument position.
	var expiring int
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM observations
		WHERE deleted_at IS NULL AND expires_at IS NOT NULL AND expires_at <= ?`, cutoff).
		Scan(&expiring); err != nil {
		return Health{}, fmt.Errorf("count expiring: %w", err)
	}
	h.ExpiringSoon = expiring

	// "Projects" comes from sessions, so an observation carrying a project
	// name no session owns means a project-name mismatch has already drifted.
	var n int
	if err := s.db.QueryRow(`
		SELECT COUNT(DISTINCT o.project) FROM observations o
		WHERE o.deleted_at IS NULL AND o.project IS NOT NULL AND o.project <> ''
		  AND NOT EXISTS (SELECT 1 FROM sessions s WHERE s.project = o.project)`).Scan(&n); err != nil {
		return Health{}, fmt.Errorf("count orphan projects: %w", err)
	}
	h.ProjectsWithoutSession = n
	return h, nil
}

// Projects lists every project name with its counts, biggest first. Project
// names are derived from sessions because Engram has no projects table.
func (s *Store) Projects() ([]ProjectStats, error) {
	rows, err := s.db.Query(`
		SELECT s.project,
		       (SELECT COUNT(*) FROM observations o WHERE o.project = s.project AND o.deleted_at IS NULL),
		       (SELECT COUNT(*) FROM observations o WHERE o.project = s.project AND o.deleted_at IS NOT NULL),
		       (SELECT COUNT(*) FROM sessions x WHERE x.project = s.project),
		       (SELECT COUNT(*) FROM user_prompts p WHERE p.project = s.project)
		FROM sessions s GROUP BY s.project ORDER BY 2 DESC, 1 ASC`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	byName := map[string]*ProjectStats{}
	var order []string
	for rows.Next() {
		var p ProjectStats
		if err := rows.Scan(&p.Name, &p.Observations, &p.DeletedCount, &p.Sessions, &p.Prompts); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		byName[p.Name] = &p
		order = append(order, p.Name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	dirs, err := s.sessionDirectories()
	if err != nil {
		return nil, err
	}
	for name, list := range dirs {
		if p, ok := byName[name]; ok {
			sort.Strings(list)
			p.Directories = list
		}
	}

	out := make([]ProjectStats, 0, len(order))
	for _, name := range order {
		out = append(out, *byName[name])
	}
	return out, nil
}

func (s *Store) sessionDirectories() (map[string][]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT project, COALESCE(directory, '') FROM sessions`)
	if err != nil {
		return nil, fmt.Errorf("list session directories: %w", err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var project, dir string
		if err := rows.Scan(&project, &dir); err != nil {
			return nil, fmt.Errorf("scan directory: %w", err)
		}
		out[project] = append(out[project], dir)
	}
	return out, rows.Err()
}

// Sessions returns sessions newest first, optionally filtered by project.
// Active sessions come first inside each group so a stale pair is visible.
func (s *Store) Sessions(project string, includeEnded bool, limit int) ([]Session, error) {
	if limit <= 0 {
		limit = 200
	}
	q := `
		SELECT s.id, s.project, COALESCE(s.directory, ''), s.started_at, COALESCE(s.ended_at, ''),
		       COALESCE(s.summary, ''), COALESCE(s.ownership_mode, ''),
		       COALESCE(s.runtime_lease_expires_at, ''), COALESCE(s.local_creation_project, ''),
		       (SELECT COUNT(*) FROM observations o WHERE o.session_id = s.id AND o.deleted_at IS NULL),
		       COALESCE((SELECT MAX(o.created_at) FROM observations o WHERE o.session_id = s.id), '')
		FROM sessions s WHERE 1=1`
	var args []any
	if project != "" {
		q += " AND s.project = ?"
		args = append(args, project)
	}
	if !includeEnded {
		q += " AND s.ended_at IS NULL"
	}
	q += " ORDER BY s.ended_at IS NOT NULL, s.started_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var x Session
		if err := rows.Scan(&x.ID, &x.Project, &x.Directory, &x.StartedAt, &x.EndedAt,
			&x.Summary, &x.OwnershipMode, &x.RuntimeLeaseExpiresAt, &x.LocalCreationProject,
			&x.ObservationCount, &x.LatestObservationAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// Facet returns the distinct values of a column with live counts, sorted by
// count descending. Used to populate the filter menu.
func (s *Store) Facet(column string) ([]FacetValue, error) {
	// The column name comes from a fixed set in the UI, never from keystrokes.
	switch column {
	case "type", "scope", "project":
	default:
		return nil, fmt.Errorf("facet: unsupported column %q", column)
	}
	rows, err := s.db.Query(fmt.Sprintf(
		`SELECT %s AS v, COUNT(*) FROM observations
		 WHERE deleted_at IS NULL AND %s IS NOT NULL AND %s <> ''
		 GROUP BY %s ORDER BY 2 DESC, 1 ASC`, column, column, column, column))
	if err != nil {
		return nil, fmt.Errorf("facet %s: %w", column, err)
	}
	defer rows.Close()
	var out []FacetValue
	for rows.Next() {
		var f FacetValue
		if err := rows.Scan(&f.Value, &f.Count); err != nil {
			return nil, fmt.Errorf("scan facet: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FacetValue is one distinct value and how many live observations carry it.
type FacetValue struct {
	Value string
	Count int
}

// RecentActivity returns live observations from the last window, newest first.
// The dashboard uses it to show what Engram has been recording lately.
func (s *Store) RecentActivity(window time.Duration, limit int) ([]Observation, error) {
	if window <= 0 {
		window = 7 * 24 * time.Hour
	}
	if limit <= 0 {
		limit = 20
	}
	cutoff := time.Now().UTC().Add(-window).Format("2006-01-02 15:04:05")
	rows, err := s.db.Query("SELECT"+obsColumns+obsJoin+`
		WHERE o.deleted_at IS NULL AND o.created_at >= ?
		ORDER BY o.created_at DESC, o.id DESC LIMIT ?`, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("recent activity: %w", err)
	}
	return scanObservations(rows)
}

// DBSize returns the on-disk size of the database, its WAL, and their sum.
// The WAL is the interesting half: it is where unmerged writes live, and it
// is routinely larger than the database itself.
func (s *Store) DBSize() (dbSize, walSize int64, err error) {
	var st int64
	if err := fileSize(s.path, &st); err != nil {
		return 0, 0, err
	}
	if err := fileSize(s.path+"-wal", &walSize); err != nil {
		return 0, 0, err
	}
	return st, walSize, nil
}

// EmbeddingStats aggregates vector store coverage and model details if the
// observation_embeddings table exists.
func (s *Store) EmbeddingStats() (EmbeddingStats, error) {
	var exists int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='observation_embeddings'`).Scan(&exists)
	if err != nil || exists == 0 {
		return EmbeddingStats{Available: false}, nil
	}

	var stats EmbeddingStats
	stats.Available = true

	_ = s.db.QueryRow(`SELECT COUNT(*) FROM observations WHERE deleted_at IS NULL`).Scan(&stats.LiveCount)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM observation_embeddings`).Scan(&stats.TotalEmbedded)
	_ = s.db.QueryRow(`
		SELECT COUNT(*) FROM observations o
		WHERE o.deleted_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM observation_embeddings e WHERE e.observation_id = o.id)
	`).Scan(&stats.PendingCount)

	var model string
	var dims int
	var latest string
	_ = s.db.QueryRow(`
		SELECT COALESCE(model, ''), COALESCE(dimensions, 0), COALESCE(MAX(created_at), '')
		FROM observation_embeddings
		WHERE model IS NOT NULL AND model <> ''
		GROUP BY model, dimensions
		ORDER BY COUNT(*) DESC LIMIT 1
	`).Scan(&model, &dims, &latest)

	stats.Model = model
	stats.Dimensions = dims
	stats.LatestAt = latest

	if stats.LiveCount > 0 {
		covered := stats.LiveCount - stats.PendingCount
		if covered < 0 {
			covered = 0
		}
		stats.CoveragePct = float64(covered) / float64(stats.LiveCount) * 100.0
	} else if stats.TotalEmbedded > 0 {
		stats.CoveragePct = 100.0
	}

	return stats, nil
}

