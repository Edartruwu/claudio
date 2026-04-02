// Package tasks provides persistent task management backed by SQLite.
// This replaces the in-memory GlobalTaskStore with durable storage so
// tasks survive session restarts and can be resumed.
package tasks

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Task represents a tracked work item with persistent storage.
type Task struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	Subject     string    `json:"subject"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // pending, in_progress, completed, deleted
	Owner       string    `json:"owner,omitempty"`
	ActiveForm  string    `json:"active_form,omitempty"`
	BlockedBy   []string  `json:"blocked_by,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Store manages persistent task storage.
type Store struct {
	mu     sync.RWMutex
	db     *sql.DB
	nextID int
	tasks  map[string]*Task // in-memory cache
}

// NewStore creates a new persistent task store.
// If db is nil, falls back to in-memory only.
func NewStore(db *sql.DB) *Store {
	s := &Store{
		db:    db,
		tasks: make(map[string]*Task),
	}

	if db != nil {
		s.migrate()
		s.loadFromDB()
	}

	return s
}

func (s *Store) migrate() {
	if s.db == nil {
		return
	}
	s.db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		session_id TEXT,
		subject TEXT NOT NULL,
		description TEXT,
		status TEXT DEFAULT 'pending',
		owner TEXT,
		active_form TEXT,
		blocked_by TEXT,
		metadata TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
}

func (s *Store) loadFromDB() {
	if s.db == nil {
		return
	}
	rows, err := s.db.Query(`SELECT id, session_id, subject, description, status, owner, active_form, blocked_by, metadata, created_at, updated_at FROM tasks WHERE status != 'deleted' ORDER BY CAST(id AS INTEGER)`)
	if err != nil {
		return
	}
	defer rows.Close()

	maxID := 0
	for rows.Next() {
		var t Task
		var blockedByStr, metadataStr sql.NullString
		rows.Scan(&t.ID, &t.SessionID, &t.Subject, &t.Description, &t.Status, &t.Owner, &t.ActiveForm, &blockedByStr, &metadataStr, &t.CreatedAt, &t.UpdatedAt)

		if blockedByStr.Valid && blockedByStr.String != "" {
			json.Unmarshal([]byte(blockedByStr.String), &t.BlockedBy)
		}
		if metadataStr.Valid && metadataStr.String != "" {
			json.Unmarshal([]byte(metadataStr.String), &t.Metadata)
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

// Create adds a new task.
func (s *Store) Create(sessionID, subject, description, activeForm string) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	id := fmt.Sprintf("%d", s.nextID)

	task := &Task{
		ID:          id,
		SessionID:   sessionID,
		Subject:     subject,
		Description: description,
		Status:      "pending",
		ActiveForm:  activeForm,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	s.tasks[id] = task
	s.saveToDB(task)

	return task
}

// Get returns a task by ID.
func (s *Store) Get(id string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	return t, ok
}

// List returns all non-deleted tasks.
func (s *Store) List() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Task
	for _, t := range s.tasks {
		if t.Status != "deleted" {
			result = append(result, t)
		}
	}
	return result
}

// ListForSession returns tasks for a specific session.
func (s *Store) ListForSession(sessionID string) []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Task
	for _, t := range s.tasks {
		if t.SessionID == sessionID && t.Status != "deleted" {
			result = append(result, t)
		}
	}
	return result
}

// Update modifies a task's fields.
func (s *Store) Update(id string, updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}

	for key, val := range updates {
		switch key {
		case "status":
			if v, ok := val.(string); ok {
				task.Status = v
			}
		case "subject":
			if v, ok := val.(string); ok {
				task.Subject = v
			}
		case "description":
			if v, ok := val.(string); ok {
				task.Description = v
			}
		case "owner":
			if v, ok := val.(string); ok {
				task.Owner = v
			}
		case "active_form", "activeForm":
			if v, ok := val.(string); ok {
				task.ActiveForm = v
			}
		case "addBlockedBy":
			if v, ok := val.([]string); ok {
				task.BlockedBy = append(task.BlockedBy, v...)
			}
		}
	}

	task.UpdatedAt = time.Now()

	if task.Status == "deleted" {
		delete(s.tasks, id)
	}

	s.saveToDB(task)
	return nil
}

// FormatList returns a formatted string of all tasks.
func (s *Store) FormatList() string {
	tasks := s.List()
	if len(tasks) == 0 {
		return "No tasks"
	}

	var lines []string
	for _, t := range tasks {
		icon := "○"
		switch t.Status {
		case "in_progress":
			icon = "◐"
		case "completed":
			icon = "●"
		}
		line := fmt.Sprintf("#%s %s [%s] %s", t.ID, icon, t.Status, t.Subject)
		if t.Owner != "" {
			line += fmt.Sprintf(" (@%s)", t.Owner)
		}
		if len(t.BlockedBy) > 0 {
			line += fmt.Sprintf(" (blocked by: %s)", strings.Join(t.BlockedBy, ", "))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (s *Store) saveToDB(task *Task) {
	if s.db == nil {
		return
	}

	blockedBy, _ := json.Marshal(task.BlockedBy)
	metadata, _ := json.Marshal(task.Metadata)

	s.db.Exec(`INSERT OR REPLACE INTO tasks (id, session_id, subject, description, status, owner, active_form, blocked_by, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.SessionID, task.Subject, task.Description, task.Status, task.Owner, task.ActiveForm,
		string(blockedBy), string(metadata), task.CreatedAt, task.UpdatedAt)
}
