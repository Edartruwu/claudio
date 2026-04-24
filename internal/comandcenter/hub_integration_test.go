package comandcenter

import (
	"sync"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/attach"
)

// seedSession inserts a minimal cc_sessions row so that cc_messages FK passes.
func seedSession(t *testing.T, s *Storage, id string) {
	t.Helper()
	now := time.Now()
	if err := s.UpsertSession(Session{
		ID:           id,
		Name:         id,
		Status:       "active",
		CreatedAt:    now,
		LastActiveAt: now,
	}); err != nil {
		t.Fatalf("seedSession %s: %v", id, err)
	}
}

// sendToWorker pushes an envelope to the per-session event queue and waits for
// the worker goroutine to drain it before returning.  It starts the worker if
// one is not already running, then closes it (and waits for drain) once all
// envelopes have been sent.
func sendToWorker(t *testing.T, h *Hub, sessionID string, envs ...attach.Envelope) {
	t.Helper()

	h.startSessionWorker(sessionID)

	h.mu.RLock()
	ch := h.eventQueues[sessionID]
	h.mu.RUnlock()

	for _, env := range envs {
		ch <- env
	}

	// Close the channel — the worker drains then exits.
	h.stopSessionWorker(sessionID)

	// Block until the worker goroutine has fully exited so that all DB writes
	// are committed before the caller queries storage.
	if !h.WaitSessionWorker(sessionID, 2*time.Second) {
		t.Fatal("worker goroutine did not exit in time")
	}
}

// pollMessages polls storage until at least minCount messages appear for the
// session, or the 2-second deadline is exceeded.
func pollMessages(t *testing.T, s *Storage, sessionID string, minCount int) []Message {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timeout: wanted >= %d messages for session %s, got 0", minCount, sessionID)
		case <-time.After(10 * time.Millisecond):
		}
		msgs, err := s.ListMessages(sessionID, 50)
		if err != nil {
			t.Fatalf("ListMessages: %v", err)
		}
		if len(msgs) >= minCount {
			return msgs
		}
	}
}

// TestProcessEvent_AgentNameStored verifies that an EventMsgAssistant envelope
// with AgentName="alex" is persisted with the correct agent_name and session_id.
func TestProcessEvent_AgentNameStored(t *testing.T) {
	s := newTestStorage(t)
	h := NewHub(s)

	const sessionID = "sess-agent-name"
	seedSession(t, s, sessionID)

	env, err := attach.NewEnvelope(attach.EventMsgAssistant, attach.AssistantMsgPayload{
		Content:   "work done",
		AgentName: "alex",
	})
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}

	sendToWorker(t, h, sessionID, env)

	msgs := pollMessages(t, s, sessionID, 1)

	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].AgentName != "alex" {
		t.Errorf("agent_name: got %q, want %q", msgs[0].AgentName, "alex")
	}
	if msgs[0].SessionID != sessionID {
		t.Errorf("session_id: got %q, want %q", msgs[0].SessionID, sessionID)
	}
	if msgs[0].Content != "work done" {
		t.Errorf("content: got %q, want %q", msgs[0].Content, "work done")
	}
}

// TestProcessEvent_ToolUseBeforeToolResult verifies that a tool_use INSERT
// committed before the tool_result UPDATE finds the row.  Without the
// per-session sequential queue the UPDATE would be a silent no-op because
// SQLite WAL mode does not guarantee cross-connection read-your-own-writes
// within the same transaction window.
func TestProcessEvent_ToolUseBeforeToolResult(t *testing.T) {
	s := newTestStorage(t)
	h := NewHub(s)

	const sessionID = "sess-tool-order"
	const toolUseID = "tool-abc-1"
	seedSession(t, s, sessionID)

	toolUseEnv, err := attach.NewEnvelope(attach.EventMsgToolUse, attach.ToolUsePayload{
		ID:        toolUseID,
		Tool:      "bash",
		AgentName: "rafael",
	})
	if err != nil {
		t.Fatalf("NewEnvelope tool_use: %v", err)
	}

	toolResultEnv, err := attach.NewEnvelope(attach.EventMsgToolResult, attach.ToolResultPayload{
		ToolUseID: toolUseID,
		Output:    "result text",
		AgentName: "rafael",
	})
	if err != nil {
		t.Fatalf("NewEnvelope tool_result: %v", err)
	}

	// Both events sent to the same worker — guaranteed ordered delivery.
	sendToWorker(t, h, sessionID, toolUseEnv, toolResultEnv)

	msgs := pollMessages(t, s, sessionID, 1)

	// Find the tool_use row (there should be exactly one).
	var found *Message
	for i := range msgs {
		if msgs[i].ToolUseID == toolUseID {
			found = &msgs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no cc_messages row with tool_use_id=%q", toolUseID)
	}
	if found.Output != "result text" {
		t.Errorf("output: got %q, want %q — UPDATE was a no-op (INSERT not committed first?)", found.Output, "result text")
	}
}

// TestProcessEvent_SessionIsolation verifies that concurrent events for two
// different sessions are each stored under their own session_id with no
// cross-contamination.
func TestProcessEvent_SessionIsolation(t *testing.T) {
	s := newTestStorage(t)
	h := NewHub(s)

	const sessA = "sess-iso-alpha"
	const sessB = "sess-iso-beta"
	seedSession(t, s, sessA)
	seedSession(t, s, sessB)

	envA, err := attach.NewEnvelope(attach.EventMsgAssistant, attach.AssistantMsgPayload{
		Content:   "msg from agentA",
		AgentName: "agentA",
	})
	if err != nil {
		t.Fatalf("NewEnvelope sessA: %v", err)
	}

	envB, err := attach.NewEnvelope(attach.EventMsgAssistant, attach.AssistantMsgPayload{
		Content:   "msg from agentB",
		AgentName: "agentB",
	})
	if err != nil {
		t.Fatalf("NewEnvelope sessB: %v", err)
	}

	// Start both workers before sending, so they run concurrently.
	h.startSessionWorker(sessA)
	h.startSessionWorker(sessB)

	h.mu.RLock()
	chA := h.eventQueues[sessA]
	chB := h.eventQueues[sessB]
	h.mu.RUnlock()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() { defer wg.Done(); chA <- envA }()
	go func() { defer wg.Done(); chB <- envB }()

	wg.Wait()

	// Stop both workers — waits for drain via channel close.
	h.stopSessionWorker(sessA)
	h.stopSessionWorker(sessB)
	if !h.WaitSessionWorker(sessA, 2*time.Second) {
		t.Fatal("worker A goroutine did not exit in time")
	}
	if !h.WaitSessionWorker(sessB, 2*time.Second) {
		t.Fatal("worker B goroutine did not exit in time")
	}

	msgsA := pollMessages(t, s, sessA, 1)
	msgsB := pollMessages(t, s, sessB, 1)

	// Session A must only contain agentA's message.
	for _, m := range msgsA {
		if m.AgentName != "agentA" {
			t.Errorf("sess-A: unexpected agent_name %q (want agentA) — cross-contamination", m.AgentName)
		}
		if m.SessionID != sessA {
			t.Errorf("sess-A: unexpected session_id %q", m.SessionID)
		}
	}

	// Session B must only contain agentB's message.
	for _, m := range msgsB {
		if m.AgentName != "agentB" {
			t.Errorf("sess-B: unexpected agent_name %q (want agentB) — cross-contamination", m.AgentName)
		}
		if m.SessionID != sessB {
			t.Errorf("sess-B: unexpected session_id %q", m.SessionID)
		}
	}

	// Ensure no leakage: sess-A messages must not appear in sess-B query and vice versa.
	for _, m := range msgsA {
		if m.SessionID == sessB {
			t.Errorf("sess-A message leaked into sess-B: %+v", m)
		}
	}
	for _, m := range msgsB {
		if m.SessionID == sessA {
			t.Errorf("sess-B message leaked into sess-A: %+v", m)
		}
	}
}
