package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/prompts"
)

// TaskStore holds in-memory tasks (backed by SQLite in production).
type TaskStore struct {
	mu     sync.RWMutex
	tasks  map[string]*Task
	nextID int
}

// Task represents a tracked work item.
type Task struct {
	ID          string    `json:"id"`
	Subject     string    `json:"subject"`
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
			affected = append(affected, t)
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
}

type taskCreateInput struct {
	Subject     string `json:"subject"`
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
		Subject:     in.Subject,
		Description: in.Description,
		Status:      "pending",
		AssignedTo:  in.AssignedTo,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	store.tasks[id] = task
	store.mu.Unlock()

	return &Result{Content: fmt.Sprintf("Task #%s created: %s", id, in.Subject)}, nil
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
		lines = append(lines, fmt.Sprintf("#%s %s [%s] %s%s", task.ID, icon, task.Status, task.Subject, assignee))
	}

	return &Result{Content: strings.Join(lines, "\n")}, nil
}

// --- TaskUpdateTool ---

type TaskUpdateTool struct {
	deferrable
}

type taskUpdateInput struct {
	TaskID      string `json:"taskId"`
	Status      string `json:"status,omitempty"`
	Subject     string `json:"subject,omitempty"`
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
			"taskId": {"type": "string", "description": "The task ID to update"},
			"status": {"type": "string", "enum": ["pending", "in_progress", "completed", "deleted"]},
			"subject": {"type": "string"},
			"description": {"type": "string"}
		},
		"required": ["taskId"]
	}`)
}
func (t *TaskUpdateTool) IsReadOnly() bool                        { return false }
func (t *TaskUpdateTool) RequiresApproval(_ json.RawMessage) bool { return false }
func (t *TaskUpdateTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in taskUpdateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	// Accept both "taskId" and "task_id" (models sometimes use snake_case)
	if in.TaskID == "" {
		var alt struct {
			TaskID string `json:"task_id"`
		}
		_ = json.Unmarshal(input, &alt)
		in.TaskID = alt.TaskID
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
	if in.Subject != "" {
		task.Subject = in.Subject
	}
	if in.Description != "" {
		task.Description = in.Description
	}
	task.UpdatedAt = time.Now()

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
			"taskId": {"type": "string", "description": "The task ID"}
		},
		"required": ["taskId"]
	}`)
}
func (t *TaskGetTool) IsReadOnly() bool                        { return true }
func (t *TaskGetTool) RequiresApproval(_ json.RawMessage) bool { return false }
func (t *TaskGetTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in taskGetInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

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
