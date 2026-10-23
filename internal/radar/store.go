package radar

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DefaultPath is the default file location for the Action Radar SQLite database.
const DefaultPath = "~/.config/engram-manager/radar.db"

// ErrNotFound indicates a requested task or note does not exist.
var ErrNotFound = errors.New("not found")

// Store manages persistence of Action Radar tasks and notes.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens or creates the radar SQLite database with WAL and foreign keys enabled.
func Open(path string) (*Store, error) {
	abs, err := filepath.Abs(expandPath(path))
	if err != nil {
		return nil, fmt.Errorf("resolve radar path: %w", err)
	}

	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create radar directory: %w", err)
	}

	dsn := "file:" + abs + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open radar database: %w", err)
	}

	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping radar database: %w", err)
	}

	s := &Store{db: db, path: abs}
	if err := s.initSchema(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("init radar schema: %w", err)
	}

	return s, nil
}

// NewForTest wraps an in-memory or existing *sql.DB for testing.
func NewForTest(db *sql.DB) (*Store, error) {
	s := &Store{db: db, path: ":memory:"}
	if err := s.initSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("init test radar schema: %w", err)
	}
	return s, nil
}

// Path returns the absolute file path of the database.
func (s *Store) Path() string {
	return s.path
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB.
func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) initSchema(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return err
	}
	schema := `
	CREATE TABLE IF NOT EXISTS radar_tasks (
		id TEXT PRIMARY KEY,
		project TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		category TEXT NOT NULL DEFAULT 'FEATURE',
		priority TEXT NOT NULL DEFAULT 'MEDIUM',
		status TEXT NOT NULL DEFAULT 'PENDING',
		due_date TEXT,
		origin_session_id TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		completed_at TEXT
	);

	CREATE TABLE IF NOT EXISTS radar_task_notes (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		author TEXT NOT NULL DEFAULT 'user',
		content TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY (task_id) REFERENCES radar_tasks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_radar_project ON radar_tasks(project);
	CREATE INDEX IF NOT EXISTS idx_radar_status ON radar_tasks(status);
	CREATE INDEX IF NOT EXISTS idx_radar_origin_session ON radar_tasks(origin_session_id);
	CREATE INDEX IF NOT EXISTS idx_radar_priority ON radar_tasks(priority);
	CREATE INDEX IF NOT EXISTS idx_radar_created_at ON radar_tasks(created_at);
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// CreateTask persists a new task in the radar.
func (s *Store) CreateTask(ctx context.Context, in CreateTaskInput) (*Task, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, errors.New("task title is required")
	}
	project := strings.TrimSpace(in.Project)
	if project == "" {
		project = "general"
	}

	category := strings.ToUpper(strings.TrimSpace(in.Category))
	switch category {
	case CategoryFeature, CategoryBugfix, CategoryRefactor, CategoryIdea:
	default:
		category = CategoryFeature
	}

	priority := strings.ToUpper(strings.TrimSpace(in.Priority))
	switch priority {
	case PriorityHigh, PriorityMedium, PriorityLow:
	default:
		priority = PriorityMedium
	}

	id := generateID("tsk")
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	query := `
	INSERT INTO radar_tasks (
		id, project, title, description, category, priority, status,
		due_date, origin_session_id, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.ExecContext(ctx, query,
		id,
		project,
		title,
		strings.TrimSpace(in.Description),
		category,
		priority,
		StatusPending,
		in.DueDate,
		in.OriginSessionID,
		nowStr,
		nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}

	return &Task{
		ID:              id,
		Project:         project,
		Title:           title,
		Description:     strings.TrimSpace(in.Description),
		Category:        category,
		Priority:        priority,
		Status:          StatusPending,
		DueDate:         in.DueDate,
		OriginSessionID: in.OriginSessionID,
		CreatedAt:       now,
		UpdatedAt:       now,
		Notes:           []TaskNote{},
	}, nil
}

// GetTaskByID retrieves a single task along with its notes.
func (s *Store) GetTaskByID(ctx context.Context, id string) (*Task, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project, title, description, category, priority, status,
		       due_date, origin_session_id, created_at, updated_at, completed_at
		FROM radar_tasks
		WHERE id = ?
	`, id)

	var (
		t                                  Task
		createdAtStr, updatedAtStr         string
		completedAtStr                     sql.NullString
		dueDate, originSessionID           sql.NullString
	)

	err := row.Scan(
		&t.ID, &t.Project, &t.Title, &t.Description, &t.Category, &t.Priority, &t.Status,
		&dueDate, &originSessionID, &createdAtStr, &updatedAtStr, &completedAtStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan task: %w", err)
	}

	if dueDate.Valid {
		t.DueDate = &dueDate.String
	}
	if originSessionID.Valid {
		t.OriginSessionID = &originSessionID.String
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	if completedAtStr.Valid {
		cTime, err := time.Parse(time.RFC3339, completedAtStr.String)
		if err == nil {
			t.CompletedAt = &cTime
		}
	}

	notes, err := s.getNotesForTask(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	t.Notes = notes

	return &t, nil
}

// GetTasks queries tasks matching the given filter.
func (s *Store) GetTasks(ctx context.Context, filter TaskFilter) ([]Task, int, error) {
	where := []string{"1=1"}
	args := []any{}

	if p := strings.TrimSpace(filter.Project); p != "" {
		where = append(where, "project = ?")
		args = append(args, p)
	}
	if st := strings.TrimSpace(filter.Status); st != "" {
		where = append(where, "status = ?")
		args = append(args, strings.ToUpper(st))
	}
	if pr := strings.TrimSpace(filter.Priority); pr != "" {
		where = append(where, "priority = ?")
		args = append(args, strings.ToUpper(pr))
	}
	if cat := strings.TrimSpace(filter.Category); cat != "" {
		where = append(where, "category = ?")
		args = append(args, strings.ToUpper(cat))
	}
	if orig := strings.TrimSpace(filter.OriginSessionID); orig != "" {
		where = append(where, "origin_session_id = ?")
		args = append(args, orig)
	}
	if q := strings.TrimSpace(filter.Search); q != "" {
		where = append(where, "(title LIKE ? OR description LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like)
	}

	whereClause := strings.Join(where, " AND ")

	// Total count
	countQuery := "SELECT COUNT(*) FROM radar_tasks WHERE " + whereClause
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tasks: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	} else if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	selectQuery := fmt.Sprintf(`
		SELECT id, project, title, description, category, priority, status,
		       due_date, origin_session_id, created_at, updated_at, completed_at
		FROM radar_tasks
		WHERE %s
		ORDER BY 
			CASE status 
				WHEN 'IN_PROGRESS' THEN 1 
				WHEN 'PENDING' THEN 2 
				WHEN 'DEFERRED' THEN 3 
				WHEN 'COMPLETED' THEN 4 
				ELSE 5 
			END,
			CASE priority 
				WHEN 'HIGH' THEN 1 
				WHEN 'MEDIUM' THEN 2 
				WHEN 'LOW' THEN 3 
				ELSE 4 
			END,
			created_at DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	queryArgs := append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, selectQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("select tasks: %w", err)
	}
	defer rows.Close()

	tasks := make([]Task, 0)
	taskIDs := make([]string, 0)

	for rows.Next() {
		var (
			t                          Task
			createdAtStr, updatedAtStr string
			completedAtStr             sql.NullString
			dueDate, originSessionID   sql.NullString
		)

		if err := rows.Scan(
			&t.ID, &t.Project, &t.Title, &t.Description, &t.Category, &t.Priority, &t.Status,
			&dueDate, &originSessionID, &createdAtStr, &updatedAtStr, &completedAtStr,
		); err != nil {
			return nil, 0, fmt.Errorf("scan task row: %w", err)
		}

		if dueDate.Valid {
			t.DueDate = &dueDate.String
		}
		if originSessionID.Valid {
			t.OriginSessionID = &originSessionID.String
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
		if completedAtStr.Valid {
			cTime, err := time.Parse(time.RFC3339, completedAtStr.String)
			if err == nil {
				t.CompletedAt = &cTime
			}
		}
		t.Notes = []TaskNote{}

		tasks = append(tasks, t)
		taskIDs = append(taskIDs, t.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate tasks: %w", err)
	}

	// Fetch notes for the returned tasks if any
	if len(taskIDs) > 0 {
		notesMap, err := s.getNotesForTaskIDs(ctx, taskIDs)
		if err == nil {
			for i := range tasks {
				if n, ok := notesMap[tasks[i].ID]; ok {
					tasks[i].Notes = n
				}
			}
		}
	}

	return tasks, total, nil
}

// UpdateTask updates attributes of an existing task.
func (s *Store) UpdateTask(ctx context.Context, id string, in UpdateTaskInput) (*Task, error) {
	existing, err := s.GetTaskByID(ctx, id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	title := existing.Title
	if in.Title != nil {
		trimmed := strings.TrimSpace(*in.Title)
		if trimmed != "" {
			title = trimmed
		}
	}

	desc := existing.Description
	if in.Description != nil {
		desc = strings.TrimSpace(*in.Description)
	}

	category := existing.Category
	if in.Category != nil {
		cat := strings.ToUpper(strings.TrimSpace(*in.Category))
		switch cat {
		case CategoryFeature, CategoryBugfix, CategoryRefactor, CategoryIdea:
			category = cat
		}
	}

	priority := existing.Priority
	if in.Priority != nil {
		prio := strings.ToUpper(strings.TrimSpace(*in.Priority))
		switch prio {
		case PriorityHigh, PriorityMedium, PriorityLow:
			priority = prio
		}
	}

	status := existing.Status
	var completedAtVal any = nil
	if existing.CompletedAt != nil {
		completedAtVal = existing.CompletedAt.Format(time.RFC3339)
	}

	if in.Status != nil {
		st := strings.ToUpper(strings.TrimSpace(*in.Status))
		switch st {
		case StatusPending, StatusInProgress, StatusCompleted, StatusDeferred:
			status = st
			if status == StatusCompleted && existing.Status != StatusCompleted {
				completedAtVal = nowStr
			} else if status != StatusCompleted && existing.Status == StatusCompleted {
				completedAtVal = nil
			}
		}
	}

	dueDate := existing.DueDate
	if in.DueDate != nil {
		dueDate = in.DueDate
	}

	originSessionID := existing.OriginSessionID
	if in.OriginSessionID != nil {
		originSessionID = in.OriginSessionID
	}

	query := `
		UPDATE radar_tasks
		SET title = ?, description = ?, category = ?, priority = ?, status = ?,
		    due_date = ?, origin_session_id = ?, updated_at = ?, completed_at = ?
		WHERE id = ?
	`
	res, err := s.db.ExecContext(ctx, query,
		title, desc, category, priority, status,
		dueDate, originSessionID, nowStr, completedAtVal, id,
	)
	if err != nil {
		return nil, fmt.Errorf("update task: %w", err)
	}
	ra, _ := res.RowsAffected()
	if ra == 0 {
		return nil, ErrNotFound
	}

	return s.GetTaskByID(ctx, id)
}

// DeleteTask removes a task and cascades to its notes.
func (s *Store) DeleteTask(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM radar_tasks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	ra, _ := res.RowsAffected()
	if ra == 0 {
		return ErrNotFound
	}
	return nil
}

// AddNote appends a note to a radar task.
func (s *Store) AddNote(ctx context.Context, taskID string, author string, content string) (*TaskNote, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, errors.New("note content cannot be empty")
	}

	// Verify task exists
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM radar_tasks WHERE id = ?", taskID).Scan(&count); err != nil {
		return nil, fmt.Errorf("check task: %w", err)
	}
	if count == 0 {
		return nil, ErrNotFound
	}

	author = strings.TrimSpace(author)
	if author == "" {
		author = "user"
	}

	id := generateID("not")
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO radar_task_notes (id, task_id, author, content, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, taskID, author, content, nowStr)
	if err != nil {
		return nil, fmt.Errorf("insert note: %w", err)
	}

	// Update task updated_at
	_, err = tx.ExecContext(ctx, "UPDATE radar_tasks SET updated_at = ? WHERE id = ?", nowStr, taskID)
	if err != nil {
		return nil, fmt.Errorf("update task timestamp: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &TaskNote{
		ID:        id,
		TaskID:    taskID,
		Author:    author,
		Content:   content,
		CreatedAt: now,
	}, nil
}

// DeleteNote removes a specific note.
func (s *Store) DeleteNote(ctx context.Context, noteID string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM radar_task_notes WHERE id = ?", noteID)
	if err != nil {
		return fmt.Errorf("delete note: %w", err)
	}
	ra, _ := res.RowsAffected()
	if ra == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) getNotesForTask(ctx context.Context, taskID string) ([]TaskNote, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, author, content, created_at
		FROM radar_task_notes
		WHERE task_id = ?
		ORDER BY created_at ASC
	`, taskID)
	if err != nil {
		return nil, fmt.Errorf("query task notes: %w", err)
	}
	defer rows.Close()

	notes := make([]TaskNote, 0)
	for rows.Next() {
		var n TaskNote
		var cStr string
		if err := rows.Scan(&n.ID, &n.TaskID, &n.Author, &n.Content, &cStr); err != nil {
			return nil, fmt.Errorf("scan note: %w", err)
		}
		n.CreatedAt, _ = time.Parse(time.RFC3339, cStr)
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

func (s *Store) getNotesForTaskIDs(ctx context.Context, taskIDs []string) (map[string][]TaskNote, error) {
	if len(taskIDs) == 0 {
		return map[string][]TaskNote{}, nil
	}

	placeholders := make([]string, len(taskIDs))
	args := make([]any, len(taskIDs))
	for i, id := range taskIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, task_id, author, content, created_at
		FROM radar_task_notes
		WHERE task_id IN (%s)
		ORDER BY created_at ASC
	`, strings.Join(placeholders, ","))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("batch query notes: %w", err)
	}
	defer rows.Close()

	res := make(map[string][]TaskNote)
	for rows.Next() {
		var n TaskNote
		var cStr string
		if err := rows.Scan(&n.ID, &n.TaskID, &n.Author, &n.Content, &cStr); err != nil {
			return nil, fmt.Errorf("scan note: %w", err)
		}
		n.CreatedAt, _ = time.Parse(time.RFC3339, cStr)
		res[n.TaskID] = append(res[n.TaskID], n)
	}
	return res, rows.Err()
}

func generateID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
}

func expandPath(p string) string {
	if p == "" {
		p = DefaultPath
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
