package radar

import "time"

// Task category constants.
const (
	CategoryFeature  = "FEATURE"
	CategoryBugfix   = "BUGFIX"
	CategoryRefactor = "REFACTOR"
	CategoryIdea     = "IDEA"
)

// Task priority constants.
const (
	PriorityHigh   = "HIGH"
	PriorityMedium = "MEDIUM"
	PriorityLow    = "LOW"
)

// Task status constants.
const (
	StatusPending    = "PENDING"
	StatusInProgress = "IN_PROGRESS"
	StatusCompleted  = "COMPLETED"
	StatusDeferred   = "DEFERRED"
)

// Task represents a backlog/follow-up action tracked in the Action Radar.
type Task struct {
	ID              string     `json:"id"`
	Project         string     `json:"project"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	Category        string     `json:"category"`
	Priority        string     `json:"priority"`
	Status          string     `json:"status"`
	DueDate         *string    `json:"due_date,omitempty"`
	OriginSessionID *string    `json:"origin_session_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	Notes           []TaskNote `json:"notes,omitempty"`
}

// TaskNote represents a comment/context annotation attached to a task.
type TaskNote struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Author    string    `json:"author"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// TaskFilter defines criteria for querying tasks.
type TaskFilter struct {
	Project         string `json:"project"`
	Status          string `json:"status"`
	Priority        string `json:"priority"`
	Category        string `json:"category"`
	OriginSessionID string `json:"origin_session_id"`
	Search          string `json:"search"`
	Limit           int    `json:"limit"`
	Offset          int    `json:"offset"`
}

// CreateTaskInput holds payload for creating a task.
type CreateTaskInput struct {
	Project         string  `json:"project"`
	Title           string  `json:"title"`
	Description     string  `json:"description"`
	Category        string  `json:"category"`
	Priority        string  `json:"priority"`
	DueDate         *string `json:"due_date,omitempty"`
	OriginSessionID *string `json:"origin_session_id,omitempty"`
}

// UpdateTaskInput holds fields to update on an existing task.
type UpdateTaskInput struct {
	Title           *string `json:"title,omitempty"`
	Description     *string `json:"description,omitempty"`
	Category        *string `json:"category,omitempty"`
	Priority        *string `json:"priority,omitempty"`
	Status          *string `json:"status,omitempty"`
	DueDate         *string `json:"due_date,omitempty"`
	OriginSessionID *string `json:"origin_session_id,omitempty"`
}
