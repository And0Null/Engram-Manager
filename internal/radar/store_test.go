package radar

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}

	st, err := NewForTest(db)
	if err != nil {
		t.Fatalf("new test store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})
	return st
}

func strPtr(s string) *string {
	return &s
}

func TestStore_CreateAndGetTask(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	created, err := st.CreateTask(ctx, CreateTaskInput{
		Project:         "Engram-Manager",
		Title:           "Implement SSE Watcher",
		Description:     "Stream events via http.Flusher",
		Category:        "feature",
		Priority:        "high",
		DueDate:         strPtr("2026-10-30"),
		OriginSessionID: strPtr("sess-1234"),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if created.ID == "" {
		t.Errorf("expected non-empty ID")
	}
	if created.Project != "Engram-Manager" {
		t.Errorf("expected project Engram-Manager, got %s", created.Project)
	}
	if created.Category != CategoryFeature {
		t.Errorf("expected category FEATURE, got %s", created.Category)
	}
	if created.Priority != PriorityHigh {
		t.Errorf("expected priority HIGH, got %s", created.Priority)
	}
	if created.Status != StatusPending {
		t.Errorf("expected status PENDING, got %s", created.Status)
	}

	// Fetch by ID
	fetched, err := st.GetTaskByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get task by id: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, fetched.ID)
	}
	if fetched.OriginSessionID == nil || *fetched.OriginSessionID != "sess-1234" {
		t.Errorf("expected origin session sess-1234")
	}
}

func TestStore_UpdateTaskAndCompletion(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	task, err := st.CreateTask(ctx, CreateTaskInput{
		Project: "alpha",
		Title:   "Initial task",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	// Update to IN_PROGRESS
	updated, err := st.UpdateTask(ctx, task.ID, UpdateTaskInput{
		Status: strPtr("IN_PROGRESS"),
	})
	if err != nil {
		t.Fatalf("update task to IN_PROGRESS: %v", err)
	}
	if updated.Status != StatusInProgress {
		t.Errorf("expected IN_PROGRESS, got %s", updated.Status)
	}
	if updated.CompletedAt != nil {
		t.Errorf("expected completed_at to be nil while IN_PROGRESS")
	}

	// Complete task
	completed, err := st.UpdateTask(ctx, task.ID, UpdateTaskInput{
		Status: strPtr("COMPLETED"),
	})
	if err != nil {
		t.Fatalf("update task to COMPLETED: %v", err)
	}
	if completed.Status != StatusCompleted {
		t.Errorf("expected COMPLETED, got %s", completed.Status)
	}
	if completed.CompletedAt == nil {
		t.Errorf("expected completed_at to be set on completion")
	}

	// Re-open task
	reopened, err := st.UpdateTask(ctx, task.ID, UpdateTaskInput{
		Status: strPtr("PENDING"),
	})
	if err != nil {
		t.Fatalf("reopen task: %v", err)
	}
	if reopened.CompletedAt != nil {
		t.Errorf("expected completed_at to be cleared on reopening")
	}
}

func TestStore_TaskNotesAndCascade(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	task, err := st.CreateTask(ctx, CreateTaskInput{
		Project: "notes-test",
		Title:   "Task with notes",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	note1, err := st.AddNote(ctx, task.ID, "alice", "Investigated logs.")
	if err != nil {
		t.Fatalf("add note 1: %v", err)
	}
	if note1.Author != "alice" || note1.Content != "Investigated logs." {
		t.Errorf("unexpected note content: %+v", note1)
	}

	time.Sleep(10 * time.Millisecond)
	note2, err := st.AddNote(ctx, task.ID, "bob", "Reproduced bug.")
	if err != nil {
		t.Fatalf("add note 2: %v", err)
	}

	// Get task and verify notes loaded
	tWithNotes, err := st.GetTaskByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if len(tWithNotes.Notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(tWithNotes.Notes))
	}
	if tWithNotes.Notes[0].ID != note1.ID || tWithNotes.Notes[1].ID != note2.ID {
		t.Errorf("notes order mismatch")
	}

	// Delete note2
	if err := st.DeleteNote(ctx, note2.ID); err != nil {
		t.Fatalf("delete note2: %v", err)
	}
	tAfterNoteDel, err := st.GetTaskByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if len(tAfterNoteDel.Notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(tAfterNoteDel.Notes))
	}

	// Delete task, notes must cascade delete
	if err := st.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("delete task: %v", err)
	}

	var noteCount int
	err = st.DB().QueryRow("SELECT COUNT(*) FROM radar_task_notes WHERE task_id = ?", task.ID).Scan(&noteCount)
	if err != nil {
		t.Fatalf("query notes: %v", err)
	}
	if noteCount != 0 {
		t.Errorf("expected 0 notes after cascade, got %d", noteCount)
	}
}

func TestStore_GetTasksFilters(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	_, _ = st.CreateTask(ctx, CreateTaskInput{
		Project:         "proj-a",
		Title:           "Fix crash in parser",
		Category:        "BUGFIX",
		Priority:        "HIGH",
		OriginSessionID: strPtr("sess-A"),
	})
	_, _ = st.CreateTask(ctx, CreateTaskInput{
		Project:         "proj-a",
		Title:           "Add export button",
		Category:        "FEATURE",
		Priority:        "MEDIUM",
		OriginSessionID: strPtr("sess-B"),
	})
	_, _ = st.CreateTask(ctx, CreateTaskInput{
		Project:  "proj-b",
		Title:    "Write documentation",
		Category: "IDEA",
		Priority: "LOW",
	})

	// Filter by project
	tasksA, totalA, err := st.GetTasks(ctx, TaskFilter{Project: "proj-a"})
	if err != nil {
		t.Fatalf("filter project: %v", err)
	}
	if totalA != 2 || len(tasksA) != 2 {
		t.Errorf("expected 2 tasks for proj-a, got %d (total: %d)", len(tasksA), totalA)
	}

	// Filter by priority
	tasksHigh, totalHigh, err := st.GetTasks(ctx, TaskFilter{Priority: "HIGH"})
	if err != nil {
		t.Fatalf("filter priority: %v", err)
	}
	if totalHigh != 1 || len(tasksHigh) != 1 {
		t.Errorf("expected 1 high priority task, got %d", totalHigh)
	}

	// Filter by search query
	tasksSearch, totalSearch, err := st.GetTasks(ctx, TaskFilter{Search: "parser"})
	if err != nil {
		t.Fatalf("filter search: %v", err)
	}
	if totalSearch != 1 || len(tasksSearch) != 1 {
		t.Errorf("expected 1 search match, got %d", totalSearch)
	}
	if tasksSearch[0].Title != "Fix crash in parser" {
		t.Errorf("unexpected task title: %s", tasksSearch[0].Title)
	}

	// Filter by OriginSessionID
	tasksSessA, totalSessA, err := st.GetTasks(ctx, TaskFilter{OriginSessionID: "sess-A"})
	if err != nil {
		t.Fatalf("filter origin session: %v", err)
	}
	if totalSessA != 1 || len(tasksSessA) != 1 {
		t.Errorf("expected 1 match for sess-A, got %d", totalSessA)
	}
}
