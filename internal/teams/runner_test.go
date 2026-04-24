package teams

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/bus"
)

// --- TeammateState unit tests ---

func TestTeammateState_AddConversation(t *testing.T) {
	s := &TeammateState{Conversation: make([]ConversationEntry, 0)}

	s.AddConversation(ConversationEntry{Type: "text", Content: "hello"})
	s.AddConversation(ConversationEntry{Type: "tool_start", ToolName: "Bash"})

	conv := s.GetConversation()
	if len(conv) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(conv))
	}
	if conv[0].Type != "text" || conv[0].Content != "hello" {
		t.Errorf("unexpected entry 0: %+v", conv[0])
	}
	if conv[1].ToolName != "Bash" {
		t.Errorf("unexpected entry 1: %+v", conv[1])
	}
}

func TestTeammateState_AddConversation_RingBuffer(t *testing.T) {
	s := &TeammateState{Conversation: make([]ConversationEntry, 0)}

	// Fill past max
	for i := 0; i < maxConversationEntries+10; i++ {
		s.AddConversation(ConversationEntry{
			Type:    "text",
			Content: fmt.Sprintf("entry-%d", i),
		})
	}

	conv := s.GetConversation()
	if len(conv) != maxConversationEntries {
		t.Fatalf("expected %d entries, got %d", maxConversationEntries, len(conv))
	}

	// First entry should be entry-10 (oldest 10 were evicted)
	if conv[0].Content != "entry-10" {
		t.Errorf("expected first entry 'entry-10', got %q", conv[0].Content)
	}
	// Last should be the most recent
	last := conv[len(conv)-1]
	if last.Content != fmt.Sprintf("entry-%d", maxConversationEntries+9) {
		t.Errorf("unexpected last entry: %q", last.Content)
	}
}

func TestTeammateState_AddConversation_Concurrent(t *testing.T) {
	s := &TeammateState{Conversation: make([]ConversationEntry, 0)}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.AddConversation(ConversationEntry{
				Type:    "text",
				Content: fmt.Sprintf("msg-%d", n),
			})
		}(i)
	}
	wg.Wait()

	conv := s.GetConversation()
	if len(conv) != 50 {
		t.Fatalf("expected 50 entries, got %d", len(conv))
	}
}

func TestTeammateState_AddActivity(t *testing.T) {
	s := &TeammateState{}

	for i := 0; i < 8; i++ {
		s.AddActivity(fmt.Sprintf("act-%d", i))
	}

	p := s.GetProgress()
	if len(p.Activities) != 5 {
		t.Fatalf("expected max 5 activities, got %d", len(p.Activities))
	}
	// Should be the 5 most recent
	if p.Activities[0] != "act-3" {
		t.Errorf("expected first activity 'act-3', got %q", p.Activities[0])
	}
	if p.Activities[4] != "act-7" {
		t.Errorf("expected last activity 'act-7', got %q", p.Activities[4])
	}
}

func TestTeammateState_IncrToolCalls(t *testing.T) {
	s := &TeammateState{}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.IncrToolCalls()
		}()
	}
	wg.Wait()

	p := s.GetProgress()
	if p.ToolCalls != 100 {
		t.Errorf("expected 100 tool calls, got %d", p.ToolCalls)
	}
}

func TestTeammateState_GetConversation_Snapshot(t *testing.T) {
	s := &TeammateState{Conversation: make([]ConversationEntry, 0)}
	s.AddConversation(ConversationEntry{Type: "text", Content: "before"})

	snap := s.GetConversation()
	s.AddConversation(ConversationEntry{Type: "text", Content: "after"})

	// Snapshot should not see the new entry
	if len(snap) != 1 {
		t.Fatalf("snapshot should have 1 entry, got %d", len(snap))
	}
}

func TestTeammateState_GetProgress_Snapshot(t *testing.T) {
	s := &TeammateState{}
	s.AddActivity("first")
	p := s.GetProgress()

	s.AddActivity("second")

	// Snapshot should only have "first"
	if len(p.Activities) != 1 || p.Activities[0] != "first" {
		t.Errorf("progress snapshot should have only 'first', got %v", p.Activities)
	}
}

// --- TeammateRunner tests ---

type mockEventHandler struct {
	mu     sync.Mutex
	events []TeammateEvent
}

func (h *mockEventHandler) OnTeammateEvent(event TeammateEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, event)
}

func (h *mockEventHandler) getEvents() []TeammateEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]TeammateEvent, len(h.events))
	copy(out, h.events)
	return out
}

func setupRunner(t *testing.T, runFn RunAgentFunc) (*TeammateRunner, *Manager) {
	t.Helper()
	dir := t.TempDir()
	mgr := NewManager(dir, "")
	_, err := mgr.CreateTeam("test-team", "test", "sess-1", "")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	runner := NewTeammateRunner(mgr, runFn)
	return runner, mgr
}

func TestTeammateRunner_SpawnAndComplete(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "done: " + prompt, nil
	})

	handler := &mockEventHandler{}
	runner.SetEventHandler(handler)

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "worker",
		Prompt:    "do something",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if !runner.WaitForOne(state.Identity.AgentID, 5*time.Second) {
		t.Fatal("timeout waiting for teammate")
	}

	// Check state
	if state.Status != StatusComplete {
		t.Errorf("expected StatusComplete, got %s", state.Status)
	}
	// Result may have worktree preservation note appended; check prefix
	expectedPrefix := "done: do something"
	if !strings.HasPrefix(state.Result, expectedPrefix) {
		t.Errorf("unexpected result prefix: %q (expected to start with %q)", state.Result, expectedPrefix)
	}

	// Check events
	events := handler.getEvents()
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events (started + complete), got %d", len(events))
	}
	if events[0].Type != "started" {
		t.Errorf("expected first event 'started', got %q", events[0].Type)
	}
	last := events[len(events)-1]
	if last.Type != "complete" {
		t.Errorf("expected last event 'complete', got %q", last.Type)
	}
	if last.AgentName != "worker" {
		t.Errorf("expected agent name 'worker', got %q", last.AgentName)
	}

	// Check conversation has completion entry
	conv := state.GetConversation()
	if len(conv) == 0 {
		t.Fatal("expected conversation entries")
	}
	lastConv := conv[len(conv)-1]
	if lastConv.Type != "complete" {
		t.Errorf("expected last conversation entry 'complete', got %q", lastConv.Type)
	}
}

func TestTeammateRunner_SpawnAndFail(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "", fmt.Errorf("something broke")
	})

	handler := &mockEventHandler{}
	runner.SetEventHandler(handler)

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "failer",
		Prompt:    "fail please",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	if state.Status != StatusFailed {
		t.Errorf("expected StatusFailed, got %s", state.Status)
	}
	if state.Error != "something broke" {
		t.Errorf("unexpected error: %q", state.Error)
	}

	events := handler.getEvents()
	last := events[len(events)-1]
	if last.Type != "error" {
		t.Errorf("expected error event, got %q", last.Type)
	}

	conv := state.GetConversation()
	lastConv := conv[len(conv)-1]
	if lastConv.Type != "error" {
		t.Errorf("expected error conversation entry, got %q", lastConv.Type)
	}
}

func TestTeammateRunner_Kill(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "long-runner",
		Prompt:    "run forever",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Let it start
	time.Sleep(50 * time.Millisecond)

	err = runner.Kill(state.Identity.AgentID)
	if err != nil {
		t.Fatalf("Kill: %v", err)
	}

	if !runner.WaitForOne(state.Identity.AgentID, 5*time.Second) {
		t.Fatal("timeout waiting for killed teammate")
	}

	if state.Status != StatusShutdown {
		t.Errorf("expected StatusShutdown, got %s", state.Status)
	}
}

func TestTeammateRunner_GetStateByName(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "finder",
		Prompt:    "test",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer runner.Kill(state.Identity.AgentID)

	// Find by name
	found, ok := runner.GetStateByName("finder")
	if !ok {
		t.Fatal("expected to find agent by name")
	}
	if found.Identity.AgentID != state.Identity.AgentID {
		t.Errorf("found wrong agent: %s", found.Identity.AgentID)
	}

	// Not found
	_, ok = runner.GetStateByName("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent agent")
	}
}

func TestTeammateRunner_ActiveTeamName(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	if runner.ActiveTeamName() != "" {
		t.Error("expected empty team name with no agents")
	}

	state, _ := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "agent1",
		Prompt:    "test",
	})

	if runner.ActiveTeamName() != "test-team" {
		t.Errorf("expected 'test-team', got %q", runner.ActiveTeamName())
	}

	runner.Kill(state.Identity.AgentID)
	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)
}

func TestTeammateRunner_WorkingCount(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	s1, _ := runner.Spawn(SpawnConfig{TeamName: "test-team", AgentName: "a1", Prompt: "t"})
	s2, _ := runner.Spawn(SpawnConfig{TeamName: "test-team", AgentName: "a2", Prompt: "t"})
	defer runner.KillAll()

	// Both should be working
	time.Sleep(50 * time.Millisecond)
	if c := runner.WorkingCount(); c != 2 {
		t.Errorf("expected 2 working, got %d", c)
	}

	// Kill one
	runner.Kill(s1.Identity.AgentID)
	runner.WaitForOne(s1.Identity.AgentID, 5*time.Second)

	if c := runner.WorkingCount(); c != 1 {
		t.Errorf("expected 1 working after kill, got %d", c)
	}

	runner.Kill(s2.Identity.AgentID)
	runner.WaitForOne(s2.Identity.AgentID, 5*time.Second)

	if c := runner.WorkingCount(); c != 0 {
		t.Errorf("expected 0 working after kill all, got %d", c)
	}
}

func TestTeammateRunner_AllStates(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	runner.Spawn(SpawnConfig{TeamName: "test-team", AgentName: "a1", Prompt: "t"})
	runner.Spawn(SpawnConfig{TeamName: "test-team", AgentName: "a2", Prompt: "t"})
	runner.Spawn(SpawnConfig{TeamName: "test-team", AgentName: "a3", Prompt: "t"})

	states := runner.AllStates()
	if len(states) != 3 {
		t.Fatalf("expected 3 states, got %d", len(states))
	}

	runner.KillAll()
	runner.WaitForAll(5 * time.Second)
}

func TestTeammateRunner_ContextDecorator(t *testing.T) {
	type ctxKey struct{}
	decoratorCalled := false

	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		val := ctx.Value(ctxKey{})
		if val != "decorated" {
			t.Error("expected decorated context value")
		}
		return "ok", nil
	})

	runner.SetContextDecorator(func(ctx context.Context, state *TeammateState) context.Context {
		decoratorCalled = true
		return context.WithValue(ctx, ctxKey{}, "decorated")
	})

	state, _ := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "decorated-agent",
		Prompt:    "test",
	})

	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	if !decoratorCalled {
		t.Error("context decorator was not called")
	}
}

func TestTeammateRunner_ParentContext(t *testing.T) {
	type ctxKey struct{}

	parentCtx := context.WithValue(context.Background(), ctxKey{}, "from-parent")

	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		val := ctx.Value(ctxKey{})
		if val != "from-parent" {
			t.Error("expected parent context value to propagate")
		}
		return "ok", nil
	})

	runner.SetParentContext(parentCtx)

	state, _ := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "ctx-agent",
		Prompt:    "test",
	})

	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)
}

func TestTeammateRunner_MultipleAgentsParallel(t *testing.T) {
	var mu sync.Mutex
	started := make(map[string]bool)

	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		mu.Lock()
		started[prompt] = true
		mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		return "done: " + prompt, nil
	})

	handler := &mockEventHandler{}
	runner.SetEventHandler(handler)

	names := []string{"alpha", "beta", "gamma", "delta"}
	for _, name := range names {
		runner.Spawn(SpawnConfig{
			TeamName:  "test-team",
			AgentName: name,
			Prompt:    "task-" + name,
		})
	}

	if !runner.WaitForAll(10 * time.Second) {
		t.Fatal("timeout waiting for all teammates")
	}

	// All should be complete
	states := runner.AllStates()
	for _, s := range states {
		if s.Status != StatusComplete {
			t.Errorf("agent %s: expected complete, got %s", s.Identity.AgentName, s.Status)
		}
	}

	// Should have events for all agents
	events := handler.getEvents()
	startEvents := 0
	completeEvents := 0
	for _, e := range events {
		switch e.Type {
		case "started":
			startEvents++
		case "complete":
			completeEvents++
		}
	}
	if startEvents != 4 {
		t.Errorf("expected 4 started events, got %d", startEvents)
	}
	if completeEvents != 4 {
		t.Errorf("expected 4 complete events, got %d", completeEvents)
	}
}

func TestTeammateRunner_NoEventHandler(t *testing.T) {
	// Should not panic when no handler is set
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	state, _ := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "no-handler",
		Prompt:    "test",
	})

	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	if state.Status != StatusComplete {
		t.Errorf("expected complete, got %s", state.Status)
	}
}

func TestTeammateRunner_GetMailbox(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	// Before any spawn, mailbox is nil
	if runner.GetMailbox() != nil {
		t.Error("expected nil mailbox before spawn")
	}

	state, _ := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "mb-agent",
		Prompt:    "test",
	})
	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	// After spawn, mailbox should exist
	if runner.GetMailbox() == nil {
		t.Error("expected mailbox after spawn")
	}
}

func TestTeammateRunner_ContextDecoratorReceivesTeamName(t *testing.T) {
	var capturedTeamName, capturedAgentName string

	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	runner.SetContextDecorator(func(ctx context.Context, state *TeammateState) context.Context {
		capturedTeamName = state.TeamName
		capturedAgentName = state.Identity.AgentName
		return ctx
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "worker-1",
		Prompt:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	if capturedTeamName != "test-team" {
		t.Errorf("expected TeamName %q, got %q", "test-team", capturedTeamName)
	}
	if capturedAgentName != "worker-1" {
		t.Errorf("expected AgentName %q, got %q", "worker-1", capturedAgentName)
	}
}

func TestTeammateRunner_EmitEvent_NilHandler(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	// Should not panic
	runner.EmitEvent(TeammateEvent{Type: "test"})
}

// --- Model propagation tests ---

func TestTeammateRunner_SpawnStoresModelInState(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "model-agent",
		Prompt:    "test",
		Model:     "haiku",
	})
	if err != nil {
		t.Fatal(err)
	}

	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	if state.Model != "haiku" {
		t.Errorf("expected model %q on state, got %q", "haiku", state.Model)
	}
}

func TestTeammateRunner_SpawnFallsBackToTeamModel(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, "")
	// Create team with default model "sonnet"
	_, err := mgr.CreateTeam("model-team", "test", "sess-1", "sonnet")
	if err != nil {
		t.Fatal(err)
	}

	runner := NewTeammateRunner(mgr, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	// Spawn without per-agent model — should fall back to team's "sonnet"
	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "model-team",
		AgentName: "fallback-agent",
		Prompt:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	if state.Model != "sonnet" {
		t.Errorf("expected model %q (team default), got %q", "sonnet", state.Model)
	}
}

func TestTeammateRunner_PerAgentModelOverridesTeamDefault(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, "")
	// Create team with default model "haiku"
	_, err := mgr.CreateTeam("override-team", "test", "sess-1", "haiku")
	if err != nil {
		t.Fatal(err)
	}

	runner := NewTeammateRunner(mgr, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	// Spawn with per-agent model "opus" — should override team's "haiku"
	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "override-team",
		AgentName: "opus-agent",
		Prompt:    "test",
		Model:     "opus",
	})
	if err != nil {
		t.Fatal(err)
	}

	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	if state.Model != "opus" {
		t.Errorf("expected model %q (per-agent override), got %q", "opus", state.Model)
	}
}

func TestTeammateRunner_ModelPassedToContextDecorator(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, "")
	_, err := mgr.CreateTeam("ctx-model-team", "test", "sess-1", "")
	if err != nil {
		t.Fatal(err)
	}

	var capturedModel string

	runner := NewTeammateRunner(mgr, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	runner.SetContextDecorator(func(ctx context.Context, state *TeammateState) context.Context {
		capturedModel = state.Model
		return ctx
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "ctx-model-team",
		AgentName: "ctx-agent",
		Prompt:    "test",
		Model:     "deepseek-r1-70b",
	})
	if err != nil {
		t.Fatal(err)
	}

	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	if capturedModel != "deepseek-r1-70b" {
		t.Errorf("context decorator got model %q, want %q", capturedModel, "deepseek-r1-70b")
	}
}

func TestTeammateRunner_MixedModelsInTeam(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, "")
	// Team default is "haiku"
	_, err := mgr.CreateTeam("mixed-team", "test", "sess-1", "haiku")
	if err != nil {
		t.Fatal(err)
	}

	runner := NewTeammateRunner(mgr, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	// Agent 1: no model → uses team default "haiku"
	s1, _ := runner.Spawn(SpawnConfig{
		TeamName:  "mixed-team",
		AgentName: "agent-1",
		Prompt:    "simple task",
	})

	// Agent 2: explicit "opus"
	s2, _ := runner.Spawn(SpawnConfig{
		TeamName:  "mixed-team",
		AgentName: "agent-2",
		Prompt:    "complex task",
		Model:     "opus",
	})

	// Agent 3: explicit "deepseek-r1-70b" (provider model)
	s3, _ := runner.Spawn(SpawnConfig{
		TeamName:  "mixed-team",
		AgentName: "agent-3",
		Prompt:    "reasoning task",
		Model:     "deepseek-r1-70b",
	})

	runner.WaitForAll(5 * time.Second)

	if s1.Model != "haiku" {
		t.Errorf("agent-1: expected %q, got %q", "haiku", s1.Model)
	}
	if s2.Model != "opus" {
		t.Errorf("agent-2: expected %q, got %q", "opus", s2.Model)
	}
	if s3.Model != "deepseek-r1-70b" {
		t.Errorf("agent-3: expected %q, got %q", "deepseek-r1-70b", s3.Model)
	}
}

func TestTeammateRunner_NoModelNoTeamDefault(t *testing.T) {
	// Team has no default model, agent has no model → empty (inherits parent session)
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "no-model-agent",
		Prompt:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	if state.Model != "" {
		t.Errorf("expected empty model (inherit parent), got %q", state.Model)
	}
}

// --- Per-team mailbox tests ---

func TestTeammateRunner_PerTeamMailboxIsolation(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, "")
	mgr.CreateTeam("team-a", "test", "sess-1", "")
	mgr.CreateTeam("team-b", "test", "sess-2", "")

	runner := NewTeammateRunner(mgr, func(ctx context.Context, system, prompt string) (string, error) {
		return "done: " + prompt, nil
	})

	s1, _ := runner.Spawn(SpawnConfig{TeamName: "team-a", AgentName: "agent-a", Prompt: "task a"})
	s2, _ := runner.Spawn(SpawnConfig{TeamName: "team-b", AgentName: "agent-b", Prompt: "task b"})

	runner.WaitForOne(s1.Identity.AgentID, 5*time.Second)
	runner.WaitForOne(s2.Identity.AgentID, 5*time.Second)

	// Each team should have its own mailbox
	mbA := runner.getMailbox("team-a")
	mbB := runner.getMailbox("team-b")

	if mbA == nil {
		t.Fatal("expected mailbox for team-a")
	}
	if mbB == nil {
		t.Fatal("expected mailbox for team-b")
	}
	if mbA == mbB {
		t.Error("team-a and team-b should have different mailbox instances")
	}

	// team-lead inbox for team-a should have agent-a's completion message
	msgsA, _ := mbA.ReadAll("team-lead")
	msgsB, _ := mbB.ReadAll("team-lead")

	if len(msgsA) == 0 {
		t.Error("expected team-lead message in team-a mailbox")
	}
	if len(msgsB) == 0 {
		t.Error("expected team-lead message in team-b mailbox")
	}

	// Messages should be from the correct agents
	if len(msgsA) > 0 && msgsA[0].From != "agent-a" {
		t.Errorf("team-a message from %q, expected agent-a", msgsA[0].From)
	}
	if len(msgsB) > 0 && msgsB[0].From != "agent-b" {
		t.Errorf("team-b message from %q, expected agent-b", msgsB[0].From)
	}
}

func TestTeammateRunner_GetMailbox_ActiveTeam(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, "")
	mgr.CreateTeam("team-x", "test", "sess-1", "")
	mgr.CreateTeam("team-y", "test", "sess-2", "")

	runner := NewTeammateRunner(mgr, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	// Spawn in both teams to create mailboxes
	s1, _ := runner.Spawn(SpawnConfig{TeamName: "team-x", AgentName: "ax", Prompt: "t"})
	s2, _ := runner.Spawn(SpawnConfig{TeamName: "team-y", AgentName: "ay", Prompt: "t"})
	runner.WaitForOne(s1.Identity.AgentID, 5*time.Second)
	runner.WaitForOne(s2.Identity.AgentID, 5*time.Second)

	// Set active team to team-x
	runner.SetActiveTeam("team-x")
	mb := runner.GetMailbox()
	if mb == nil {
		t.Fatal("expected mailbox for active team")
	}

	// Should be team-x's mailbox
	mbX := runner.getMailbox("team-x")
	if mb != mbX {
		t.Error("GetMailbox() should return active team's mailbox")
	}

	// Switch active team
	runner.SetActiveTeam("team-y")
	mb = runner.GetMailbox()
	mbY := runner.getMailbox("team-y")
	if mb != mbY {
		t.Error("GetMailbox() should return team-y's mailbox after switching")
	}
}

func TestTeammateRunner_TaskCompleter_Success(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "done", nil
	})

	var mu sync.Mutex
	var completedIDs []string
	var completedStatus string
	runner.SetTaskCompleter(func(taskIDs []string, status, sessionID string) {
		mu.Lock()
		completedIDs = taskIDs
		completedStatus = status
		mu.Unlock()
	})

	state, _ := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "task-agent",
		Prompt:    "do work",
		TaskIDs:   []string{"42"},
	})

	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if len(completedIDs) == 0 || completedIDs[0] != "42" {
		t.Errorf("expected task IDs [\"42\"], got %v", completedIDs)
	}
	if completedStatus != "completed" {
		t.Errorf("expected status %q, got %q", "completed", completedStatus)
	}
}

func TestTeammateRunner_TaskCompleter_Failure(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "", fmt.Errorf("boom")
	})

	var mu sync.Mutex
	var completedIDs []string
	var completedStatus string
	runner.SetTaskCompleter(func(taskIDs []string, status, sessionID string) {
		mu.Lock()
		completedIDs = taskIDs
		completedStatus = status
		mu.Unlock()
	})

	state, _ := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "fail-agent",
		Prompt:    "fail",
		TaskIDs:   []string{"99"},
	})

	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if len(completedIDs) == 0 || completedIDs[0] != "99" {
		t.Errorf("expected task IDs [\"99\"], got %v", completedIDs)
	}
	if completedStatus != "failed" {
		t.Errorf("expected status %q, got %q", "failed", completedStatus)
	}
}

func TestTeammateRunner_CompletionMailboxMessage(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "result text here", nil
	})

	state, _ := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "notifier",
		Prompt:    "notify test",
	})

	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	mb := runner.getMailbox("test-team")
	if mb == nil {
		t.Fatal("expected mailbox for test-team")
	}

	msgs, err := mb.ReadAll("team-lead")
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected completion message in team-lead inbox")
	}

	msg := msgs[0]
	if msg.From != "notifier" {
		t.Errorf("expected from %q, got %q", "notifier", msg.From)
	}
	if !strings.Contains(msg.Text, "result text here") {
		t.Errorf("expected result in message text, got %q", msg.Text)
	}
	if !strings.Contains(msg.Summary, "notifier") {
		t.Errorf("expected agent name in summary, got %q", msg.Summary)
	}
}

func TestTeammateRunner_GetMailbox_NoActiveTeam(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, "")
	runner := NewTeammateRunner(mgr, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	// No active team, no agents — should return nil
	if runner.GetMailbox() != nil {
		t.Error("expected nil mailbox when no active team")
	}
}

// --- Team cleanup tests ---

func TestTeammateRunner_KillTeam_OnlyAffectsTargetTeam(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, "")
	if _, err := mgr.CreateTeam("team-a", "", "sess-a", ""); err != nil {
		t.Fatalf("CreateTeam team-a: %v", err)
	}
	if _, err := mgr.CreateTeam("team-b", "", "sess-b", ""); err != nil {
		t.Fatalf("CreateTeam team-b: %v", err)
	}

	runner := NewTeammateRunner(mgr, func(ctx context.Context, system, prompt string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	t.Cleanup(func() {
		runner.KillAll()
		runner.WaitForTeam("team-b", 5*time.Second)
	})

	a1, _ := runner.Spawn(SpawnConfig{TeamName: "team-a", AgentName: "a1", Prompt: "t"})
	a2, _ := runner.Spawn(SpawnConfig{TeamName: "team-a", AgentName: "a2", Prompt: "t"})
	b1, _ := runner.Spawn(SpawnConfig{TeamName: "team-b", AgentName: "b1", Prompt: "t"})

	time.Sleep(50 * time.Millisecond)
	if c := runner.WorkingCount(); c != 3 {
		t.Fatalf("expected 3 working before KillTeam, got %d", c)
	}

	runner.KillTeam("team-a")
	if !runner.WaitForTeam("team-a", 5*time.Second) {
		t.Fatal("WaitForTeam timed out for team-a")
	}

	// team-a members should be idle
	if !a1.IsIdle || !a2.IsIdle {
		t.Error("expected team-a members to be idle after KillTeam")
	}
	// team-b member should still be running
	if b1.IsIdle {
		t.Error("team-b member should not have been killed")
	}
	if c := runner.WorkingCount(); c != 1 {
		t.Errorf("expected 1 still working (team-b), got %d", c)
	}
}

func TestTeammateRunner_CleanupTeam_RemovesStateAndMailbox(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "worker",
		Prompt:    "t",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Mailbox is created lazily in Spawn — confirm it exists.
	if runner.getMailbox("test-team") == nil {
		t.Fatal("expected mailbox to exist after Spawn")
	}
	runner.SetActiveTeam("test-team")

	runner.KillTeam("test-team")
	if !runner.WaitForTeam("test-team", 5*time.Second) {
		t.Fatal("WaitForTeam timed out")
	}

	runner.CleanupTeam("test-team")

	// Teammate state should be gone
	if _, ok := runner.GetState(state.Identity.AgentID); ok {
		t.Error("expected teammate state to be removed after CleanupTeam")
	}
	if len(runner.AllStates()) != 0 {
		t.Errorf("expected 0 states after cleanup, got %d", len(runner.AllStates()))
	}
	// Mailbox should be gone
	if runner.getMailbox("test-team") != nil {
		t.Error("expected mailbox to be removed after CleanupTeam")
	}
	// Active team pointer should be cleared
	if runner.ActiveTeamName() != "" {
		t.Errorf("expected activeTeam cleared, got %q", runner.ActiveTeamName())
	}
}

func TestTeammateRunner_CleanupTeam_DoesNotTouchOtherTeams(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, "")
	if _, err := mgr.CreateTeam("team-a", "", "sess-a", ""); err != nil {
		t.Fatalf("CreateTeam team-a: %v", err)
	}
	if _, err := mgr.CreateTeam("team-b", "", "sess-b", ""); err != nil {
		t.Fatalf("CreateTeam team-b: %v", err)
	}

	runner := NewTeammateRunner(mgr, func(ctx context.Context, system, prompt string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	t.Cleanup(func() {
		runner.KillAll()
		runner.WaitForTeam("team-b", 5*time.Second)
	})

	runner.Spawn(SpawnConfig{TeamName: "team-a", AgentName: "a1", Prompt: "t"})
	bState, _ := runner.Spawn(SpawnConfig{TeamName: "team-b", AgentName: "b1", Prompt: "t"})

	runner.KillTeam("team-a")
	runner.WaitForTeam("team-a", 5*time.Second)
	runner.CleanupTeam("team-a")

	// team-b state must remain
	if _, ok := runner.GetState(bState.Identity.AgentID); !ok {
		t.Error("team-b state should still exist after CleanupTeam(team-a)")
	}
	if runner.getMailbox("team-b") == nil {
		t.Error("team-b mailbox should still exist after CleanupTeam(team-a)")
	}
}

func TestTeammateRunner_CreateDeleteCycles_NoLeak(t *testing.T) {
	// Verify that repeatedly creating and tearing down teams doesn't grow
	// the runner's internal maps.
	dir := t.TempDir()
	mgr := NewManager(dir, "")
	runner := NewTeammateRunner(mgr, func(ctx context.Context, system, prompt string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("cycle-team-%d", i)
		if _, err := mgr.CreateTeam(name, "", "sess", ""); err != nil {
			t.Fatalf("CreateTeam %s: %v", name, err)
		}
		runner.Spawn(SpawnConfig{TeamName: name, AgentName: "w1", Prompt: "t"})
		runner.Spawn(SpawnConfig{TeamName: name, AgentName: "w2", Prompt: "t"})

		runner.KillTeam(name)
		if !runner.WaitForTeam(name, 5*time.Second) {
			t.Fatalf("WaitForTeam %s timed out", name)
		}
		if err := mgr.DeleteTeam(name); err != nil {
			t.Fatalf("DeleteTeam %s: %v", name, err)
		}
		runner.CleanupTeam(name)
	}

	if n := len(runner.AllStates()); n != 0 {
		t.Errorf("expected 0 teammate states after cycles, got %d", n)
	}
	// mailboxes map should also be empty — peek under the lock.
	runner.mu.RLock()
	mbCount := len(runner.mailboxes)
	runner.mu.RUnlock()
	if mbCount != 0 {
		t.Errorf("expected 0 mailboxes after cycles, got %d", mbCount)
	}
}

func TestTeammateRunner_WaitForTeam_TimeoutOnUnknownTeam(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	// Unknown team has no members → considered all-idle immediately, returns true.
	if !runner.WaitForTeam("nonexistent", 1*time.Second) {
		t.Error("expected WaitForTeam to return true for a team with no members")
	}
}

// --- Memory-aware spawn tests ---

func TestTeammateRunner_Spawn_UsesMemoryAwareRunner(t *testing.T) {
	var plainCalled, memCalled int32
	var capturedMemDir string
	var memMu sync.Mutex

	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		memMu.Lock()
		plainCalled++
		memMu.Unlock()
		return "plain", nil
	})

	runner.SetRunAgentWithMemory(func(ctx context.Context, system, prompt, memoryDir string) (string, error) {
		memMu.Lock()
		memCalled++
		capturedMemDir = memoryDir
		memMu.Unlock()
		return "with-memory: " + memoryDir, nil
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "crystallized-worker",
		Prompt:    "do work",
		MemoryDir: "/tmp/agents/crystal/memory",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if !runner.WaitForOne(state.Identity.AgentID, 5*time.Second) {
		t.Fatal("timeout waiting for teammate")
	}

	memMu.Lock()
	defer memMu.Unlock()

	if memCalled != 1 {
		t.Errorf("expected memory-aware runner called once, got %d", memCalled)
	}
	if plainCalled != 0 {
		t.Errorf("expected plain runner NOT called, got %d", plainCalled)
	}
	if capturedMemDir != "/tmp/agents/crystal/memory" {
		t.Errorf("expected memory dir propagated, got %q", capturedMemDir)
	}
	// Result may have worktree preservation note appended; check prefix
	expectedPrefix := "with-memory: /tmp/agents/crystal/memory"
	if !strings.HasPrefix(state.Result, expectedPrefix) {
		t.Errorf("expected result from memory-aware runner, got %q", state.Result)
	}
	if state.MemoryDir != "/tmp/agents/crystal/memory" {
		t.Errorf("expected state.MemoryDir to be set, got %q", state.MemoryDir)
	}
}

func TestTeammateRunner_Spawn_FallsBackToPlainRunner_NoMemoryDir(t *testing.T) {
	var plainCalled, memCalled int32
	var mu sync.Mutex

	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		mu.Lock()
		plainCalled++
		mu.Unlock()
		return "plain-result", nil
	})

	runner.SetRunAgentWithMemory(func(ctx context.Context, system, prompt, memoryDir string) (string, error) {
		mu.Lock()
		memCalled++
		mu.Unlock()
		return "should-not-be-called", nil
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "plain-worker",
		Prompt:    "do work",
		// MemoryDir intentionally empty
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if !runner.WaitForOne(state.Identity.AgentID, 5*time.Second) {
		t.Fatal("timeout waiting for teammate")
	}

	mu.Lock()
	defer mu.Unlock()

	if plainCalled != 1 {
		t.Errorf("expected plain runner called once, got %d", plainCalled)
	}
	if memCalled != 0 {
		t.Errorf("expected memory-aware runner NOT called, got %d", memCalled)
	}
	// Result may have worktree preservation note appended; check prefix
	expectedPrefix := "plain-result"
	if !strings.HasPrefix(state.Result, expectedPrefix) {
		t.Errorf("expected plain result, got %q", state.Result)
	}
}

// TestTeammateRunner_EmitEvent_CallsHandler verifies that a registered handler
// is invoked synchronously with the exact event passed to EmitEvent.
func TestTeammateRunner_EmitEvent_CallsHandler(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	handler := &mockEventHandler{}
	runner.SetEventHandler(handler)

	event := TeammateEvent{AgentName: "alex", Type: "complete"}
	runner.EmitEvent(event)

	events := handler.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].AgentName != "alex" {
		t.Errorf("AgentName: want %q, got %q", "alex", events[0].AgentName)
	}
	if events[0].Type != "complete" {
		t.Errorf("Type: want %q, got %q", "complete", events[0].Type)
	}
}

// TestTeammateRunner_EmitEvent_NoHandler_NoPanic verifies that calling EmitEvent
// with no handler registered does not panic and completes normally.
func TestTeammateRunner_EmitEvent_NoHandler_NoPanic(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})
	// No handler registered — must not panic.
	runner.EmitEvent(TeammateEvent{AgentName: "alex", Type: "complete"})
}

func TestTeammateRunner_Spawn_FallsBackToPlainRunner_NoMemoryRunnerSet(t *testing.T) {
	var plainCalled int32
	var mu sync.Mutex

	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		mu.Lock()
		plainCalled++
		mu.Unlock()
		return "plain-only", nil
	})
	// Intentionally do NOT call SetRunAgentWithMemory.

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "worker",
		Prompt:    "do work",
		MemoryDir: "/some/memory/dir", // set, but no memory runner wired
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if !runner.WaitForOne(state.Identity.AgentID, 5*time.Second) {
		t.Fatal("timeout waiting for teammate")
	}

	mu.Lock()
	defer mu.Unlock()

	if plainCalled != 1 {
		t.Errorf("expected plain runner called once as fallback, got %d", plainCalled)
	}
	// Result may have worktree preservation note appended; check prefix
	expectedPrefix := "plain-only"
	if !strings.HasPrefix(state.Result, expectedPrefix) {
		t.Errorf("expected plain result, got %q", state.Result)
	}
}

// TestTeammateRunner_PublishesSessionID verifies that after SetSessionID("sess-123"),
// the bus.Event published on completion carries e.SessionID == "sess-123".
func TestTeammateRunner_PublishesSessionID(t *testing.T) {
	b := bus.New()
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "result text", nil
	})
	runner.SetBus(b)
	runner.SetSessionID("sess-123")

	var gotEvent bus.Event
	var once sync.Once
	done := make(chan struct{})

	b.Subscribe(attach.EventAgentStatus, func(event bus.Event) {
		once.Do(func() {
			gotEvent = event
			close(done)
		})
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "worker",
		Prompt:    "do work",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !runner.WaitForOne(state.Identity.AgentID, 5*time.Second) {
		t.Fatal("timeout waiting for teammate")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no bus event published")
	}

	if gotEvent.SessionID != "sess-123" {
		t.Errorf("SessionID = %q, want %q", gotEvent.SessionID, "sess-123")
	}
}

// TestTeammateRunner_PublishesResult verifies that AgentStatusPayload.Result
// contains the teammate's output text when status is done.
func TestTeammateRunner_PublishesResult(t *testing.T) {
	const wantResult = "task finished successfully"
	b := bus.New()
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return wantResult, nil
	})
	runner.SetBus(b)
	runner.SetSessionID("sess-abc")

	type captured struct {
		event bus.Event
		done  chan struct{}
	}
	cap := &captured{done: make(chan struct{})}
	var once sync.Once

	b.Subscribe(attach.EventAgentStatus, func(event bus.Event) {
		var p attach.AgentStatusPayload
		if err := json.Unmarshal(event.Payload, &p); err != nil {
			return
		}
		if p.Status == "done" {
			once.Do(func() {
				cap.event = event
				close(cap.done)
			})
		}
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "worker",
		Prompt:    "do work",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !runner.WaitForOne(state.Identity.AgentID, 5*time.Second) {
		t.Fatal("timeout waiting for teammate")
	}

	select {
	case <-cap.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no done event published")
	}

	var p attach.AgentStatusPayload
	if err := json.Unmarshal(cap.event.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p.Result != wantResult {
		t.Errorf("Result = %q, want %q", p.Result, wantResult)
	}
	if p.Status != "done" {
		t.Errorf("Status = %q, want %q", p.Status, "done")
	}
}

// --- Grandchild routing regression tests ---
// These tests guard against the bug where teammates spawned by teammates
// were having their completions injected into the main engine.

// TestTeammateRunner_SpawnStoresParentAgentID verifies that ParentAgentID from
// SpawnConfig is persisted on the TeammateState, so downstream consumers (e.g.
// the EventAgentStatus subscriber in root.go) can identify grandchildren.
func TestTeammateRunner_SpawnStoresParentAgentID(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:      "test-team",
		AgentName:     "grandchild",
		Prompt:        "sub-task",
		ParentAgentID: "agent-alex-id",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	if state.ParentAgentID != "agent-alex-id" {
		t.Errorf("ParentAgentID = %q, want %q", state.ParentAgentID, "agent-alex-id")
	}
}

// TestTeammateRunner_SpawnWithParent_NotLead verifies that an agent spawned with a
// ParentAgentID is NOT marked as lead. Only direct children of the main session
// (no parent) are leads.
func TestTeammateRunner_SpawnWithParent_NotLead(t *testing.T) {
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "ok", nil
	})

	// Direct child — no parent → should be lead.
	directChild, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "direct-child",
		Prompt:    "top-level task",
	})
	if err != nil {
		t.Fatalf("Spawn direct child: %v", err)
	}
	runner.WaitForOne(directChild.Identity.AgentID, 5*time.Second)

	if !directChild.Identity.IsLead {
		t.Error("direct child (no parent) must be marked as lead")
	}

	// Grandchild — has parent → must NOT be lead.
	grandchild, err := runner.Spawn(SpawnConfig{
		TeamName:      "test-team",
		AgentName:     "grandchild",
		Prompt:        "sub-task",
		ParentAgentID: directChild.Identity.AgentID,
	})
	if err != nil {
		t.Fatalf("Spawn grandchild: %v", err)
	}
	runner.WaitForOne(grandchild.Identity.AgentID, 5*time.Second)

	if grandchild.Identity.IsLead {
		t.Error("grandchild (has parent) must NOT be marked as lead")
	}
}

// TestTeammateRunner_PublishesParentAgentID is the critical regression test.
//
// When a teammate (grandchild) completes, the EventAgentStatus payload MUST
// carry its ParentAgentID so the root.go subscriber can filter it out and avoid
// injecting the result into the main engine's injectCh.
func TestTeammateRunner_PublishesParentAgentID(t *testing.T) {
	b := bus.New()
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "grandchild done", nil
	})
	runner.SetBus(b)
	runner.SetSessionID("sess-xyz")

	type captured struct {
		payload attach.AgentStatusPayload
		done    chan struct{}
	}
	cap := &captured{done: make(chan struct{})}
	var once sync.Once

	b.Subscribe(attach.EventAgentStatus, func(event bus.Event) {
		var p attach.AgentStatusPayload
		if err := json.Unmarshal(event.Payload, &p); err != nil {
			return
		}
		if p.Status == "done" && p.Name == "grandchild" {
			once.Do(func() {
				cap.payload = p
				close(cap.done)
			})
		}
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:      "test-team",
		AgentName:     "grandchild",
		Prompt:        "sub-task",
		ParentAgentID: "agent-alex-id",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	runner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	select {
	case <-cap.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no done event published for grandchild")
	}

	if cap.payload.ParentAgentID != "agent-alex-id" {
		t.Errorf("EventAgentStatus.ParentAgentID = %q, want %q",
			cap.payload.ParentAgentID, "agent-alex-id")
	}
	if cap.payload.Status != "done" {
		t.Errorf("Status = %q, want %q", cap.payload.Status, "done")
	}
}

// TestTeammateRunner_CompletionNoDoubleEmit verifies that a terminal EventAgentStatus
// (status "done" or "failed") is emitted exactly once per agent run.
// The stopHeartbeat channel is closed before the terminal event fires, so the
// heartbeat goroutine cannot race and emit an extra "working" status that confuses
// UI clients into showing an incorrect state after completion.
func TestTeammateRunner_CompletionNoDoubleEmit(t *testing.T) {
	b := bus.New()
	runner, _ := setupRunner(t, func(ctx context.Context, system, prompt string) (string, error) {
		return "finished", nil
	})
	runner.SetBus(b)
	runner.SetSessionID("sess-dedup")

	var mu sync.Mutex
	terminalCount := 0
	done := make(chan struct{})

	b.Subscribe(attach.EventAgentStatus, func(event bus.Event) {
		var p attach.AgentStatusPayload
		if err := json.Unmarshal(event.Payload, &p); err != nil {
			return
		}
		if p.Status == "done" || p.Status == "failed" || p.Status == "waiting" {
			mu.Lock()
			terminalCount++
			if terminalCount == 1 {
				close(done)
			}
			mu.Unlock()
		}
	})

	state, err := runner.Spawn(SpawnConfig{
		TeamName:  "test-team",
		AgentName: "dedup-worker",
		Prompt:    "finish quickly",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !runner.WaitForOne(state.Identity.AgentID, 5*time.Second) {
		t.Fatal("timeout waiting for teammate")
	}

	// Wait for the terminal event to arrive.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no terminal EventAgentStatus received")
	}

	// Give a brief window for any phantom second event to arrive.
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	got := terminalCount
	mu.Unlock()

	if got != 1 {
		t.Errorf("terminal EventAgentStatus count = %d, want exactly 1 (double-emit detected)", got)
	}
}
