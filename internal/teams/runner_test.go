package teams

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
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
	mgr := NewManager(dir)
	_, err := mgr.CreateTeam("test-team", "test", "sess-1")
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
	if state.Result != "done: do something" {
		t.Errorf("unexpected result: %q", state.Result)
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
