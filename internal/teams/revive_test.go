package teams

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
)

func textContent(s string) json.RawMessage {
	b, _ := json.Marshal([]map[string]string{{"type": "text", "text": s}})
	return b
}

// waitForIdle blocks until the state reports IsIdle (or times out).
func waitForIdle(t *testing.T, state *TeammateState, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state.mu.Lock()
		idle := state.IsIdle
		state.mu.Unlock()
		if idle {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func TestTeammateRunner_Revive_NoOpIfWorking(t *testing.T) {
	// Use a run function that blocks until we release it, so the agent stays
	// in StatusWorking while Revive is called.
	release := make(chan struct{})
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		<-release
		return "done", nil
	})
	runner.SetRunAgentResume(func(ctx context.Context, system, memoryDir string, history []api.Message, newMessage string) (string, error) {
		t.Error("resume should not be called while agent is still working")
		return "", nil
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "worker",
		Prompt:    "initial",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Agent is working; Revive must be a no-op (no error, no resume call).
	if err := runner.Revive("worker", "new instruction"); err != nil {
		t.Fatalf("Revive returned error: %v", err)
	}

	close(release)
	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)
}

func TestTeammateRunner_Revive_ErrorsIfAgentMissing(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "done", nil
	})
	runner.SetRunAgentResume(func(ctx context.Context, system, memoryDir string, history []api.Message, newMessage string) (string, error) {
		return "", nil
	})

	err := runner.Revive("nonexistent", "hello")
	if err == nil {
		t.Fatal("expected error for missing agent, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %q", err.Error())
	}
}

func TestTeammateRunner_Revive_ErrorsIfResumeNotWired(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "done", nil
	})
	// Deliberately NOT calling SetRunAgentResume.

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "worker",
		Prompt:    "task",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !runner.WaitForOne(state.Identity.AgentID, 5*time.Second) {
		t.Fatal("initial run did not finish")
	}

	err = runner.Revive("worker", "try again")
	if err == nil {
		t.Fatal("expected error because resume callback is not wired")
	}
	if !strings.Contains(err.Error(), "resume") {
		t.Errorf("expected 'resume' in error, got %q", err.Error())
	}
}

func TestTeammateRunner_Revive_NoOpIfShutdown(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "done", nil
	})
	resumeCalled := false
	runner.SetRunAgentResume(func(ctx context.Context, system, memoryDir string, history []api.Message, newMessage string) (string, error) {
		resumeCalled = true
		return "", nil
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "worker",
		Prompt:    "task",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !runner.WaitForOne(state.Identity.AgentID, 5*time.Second) {
		t.Fatal("initial run did not finish")
	}

	// Force shutdown status manually (simulating a user kill)
	state.mu.Lock()
	state.Status = StatusShutdown
	state.mu.Unlock()

	if err := runner.Revive("worker", "wake up"); err != nil {
		t.Fatalf("Revive returned error: %v", err)
	}
	// Give any rogue goroutine a moment
	time.Sleep(50 * time.Millisecond)
	if resumeCalled {
		t.Error("resume should not be called when agent is in StatusShutdown")
	}
}

func TestTeammateRunner_Revive_ResumesCompletedAgent(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "first-done", nil
	})

	var (
		resumeMu       sync.Mutex
		resumeHistory  []api.Message
		resumeMessage  string
		resumeMemDir   string
		resumeSystem   string
		resumeCalled   bool
	)
	runner.SetRunAgentResume(func(ctx context.Context, system, memoryDir string, history []api.Message, newMessage string) (string, error) {
		resumeMu.Lock()
		defer resumeMu.Unlock()
		resumeCalled = true
		resumeHistory = history
		resumeMessage = newMessage
		resumeMemDir = memoryDir
		resumeSystem = system
		return "second-done", nil
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "worker",
		Prompt:    "initial task",
		MemoryDir: "/tmp/mem",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !runner.WaitForOne(state.Identity.AgentID, 5*time.Second) {
		t.Fatal("initial run did not finish")
	}

	// Seed some fake engine messages on the state so we can verify they are
	// passed through to the resume callback.
	state.mu.Lock()
	state.EngineMessages = []api.Message{
		{Role: "user", Content: textContent("initial task")},
		{Role: "assistant", Content: textContent("first-done")},
	}
	state.SystemPrompt = "you are a worker"
	state.mu.Unlock()

	// Sanity: state must be idle/complete before revive
	if state.Status != StatusComplete {
		t.Fatalf("expected StatusComplete before revive, got %s", state.Status)
	}

	if err := runner.Revive("worker", "follow-up instruction"); err != nil {
		t.Fatalf("Revive: %v", err)
	}

	// Wait for the revived goroutine to finish
	if !waitForIdle(t, state, 5*time.Second) {
		t.Fatal("revived agent did not become idle")
	}

	resumeMu.Lock()
	defer resumeMu.Unlock()

	if !resumeCalled {
		t.Fatal("resume callback was not invoked")
	}
	if resumeMessage != "follow-up instruction" {
		t.Errorf("resume message = %q, want %q", resumeMessage, "follow-up instruction")
	}
	if resumeMemDir != "/tmp/mem" {
		t.Errorf("resume memoryDir = %q, want /tmp/mem", resumeMemDir)
	}
	if resumeSystem != "you are a worker" {
		t.Errorf("resume system = %q, want 'you are a worker'", resumeSystem)
	}
	if len(resumeHistory) != 2 {
		t.Fatalf("resume history len = %d, want 2", len(resumeHistory))
	}
	if resumeHistory[0].Role != "user" || resumeHistory[1].Role != "assistant" {
		t.Errorf("unexpected history roles: %q / %q", resumeHistory[0].Role, resumeHistory[1].Role)
	}

	// After revival, state should reflect the new result and be complete again.
	if state.Status != StatusComplete {
		t.Errorf("expected StatusComplete after revive, got %s", state.Status)
	}
	if state.Result != "second-done" {
		t.Errorf("expected Result 'second-done', got %q", state.Result)
	}
}

func TestTeammateRunner_Revive_CapturesErrorOnFailure(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})
	runner.SetRunAgentResume(func(ctx context.Context, system, memoryDir string, history []api.Message, newMessage string) (string, error) {
		return "", fmt.Errorf("resume boom")
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "worker",
		Prompt:    "initial",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !runner.WaitForOne(state.Identity.AgentID, 5*time.Second) {
		t.Fatal("initial run did not finish")
	}

	if err := runner.Revive("worker", "try again"); err != nil {
		t.Fatalf("Revive: %v", err)
	}
	if !waitForIdle(t, state, 5*time.Second) {
		t.Fatal("revived agent did not become idle")
	}

	if state.Status != StatusFailed {
		t.Errorf("expected StatusFailed, got %s", state.Status)
	}
	if !strings.Contains(state.Error, "resume boom") {
		t.Errorf("expected error to contain 'resume boom', got %q", state.Error)
	}
}

func TestTeammateRunner_Revive_EmitsCompleteEvent(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "first", nil
	})
	runner.SetRunAgentResume(func(ctx context.Context, system, memoryDir string, history []api.Message, newMessage string) (string, error) {
		return "second", nil
	})

	handler := &mockEventHandler{}
	runner.SetEventHandler(handler)

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "worker",
		Prompt:    "task",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !runner.WaitForOne(state.Identity.AgentID, 5*time.Second) {
		t.Fatal("initial run did not finish")
	}

	preCount := len(handler.getEvents())

	if err := runner.Revive("worker", "more"); err != nil {
		t.Fatalf("Revive: %v", err)
	}
	if !waitForIdle(t, state, 5*time.Second) {
		t.Fatal("revived agent did not become idle")
	}
	// Events are emitted from the goroutine — give it a tick to flush
	time.Sleep(20 * time.Millisecond)

	events := handler.getEvents()
	if len(events) <= preCount {
		t.Fatalf("expected new events after revive; pre=%d post=%d", preCount, len(events))
	}
	last := events[len(events)-1]
	if last.Type != "complete" {
		t.Errorf("expected last event type 'complete', got %q", last.Type)
	}
	if last.AgentName != "worker" {
		t.Errorf("expected event agent name 'worker', got %q", last.AgentName)
	}
}

func TestTeammateRunner_Revive_MessagesSinkCapturesHistory(t *testing.T) {
	// Verify the initial run installs a messages sink that updates
	// state.EngineMessages, so a subsequent Revive has real history to replay.
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		if sink := GetMessagesSink(ctx); sink != nil {
			sink([]api.Message{
				{Role: "user", Content: textContent(prompt)},
				{Role: "assistant", Content: textContent("answer")},
			})
		}
		return "answer", nil
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "worker",
		Prompt:    "hello",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !runner.WaitForOne(state.Identity.AgentID, 5*time.Second) {
		t.Fatal("initial run did not finish")
	}

	state.mu.Lock()
	msgs := state.EngineMessages
	state.mu.Unlock()

	if len(msgs) != 2 {
		t.Fatalf("expected 2 engine messages captured via sink, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Errorf("unexpected roles: %q / %q", msgs[0].Role, msgs[1].Role)
	}
}

func TestTeammateRunner_Revive_ResumeHistoryContextHelper(t *testing.T) {
	// The resume callback should be able to read pre-existing history via the
	// WithResumeHistory / GetResumeHistory context helpers when the runner's
	// contextDecorator or app wiring injects them. Here we exercise the helpers
	// directly to ensure round-trip works.
	history := []api.Message{
		{Role: "user", Content: textContent("prior")},
	}
	ctx := WithResumeHistory(context.Background(), history)
	got := GetResumeHistory(ctx)
	if len(got) != 1 || got[0].Role != "user" {
		t.Errorf("WithResumeHistory/GetResumeHistory round-trip failed: %+v", got)
	}

	// Empty context returns nil
	if GetResumeHistory(context.Background()) != nil {
		t.Error("expected nil history from empty context")
	}
}
