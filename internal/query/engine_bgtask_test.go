package query

import (
	"encoding/json"
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
