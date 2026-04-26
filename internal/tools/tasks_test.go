package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/bus"
	_ "modernc.org/sqlite"
)

// freshStore returns a clean TaskStore for testing.
func freshStore() *TaskStore {
	return &TaskStore{
		tasks: make(map[string]*Task),
	}
}

// --- TaskStore unit tests ---

func TestTaskStore_CompleteByAssignee(t *testing.T) {
	store := freshStore()
	store.tasks["1"] = &Task{ID: "1", Subject: "task-a", Status: "pending", AssignedTo: "agent-x"}
	store.tasks["2"] = &Task{ID: "2", Subject: "task-b", Status: "in_progress", AssignedTo: "agent-x"}
	store.tasks["3"] = &Task{ID: "3", Subject: "task-c", Status: "pending", AssignedTo: "agent-y"}

	affected := store.CompleteByAssignee("agent-x", "completed", "")
	if len(affected) != 2 {
		t.Fatalf("expected 2 affected tasks, got %d", len(affected))
	}

	for _, task := range affected {
		if task.Status != "completed" {
			t.Errorf("task %s: expected completed, got %s", task.ID, task.Status)
		}
	}

	// agent-y's task should be unchanged
	if store.tasks["3"].Status != "pending" {
		t.Errorf("agent-y task should still be pending, got %s", store.tasks["3"].Status)
	}
}

func TestTaskStore_CompleteByAssignee_SkipsCompleted(t *testing.T) {
	store := freshStore()
	store.tasks["1"] = &Task{ID: "1", Status: "completed", AssignedTo: "agent-x"}
	store.tasks["2"] = &Task{ID: "2", Status: "deleted", AssignedTo: "agent-x"}
	store.tasks["3"] = &Task{ID: "3", Status: "pending", AssignedTo: "agent-x"}

	affected := store.CompleteByAssignee("agent-x", "failed", "")
	if len(affected) != 1 {
		t.Fatalf("expected 1 affected task, got %d", len(affected))
	}
	if affected[0].ID != "3" {
		t.Errorf("expected task 3, got %s", affected[0].ID)
	}
	// Already-completed task should not change
	if store.tasks["1"].Status != "completed" {
		t.Errorf("task 1 should remain completed")
	}
}

func TestTaskStore_ByAssignee(t *testing.T) {
	store := freshStore()
	store.tasks["1"] = &Task{ID: "1", Status: "pending", AssignedTo: "bob"}
	store.tasks["2"] = &Task{ID: "2", Status: "completed", AssignedTo: "bob"}
	store.tasks["3"] = &Task{ID: "3", Status: "deleted", AssignedTo: "bob"}
	store.tasks["4"] = &Task{ID: "4", Status: "pending", AssignedTo: "alice"}

	tasks := store.ByAssignee("bob")
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks for bob (excluding deleted), got %d", len(tasks))
	}
}

func TestTaskStore_List_ExcludesDeleted(t *testing.T) {
	store := freshStore()
	store.tasks["1"] = &Task{ID: "1", Status: "pending"}
	store.tasks["2"] = &Task{ID: "2", Status: "deleted"}

	list := store.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 task (excluding deleted), got %d", len(list))
	}
}

// --- TaskUpdateTool tests ---

func TestTaskUpdateTool_AcceptsSnakeCaseTaskID(t *testing.T) {
	// Save and restore global store
	orig := GlobalTaskStore
	defer func() { GlobalTaskStore = orig }()

	GlobalTaskStore = freshStore()
	GlobalTaskStore.tasks["5"] = &Task{ID: "5", Subject: "test", Status: "pending"}

	tool := &TaskUpdateTool{}
	input := json.RawMessage(`{"task_id": "5", "status": "completed"}`)

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if GlobalTaskStore.tasks["5"].Status != "completed" {
		t.Errorf("expected completed, got %s", GlobalTaskStore.tasks["5"].Status)
	}
}

func TestTaskUpdateTool_StripsHashPrefix(t *testing.T) {
	orig := GlobalTaskStore
	defer func() { GlobalTaskStore = orig }()

	GlobalTaskStore = freshStore()
	GlobalTaskStore.tasks["7"] = &Task{ID: "7", Subject: "test", Status: "pending"}

	tool := &TaskUpdateTool{}
	input := json.RawMessage(`{"taskId": "#7", "status": "in_progress"}`)

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if GlobalTaskStore.tasks["7"].Status != "in_progress" {
		t.Errorf("expected in_progress, got %s", GlobalTaskStore.tasks["7"].Status)
	}
}

func TestTaskUpdateTool_CamelCaseTakesPriority(t *testing.T) {
	orig := GlobalTaskStore
	defer func() { GlobalTaskStore = orig }()

	GlobalTaskStore = freshStore()
	GlobalTaskStore.tasks["1"] = &Task{ID: "1", Subject: "a", Status: "pending"}
	GlobalTaskStore.tasks["2"] = &Task{ID: "2", Subject: "b", Status: "pending"}

	tool := &TaskUpdateTool{}
	// Both taskId and task_id present — camelCase should win
	input := json.RawMessage(`{"taskId": "1", "task_id": "2", "status": "completed"}`)

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if GlobalTaskStore.tasks["1"].Status != "completed" {
		t.Errorf("expected task 1 completed (camelCase priority)")
	}
	if GlobalTaskStore.tasks["2"].Status != "pending" {
		t.Errorf("task 2 should be unchanged")
	}
}

func TestTaskUpdateTool_NotFound(t *testing.T) {
	orig := GlobalTaskStore
	defer func() { GlobalTaskStore = orig }()

	GlobalTaskStore = freshStore()

	tool := &TaskUpdateTool{}
	input := json.RawMessage(`{"taskId": "999"}`)

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error result for non-existent task")
	}
}

// --- TaskCreateTool tests ---

func TestTaskCreateTool_CreatesTask(t *testing.T) {
	orig := GlobalTaskStore
	defer func() { GlobalTaskStore = orig }()

	GlobalTaskStore = freshStore()

	tool := &TaskCreateTool{}
	input := json.RawMessage(`{"subject": "Write tests", "description": "Cover all cases", "assigned_to": "agent-1"}`)

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Write tests") {
		t.Errorf("expected subject in result, got %s", result.Content)
	}

	tasks := GlobalTaskStore.List()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	task := tasks[0]
	if task.Subject != "Write tests" {
		t.Errorf("subject = %q", task.Subject)
	}
	if task.AssignedTo != "agent-1" {
		t.Errorf("assigned_to = %q, expected agent-1", task.AssignedTo)
	}
	if task.Status != "pending" {
		t.Errorf("status = %q, expected pending", task.Status)
	}
}

// --- TaskListTool tests ---

func TestTaskListTool_EmptyStore(t *testing.T) {
	orig := GlobalTaskStore
	defer func() { GlobalTaskStore = orig }()

	GlobalTaskStore = freshStore()

	tool := &TaskListTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "No tasks" {
		t.Errorf("expected 'No tasks', got %q", result.Content)
	}
}

func TestTaskListTool_ShowsAssignee(t *testing.T) {
	orig := GlobalTaskStore
	defer func() { GlobalTaskStore = orig }()

	GlobalTaskStore = freshStore()
	GlobalTaskStore.tasks["1"] = &Task{ID: "1", Subject: "do stuff", Status: "pending", AssignedTo: "bot"}

	tool := &TaskListTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "bot") {
		t.Errorf("expected assignee in output, got %q", result.Content)
	}
}

// --- TaskGetTool tests ---

func TestTaskGetTool_ReturnsJSON(t *testing.T) {
	orig := GlobalTaskStore
	defer func() { GlobalTaskStore = orig }()

	GlobalTaskStore = freshStore()
	GlobalTaskStore.tasks["10"] = &Task{ID: "10", Subject: "get test", Status: "completed", AssignedTo: "x"}

	tool := &TaskGetTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"taskId": "10"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	var task Task
	if err := json.Unmarshal([]byte(result.Content), &task); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if task.Subject != "get test" {
		t.Errorf("subject = %q", task.Subject)
	}
}

// --- End-to-end: create → assign → complete by assignee ---

func TestTaskFlow_CreateAndCompleteByAssignee(t *testing.T) {
	orig := GlobalTaskStore
	defer func() { GlobalTaskStore = orig }()

	GlobalTaskStore = freshStore()

	// Create two tasks assigned to same agent
	create := &TaskCreateTool{}
	create.Execute(context.Background(), json.RawMessage(`{"subject": "Task A", "description": "a", "assigned_to": "worker-1"}`))
	create.Execute(context.Background(), json.RawMessage(`{"subject": "Task B", "description": "b", "assigned_to": "worker-1"}`))
	create.Execute(context.Background(), json.RawMessage(`{"subject": "Task C", "description": "c", "assigned_to": "worker-2"}`))

	// Simulate worker-1 completing
	affected := GlobalTaskStore.CompleteByAssignee("worker-1", "completed", "")
	if len(affected) != 2 {
		t.Fatalf("expected 2 tasks completed, got %d", len(affected))
	}

	// worker-2's task should still be pending
	w2Tasks := GlobalTaskStore.ByAssignee("worker-2")
	if len(w2Tasks) != 1 || w2Tasks[0].Status != "pending" {
		t.Error("worker-2's task should remain pending")
	}

	// All worker-1 tasks should be completed
	w1Tasks := GlobalTaskStore.ByAssignee("worker-1")
	for _, task := range w1Tasks {
		if task.Status != "completed" {
			t.Errorf("worker-1 task %s should be completed, got %s", task.ID, task.Status)
		}
	}
}

// --- Bus publish tests ---

func TestTaskCreateTool_PublishesEvent(t *testing.T) {
	orig := GlobalTaskStore
	defer func() { GlobalTaskStore = orig }()

	GlobalTaskStore = freshStore()

	// Create event bus and subscribe
	eventBus := bus.New()
	events := make(chan bus.Event, 1)
	unsub := eventBus.Subscribe(attach.EventTaskCreated, func(e bus.Event) {
		events <- e
	})
	defer unsub()

	// Create tool with bus
	tool := &TaskCreateTool{bus: eventBus}
	input := json.RawMessage(`{"subject": "Test Task", "description": "Test", "assigned_to": "agent-1"}`)

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	// Verify event published
	select {
	case evt := <-events:
		if evt.Type != attach.EventTaskCreated {
			t.Errorf("expected %s, got %s", attach.EventTaskCreated, evt.Type)
		}

		var payload attach.TaskCreatedPayload
		if err := json.Unmarshal(evt.Payload, &payload); err != nil {
			t.Fatalf("failed to unmarshal payload: %v", err)
		}
		if payload.Subject != "Test Task" {
			t.Errorf("expected subject 'Test Task', got %q", payload.Subject)
		}
		if payload.Status != "pending" {
			t.Errorf("expected status 'pending', got %q", payload.Status)
		}
		if payload.AssignedTo != "agent-1" {
			t.Errorf("expected assigned_to 'agent-1', got %q", payload.AssignedTo)
		}
	case <-make(chan struct{}): // timeout
		t.Error("event not published")
	}
}

func TestTaskStore_CompleteByIDs_PublishesEvent(t *testing.T) {
	store := freshStore()
	store.tasks["1"] = &Task{ID: "1", Subject: "t", Status: "pending"}
	store.tasks["2"] = &Task{ID: "2", Subject: "t", Status: "in_progress"}

	b := bus.New()
	store.bus = b

	events := make(chan bus.Event, 2)
	unsub := b.Subscribe(attach.EventTaskUpdated, func(e bus.Event) { events <- e })
	defer unsub()

	affected := store.CompleteByIDs([]string{"1", "2"}, "completed", "")
	if len(affected) != 2 {
		t.Fatalf("expected 2 affected tasks, got %d", len(affected))
	}

	for i := 0; i < 2; i++ {
		select {
		case evt := <-events:
			if evt.Type != attach.EventTaskUpdated {
				t.Errorf("event %d: Type = %q, want %q", i, evt.Type, attach.EventTaskUpdated)
			}
			var payload attach.TaskUpdatedPayload
			if err := json.Unmarshal(evt.Payload, &payload); err != nil {
				t.Fatalf("event %d: unmarshal payload: %v", i, err)
			}
			if payload.Status != "completed" {
				t.Errorf("event %d: Status = %q, want %q", i, payload.Status, "completed")
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("timeout waiting for event %d", i)
		}
	}
}

func TestTaskStore_CompleteByAssignee_PublishesEvent(t *testing.T) {
	store := freshStore()
	store.tasks["1"] = &Task{ID: "1", Subject: "t", Status: "pending", AssignedTo: "agent-x"}
	store.tasks["2"] = &Task{ID: "2", Subject: "t", Status: "in_progress", AssignedTo: "agent-x"}

	b := bus.New()
	store.bus = b

	events := make(chan bus.Event, 2)
	unsub := b.Subscribe(attach.EventTaskUpdated, func(e bus.Event) { events <- e })
	defer unsub()

	affected := store.CompleteByAssignee("agent-x", "failed", "")
	if len(affected) != 2 {
		t.Fatalf("expected 2 affected tasks, got %d", len(affected))
	}

	for i := 0; i < 2; i++ {
		select {
		case evt := <-events:
			if evt.Type != attach.EventTaskUpdated {
				t.Errorf("event %d: Type = %q, want %q", i, evt.Type, attach.EventTaskUpdated)
			}
			var payload attach.TaskUpdatedPayload
			if err := json.Unmarshal(evt.Payload, &payload); err != nil {
				t.Fatalf("event %d: unmarshal payload: %v", i, err)
			}
			if payload.Status != "failed" {
				t.Errorf("event %d: Status = %q, want %q", i, payload.Status, "failed")
			}
			if payload.AssignedTo != "agent-x" {
				t.Errorf("event %d: AssignedTo = %q, want %q", i, payload.AssignedTo, "agent-x")
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("timeout waiting for event %d", i)
		}
	}
}

func TestTaskCreateTool_UsesCorrectSessionID(t *testing.T) {
	orig := GlobalTaskStore
	defer func() { GlobalTaskStore = orig }()

	// Open in-memory SQLite and create the team_tasks table.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE team_tasks (
		id TEXT NOT NULL,
		session_id TEXT NOT NULL DEFAULT '',
		subject TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		assigned_to TEXT,
		blocks TEXT NOT NULL DEFAULT '[]',
		blocked_by TEXT NOT NULL DEFAULT '[]',
		metadata TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME,
		updated_at DATETIME,
		PRIMARY KEY (id, session_id)
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	GlobalTaskStore = freshStore()
	GlobalTaskStore.db = db

	const wantSession = "session-abc-123"
	tool := &TaskCreateTool{SessionID: wantSession}
	input := json.RawMessage(`{"subject": "Wire test", "description": "Check session_id"}`)

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	// Query the DB and verify session_id matches what was wired in.
	var gotSession string
	err = db.QueryRow(`SELECT session_id FROM team_tasks LIMIT 1`).Scan(&gotSession)
	if err != nil {
		t.Fatalf("query team_tasks: %v", err)
	}
	if gotSession != wantSession {
		t.Errorf("session_id in DB = %q, want %q", gotSession, wantSession)
	}
}

func TestTaskUpdateTool_PublishesEvent(t *testing.T) {
	orig := GlobalTaskStore
	defer func() { GlobalTaskStore = orig }()

	GlobalTaskStore = freshStore()
	GlobalTaskStore.tasks["5"] = &Task{ID: "5", Subject: "test", Status: "pending"}

	// Create event bus and subscribe
	eventBus := bus.New()
	events := make(chan bus.Event, 1)
	unsub := eventBus.Subscribe(attach.EventTaskUpdated, func(e bus.Event) {
		events <- e
	})
	defer unsub()

	// Update tool with bus
	tool := &TaskUpdateTool{bus: eventBus}
	input := json.RawMessage(`{"taskId": "5", "status": "completed"}`)

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	// Verify event published
	select {
	case evt := <-events:
		if evt.Type != attach.EventTaskUpdated {
			t.Errorf("expected %s, got %s", attach.EventTaskUpdated, evt.Type)
		}

		var payload attach.TaskUpdatedPayload
		if err := json.Unmarshal(evt.Payload, &payload); err != nil {
			t.Fatalf("failed to unmarshal payload: %v", err)
		}
		if payload.ID != "5" {
			t.Errorf("expected id '5', got %q", payload.ID)
		}
		if payload.Status != "completed" {
			t.Errorf("expected status 'completed', got %q", payload.Status)
		}
	case <-make(chan struct{}): // timeout
		t.Error("event not published")
	}
}

// openTestDB creates an in-memory SQLite DB with the team_tasks table for session ID tests.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE team_tasks (
		id TEXT NOT NULL,
		session_id TEXT NOT NULL DEFAULT '',
		subject TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		assigned_to TEXT,
		blocks TEXT NOT NULL DEFAULT '[]',
		blocked_by TEXT NOT NULL DEFAULT '[]',
		metadata TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME,
		updated_at DATETIME,
		PRIMARY KEY (id, session_id)
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

func TestCompleteByIDs_UsesCorrectSessionID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	store := freshStore()
	store.db = db
	store.tasks["1"] = &Task{ID: "1", Subject: "task-one", Status: "pending"}
	store.tasks["2"] = &Task{ID: "2", Subject: "task-two", Status: "in_progress"}

	const wantSession = "session-complete-ids-test"
	affected := store.CompleteByIDs([]string{"1", "2"}, "completed", wantSession)
	if len(affected) != 2 {
		t.Fatalf("expected 2 affected tasks, got %d", len(affected))
	}

	rows, err := db.Query(`SELECT session_id FROM team_tasks`)
	if err != nil {
		t.Fatalf("query team_tasks: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var gotSession string
		if err := rows.Scan(&gotSession); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if gotSession != wantSession {
			t.Errorf("session_id in DB = %q, want %q", gotSession, wantSession)
		}
		count++
	}
	if count != 2 {
		t.Errorf("expected 2 rows in DB, got %d", count)
	}
}

func TestCompleteByAssignee_UsesCorrectSessionID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	store := freshStore()
	store.db = db
	store.tasks["1"] = &Task{ID: "1", Subject: "task-a", Status: "pending", AssignedTo: "agent-q"}
	store.tasks["2"] = &Task{ID: "2", Subject: "task-b", Status: "in_progress", AssignedTo: "agent-q"}

	const wantSession = "session-complete-assignee-test"
	affected := store.CompleteByAssignee("agent-q", "completed", wantSession)
	if len(affected) != 2 {
		t.Fatalf("expected 2 affected tasks, got %d", len(affected))
	}

	rows, err := db.Query(`SELECT session_id FROM team_tasks`)
	if err != nil {
		t.Fatalf("query team_tasks: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var gotSession string
		if err := rows.Scan(&gotSession); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if gotSession != wantSession {
			t.Errorf("session_id in DB = %q, want %q", gotSession, wantSession)
		}
		count++
	}
	if count != 2 {
		t.Errorf("expected 2 rows in DB, got %d", count)
	}
}

func TestTaskUpdateTool_UsesCorrectSessionID(t *testing.T) {
	orig := GlobalTaskStore
	defer func() { GlobalTaskStore = orig }()

	db := openTestDB(t)
	defer db.Close()

	GlobalTaskStore = freshStore()
	GlobalTaskStore.db = db

	// Pre-populate the DB row so UPDATE OR REPLACE can find it.
	_, err := db.Exec(`INSERT INTO team_tasks (id, session_id, subject, description, status, assigned_to, created_at, updated_at)
		VALUES ('10', '', 'original', '', 'pending', NULL, datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert seed row: %v", err)
	}
	GlobalTaskStore.tasks["10"] = &Task{ID: "10", Subject: "original", Status: "pending"}

	const wantSession = "session-update-tool-test"
	tool := &TaskUpdateTool{SessionID: wantSession}
	input := json.RawMessage(`{"taskId": "10", "status": "completed"}`)

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	// The INSERT OR REPLACE writes a new row with the correct session_id.
	var gotSession string
	err = db.QueryRow(`SELECT session_id FROM team_tasks WHERE id = '10' AND session_id = ?`, wantSession).Scan(&gotSession)
	if err != nil {
		t.Fatalf("query team_tasks for session %q: %v", wantSession, err)
	}
	if gotSession != wantSession {
		t.Errorf("session_id in DB = %q, want %q", gotSession, wantSession)
	}
}

// --- Fix #1 & #2: saveToDBWithSession returns error; saveToDB logs it ---

func TestSaveToDBWithSession_ReturnsError_WhenDBClosed(t *testing.T) {
	db := openTestDB(t)
	// Close immediately — all subsequent DB operations must fail.
	db.Close()

	store := freshStore()
	store.db = db

	task := &Task{ID: "1", Subject: "test", Status: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	err := store.saveToDBWithSession(task, "session-x")
	if err == nil {
		t.Error("expected error from saveToDBWithSession with closed DB, got nil")
	}
}

func TestSaveToDB_DoesNotPanic_WhenDBClosed(t *testing.T) {
	// saveToDB should log the error, never panic.
	db := openTestDB(t)
	db.Close()

	store := freshStore()
	store.db = db

	task := &Task{ID: "1", Subject: "test", Status: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	// Must not panic.
	store.saveToDB(task)
}

// --- Fix #3: LoadForSession returns error ---

func TestLoadForSession_ReturnsError_WhenDBClosed(t *testing.T) {
	db := openTestDB(t)
	db.Close()

	store := freshStore()
	store.db = db

	err := store.LoadForSession("some-session")
	if err == nil {
		t.Error("expected error from LoadForSession with closed DB, got nil")
	}
}

func TestLoadForSession_NoopOnEmptySessionID(t *testing.T) {
	store := freshStore()
	store.tasks["1"] = &Task{ID: "1", Subject: "existing", Status: "pending"}
	store.currentSession = "real-session"

	// Empty sessionID → no-op, existing tasks must survive.
	err := store.LoadForSession("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := store.tasks["1"]; !ok {
		t.Error("task was wiped despite empty sessionID no-op")
	}
}

// --- Fix CompleteByAssignee: rolls back on partial DB failure ---

func TestCompleteByAssignee_RollsBackOnPartialFailure(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Trigger that forces any INSERT with id = '2' to fail.
	_, err := db.Exec(`
		CREATE TRIGGER fail_assignee_task_id_2
		BEFORE INSERT ON team_tasks
		WHEN NEW.id = '2'
		BEGIN
			SELECT RAISE(ABORT, 'forced failure for rollback test');
		END
	`)
	if err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	store := freshStore()
	store.db = db
	store.tasks["1"] = &Task{ID: "1", Subject: "task-one", Status: "pending", AssignedTo: "agent-r"}
	store.tasks["2"] = &Task{ID: "2", Subject: "task-two", Status: "in_progress", AssignedTo: "agent-r"}

	// Call; task "2" triggers DB error → rollback.
	store.CompleteByAssignee("agent-r", "completed", "sess-rollback")

	// After rollback, DB must have 0 rows.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM team_tasks`).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows after rollback, got %d — partial write leaked", count)
	}
}

// --- Fix CompleteByAssignee: falls back to currentSession when sessionID is empty ---

func TestCompleteByAssignee_EmptySessionIDFallback(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	store := freshStore()
	store.db = db
	store.currentSession = "fallback-session"
	store.tasks["1"] = &Task{ID: "1", Subject: "task", Status: "pending", AssignedTo: "agent-fb"}

	// Pass empty sessionID → must fall back to store.currentSession.
	affected := store.CompleteByAssignee("agent-fb", "completed", "")
	if len(affected) != 1 {
		t.Fatalf("expected 1 affected, got %d", len(affected))
	}

	var gotSession string
	err := db.QueryRow(`SELECT session_id FROM team_tasks WHERE id = '1'`).Scan(&gotSession)
	if err != nil {
		t.Fatalf("query team_tasks: %v", err)
	}
	if gotSession != "fallback-session" {
		t.Errorf("session_id in DB = %q, want %q", gotSession, "fallback-session")
	}
}

// --- Fix #4: CompleteByIDs rolls back on partial DB failure ---

func TestCompleteByIDs_RollsBackOnPartialFailure(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Trigger that forces any INSERT with id = '2' to fail.
	// This simulates a mid-transaction write error.
	_, err := db.Exec(`
		CREATE TRIGGER fail_task_id_2
		BEFORE INSERT ON team_tasks
		WHEN NEW.id = '2'
		BEGIN
			SELECT RAISE(ABORT, 'forced failure for rollback test');
		END
	`)
	if err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	store := freshStore()
	store.db = db
	store.tasks["1"] = &Task{ID: "1", Subject: "task-one", Status: "pending"}
	store.tasks["2"] = &Task{ID: "2", Subject: "task-two", Status: "in_progress"}

	// Call with both IDs; task "2" will cause DB error → rollback.
	store.CompleteByIDs([]string{"1", "2"}, "completed", "sess-rollback")

	// After rollback, DB must have 0 rows (no partial writes).
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM team_tasks`).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows after rollback, got %d — partial write leaked", count)
	}
}

// --- Fix #5: CompleteByIDs falls back to currentSession when sessionID is empty ---

func TestCompleteByIDs_EmptySessionIDFallback(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	store := freshStore()
	store.db = db
	store.currentSession = "fallback-session"
	store.tasks["1"] = &Task{ID: "1", Subject: "task", Status: "pending"}

	// Pass empty sessionID → must fall back to store.currentSession.
	affected := store.CompleteByIDs([]string{"1"}, "completed", "")
	if len(affected) != 1 {
		t.Fatalf("expected 1 affected, got %d", len(affected))
	}

	var gotSession string
	err := db.QueryRow(`SELECT session_id FROM team_tasks WHERE id = '1'`).Scan(&gotSession)
	if err != nil {
		t.Fatalf("query team_tasks: %v", err)
	}
	if gotSession != "fallback-session" {
		t.Errorf("session_id in DB = %q, want %q", gotSession, "fallback-session")
	}
}
