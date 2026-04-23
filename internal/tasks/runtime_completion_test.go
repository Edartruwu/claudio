package tasks

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntime_CompletionChannel_Completed(t *testing.T) {
	dir := t.TempDir()
	rt := NewRuntime(dir)

	// Register a task and transition to running.
	ts := &TaskState{
		ID:     "b1",
		Type:   TypeShell,
		Status: StatusRunning,
	}
	rt.Register(ts)

	// Transition to completed → should appear on CompletionCh.
	rt.SetStatus("b1", StatusCompleted, "")

	select {
	case r := <-rt.CompletionCh():
		if r.ID != "b1" {
			t.Errorf("expected ID b1, got %s", r.ID)
		}
		if r.Err != "" {
			t.Errorf("unexpected error: %s", r.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for completion")
	}
}

func TestRuntime_CompletionChannel_Failed(t *testing.T) {
	dir := t.TempDir()
	rt := NewRuntime(dir)

	ts := &TaskState{
		ID:     "b2",
		Type:   TypeShell,
		Status: StatusRunning,
	}
	rt.Register(ts)

	rt.SetStatus("b2", StatusFailed, "exit 1")

	select {
	case r := <-rt.CompletionCh():
		if r.ID != "b2" {
			t.Errorf("expected ID b2, got %s", r.ID)
		}
		if r.Err != "exit 1" {
			t.Errorf("expected error 'exit 1', got %q", r.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for completion")
	}
}

func TestRuntime_CompletionChannel_Killed(t *testing.T) {
	dir := t.TempDir()
	rt := NewRuntime(dir)

	ts := &TaskState{
		ID:     "b3",
		Type:   TypeShell,
		Status: StatusRunning,
	}
	rt.Register(ts)

	rt.SetStatus("b3", StatusKilled, "killed by user")

	select {
	case r := <-rt.CompletionCh():
		if r.ID != "b3" {
			t.Errorf("expected ID b3, got %s", r.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for completion")
	}
}

func TestRuntime_CompletionChannel_WithOutput(t *testing.T) {
	dir := t.TempDir()
	rt := NewRuntime(dir)

	// Create an output file.
	outPath := filepath.Join(dir, "b4.output")
	os.WriteFile(outPath, []byte("hello world output"), 0600)

	ts := &TaskState{
		ID:         "b4",
		Type:       TypeShell,
		Status:     StatusRunning,
		OutputFile: outPath,
	}
	rt.Register(ts)

	rt.SetStatus("b4", StatusCompleted, "")

	select {
	case r := <-rt.CompletionCh():
		if r.Output != "hello world output" {
			t.Errorf("expected output 'hello world output', got %q", r.Output)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for completion")
	}
}

func TestRuntime_CompletionChannel_WithExitCode(t *testing.T) {
	dir := t.TempDir()
	rt := NewRuntime(dir)

	exitCode := 42
	ts := &TaskState{
		ID:       "b5",
		Type:     TypeShell,
		Status:   StatusRunning,
		ExitCode: &exitCode,
	}
	rt.Register(ts)

	rt.SetStatus("b5", StatusFailed, "non-zero exit")

	select {
	case r := <-rt.CompletionCh():
		if r.ExitCode != 42 {
			t.Errorf("expected exit code 42, got %d", r.ExitCode)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for completion")
	}
}

func TestRuntime_CompletionChannel_NonTerminalNoSend(t *testing.T) {
	dir := t.TempDir()
	rt := NewRuntime(dir)

	ts := &TaskState{
		ID:     "b6",
		Type:   TypeShell,
		Status: StatusPending,
	}
	rt.Register(ts)

	// Transition to running (non-terminal) → should NOT send.
	rt.SetStatus("b6", StatusRunning, "")

	select {
	case r := <-rt.CompletionCh():
		t.Fatalf("should not receive for non-terminal, got %+v", r)
	case <-time.After(50 * time.Millisecond):
		// Expected: nothing received.
	}
}

func TestRuntime_CompletionChannel_NonBlocking(t *testing.T) {
	dir := t.TempDir()
	rt := NewRuntime(dir)

	// Fill the channel to capacity (64).
	for i := 0; i < 64; i++ {
		rt.completionCh <- TaskResult{ID: "fill"}
	}

	// Next SetStatus should not block (non-blocking send drops).
	ts := &TaskState{
		ID:     "b7",
		Type:   TypeShell,
		Status: StatusRunning,
	}
	rt.Register(ts)

	done := make(chan struct{})
	go func() {
		rt.SetStatus("b7", StatusCompleted, "")
		close(done)
	}()

	select {
	case <-done:
		// Good — didn't block.
	case <-time.After(time.Second):
		t.Fatal("SetStatus blocked on full channel")
	}
}
