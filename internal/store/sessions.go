package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// SessionByID fetches a single session by its ID.
func (s *Store) SessionByID(sessionID string) (*Session, error) {
	if sessionID == "" {
		return nil, errors.New("empty session id")
	}

	q := `SELECT s.id, s.project, COALESCE(s.directory, ''), s.started_at, COALESCE(s.ended_at, ''),
		COALESCE(s.summary, ''), COALESCE(s.ownership_mode, ''),
		COALESCE(s.runtime_lease_expires_at, ''), COALESCE(s.local_creation_project, ''),
		(SELECT COUNT(*) FROM observations o WHERE o.session_id = s.id AND o.deleted_at IS NULL),
		COALESCE((SELECT MAX(o.created_at) FROM observations o WHERE o.session_id = s.id), '')
	FROM sessions s WHERE s.id = ?`

	var x Session
	err := s.db.QueryRow(q, sessionID).Scan(
		&x.ID, &x.Project, &x.Directory, &x.StartedAt, &x.EndedAt,
		&x.Summary, &x.OwnershipMode, &x.RuntimeLeaseExpiresAt,
		&x.LocalCreationProject, &x.ObservationCount, &x.LatestObservationAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get session by id: %w", err)
	}

	return &x, nil
}
