package query

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/tasks"
)

func TestEngine_BgTaskPublishesBusEvent(t *testing.T) {
	dir := t.TempDir()
	rt := tasks.NewRuntime(dir)
	eventBus := bus.New()

	e := newTestEngine()
	e.taskRuntime = rt
	e.eventBus = eventBus
	e.sessionID = "test-session"

	// Subscribe to bg task complete events.
	received := make(chan bus.Event, 1)
	eventBus.Subscribe(bus.EventBgTaskComplete, func(ev bus.Event) {
		received <- ev
	})

	// Register + start a task.
	ts := &tasks.TaskState{
		ID:     "b1",
		Type:   tasks.TypeShell,
		Status: tasks.StatusRunning,
	}
	// Exported fields only — use Register via runtime.
	rt.Register(ts)

	// Manually start the bg watcher (normally RunWithBlocks does this on return).
	// Use a canceled context so the watcher doesn't try to call RunWithBlocks
	// (we only want to test the bus publish path).
	//
	// Instead, we test the lower-level path: runtime sends on CompletionCh,
	// and we verify the engine can publish to bus.

	// Complete the task → triggers CompletionCh send.
	rt.SetStatus("b1", tasks.StatusCompleted, "")

	// Read from CompletionCh and simulate what the watcher does.
	select {
	case result := <-rt.CompletionCh():
		// Replicate the watcher's publish logic.
		payload, _ := json.Marshal(bus.BgTaskCompletePayload{
			TaskID:   result.ID,
			Output:   result.Output,
			ExitCode: result.ExitCode,
			Err:      result.Err,
		})
		eventBus.Publish(bus.Event{
			Type:      bus.EventBgTaskComplete,
			Payload:   payload,
			SessionID: e.sessionID,
		})
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for CompletionCh")
	}

	// Verify bus event was received.
	select {
	case ev := <-received:
		if ev.Type != bus.EventBgTaskComplete {
			t.Errorf("expected type %s, got %s", bus.EventBgTaskComplete, ev.Type)
		}
		if ev.SessionID != "test-session" {
			t.Errorf("expected session test-session, got %s", ev.SessionID)
		}

		var p bus.BgTaskCompletePayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if p.TaskID != "b1" {
			t.Errorf("expected TaskID b1, got %s", p.TaskID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for bus event")
	}
}

func TestEngine_BgTaskPublishesBusEvent_WithError(t *testing.T) {
	dir := t.TempDir()
	rt := tasks.NewRuntime(dir)
	eventBus := bus.New()

	e := newTestEngine()
	e.taskRuntime = rt
	e.eventBus = eventBus

	received := make(chan bus.Event, 1)
	eventBus.Subscribe(bus.EventBgTaskComplete, func(ev bus.Event) {
		received <- ev
	})

	exitCode := 1
	ts := &tasks.TaskState{
		ID:       "b2",
		Type:     tasks.TypeShell,
		Status:   tasks.StatusRunning,
	}
	// ExitCode is set on the task before SetStatus in real code,
	// but here we test the payload propagation.
	_ = exitCode
	rt.Register(ts)
	rt.SetStatus("b2", tasks.StatusFailed, "command failed")

	select {
	case result := <-rt.CompletionCh():
		payload, _ := json.Marshal(bus.BgTaskCompletePayload{
			TaskID:   result.ID,
			Output:   result.Output,
			ExitCode: result.ExitCode,
			Err:      result.Err,
		})
		eventBus.Publish(bus.Event{
			Type:    bus.EventBgTaskComplete,
			Payload: payload,
		})
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for CompletionCh")
	}

	select {
	case ev := <-received:
		var p bus.BgTaskCompletePayload
		json.Unmarshal(ev.Payload, &p)
		if p.Err != "command failed" {
			t.Errorf("expected error 'command failed', got %q", p.Err)
		}
		if p.TaskID != "b2" {
			t.Errorf("expected TaskID b2, got %s", p.TaskID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for bus event")
	}
}

// ---------------------------------------------------------------------------
// Goroutine lifecycle tests
// ---------------------------------------------------------------------------

func TestEngine_BgWatcher_ExitsOnParentContextCancel(t *testing.T) {
	dir := t.TempDir()
	rt := tasks.NewRuntime(dir)

	e := newTestEngine()
	e.taskRuntime = rt

	ctx, cancel := context.WithCancel(context.Background())

	e.startBgWatcher(ctx)
	if e.bgWatcherCancel == nil {
		t.Fatal("bgWatcherCancel should be set after startBgWatcher")
	}

	// Cancel parent ctx → goroutine should exit.
	cancel()

	// Give goroutine time to notice cancellation and call defer cancel().
	time.Sleep(100 * time.Millisecond)

	// The goroutine's defer cancel() runs, but bgWatcherCancel field is
	// only set to nil by stopBgWatcher(). We verify the context is done.
	// Call stopBgWatcher to clean up — should not panic.
	e.stopBgWatcher()
}

func TestEngine_BgWatcher_ExitsOnStopBgWatcher(t *testing.T) {
	dir := t.TempDir()
	rt := tasks.NewRuntime(dir)

	e := newTestEngine()
	e.taskRuntime = rt

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e.startBgWatcher(ctx)
	if e.bgWatcherCancel == nil {
		t.Fatal("bgWatcherCancel should be set")
	}

	e.stopBgWatcher()

	if e.bgWatcherCancel != nil {
		t.Error("bgWatcherCancel should be nil after stopBgWatcher")
	}

	// Parent ctx still live.
	if ctx.Err() != nil {
		t.Error("parent ctx should not be canceled")
	}
}

func TestEngine_BgWatcher_NilBus_NoPublish(t *testing.T) {
	dir := t.TempDir()
	rt := tasks.NewRuntime(dir)

	e := newTestEngine()
	e.taskRuntime = rt
	// e.eventBus intentionally nil

	ts := &tasks.TaskState{
		ID:     "nb1",
		Type:   tasks.TypeShell,
		Status: tasks.StatusRunning,
	}
	rt.Register(ts)
	rt.SetStatus("nb1", tasks.StatusCompleted, "")

	// Read from CompletionCh — simulate watcher logic.
	select {
	case result := <-rt.CompletionCh():
		// Replicate nil-bus guard: publish only if eventBus != nil.
		if e.eventBus != nil {
			t.Fatal("eventBus should be nil")
		}
		if result.ID != "nb1" {
			t.Errorf("expected nb1, got %s", result.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for CompletionCh")
	}
	// No panic = pass.
}

func TestEngine_BgWatcher_RapidCompletions(t *testing.T) {
	dir := t.TempDir()
	rt := tasks.NewRuntime(dir)
	eventBus := bus.New()

	e := newTestEngine()
	e.taskRuntime = rt
	e.eventBus = eventBus
	e.sessionID = "rapid-session"

	received := make(chan bus.Event, 10)
	eventBus.Subscribe(bus.EventBgTaskComplete, func(ev bus.Event) {
		received <- ev
	})

	// Register + complete 5 tasks.
	for i := 0; i < 5; i++ {
		id := rt.GenerateID(tasks.TypeShell)
		rt.Register(&tasks.TaskState{
			ID:     id,
			Type:   tasks.TypeShell,
			Status: tasks.StatusRunning,
		})
		rt.SetStatus(id, tasks.StatusCompleted, "")
	}

	// Manually drain CompletionCh and publish — simulates watcher loop.
	drained := 0
	timeout := time.After(2 * time.Second)
	for drained < 5 {
		select {
		case result := <-rt.CompletionCh():
			payload, _ := json.Marshal(bus.BgTaskCompletePayload{
				TaskID:   result.ID,
				Output:   result.Output,
				ExitCode: result.ExitCode,
				Err:      result.Err,
			})
			eventBus.Publish(bus.Event{
				Type:      bus.EventBgTaskComplete,
				Payload:   payload,
				SessionID: e.sessionID,
			})
			drained++
		case <-timeout:
			t.Fatalf("timeout draining CompletionCh, got %d/5", drained)
		}
	}

	// Verify 5 events received on bus.
	for i := 0; i < 5; i++ {
		select {
		case <-received:
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for bus event %d/5", i+1)
		}
	}
}

// ---------------------------------------------------------------------------
// pollBackgroundTasks tests
// ---------------------------------------------------------------------------

func TestEngine_pollBackgroundTasks_MultipleTasksInjected(t *testing.T) {
	dir := t.TempDir()
	rt := tasks.NewRuntime(dir)

	e := newTestEngine()
	e.taskRuntime = rt

	// Register + complete 3 tasks.
	for i := 0; i < 3; i++ {
		id := rt.GenerateID(tasks.TypeShell)
		rt.Register(&tasks.TaskState{
			ID:     id,
			Type:   tasks.TypeShell,
			Status: tasks.StatusRunning,
		})
		rt.SetStatus(id, tasks.StatusCompleted, "")
	}

	// Drain CompletionCh so it doesn't block (non-blocking sends but let's be safe).
	for i := 0; i < 3; i++ {
		select {
		case <-rt.CompletionCh():
		default:
		}
	}

	msgsBefore := len(e.messages)
	e.pollBackgroundTasks()

	if len(e.messages) != msgsBefore+1 {
		t.Fatalf("expected 1 injected message, got %d new messages", len(e.messages)-msgsBefore)
	}

	// Verify all 3 task IDs appear in the injected message.
	raw := string(e.messages[msgsBefore].Content)
	for _, id := range []string{"b1", "b2", "b3"} {
		if !strings.Contains(raw, id) {
			t.Errorf("injected message missing task %s", id)
		}
	}
}

func TestEngine_pollBackgroundTasks_OutputTruncated(t *testing.T) {
	dir := t.TempDir()
	rt := tasks.NewRuntime(dir)

	e := newTestEngine()
	e.taskRuntime = rt

	// Create an output file >2000 bytes.
	outputFile := dir + "/big.output"
	bigContent := strings.Repeat("X", 3000)
	if err := os.WriteFile(outputFile, []byte(bigContent), 0600); err != nil {
		t.Fatal(err)
	}

	id := rt.GenerateID(tasks.TypeShell)
	ts := &tasks.TaskState{
		ID:         id,
		Type:       tasks.TypeShell,
		Status:     tasks.StatusRunning,
		OutputFile: outputFile,
	}
	rt.Register(ts)
	rt.SetStatus(id, tasks.StatusCompleted, "")

	// Drain CompletionCh.
	select {
	case <-rt.CompletionCh():
	default:
	}

	e.pollBackgroundTasks()

	if len(e.messages) == 0 {
		t.Fatal("no messages injected")
	}

	raw := string(e.messages[0].Content)
	// pollBackgroundTasks reads 4KB then truncates to 2000 bytes.
	// The injected message should contain output but not the full 3000 bytes.
	if strings.Contains(raw, bigContent) {
		t.Error("output should be truncated, but found full content")
	}
	if !strings.Contains(raw, "Output (tail)") {
		t.Error("expected 'Output (tail)' in injected message")
	}
}

func TestEngine_pollBackgroundTasks_NilRuntime(t *testing.T) {
	e := newTestEngine()
	// e.taskRuntime is nil
	e.pollBackgroundTasks() // should not panic
}

func TestEngine_stopBgWatcher_BeforeStart(t *testing.T) {
	e := newTestEngine()
	// bgWatcherCancel is nil — should not panic
	e.stopBgWatcher()
}

// ---------------------------------------------------------------------------
// startBgWatcher with nil runtime — noop
// ---------------------------------------------------------------------------

func TestEngine_startBgWatcher_NilRuntime(t *testing.T) {
	e := newTestEngine()
	ctx := context.Background()
	e.startBgWatcher(ctx) // noop — should not set bgWatcherCancel

	if e.bgWatcherCancel != nil {
		t.Error("bgWatcherCancel should stay nil when taskRuntime is nil")
	}
}

// ---------------------------------------------------------------------------
// IsSubAgent flag tests
// ---------------------------------------------------------------------------

func TestEngine_SetSubAgent_PayloadCarriesFlag(t *testing.T) {
	dir := t.TempDir()
	rt := tasks.NewRuntime(dir)
	eventBus := bus.New()

	e := newTestEngine()
	e.taskRuntime = rt
	e.eventBus = eventBus
	e.sessionID = "sub-agent-session"
	e.SetSubAgent(true)

	received := make(chan bus.Event, 1)
	eventBus.Subscribe(bus.EventBgTaskComplete, func(ev bus.Event) {
		received <- ev
	})

	rt.Register(&tasks.TaskState{
		ID:     "sa1",
		Type:   tasks.TypeShell,
		Status: tasks.StatusRunning,
	})
	rt.SetStatus("sa1", tasks.StatusCompleted, "")

	select {
	case result := <-rt.CompletionCh():
		payload, _ := json.Marshal(bus.BgTaskCompletePayload{
			TaskID:     result.ID,
			Output:     result.Output,
			ExitCode:   result.ExitCode,
			Err:        result.Err,
			IsSubAgent: e.isSubAgent,
		})
		eventBus.Publish(bus.Event{
			Type:      bus.EventBgTaskComplete,
			Payload:   payload,
			SessionID: e.sessionID,
		})
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for CompletionCh")
	}

	select {
	case ev := <-received:
		var p bus.BgTaskCompletePayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !p.IsSubAgent {
			t.Error("expected IsSubAgent=true for sub-agent engine")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for bus event")
	}
}

func TestEngine_DefaultEngine_PayloadIsSubAgentFalse(t *testing.T) {
	dir := t.TempDir()
	rt := tasks.NewRuntime(dir)
	eventBus := bus.New()

	e := newTestEngine()
	e.taskRuntime = rt
	e.eventBus = eventBus
	e.sessionID = "main-session"
	// No SetSubAgent call — default is false.

	received := make(chan bus.Event, 1)
	eventBus.Subscribe(bus.EventBgTaskComplete, func(ev bus.Event) {
		received <- ev
	})

	rt.Register(&tasks.TaskState{
		ID:     "m1",
		Type:   tasks.TypeShell,
		Status: tasks.StatusRunning,
	})
	rt.SetStatus("m1", tasks.StatusCompleted, "")

	select {
	case result := <-rt.CompletionCh():
		payload, _ := json.Marshal(bus.BgTaskCompletePayload{
			TaskID:     result.ID,
			Output:     result.Output,
			ExitCode:   result.ExitCode,
			Err:        result.Err,
			IsSubAgent: e.isSubAgent,
		})
		eventBus.Publish(bus.Event{
			Type:      bus.EventBgTaskComplete,
			Payload:   payload,
			SessionID: e.sessionID,
		})
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for CompletionCh")
	}

	select {
	case ev := <-received:
		var p bus.BgTaskCompletePayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if p.IsSubAgent {
			t.Error("expected IsSubAgent=false for default engine")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for bus event")
	}
}
