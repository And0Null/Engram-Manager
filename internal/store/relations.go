package store

import (
	"fmt"
	"strings"
	"time"
)

// RelationFilter narrows the query for memory relations.
type RelationFilter struct {
	ObservationID string // matches source_id or target_id
	SourceID      string
	TargetID      string
	Relation      string
	Status        string
	Limit         int
	Offset        int
}

// Relations returns memory relations matching the filter, newest first.
// It left-joins observations on both sides to provide immediate context titles.
func (s *Store) Relations(f RelationFilter) ([]MemoryRelation, error) {
	var where []string
	var args []any

	if f.ObservationID != "" {
		where = append(where, `(r.source_id = ? OR r.target_id = ?
			OR r.source_id IN (SELECT sync_id FROM observations WHERE id = CAST(? AS INTEGER))
			OR r.target_id IN (SELECT sync_id FROM observations WHERE id = CAST(? AS INTEGER)))`)
		args = append(args, f.ObservationID, f.ObservationID, f.ObservationID, f.ObservationID)
	}
	if f.SourceID != "" {
		where = append(where, "r.source_id = ?")
		args = append(args, f.SourceID)
	}
	if f.TargetID != "" {
		where = append(where, "r.target_id = ?")
		args = append(args, f.TargetID)
	}
	if f.Relation != "" {
		where = append(where, "r.relation = ?")
		args = append(args, f.Relation)
	}
	if f.Status != "" {
		where = append(where, "r.judgment_status = ?")
		args = append(args, f.Status)
	}

	q := `
		SELECT
			r.id, r.sync_id, COALESCE(r.source_id, ''), COALESCE(r.target_id, ''),
			r.relation, COALESCE(r.reason, ''), COALESCE(r.evidence, ''),
			COALESCE(r.confidence, 0.0), r.judgment_status,
			COALESCE(r.marked_by_actor, ''), COALESCE(r.marked_by_kind, ''),
			COALESCE(r.marked_by_model, ''), COALESCE(r.session_id, ''),
			r.created_at, r.updated_at,
			COALESCE(osrc.id, 0), COALESCE(osrc.title, ''), COALESCE(osrc.type, ''), COALESCE(osrc.project, ''),
			COALESCE(otgt.id, 0), COALESCE(otgt.title, ''), COALESCE(otgt.type, ''), COALESCE(otgt.project, '')
		FROM memory_relations r
		LEFT JOIN observations osrc ON (osrc.sync_id = r.source_id OR CAST(osrc.id AS TEXT) = r.source_id)
		LEFT JOIN observations otgt ON (otgt.sync_id = r.target_id OR CAST(otgt.id AS TEXT) = r.target_id)`

	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY r.created_at DESC, r.id DESC"

	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	q += " LIMIT ? OFFSET ?"
	args = append(args, limit, f.Offset)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list memory relations: %w", err)
	}
	defer rows.Close()

	var out []MemoryRelation
	for rows.Next() {
		var mr MemoryRelation
		if err := rows.Scan(
			&mr.ID, &mr.SyncID, &mr.SourceID, &mr.TargetID,
			&mr.Relation, &mr.Reason, &mr.Evidence,
			&mr.Confidence, &mr.JudgmentStatus,
			&mr.MarkedByActor, &mr.MarkedByKind,
			&mr.MarkedByModel, &mr.SessionID,
			&mr.CreatedAt, &mr.UpdatedAt,
			&mr.SourceObsID, &mr.SourceTitle, &mr.SourceType, &mr.SourceProject,
			&mr.TargetObsID, &mr.TargetTitle, &mr.TargetType, &mr.TargetProject,
		); err != nil {
			return nil, fmt.Errorf("scan memory relation: %w", err)
		}
		out = append(out, mr)
	}
	return out, rows.Err()
}

// RelationsSummary aggregates counts across relations and judgment statuses.
func (s *Store) RelationsSummary() (RelationsSummary, error) {
	sum := RelationsSummary{
		ByKind:   make(map[string]int),
		ByStatus: make(map[string]int),
	}

	rows, err := s.db.Query(`
		SELECT relation, judgment_status, COUNT(*)
		FROM memory_relations
		GROUP BY relation, judgment_status`)
	if err != nil {
		return sum, fmt.Errorf("aggregate memory relations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var rel, status string
		var count int
		if err := rows.Scan(&rel, &status, &count); err != nil {
			return sum, fmt.Errorf("scan relation count: %w", err)
		}
		sum.Total += count
		sum.ByKind[rel] += count
		sum.ByStatus[status] += count
		if rel == "conflicts_with" {
			sum.Conflicts += count
		}
		if status == "pending" {
			sum.Pending += count
		}
	}
	return sum, rows.Err()
}

// ActivitySummary aggregates observation creations in the last 24h, 7d, 30d,
// as well as breakdowns by type and scope.
func (s *Store) ActivitySummary() (ActivityStats, error) {
	stats := ActivityStats{
		ByType:  make(map[string]int),
		ByScope: make(map[string]int),
	}

	now := time.Now().UTC()
	c24h := now.Add(-24 * time.Hour).Format("2006-01-02 15:04:05")
	c7d := now.Add(-7 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	c30d := now.Add(-30 * 24 * time.Hour).Format("2006-01-02 15:04:05")

	err := s.db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM observations WHERE deleted_at IS NULL AND created_at >= ?),
			(SELECT COUNT(*) FROM observations WHERE deleted_at IS NULL AND created_at >= ?),
			(SELECT COUNT(*) FROM observations WHERE deleted_at IS NULL AND created_at >= ?)`,
		c24h, c7d, c30d).Scan(&stats.Last24Hours, &stats.Last7Days, &stats.Last30Days)
	if err != nil {
		return stats, fmt.Errorf("aggregate activity windows: %w", err)
	}

	// By type
	typeRows, err := s.db.Query(`
		SELECT type, COUNT(*) FROM observations
		WHERE deleted_at IS NULL AND type IS NOT NULL AND type <> ''
		GROUP BY type ORDER BY 2 DESC`)
	if err == nil {
		defer typeRows.Close()
		for typeRows.Next() {
			var t string
			var count int
			if err := typeRows.Scan(&t, &count); err == nil {
				stats.ByType[t] = count
			}
		}
	}

	// By scope
	scopeRows, err := s.db.Query(`
		SELECT scope, COUNT(*) FROM observations
		WHERE deleted_at IS NULL AND scope IS NOT NULL AND scope <> ''
		GROUP BY scope ORDER BY 2 DESC`)
	if err == nil {
		defer scopeRows.Close()
		for scopeRows.Next() {
			var sc string
			var count int
			if err := scopeRows.Scan(&sc, &count); err == nil {
				stats.ByScope[sc] = count
			}
		}
	}

	return stats, nil
}

// ReviewQueue returns observations that have an active review scheduled
// or are due for review, earliest review_after first.
func (s *Store) ReviewQueue(limit int) ([]Observation, error) {
	if limit <= 0 {
		limit = 50
	}
	q := "SELECT" + obsColumns + obsJoin + `
		WHERE o.deleted_at IS NULL
		  AND o.review_after IS NOT NULL
		  AND o.review_after <> ''
		ORDER BY o.review_after ASC, o.id ASC LIMIT ?`

	rows, err := s.db.Query(q, limit)
	if err != nil {
		return nil, fmt.Errorf("list review queue: %w", err)
	}
	return scanObservations(rows)
}

// SessionTimeline returns all live observations created in a given session,
// ordered chronologically from first to last.
func (s *Store) SessionTimeline(sessionID string) ([]Observation, error) {
	if sessionID == "" {
		return nil, nil
	}
	q := "SELECT" + obsColumns + obsJoin + `
		WHERE o.session_id = ? AND o.deleted_at IS NULL
		ORDER BY o.created_at ASC, o.id ASC`

	rows, err := s.db.Query(q, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session timeline: %w", err)
	}
	return scanObservations(rows)
}

// SessionPrompts returns user prompts recorded for a session in order.
func (s *Store) SessionPrompts(sessionID string) ([]Prompt, error) {
	if sessionID == "" {
		return nil, nil
	}
	rows, err := s.db.Query(`
		SELECT id, session_id, content, created_at
		FROM user_prompts
		WHERE session_id = ?
		ORDER BY created_at ASC, id ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session prompts: %w", err)
	}
	defer rows.Close()

	var out []Prompt
	for rows.Next() {
		var p Prompt
		if err := rows.Scan(&p.ID, &p.SessionID, &p.Content, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan prompt: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
