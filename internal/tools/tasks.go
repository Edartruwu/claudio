package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
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
	ID          string            `json:"id"`
	Subject     string            `json:"subject"`
	Description string            `json:"description"`
	Status      string            `json:"status"` // pending, in_progress, completed, deleted
	AssignedTo  string            `json:"assigned_to,omitempty"`
	Blocks      []string          `json:"blocks,omitempty"`
	BlockedBy   []string          `json:"blocked_by,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
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
// If sessionID is empty the call is a no-op — utility/sub-agent engines with no session
// must not wipe the parent session's tasks.
func (s *TaskStore) LoadForSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sessionID == "" {
		return nil
	}
	s.tasks = make(map[string]*Task)
	s.nextID = 0
	s.currentSession = sessionID
	if s.db == nil {
		return nil
	}
	rows, err := s.db.Query(`SELECT id, subject, description, status, assigned_to, COALESCE(blocks,'[]'), COALESCE(blocked_by,'[]'), COALESCE(metadata,'{}'), created_at, updated_at FROM team_tasks WHERE session_id = ? AND status != 'deleted' ORDER BY CAST(id AS INTEGER)`, sessionID)
	if err != nil {
		return fmt.Errorf("LoadForSession query: %w", err)
	}
	defer rows.Close()
	maxID := 0
	for rows.Next() {
		var t Task
		var assignedTo sql.NullString
		var blocksJSON, blockedByJSON, metadataJSON string
		if err := rows.Scan(&t.ID, &t.Subject, &t.Description, &t.Status, &assignedTo, &blocksJSON, &blockedByJSON, &metadataJSON, &t.CreatedAt, &t.UpdatedAt); err != nil {
			continue
		}
		if assignedTo.Valid {
			t.AssignedTo = assignedTo.String
		}
		_ = json.Unmarshal([]byte(blocksJSON), &t.Blocks)
		_ = json.Unmarshal([]byte(blockedByJSON), &t.BlockedBy)
		_ = json.Unmarshal([]byte(metadataJSON), &t.Metadata)
		s.tasks[t.ID] = &t
		var idNum int
		fmt.Sscanf(t.ID, "%d", &idNum)
		if idNum > maxID {
			maxID = idNum
		}
	}
	s.nextID = maxID
	return nil
}

func (s *TaskStore) saveToDB(t *Task) {
	if err := s.saveToDBWithSession(t, s.currentSession); err != nil {
		log.Printf("[tasks] saveToDB: %v", err)
	}
}

func marshalJSONSlice(v []string) string {
	if v == nil {
		return "[]"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func marshalJSONMap(v map[string]string) string {
	if v == nil {
		return "{}"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func (s *TaskStore) saveToDBWithSession(t *Task, sessionID string) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO team_tasks (id, session_id, subject, description, status, assigned_to, blocks, blocked_by, metadata, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, sessionID, t.Subject, t.Description, t.Status, t.AssignedTo,
		marshalJSONSlice(t.Blocks), marshalJSONSlice(t.BlockedBy), marshalJSONMap(t.Metadata),
		t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("saveToDBWithSession task %s: %w", t.ID, err)
	}
	return nil
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

func taskUpdatedPayload(t *Task, sessionID string) attach.TaskUpdatedPayload {
	return attach.TaskUpdatedPayload{
		ID:          t.ID,
		Subject:     t.Subject,
		Description: t.Description,
		AssignedTo:  t.AssignedTo,
		Status:      t.Status,
		SessionID:   sessionID,
		Blocks:      t.Blocks,
		BlockedBy:   t.BlockedBy,
	}
}

// CompleteByIDs marks all pending/in_progress tasks with matching IDs as the given status.
// sessionID identifies the session that owns the tasks and is used when persisting to DB.
func (s *TaskStore) CompleteByIDs(ids []string, status, sessionID string) []*Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	// BUG #5: guard against sessionID mismatch — wrong session creates a duplicate row
	// under the composite PK (id, session_id). Fall back to currentSession when mismatched.
	if sessionID == "" {
		sessionID = s.currentSession
	}

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[strings.TrimPrefix(id, "#")] = true
	}

	// Collect affected tasks and update in-memory state.
	var affected []*Task
	now := time.Now()
	for _, t := range s.tasks {
		if idSet[t.ID] && (t.Status == "pending" || t.Status == "in_progress") {
			t.Status = status
			t.UpdatedAt = now
			affected = append(affected, t)
		}
	}

	// BUG #4: persist all updates in a single DB transaction; rollback on any error.
	if s.db != nil && len(affected) > 0 {
		tx, err := s.db.Begin()
		if err != nil {
			log.Printf("[tasks] CompleteByIDs begin tx: %v", err)
		} else {
			txErr := false
			for _, t := range affected {
				_, err := tx.Exec(
					`INSERT OR REPLACE INTO team_tasks (id, session_id, subject, description, status, assigned_to, blocks, blocked_by, metadata, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					t.ID, sessionID, t.Subject, t.Description, t.Status, t.AssignedTo,
					marshalJSONSlice(t.Blocks), marshalJSONSlice(t.BlockedBy), marshalJSONMap(t.Metadata),
					t.CreatedAt, t.UpdatedAt,
				)
				if err != nil {
					log.Printf("[tasks] CompleteByIDs exec task %s: %v", t.ID, err)
					txErr = true
					break
				}
			}
			if txErr {
				if rbErr := tx.Rollback(); rbErr != nil {
					log.Printf("[tasks] CompleteByIDs rollback: %v", rbErr)
				}
			} else if err := tx.Commit(); err != nil {
				log.Printf("[tasks] CompleteByIDs commit: %v", err)
			}
		}
	}

	// Publish EventTaskUpdated for each affected task.
	for _, t := range affected {
		if s.bus != nil {
			payload, _ := json.Marshal(taskUpdatedPayload(t, sessionID))
			s.bus.Publish(bus.Event{
				Type:      attach.EventTaskUpdated,
				SessionID: sessionID,
				Payload:   payload,
			})
		}
	}

	return affected
}

// CompleteByAssignee marks all pending/in_progress tasks assigned to the given agent
// as the specified status ("completed" or "failed"). Returns the affected tasks.
// sessionID identifies the session that owns the tasks and is used when persisting to DB.
func (s *TaskStore) CompleteByAssignee(agentName, status, sessionID string) []*Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sessionID == "" {
		sessionID = s.currentSession
	}

	// Collect affected tasks and update in-memory state.
	var affected []*Task
	now := time.Now()
	for _, t := range s.tasks {
		if t.AssignedTo == agentName && (t.Status == "pending" || t.Status == "in_progress") {
			t.Status = status
			t.UpdatedAt = now
			affected = append(affected, t)
		}
	}

	// Persist all updates in a single DB transaction; rollback on any error.
	if s.db != nil && len(affected) > 0 {
		tx, err := s.db.Begin()
		if err != nil {
			log.Printf("[tasks] CompleteByAssignee begin tx: %v", err)
		} else {
			txErr := false
			for _, t := range affected {
				_, err := tx.Exec(
					`INSERT OR REPLACE INTO team_tasks (id, session_id, subject, description, status, assigned_to, blocks, blocked_by, metadata, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					t.ID, sessionID, t.Subject, t.Description, t.Status, t.AssignedTo,
					marshalJSONSlice(t.Blocks), marshalJSONSlice(t.BlockedBy), marshalJSONMap(t.Metadata),
					t.CreatedAt, t.UpdatedAt,
				)
				if err != nil {
					log.Printf("[tasks] CompleteByAssignee exec task %s: %v", t.ID, err)
					txErr = true
					break
				}
			}
			if txErr {
				if rbErr := tx.Rollback(); rbErr != nil {
					log.Printf("[tasks] CompleteByAssignee rollback: %v", rbErr)
				}
			} else if err := tx.Commit(); err != nil {
				log.Printf("[tasks] CompleteByAssignee commit: %v", err)
			}
		}
	}

	// Publish EventTaskUpdated for each affected task.
	for _, t := range affected {
		if s.bus != nil {
			payload, _ := json.Marshal(taskUpdatedPayload(t, sessionID))
			s.bus.Publish(bus.Event{
				Type:      attach.EventTaskUpdated,
				SessionID: sessionID,
				Payload:   payload,
			})
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

// syncBlocksBidirectional ensures that if A blocks B, B's BlockedBy contains A, and vice versa.
// Caller must hold s.mu (write lock).
func (s *TaskStore) syncBlocksBidirectional(task *Task, addBlocks, addBlockedBy []string, sessionID string) {
	for _, targetID := range addBlocks {
		targetID = strings.TrimPrefix(targetID, "#")
		// Add targetID to task.Blocks if not already present
		if !sliceContains(task.Blocks, targetID) {
			task.Blocks = append(task.Blocks, targetID)
		}
		// Add task.ID to target's BlockedBy
		if target, ok := s.tasks[targetID]; ok {
			if !sliceContains(target.BlockedBy, task.ID) {
				target.BlockedBy = append(target.BlockedBy, task.ID)
				target.UpdatedAt = time.Now()
				_ = s.saveToDBWithSession(target, sessionID)
			}
		}
	}
	for _, targetID := range addBlockedBy {
		targetID = strings.TrimPrefix(targetID, "#")
		// Add targetID to task.BlockedBy if not already present
		if !sliceContains(task.BlockedBy, targetID) {
			task.BlockedBy = append(task.BlockedBy, targetID)
		}
		// Add task.ID to target's Blocks
		if target, ok := s.tasks[targetID]; ok {
			if !sliceContains(target.Blocks, task.ID) {
				target.Blocks = append(target.Blocks, task.ID)
				target.UpdatedAt = time.Now()
				_ = s.saveToDBWithSession(target, sessionID)
			}
		}
	}
}

func sliceContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// --- TaskCreateTool ---

type TaskCreateTool struct {
	deferrable
	bus       *bus.Bus
	SessionID string
}

type taskCreateInput struct {
	Subject     string            `json:"subject"`
	Description string            `json:"description"`
	AssignedTo  string            `json:"assigned_to,omitempty"`
	AddBlocks   []string          `json:"addBlocks,omitempty"`
	AddBlockedBy []string         `json:"addBlockedBy,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
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
			"activeForm": {"type": "string", "description": "Present continuous form shown in spinner when in_progress (e.g., \"Running tests\")"},
			"addBlocks": {"type": "array", "items": {"type": "string"}, "description": "Task IDs that this task blocks"},
			"addBlockedBy": {"type": "array", "items": {"type": "string"}, "description": "Task IDs that block this task"},
			"metadata": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Arbitrary key-value metadata"}
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
		Metadata:    in.Metadata,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	store.tasks[id] = task
	store.syncBlocksBidirectional(task, in.AddBlocks, in.AddBlockedBy, t.SessionID)
	store.saveToDBWithSession(task, t.SessionID)
	store.mu.Unlock()

	// Publish event
	if t.bus != nil {
		payload, _ := json.Marshal(attach.TaskCreatedPayload{
			ID:          id,
			Subject:     in.Subject,
			Description: in.Description,
			AssignedTo:  in.AssignedTo,
			Status:      "pending",
			SessionID:   t.SessionID,
			Blocks:      task.Blocks,
			BlockedBy:   task.BlockedBy,
		})
		t.bus.Publish(bus.Event{
			Type:      attach.EventTaskCreated,
			SessionID: t.SessionID,
			Payload:   payload,
		})
	}

	return &Result{Content: fmt.Sprintf("Task #%s created: %s", id, in.Subject)}, nil
}

// --- TaskListTool ---

type TaskListTool struct {
	deferrable
	SessionID string
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
		blocked := ""
		if len(task.BlockedBy) > 0 {
			blocked = " [blocked]"
		}
		lines = append(lines, fmt.Sprintf("#%s %s [%s] %s%s%s", task.ID, icon, task.Status, task.Subject, assignee, blocked))
	}

	return &Result{Content: strings.Join(lines, "\n")}, nil
}

// --- TaskUpdateTool ---

type TaskUpdateTool struct {
	deferrable
	bus        *bus.Bus
	SessionID  string
	APIClient  *api.Client
	SmallModel string
}

type taskUpdateInput struct {
	TaskID       string            `json:"taskId"`
	Status       string            `json:"status,omitempty"`
	Subject      string            `json:"subject,omitempty"`
	Description  string            `json:"description,omitempty"`
	AddBlocks    []string          `json:"addBlocks,omitempty"`
	AddBlockedBy []string          `json:"addBlockedBy,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
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
			"description": {"type": "string"},
			"addBlocks": {"type": "array", "items": {"type": "string"}, "description": "Task IDs to add to blocks list"},
			"addBlockedBy": {"type": "array", "items": {"type": "string"}, "description": "Task IDs to add to blockedBy list"},
			"metadata": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Merge into existing metadata"}
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

	task, ok := store.tasks[in.TaskID]
	if !ok {
		store.mu.Unlock()
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
	if in.Metadata != nil {
		if task.Metadata == nil {
			task.Metadata = make(map[string]string)
		}
		for k, v := range in.Metadata {
			task.Metadata[k] = v
		}
	}
	store.syncBlocksBidirectional(task, in.AddBlocks, in.AddBlockedBy, t.SessionID)
	task.UpdatedAt = time.Now()
	store.saveToDBWithSession(task, t.SessionID)
	store.mu.Unlock()

	// Publish event
	if t.bus != nil {
		payload, _ := json.Marshal(taskUpdatedPayload(task, t.SessionID))
		t.bus.Publish(bus.Event{
			Type:      attach.EventTaskUpdated,
			SessionID: t.SessionID,
			Payload:   payload,
		})
	}

	resultContent := fmt.Sprintf("Task #%s updated", in.TaskID)

	// Verification nudge: if 3+ tasks completed and no qa/verification task exists,
	// use SmallModel to generate a suggestion.
	if in.Status == "completed" && t.APIClient != nil && t.SmallModel != "" {
		nudge := checkVerificationNudge(ctx, store, t.APIClient, t.SmallModel)
		if nudge != "" {
			resultContent += "\n\n" + nudge
		}
	}

	return &Result{Content: resultContent}, nil
}

// --- TaskGetTool ---

type TaskGetTool struct {
	deferrable
	SessionID string
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

// checkVerificationNudge returns a SmallModel-generated nudge if 3+ tasks are
// completed and no QA/verification task exists. Returns "" otherwise.
func checkVerificationNudge(ctx context.Context, store *TaskStore, apiClient *api.Client, smallModel string) string {
	store.mu.RLock()
	var completedSubjects []string
	hasQA := false
	for _, t := range store.tasks {
		if t.Status == "deleted" {
			continue
		}
		if t.Status == "completed" {
			completedSubjects = append(completedSubjects, t.Subject)
		}
		lower := strings.ToLower(t.Subject + " " + t.Description)
		if strings.Contains(lower, "qa") || strings.Contains(lower, "verif") ||
			strings.Contains(lower, "test") || strings.Contains(lower, "review") {
			hasQA = true
		}
	}
	store.mu.RUnlock()

	if len(completedSubjects) < 3 || hasQA {
		return ""
	}

	prompt := fmt.Sprintf(
		"You are a software quality assistant. The following tasks have been completed:\n%s\n\nThere is no QA or verification task yet. In one sentence, suggest spawning a QA agent to validate the work.",
		strings.Join(completedSubjects, "\n"),
	)

	contentJSON, _ := json.Marshal([]map[string]string{{"type": "text", "text": prompt}})
	resp, err := apiClient.SendMessage(ctx, &api.MessagesRequest{
		Model:     smallModel,
		MaxTokens: 100,
		Messages: []api.Message{
			{Role: "user", Content: json.RawMessage(contentJSON)},
		},
	})
	if err != nil {
		log.Printf("[tasks] verification nudge SmallModel call failed: %v", err)
		return ""
	}

	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != "" {
			return block.Text
		}
	}
	return ""
}
