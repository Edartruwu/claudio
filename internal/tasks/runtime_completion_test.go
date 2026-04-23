package tasks

import (
	"os"
	"path/filepath"
	"sync"
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

// TestRuntime_SetStatus_MissingOutputFile verifies that SetStatus(Completed)
// with a non-existent OutputFile does not panic and sends a TaskResult with
// empty Output on CompletionCh.
func TestRuntime_SetStatus_MissingOutputFile(t *testing.T) {
	dir := t.TempDir()
	rt := NewRuntime(dir)

	ts := &TaskState{
		ID:         "miss1",
		Type:       TypeShell,
		Status:     StatusRunning,
		OutputFile: "/nonexistent/path/to/output.txt",
	}
	rt.Register(ts)

	rt.SetStatus("miss1", StatusCompleted, "")

	select {
	case r := <-rt.CompletionCh():
		if r.ID != "miss1" {
			t.Errorf("expected ID miss1, got %s", r.ID)
		}
		if r.Output != "" {
			t.Errorf("expected empty Output for missing file, got %q", r.Output)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for completion")
	}
}

// TestRuntime_SetStatus_UnknownTaskID verifies that SetStatus on an unknown ID
// does not panic and does not send on CompletionCh.
func TestRuntime_SetStatus_UnknownTaskID(t *testing.T) {
	dir := t.TempDir()
	rt := NewRuntime(dir)

	// No tasks registered — this should return silently.
	rt.SetStatus("unknown-id", StatusCompleted, "")

	select {
	case r := <-rt.CompletionCh():
		t.Fatalf("should not receive for unknown task, got %+v", r)
	case <-time.After(50 * time.Millisecond):
		// Expected: nothing received.
	}
}

// TestRuntime_ConcurrentSetStatus verifies that 10 goroutines each calling
// SetStatus(Completed) on separate tasks all produce results on CompletionCh
// with no data races.
func TestRuntime_ConcurrentSetStatus(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rt := NewRuntime(dir)

	const n = 10
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = rt.GenerateID(TypeShell)
		rt.Register(&TaskState{
			ID:     ids[i],
			Type:   TypeShell,
			Status: StatusRunning,
		})
	}

	var wg sync.WaitGroup
	wg.Add(n)
	for _, id := range ids {
		id := id
		go func() {
			defer wg.Done()
			rt.SetStatus(id, StatusCompleted, "")
		}()
	}
	wg.Wait()

	// Drain channel — expect all n completions (ch cap=64 > n=10).
	received := 0
	deadline := time.After(2 * time.Second)
	for received < n {
		select {
		case <-rt.CompletionCh():
			received++
		case <-deadline:
			t.Fatalf("timeout: only received %d/%d completions", received, n)
		}
	}
}

// TestRuntime_CompletionChannel_StressNoBlock registers and completes 100 tasks
// rapidly. With ch cap=64, up to 36 sends are silently dropped. Verifies no
// goroutine deadlock or panic occurs.
func TestRuntime_CompletionChannel_StressNoBlock(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rt := NewRuntime(dir)

	const n = 100
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = rt.GenerateID(TypeShell)
		rt.Register(&TaskState{
			ID:     ids[i],
			Type:   TypeShell,
			Status: StatusRunning,
		})
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		wg.Add(n)
		for _, id := range ids {
			id := id
			go func() {
				defer wg.Done()
				rt.SetStatus(id, StatusCompleted, "")
			}()
		}
		wg.Wait()
	}()

	select {
	case <-done:
		// All SetStatus calls returned without blocking.
	case <-time.After(5 * time.Second):
		t.Fatal("stress test: goroutines blocked or deadlocked")
	}
}

// TestRuntime_PollResults_Empty verifies that PollResults returns an empty
// slice when no tasks are in a terminal state.
func TestRuntime_PollResults_Empty(t *testing.T) {
	dir := t.TempDir()
	rt := NewRuntime(dir)

	rt.Register(&TaskState{
		ID:     "p1",
		Type:   TypeShell,
		Status: StatusRunning,
	})

	results := rt.PollResults()
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// TestRuntime_PollResults_MultipleTasks verifies that PollResults returns all
// completed tasks once, and a second call returns none.
func TestRuntime_PollResults_MultipleTasks(t *testing.T) {
	dir := t.TempDir()
	rt := NewRuntime(dir)

	for i, id := range []string{"p2", "p3", "p4"} {
		_ = i
		rt.Register(&TaskState{
			ID:     id,
			Type:   TypeShell,
			Status: StatusRunning,
		})
		rt.SetStatus(id, StatusCompleted, "")
	}
	// Drain completionCh to avoid interference.
	for len(rt.CompletionCh()) > 0 {
		<-rt.CompletionCh()
	}

	first := rt.PollResults()
	if len(first) != 3 {
		t.Errorf("expected 3 results on first poll, got %d", len(first))
	}

	second := rt.PollResults()
	if len(second) != 0 {
		t.Errorf("expected 0 results on second poll, got %d", len(second))
	}
}

// TestRuntime_PollResults_AlreadyPolled verifies that a task returned by
// PollResults is not returned by a subsequent call.
func TestRuntime_PollResults_AlreadyPolled(t *testing.T) {
	dir := t.TempDir()
	rt := NewRuntime(dir)

	rt.Register(&TaskState{
		ID:     "p5",
		Type:   TypeShell,
		Status: StatusRunning,
	})
	rt.SetStatus("p5", StatusCompleted, "")
	// Drain completionCh.
	select {
	case <-rt.CompletionCh():
	case <-time.After(time.Second):
		t.Fatal("timeout draining channel")
	}

	first := rt.PollResults()
	if len(first) != 1 {
		t.Fatalf("expected 1 result, got %d", len(first))
	}

	second := rt.PollResults()
	if len(second) != 0 {
		t.Errorf("already-polled task should not appear again, got %d", len(second))
	}
}

// TestRuntime_ReadDelta_EmptyFile verifies that ReadDelta on an empty file
// returns ("", 0, nil).
func TestRuntime_ReadDelta_EmptyFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "rdelta-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.Close()

	content, newOffset, err := ReadDelta(f.Name(), 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "" {
		t.Errorf("expected empty content, got %q", content)
	}
	if newOffset != 0 {
		t.Errorf("expected offset 0, got %d", newOffset)
	}
}

// TestRuntime_ReadDelta_OffsetBeyondEnd verifies that ReadDelta with an offset
// greater than the file size returns ("", same offset, nil).
func TestRuntime_ReadDelta_OffsetBeyondEnd(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "rdelta-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.WriteString("hello")
	f.Close()

	const beyondOffset = int64(9999)
	content, newOffset, err := ReadDelta(f.Name(), beyondOffset, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "" {
		t.Errorf("expected empty content, got %q", content)
	}
	if newOffset != beyondOffset {
		t.Errorf("expected offset %d unchanged, got %d", beyondOffset, newOffset)
	}
}
