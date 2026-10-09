// Package store reads the Engram SQLite database read-only.
//
// Every write path goes through the Engram HTTP API instead, so this package
// never needs write access and never has to replicate Engram's soft-delete,
// conflict-relation, or dedupe rules.
package store

import "time"

// Observation mirrors one row of the observations table.
//
// DeletedAt is non-nil for soft-deleted memories; those rows stay in the
// table on purpose, so a filter on it is a real distinction, not an edge case.
type Observation struct {
	ID              int64
	SessionID       string
	Type            string
	Title           string
	Content         string
	ToolName        string
	Project         string
	Scope           string
	TopicKey        string
	RevisionCount   int
	DuplicateCount  int
	LastSeenAt      string
	Pinned          bool
	CreatedAt       string
	UpdatedAt       string
	DeletedAt       string
	ReviewAfter     string
	ExpiresAt       string
	SyncID          string
	NormalizedHash  string
	EmbeddingModel  string
	SessionEndedAt  string
	SessionDirector string
}

// Deleted reports whether this observation was soft-deleted.
func (o Observation) Deleted() bool { return o.DeletedAt != "" }

// Preview returns a single-line excerpt of the content for list rendering.
func (o Observation) Preview(n int) string {
	s := oneLine(o.Content)
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func oneLine(s string) string {
	out := make([]rune, 0, len(s))
	var lastSpace bool
	for _, r := range s {
		switch r {
		case '\n', '\r', '\t':
			r = ' '
		}
		if r == ' ' && lastSpace {
			continue
		}
		lastSpace = r == ' '
		out = append(out, r)
	}
	return string(out)
}

// Session mirrors the sessions table. Engram has no projects table: project
// names are derived from sessions, which is why session drift is the usual
// source of a memory landing under the wrong project.
type Session struct {
	ID                    string
	Project               string
	Directory             string
	StartedAt             string
	EndedAt               string
	Summary               string
	OwnershipMode         string
	RuntimeLeaseExpiresAt string
	LocalCreationProject  string
	ObservationCount      int
	LatestObservationAt   string
}

// Active reports whether the session has no ended_at.
func (s Session) Active() bool { return s.EndedAt == "" }

// Prompt mirrors user_prompts.
type Prompt struct {
	ID        int64
	SessionID string
	Content   string
	CreatedAt string
}

// ProjectStats aggregates observation counts per project name.
type ProjectStats struct {
	Name         string
	Observations int
	Sessions     int
	Prompts      int
	Directories  []string
	DeletedCount int
}

// Health is a store-level snapshot for the metrics screen. It is deliberately
// narrower than `engram doctor`: this is fast local aggregation, not a
// diagnostic verdict.
type Health struct {
	TotalObservations      int
	LiveObservations       int
	DeletedCount           int
	TotalSessions          int
	ActiveSessions         int
	TotalPrompts           int
	Projects               int
	OrphanSessions         int
	AmbiguousSessions      int
	ProjectsWithoutSession int
	DuplicateCount         int
	PinnedCount            int
	ExpiringSoon           int
}

// Time helpers. Engram stores timestamps as SQLite datetime('now') strings,
// i.e. "YYYY-MM-DD HH:MM:SS" in UTC with no zone suffix.

const sqliteTimeLayout = "2006-01-02 15:04:05"

// ParseTime parses an Engram timestamp, returning the zero time for empty or
// unparseable input rather than an error: a broken timestamp must never take
// down a list render.
func ParseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{sqliteTimeLayout, time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
