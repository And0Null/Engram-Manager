package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrNotFound is returned when an observation id does not resolve.
var ErrNotFound = errors.New("observation not found")

// Filter selects and narrows an observation list. The zero value lists live
// observations, most recently updated first.
type Filter struct {
	Query          string // substring over title and content
	Project        string
	Type           string
	Scope          string
	SessionID      string
	TopicKey       string
	PinnedOnly     bool
	IncludeDeleted bool
	Limit          int
	Offset         int
}

// obsColumns is the shared projection, including the joined session columns so
// a list row can show where a memory came from without a second query.
const obsColumns = `
	o.id, COALESCE(o.session_id, ''), o.type, o.title, o.content, COALESCE(o.tool_name, ''), COALESCE(o.project, ''),
	o.scope, COALESCE(o.topic_key, ''), o.revision_count, o.duplicate_count, COALESCE(o.last_seen_at, ''),
	o.pinned, o.created_at, o.updated_at, COALESCE(o.deleted_at, ''),
	COALESCE(o.review_after, ''), COALESCE(o.expires_at, ''),
	COALESCE(o.sync_id, ''), COALESCE(o.normalized_hash, ''),
	COALESCE(o.embedding_model, ''),
	COALESCE(s.ended_at, ''), COALESCE(s.directory, '')`

const obsJoin = ` FROM observations o
	LEFT JOIN sessions s ON s.id = o.session_id `

// obsVectorColumn reports vector presence in the same round trip as the row,
// so a list can show the badge without a query per memory. Without the table
// the projection is a constant 0: a missing index must read as "no vector
// store", never as a crash or a silent "not embedded" claim per row.
func (s *Store) obsVectorColumn() string {
	if !s.hasVectorTable() {
		return ", 0"
	}
	return `, EXISTS(SELECT 1 FROM observation_embeddings e WHERE e.observation_id = o.id)`
}

func scanObservations(rows *sql.Rows) ([]Observation, error) {
	defer rows.Close()
	var out []Observation
	for rows.Next() {
		var o Observation
		err := rows.Scan(
			&o.ID, &o.SessionID, &o.Type, &o.Title, &o.Content, &o.ToolName, &o.Project,
			&o.Scope, &o.TopicKey, &o.RevisionCount, &o.DuplicateCount, &o.LastSeenAt,
			&o.Pinned, &o.CreatedAt, &o.UpdatedAt, &o.DeletedAt,
			&o.ReviewAfter, &o.ExpiresAt, &o.SyncID, &o.NormalizedHash,
			&o.EmbeddingModel, &o.SessionEndedAt, &o.SessionDirector,
			&o.HasVector,
		)
		if err != nil {
			return nil, fmt.Errorf("scan observation: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// List returns observations matching the filter.
func (s *Store) List(f Filter) ([]Observation, error) {
	var where []string
	var args []any

	if f.Query != "" {
		// Route through FTS5 rather than LIKE: the trigram index is what
		// mem_search already uses, so results match what the agent sees.
		where = append(where, "o.rowid IN (SELECT rowid FROM observations_fts WHERE observations_fts MATCH ?)")
		args = append(args, ftsPhrase(f.Query))
	}
	if f.Project != "" {
		where = append(where, "o.project = ?")
		args = append(args, f.Project)
	}
	if f.Type != "" {
		where = append(where, "o.type = ?")
		args = append(args, f.Type)
	}
	if f.Scope != "" {
		where = append(where, "o.scope = ?")
		args = append(args, f.Scope)
	}
	if f.SessionID != "" {
		where = append(where, "o.session_id = ?")
		args = append(args, f.SessionID)
	}
	if f.TopicKey != "" {
		where = append(where, "o.topic_key = ?")
		args = append(args, f.TopicKey)
	}
	if f.PinnedOnly {
		where = append(where, "o.pinned = 1")
	}
	if !f.IncludeDeleted {
		where = append(where, "o.deleted_at IS NULL")
	}

	q := "SELECT" + obsColumns + s.obsVectorColumn() + obsJoin
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	// updated_at first so an edited memory surfaces; id breaks ties so paging
	// is stable when timestamps collide.
	q += " ORDER BY o.updated_at DESC, o.id DESC"

	limit := f.Limit
	if limit <= 0 {
		limit = 200
	}
	q += " LIMIT ? OFFSET ?"
	args = append(args, limit, f.Offset)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list observations: %w", err)
	}
	return scanObservations(rows)
}

// Get returns one observation by id, including soft-deleted rows.
func (s *Store) Get(id int64) (Observation, error) {
	rows, err := s.db.Query("SELECT"+obsColumns+s.obsVectorColumn()+obsJoin+" WHERE o.id = ?", id)
	if err != nil {
		return Observation{}, fmt.Errorf("get observation %d: %w", id, err)
	}
	list, err := scanObservations(rows)
	if err != nil {
		return Observation{}, err
	}
	if len(list) == 0 {
		return Observation{}, fmt.Errorf("%w: %d", ErrNotFound, id)
	}
	s.enrichVector(&list[0])
	return list[0], nil
}

func (s *Store) enrichVector(obs *Observation) {
	var dims int
	var model string
	if err := s.db.QueryRow("SELECT dimensions, model FROM observation_embeddings WHERE observation_id = ?", obs.ID).Scan(&dims, &model); err == nil {
		obs.HasVector = true
		obs.VectorDims = dims
		obs.VectorModel = model
	}
}

// GetByRef resolves an observation either by numeric ID or by sync_id string.
func (s *Store) GetByRef(ref string) (Observation, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Observation{}, fmt.Errorf("%w: empty ref", ErrNotFound)
	}
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return s.Get(id)
	}
	rows, err := s.db.Query("SELECT"+obsColumns+s.obsVectorColumn()+obsJoin+" WHERE o.sync_id = ?", ref)
	if err != nil {
		return Observation{}, fmt.Errorf("get observation by sync_id %q: %w", ref, err)
	}
	list, err := scanObservations(rows)
	if err != nil {
		return Observation{}, err
	}
	if len(list) == 0 {
		return Observation{}, fmt.Errorf("%w: %s", ErrNotFound, ref)
	}
	s.enrichVector(&list[0])
	return list[0], nil
}

// Count returns how many observations match the filter, ignoring paging. The
// list screen uses it for "showing N of M".
func (s *Store) Count(f Filter) (int, error) {
	f2 := f
	f2.Limit, f2.Offset = 0, 0
	var where []string
	var args []any
	if f2.Query != "" {
		where = append(where, "o.rowid IN (SELECT rowid FROM observations_fts WHERE observations_fts MATCH ?)")
		args = append(args, ftsPhrase(f2.Query))
	}
	if f2.Project != "" {
		where = append(where, "o.project = ?")
		args = append(args, f2.Project)
	}
	if f2.Type != "" {
		where = append(where, "o.type = ?")
		args = append(args, f2.Type)
	}
	if f2.Scope != "" {
		where = append(where, "o.scope = ?")
		args = append(args, f2.Scope)
	}
	if f2.TopicKey != "" {
		where = append(where, "o.topic_key = ?")
		args = append(args, f2.TopicKey)
	}
	if f2.PinnedOnly {
		where = append(where, "o.pinned = 1")
	}
	if !f2.IncludeDeleted {
		where = append(where, "o.deleted_at IS NULL")
	}
	q := "SELECT COUNT(*)" + obsJoin
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	var n int
	if err := s.db.QueryRow(q, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count observations: %w", err)
	}
	return n, nil
}

// Version is one historical revision of an observation.
type Version struct {
	ObservationID int64
	Version       int
	Title         string
	Content       string
	CreatedAt     string
}

// Versions returns the stored revision history of an observation, newest first.
// This is the only local record of how a memory's wording drifted over time.
func (s *Store) Versions(id int64) ([]Version, error) {
	rows, err := s.db.Query(`
		SELECT observation_id, version, COALESCE(title, ''), content, COALESCE(created_at, '')
		FROM observation_versions WHERE observation_id = ?
		ORDER BY version DESC`, id)
	if err != nil {
		return nil, fmt.Errorf("list versions for %d: %w", id, err)
	}
	defer rows.Close()
	var out []Version
	for rows.Next() {
		var v Version
		if err := rows.Scan(&v.ObservationID, &v.Version, &v.Title, &v.Content, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Relation is one memory_relations row: how one memory supersedes or relates
// to another, and whether a human has judged that call.
type Relation struct {
	ID             int64
	SourceID       int64
	TargetID       int64
	Relation       string
	Reason         string
	JudgmentStatus string
	Confidence     float64
	MarkedByActor  string
	MarkedByModel  string
	CreatedAt      string
	UpdatedAt      string
}

// RelationsFor returns every relation touching the observation in either
// direction. Superseded relations are excluded because they no longer describe
// the current graph.
func (s *Store) RelationsFor(id int64) ([]Relation, error) {
	rows, err := s.db.Query(`
		SELECT id, COALESCE(source_id, 0), COALESCE(target_id, 0), relation,
		       COALESCE(reason, ''), judgment_status, COALESCE(confidence, 0),
		       COALESCE(marked_by_actor, ''), COALESCE(marked_by_model, ''),
		       created_at, updated_at
		FROM memory_relations
		WHERE (source_id = ? OR target_id = ?) AND superseded_at IS NULL
		ORDER BY created_at DESC`, id, id)
	if err != nil {
		return nil, fmt.Errorf("list relations for %d: %w", id, err)
	}
	defer rows.Close()
	var out []Relation
	for rows.Next() {
		var r Relation
		if err := rows.Scan(&r.ID, &r.SourceID, &r.TargetID, &r.Relation, &r.Reason,
			&r.JudgmentStatus, &r.Confidence, &r.MarkedByActor, &r.MarkedByModel,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan relation: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Timeline returns observations recorded close to the given one in the same
// session, oldest first — the local equivalent of `engram timeline`.
func (s *Store) Timeline(id int64, before, after int) ([]Observation, error) {
	if before <= 0 {
		before = 10
	}
	if after <= 0 {
		after = 10
	}
	anchor, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query("SELECT"+obsColumns+s.obsVectorColumn()+obsJoin+`
		WHERE o.deleted_at IS NULL
		  AND ((o.session_id = ? AND o.created_at <= ? AND o.id <> ?)
		    OR (o.session_id = ? AND o.created_at > ?))
		ORDER BY o.created_at ASC, o.id ASC
		LIMIT ?`, anchor.SessionID, anchor.CreatedAt, id, anchor.SessionID, anchor.CreatedAt, before+after)
	if err != nil {
		return nil, fmt.Errorf("timeline for %d: %w", id, err)
	}
	return scanObservations(rows)
}

// SearchPrompts runs a substring search over saved prompts.
func (s *Store) SearchPrompts(query string, limit int) ([]Prompt, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`
		SELECT id, session_id, content, created_at FROM user_prompts
		WHERE content LIKE ? ESCAPE '\' ORDER BY created_at DESC LIMIT ?`,
		"%"+escapeLike(query)+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("search prompts: %w", err)
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

// escapeLike neutralizes LIKE wildcards so a literal % in a search box cannot
// turn into a full-table scan the user did not ask for.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
