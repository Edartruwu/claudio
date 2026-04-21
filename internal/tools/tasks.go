package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/prompts"
)

// TaskStore holds tasks in memory and optionally persists them to SQLite.
type TaskStore struct {
	mu             sync.RWMutex
	tasks          map[string]*Task
	nextID         int
	db             *sql.DB
	currentSession string
	bus            *bus.Bus
}

// Task represents a tracked work item.
type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // pending, in_progress, completed, deleted
	AssignedTo  string    `json:"assigned_to,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GlobalTaskStore is the shared task store.
var GlobalTaskStore = &TaskStore{
	tasks: make(map[string]*Task),
}

// SetDB wires a SQLite database for persistence, then loads any existing tasks.
func (s *TaskStore) SetDB(db *sql.DB) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = db
	s.initDB()
}

func (s *TaskStore) initDB() {
	// Table is created and migrated by internal/storage/db.go — nothing to do here.
}

// LoadForSession clears the in-memory store and loads only tasks belonging to sessionID.
func (s *TaskStore) LoadForSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = make(map[string]*Task)
	s.nextID = 0
	s.currentSession = sessionID
	if s.db == nil || sessionID == "" {
		return
	}
	rows, err := s.db.Query(`SELECT id, title, description, status, assigned_to, created_at, updated_at FROM team_tasks WHERE session_id = ? AND status != 'deleted' ORDER BY CAST(id AS INTEGER)`, sessionID)
	if err != nil {
		return
	}
	defer rows.Close()
	maxID := 0
	for rows.Next() {
		var t Task
		var assignedTo sql.NullString
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Status, &assignedTo, &t.CreatedAt, &t.UpdatedAt); err != nil {
			continue
		}
		if assignedTo.Valid {
			t.AssignedTo = assignedTo.String
		}
		s.tasks[t.ID] = &t
		var idNum int
		fmt.Sscanf(t.ID, "%d", &idNum)
		if idNum > maxID {
			maxID = idNum
		}
	}
	s.nextID = maxID
}

func (s *TaskStore) saveToDB(t *Task) {
	s.saveToDBWithSession(t, s.currentSession)
}

func (s *TaskStore) saveToDBWithSession(t *Task, sessionID string) {
	if s.db == nil {
		return
	}
	s.db.Exec(`INSERT OR REPLACE INTO team_tasks (id, session_id, title, description, status, assigned_to, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, sessionID, t.Title, t.Description, t.Status, t.AssignedTo, t.CreatedAt, t.UpdatedAt)
}

// List returns all non-deleted tasks sorted by numeric ID.
func (s *TaskStore) List() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Task
	for _, t := range s.tasks {
		if t.Status != "deleted" {
			out = append(out, t)
		}
	}
	return out
}

// Get returns a task by ID, or (nil, false) if not found or deleted.
func (s *TaskStore) Get(id string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[strings.TrimPrefix(id, "#")]
	if !ok || t.Status == "deleted" {
		return nil, false
	}
	return t, true
}

// CompleteByIDs marks all pending/in_progress tasks with matching IDs as the given status.
func (s *TaskStore) CompleteByIDs(ids []string, status string) []*Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	var affected []*Task
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[strings.TrimPrefix(id, "#")] = true
	}
	for _, t := range s.tasks {
		if idSet[t.ID] && (t.Status == "pending" || t.Status == "in_progress") {
			t.Status = status
			t.UpdatedAt = time.Now()
			s.saveToDB(t)
			affected = append(affected, t)
			
			// Publish EventTaskUpdated
			if s.bus != nil {
				payload, _ := json.Marshal(attach.TaskUpdatedPayload{
					ID:          t.ID,
					Title:       t.Title,
					Description: t.Description,
					AssignedTo:  t.AssignedTo,
					Status:      t.Status,
					SessionID:   s.currentSession,
				})
				s.bus.Publish(bus.Event{
					Type:      attach.EventTaskUpdated,
					SessionID: s.currentSession,
					Payload:   payload,
				})
			}
		}
	}
	return affected
}

// CompleteByAssignee marks all pending/in_progress tasks assigned to the given agent
// as the specified status ("completed" or "failed"). Returns the affected tasks.
func (s *TaskStore) CompleteByAssignee(agentName, status string) []*Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	var affected []*Task
	for _, t := range s.tasks {
		if t.AssignedTo == agentName && (t.Status == "pending" || t.Status == "in_progress") {
			t.Status = status
			t.UpdatedAt = time.Now()
			s.saveToDB(t)
			affected = append(affected, t)
			
			// Publish EventTaskUpdated
			if s.bus != nil {
				payload, _ := json.Marshal(attach.TaskUpdatedPayload{
					ID:          t.ID,
					Title:       t.Title,
					Description: t.Description,
					AssignedTo:  t.AssignedTo,
					Status:      t.Status,
					SessionID:   s.currentSession,
				})
				s.bus.Publish(bus.Event{
					Type:      attach.EventTaskUpdated,
					SessionID: s.currentSession,
					Payload:   payload,
				})
			}
		}
	}
	return affected
}

// ByAssignee returns all non-deleted tasks assigned to the given agent.
func (s *TaskStore) ByAssignee(agentName string) []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Task
	for _, t := range s.tasks {
		if t.AssignedTo == agentName && t.Status != "deleted" {
			out = append(out, t)
		}
	}
	return out
}

// --- TaskCreateTool ---

type TaskCreateTool struct {
	deferrable
	bus       *bus.Bus
	SessionID string
}

type taskCreateInput struct {
	Title       string `json:"subject"`
	Description string `json:"description"`
	AssignedTo  string `json:"assigned_to,omitempty"`
}

func (t *TaskCreateTool) Name() string { return "TaskCreate" }
func (t *TaskCreateTool) Description() string {
	return prompts.TaskCreateDescription()
}
func (t *TaskCreateTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"subject": {"type": "string", "description": "A brief title for the task"},
			"description": {"type": "string", "description": "What needs to be done"},
			"assigned_to": {"type": "string", "description": "Agent name to assign this task to (for team coordination)"},
			"activeForm": {"type": "string", "description": "Present continuous form shown in spinner when in_progress (e.g., \"Running tests\")"}
		},
		"required": ["subject", "description"]
	}`)
}
func (t *TaskCreateTool) IsReadOnly() bool                        { return false }
func (t *TaskCreateTool) RequiresApproval(_ json.RawMessage) bool { return false }
func (t *TaskCreateTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in taskCreateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	store := GlobalTaskStore
	store.mu.Lock()
	store.nextID++
	id := fmt.Sprintf("%d", store.nextID)
	task := &Task{
		ID:          id,
		Title:       in.Title,
		Description: in.Description,
		Status:      "pending",
		AssignedTo:  in.AssignedTo,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	store.tasks[id] = task
	store.saveToDBWithSession(task, t.SessionID)
	store.mu.Unlock()

	// Publish event
	if t.bus != nil {
		payload, _ := json.Marshal(attach.TaskCreatedPayload{
			ID:          id,
			Title:       in.Title,
			Description: in.Description,
			AssignedTo:  in.AssignedTo,
			Status:      "pending",
		})
		t.bus.Publish(bus.Event{
			Type:    attach.EventTaskCreated,
			Payload: payload,
		})
	}

	return &Result{Content: fmt.Sprintf("Task #%s created: %s", id, in.Title)}, nil
}

// --- TaskListTool ---

type TaskListTool struct {
	deferrable
}

func (t *TaskListTool) Name() string { return "TaskList" }
func (t *TaskListTool) Description() string {
	return prompts.TaskListDescription()
}
func (t *TaskListTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}
func (t *TaskListTool) IsReadOnly() bool                        { return true }
func (t *TaskListTool) RequiresApproval(_ json.RawMessage) bool { return false }
func (t *TaskListTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	store := GlobalTaskStore
	store.mu.RLock()
	defer store.mu.RUnlock()

	if len(store.tasks) == 0 {
		return &Result{Content: "No tasks"}, nil
	}

	var lines []string
	for _, task := range store.tasks {
		if task.Status == "deleted" {
			continue
		}
		icon := "○"
		switch task.Status {
		case "in_progress":
			icon = "◐"
		case "completed":
			icon = "●"
		}
		assignee := ""
		if task.AssignedTo != "" {
			assignee = fmt.Sprintf(" → %s", task.AssignedTo)
		}
		lines = append(lines, fmt.Sprintf("#%s %s [%s] %s%s", task.ID, icon, task.Status, task.Title, assignee))
	}

	return &Result{Content: strings.Join(lines, "\n")}, nil
}

// --- TaskUpdateTool ---

type TaskUpdateTool struct {
	deferrable
	bus *bus.Bus
}

type taskUpdateInput struct {
	TaskID      string `json:"taskId"`
	Status      string `json:"status,omitempty"`
	Title       string `json:"subject,omitempty"`
	Description string `json:"description,omitempty"`
}

func (t *TaskUpdateTool) Name() string { return "TaskUpdate" }
func (t *TaskUpdateTool) Description() string {
	return prompts.TaskUpdateDescription()
}
func (t *TaskUpdateTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"taskId": {"type": "string", "description": "The task ID to update (also accepted as 'id' or 'task_id')"},
			"id": {"type": "string", "description": "Alias for taskId"},
			"task_id": {"type": "string", "description": "Alias for taskId"},
			"status": {"type": "string", "enum": ["pending", "in_progress", "completed", "deleted"]},
			"subject": {"type": "string"},
			"description": {"type": "string"}
		}
	}`)
}
func (t *TaskUpdateTool) IsReadOnly() bool                        { return false }
func (t *TaskUpdateTool) RequiresApproval(_ json.RawMessage) bool { return false }
func (t *TaskUpdateTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in taskUpdateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	// Accept "task_id" or plain "id" (models sometimes use snake_case or shorthand)
	if in.TaskID == "" {
		var alt struct {
			TaskID string `json:"task_id"`
			ID     string `json:"id"`
		}
		_ = json.Unmarshal(input, &alt)
		if alt.TaskID != "" {
			in.TaskID = alt.TaskID
		} else {
			in.TaskID = alt.ID
		}
	}

	// Strip leading "#" if present (e.g. "#3" -> "3")
	in.TaskID = strings.TrimPrefix(in.TaskID, "#")

	store := GlobalTaskStore
	store.mu.Lock()
	defer store.mu.Unlock()

	task, ok := store.tasks[in.TaskID]
	if !ok {
		return &Result{Content: fmt.Sprintf("Task #%s not found", in.TaskID), IsError: true}, nil
	}

	if in.Status != "" {
		task.Status = in.Status
	}
	if in.Title != "" {
		task.Title = in.Title
	}
	if in.Description != "" {
		task.Description = in.Description
	}
	task.UpdatedAt = time.Now()
	store.saveToDB(task)

	// Publish event
	if t.bus != nil {
		payload, _ := json.Marshal(attach.TaskUpdatedPayload{
			ID:          in.TaskID,
			Title:       task.Title,
			Description: task.Description,
			AssignedTo:  task.AssignedTo,
			Status:      task.Status,
		})
		t.bus.Publish(bus.Event{
			Type:    attach.EventTaskUpdated,
			Payload: payload,
		})
	}

	return &Result{Content: fmt.Sprintf("Task #%s updated", in.TaskID)}, nil
}

// --- TaskGetTool ---

type TaskGetTool struct {
	deferrable
}

type taskGetInput struct {
	TaskID string `json:"taskId"`
}

func (t *TaskGetTool) Name() string { return "TaskGet" }
func (t *TaskGetTool) Description() string {
	return prompts.TaskGetDescription()
}
func (t *TaskGetTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"taskId": {"type": "string", "description": "The task ID (also accepted as 'id' or 'task_id')"},
			"id": {"type": "string", "description": "Alias for taskId"},
			"task_id": {"type": "string", "description": "Alias for taskId"}
		}
	}`)
}
func (t *TaskGetTool) IsReadOnly() bool                        { return true }
func (t *TaskGetTool) RequiresApproval(_ json.RawMessage) bool { return false }
func (t *TaskGetTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in taskGetInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	// Accept "task_id" or plain "id" as fallbacks
	if in.TaskID == "" {
		var alt struct {
			TaskID string `json:"task_id"`
			ID     string `json:"id"`
		}
		_ = json.Unmarshal(input, &alt)
		if alt.TaskID != "" {
			in.TaskID = alt.TaskID
		} else {
			in.TaskID = alt.ID
		}
	}
	// Strip leading "#" if present
	in.TaskID = strings.TrimPrefix(in.TaskID, "#")

	store := GlobalTaskStore
	store.mu.RLock()
	defer store.mu.RUnlock()

	task, ok := store.tasks[in.TaskID]
	if !ok {
		return &Result{Content: fmt.Sprintf("Task #%s not found", in.TaskID), IsError: true}, nil
	}

	data, _ := json.MarshalIndent(task, "", "  ")
	return &Result{Content: string(data)}, nil
}
