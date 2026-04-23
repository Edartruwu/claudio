// Package tasks provides background task execution infrastructure.
// This is the runtime for long-running background operations (shell commands,
// sub-agents, dream/memory consolidation). Separate from the TodoV2 task
// tracking in store.go which is AI-visible planning.
package tasks

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TaskType identifies what kind of background task this is.
type TaskType string

const (
	TypeShell TaskType = "local_bash"
	TypeAgent TaskType = "local_agent"
	TypeDream TaskType = "dream"
)

// TaskStatus represents the lifecycle state.
type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
	StatusKilled    TaskStatus = "killed"
)

// IsTerminal returns true if the task will not transition further.
func (s TaskStatus) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusKilled
}

// TaskState holds the current state of a background task.
type TaskState struct {
	ID          string     `json:"id"`
	Type        TaskType   `json:"type"`
	Status      TaskStatus `json:"status"`
	Description string     `json:"description"`
	SessionID   string     `json:"session_id,omitempty"` // owning session for access control
	StartTime   time.Time  `json:"start_time"`
	EndTime     *time.Time `json:"end_time,omitempty"`
	Error       string     `json:"error,omitempty"`

	// Output tracking
	OutputFile   string `json:"output_file"`
	OutputOffset int64  `json:"output_offset"` // bytes read so far

	// Shell-specific
	Command  string `json:"command,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`

	// Agent-specific
	AgentType  string `json:"agent_type,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
	ToolCalls  int    `json:"tool_calls,omitempty"`
	TokensUsed int    `json:"tokens_used,omitempty"`

	// Internal
	cancel context.CancelFunc `json:"-"`
}

// TaskOutput provides disk-backed output streaming for background tasks.
type TaskOutput struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	written  int64
	maxBytes int64 // 5GB cap
}

// NewTaskOutput creates a new output file for a task.
func NewTaskOutput(outputDir, taskID string) (*TaskOutput, error) {
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return nil, err
	}

	path := filepath.Join(outputDir, taskID+".output")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}

	return &TaskOutput{
		file:     f,
		path:     path,
		maxBytes: 5 * 1024 * 1024 * 1024, // 5GB
	}, nil
}

// Write appends data to the output file.
func (o *TaskOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.written+int64(len(p)) > o.maxBytes {
		return 0, fmt.Errorf("task output exceeded 5GB limit")
	}

	n, err := o.file.Write(p)
	o.written += int64(n)
	return n, err
}

// Close closes the output file.
func (o *TaskOutput) Close() error {
	if o.file != nil {
		return o.file.Close()
	}
	return nil
}

// Path returns the output file path.
func (o *TaskOutput) Path() string { return o.path }

// Size returns bytes written so far.
func (o *TaskOutput) Size() int64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.written
}

// ReadDelta reads new output since the given offset.
// Returns the content and the new offset.
func ReadDelta(path string, offset int64, maxBytes int64) (string, int64, error) {
	if maxBytes <= 0 {
		maxBytes = 32 * 1024 // 32KB default
	}

	f, err := os.Open(path)
	if err != nil {
		return "", offset, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", offset, err
	}

	if info.Size() <= offset {
		return "", offset, nil // no new data
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", offset, err
	}

	readSize := info.Size() - offset
	if readSize > maxBytes {
		readSize = maxBytes
	}

	buf := make([]byte, readSize)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", offset, err
	}

	return string(buf[:n]), offset + int64(n), nil
}

// TaskResult is a lightweight snapshot sent on the completion channel
// when a background task reaches a terminal state.
type TaskResult struct {
	ID       string
	Output   string
	ExitCode int
	Err      string
}

// Runtime manages all background tasks.
type Runtime struct {
	mu           sync.RWMutex
	tasks        map[string]*TaskState
	outputDir    string
	nextID       int
	idPrefix     map[TaskType]string
	completionCh chan TaskResult
}

// NewRuntime creates a new task runtime.
func NewRuntime(outputDir string) *Runtime {
	os.MkdirAll(outputDir, 0700)
	return &Runtime{
		tasks:        make(map[string]*TaskState),
		outputDir:    outputDir,
		completionCh: make(chan TaskResult, 64),
		idPrefix: map[TaskType]string{
			TypeShell: "b",
			TypeAgent: "a",
			TypeDream: "d",
		},
	}
}

// GenerateID creates a unique task ID with a type-specific prefix.
func (r *Runtime) GenerateID(taskType TaskType) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	prefix := r.idPrefix[taskType]
	if prefix == "" {
		prefix = "x"
	}
	return fmt.Sprintf("%s%d", prefix, r.nextID)
}

// Register adds a task to the runtime.
func (r *Runtime) Register(state *TaskState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[state.ID] = state
}

// Get returns a task by ID.
func (r *Runtime) Get(id string) (*TaskState, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[id]
	return t, ok
}

// GetForSession returns a task by ID, validating session ownership.
// If sessionID is non-empty and doesn't match the task's SessionID, returns not-found.
func (r *Runtime) GetForSession(id, sessionID string) (*TaskState, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[id]
	if !ok {
		return nil, false
	}
	if sessionID != "" && t.SessionID != "" && t.SessionID != sessionID {
		return nil, false
	}
	return t, true
}

// List returns all tasks, optionally filtered by status.
func (r *Runtime) List(onlyRunning bool) []*TaskState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*TaskState
	for _, t := range r.tasks {
		if onlyRunning && t.Status.IsTerminal() {
			continue
		}
		result = append(result, t)
	}
	return result
}

// SetStatus updates a task's status.
func (r *Runtime) SetStatus(id string, status TaskStatus, errMsg string) {
	r.mu.Lock()

	t, ok := r.tasks[id]
	if !ok {
		r.mu.Unlock()
		return
	}

	t.Status = status
	if errMsg != "" {
		t.Error = errMsg
	}
	if !status.IsTerminal() {
		r.mu.Unlock()
		return
	}

	now := time.Now()
	t.EndTime = &now

	// Snapshot fields needed for the completion notification.
	result := TaskResult{
		ID:  t.ID,
		Err: t.Error,
	}
	if t.ExitCode != nil {
		result.ExitCode = *t.ExitCode
	}
	outputFile := t.OutputFile

	// Release lock before I/O.
	r.mu.Unlock()

	if outputFile != "" {
		content, _, _ := ReadDelta(outputFile, 0, 4096)
		if len(content) > 2000 {
			content = content[len(content)-2000:]
		}
		result.Output = content
	}

	// Non-blocking send — drop if nobody is reading.
	select {
	case r.completionCh <- result:
	default:
	}
}

// Kill stops a running task.
func (r *Runtime) Kill(id string) error {
	return r.KillForSession(id, "")
}

// KillForSession stops a running task, validating session ownership.
// If sessionID is non-empty and doesn't match the task's SessionID,
// returns a "not found" error to avoid leaking task existence.
func (r *Runtime) KillForSession(id, sessionID string) error {
	r.mu.RLock()
	t, ok := r.tasks[id]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	if sessionID != "" && t.SessionID != "" && t.SessionID != sessionID {
		return fmt.Errorf("task %s not found", id)
	}
	if t.Status.IsTerminal() {
		return fmt.Errorf("task %s already %s", id, t.Status)
	}

	if t.cancel != nil {
		t.cancel()
	}

	r.SetStatus(id, StatusKilled, "killed by user")
	return nil
}

// Evict removes terminal tasks older than the given duration.
func (r *Runtime) Evict(maxAge time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	evicted := 0

	for id, t := range r.tasks {
		if t.Status.IsTerminal() && t.EndTime != nil && t.EndTime.Before(cutoff) {
			// Clean up output file
			if t.OutputFile != "" {
				os.Remove(t.OutputFile)
			}
			delete(r.tasks, id)
			evicted++
		}
	}
	return evicted
}

// FormatStatus returns a human-readable status of all tasks.
func (r *Runtime) FormatStatus() string {
	tasks := r.List(false)
	if len(tasks) == 0 {
		return "No background tasks"
	}

	var sb strings.Builder
	sb.WriteString("Background Tasks:\n")

	for _, t := range tasks {
		icon := "○"
		switch t.Status {
		case StatusRunning:
			icon = "◐"
		case StatusCompleted:
			icon = "●"
		case StatusFailed:
			icon = "✗"
		case StatusKilled:
			icon = "⊘"
		}

		duration := time.Since(t.StartTime).Round(time.Second)
		if t.EndTime != nil {
			duration = t.EndTime.Sub(t.StartTime).Round(time.Second)
		}

		sb.WriteString(fmt.Sprintf("  %s [%s] %s %s (%s)\n",
			t.ID, icon, t.Status, t.Description, duration))

		if t.Error != "" {
			sb.WriteString(fmt.Sprintf("    Error: %s\n", t.Error))
		}
	}

	return sb.String()
}

// PollResults returns tasks that have completed since their last check.
// Updates OutputOffset for each returned task.
func (r *Runtime) PollResults() []*TaskState {
	r.mu.Lock()
	defer r.mu.Unlock()

	var completed []*TaskState
	for _, t := range r.tasks {
		if t.Status.IsTerminal() && t.OutputOffset == 0 {
			// Mark as polled by setting offset to -1
			t.OutputOffset = -1
			completed = append(completed, t)
		}
	}
	return completed
}

// CompletionCh returns a read-only channel that receives a TaskResult
// each time a background task reaches a terminal state (completed/failed/killed).
// The channel is buffered (cap=64); if nobody reads, sends are dropped silently.
func (r *Runtime) CompletionCh() <-chan TaskResult {
	return r.completionCh
}

// RunningCount returns the number of currently running tasks.
func (r *Runtime) RunningCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, t := range r.tasks {
		if t.Status == StatusRunning {
			count++
		}
	}
	return count
}
