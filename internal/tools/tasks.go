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
	Owner       string    `json:"owner,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GlobalTaskStore is the shared task store.
var GlobalTaskStore = &TaskStore{
	tasks: make(map[string]*Task),
}

// --- TaskCreateTool ---

type TaskCreateTool struct {
	deferrable
}

type taskCreateInput struct {
	Subject     string `json:"subject"`
	Description string `json:"description"`
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
		lines = append(lines, fmt.Sprintf("#%s %s [%s] %s", task.ID, icon, task.Status, task.Subject))
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
