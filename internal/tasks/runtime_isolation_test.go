package tasks

import (
	"testing"
	"time"
)

func TestRuntime_MultipleIndependentRuntimes(t *testing.T) {
	dir := t.TempDir()

	rt1 := NewRuntime(dir)
	rt2 := NewRuntime(dir)

	// Register and complete a task on rt1 only.
	ts := &TaskState{
		ID:     "b1",
		Type:   TypeShell,
		Status: StatusRunning,
	}
	rt1.Register(ts)
	rt1.SetStatus("b1", StatusCompleted, "")

	// rt1.CompletionCh should receive the result.
	select {
	case r := <-rt1.CompletionCh():
		if r.ID != "b1" {
			t.Errorf("rt1: expected ID b1, got %s", r.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("rt1: timeout waiting for completion")
	}

	// rt2.CompletionCh must be empty — task was not registered there.
	select {
	case r := <-rt2.CompletionCh():
		t.Errorf("rt2: unexpected result %s — runtimes not independent", r.ID)
	default:
		// correct: nothing on rt2
	}
}

func TestRuntime_SubAgentIsolation(t *testing.T) {
	dir := t.TempDir()

	parent := NewRuntime(dir)
	child := NewRuntime(dir)

	// Simulate sub-agent spawning a task on child runtime.
	ts := &TaskState{
		ID:     "a1",
		Type:   TypeAgent,
		Status: StatusRunning,
	}
	child.Register(ts)
	child.SetStatus("a1", StatusCompleted, "")

	// Completion must appear on child.
	select {
	case r := <-child.CompletionCh():
		if r.ID != "a1" {
			t.Errorf("child: expected ID a1, got %s", r.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("child: timeout waiting for completion")
	}

	// Parent must have nothing — task was never registered on parent.
	select {
	case r := <-parent.CompletionCh():
		t.Errorf("parent: got unexpected result %s — sub-agent isolation broken", r.ID)
	default:
		// correct: parent is clean
	}
}
