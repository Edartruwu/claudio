package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/bus"
)

// publishAgentStatus is a test helper that publishes an EventAgentStatus bus event.
func publishAgentStatus(b *bus.Bus, sessionID, name, status, result string) {
	payload, _ := json.Marshal(attach.AgentStatusPayload{
		Name:   name,
		Status: status,
		Result: result,
	})
	b.Publish(bus.Event{
		Type:      attach.EventAgentStatus,
		Payload:   payload,
		SessionID: sessionID,
	})
}

// publishAgentStatusWithParent publishes an EventAgentStatus with a non-empty ParentAgentID.
func publishAgentStatusWithParent(b *bus.Bus, sessionID, name, status, result, parentID string) {
	payload, _ := json.Marshal(attach.AgentStatusPayload{
		Name:          name,
		Status:        status,
		Result:        result,
		ParentAgentID: parentID,
	})
	b.Publish(bus.Event{
		Type:      attach.EventAgentStatus,
		Payload:   payload,
		SessionID: sessionID,
	})
}

// TestTeammateNotification_InjectsWithoutUserMessage is the critical regression test.
//
// The historical bug: teammate completion only fired on the next user message because
// the engine was idle and nothing woke it. The fix injects directly into injectCh.
//
// This test asserts that a message arrives on injectCh within 100ms after publishing
// an EventAgentStatus{status:"done"} — WITHOUT sending any user message.
// If a user message is required first, the channel will stay empty and t.Fatal fires.
func TestTeammateNotification_InjectsWithoutUserMessage(t *testing.T) {
	b := bus.New()
	injectCh := make(chan attach.UserMsgPayload, 1)

	unsub := SubscribeTeammateNotifications(b, "s1", injectCh)
	defer unsub()

	// Publish the done event — no user message sent, no engine running.
	publishAgentStatus(b, "s1", "alex", "done", "merged cleanly")

	select {
	case msg := <-injectCh:
		// Success: engine was woken without a user message.
		if msg.Content == "" {
			t.Error("injected message content is empty")
		}
	case <-time.After(100 * time.Millisecond):
		// FAIL: this is the exact historical bug — engine stayed idle.
		t.Fatal("no injection within 100ms: engine not woken without user message (regression)")
	}
}

// TestTeammateNotification_FiltersWrongSession verifies that events for a
// different session ID are not injected into injectCh.
func TestTeammateNotification_FiltersWrongSession(t *testing.T) {
	b := bus.New()
	injectCh := make(chan attach.UserMsgPayload, 1)

	unsub := SubscribeTeammateNotifications(b, "s1", injectCh)
	defer unsub()

	// Publish for a different session.
	publishAgentStatus(b, "other-session", "alex", "done", "some result")

	select {
	case msg := <-injectCh:
		t.Errorf("unexpected injection for wrong session: %q", msg.Content)
	case <-time.After(50 * time.Millisecond):
		// Correct: nothing injected for wrong session.
	}
}

// TestTeammateNotification_IgnoresWorkingStatus verifies that in-progress
// (status:"working") events do not inject into injectCh — only "done" does.
func TestTeammateNotification_IgnoresWorkingStatus(t *testing.T) {
	b := bus.New()
	injectCh := make(chan attach.UserMsgPayload, 1)

	unsub := SubscribeTeammateNotifications(b, "s1", injectCh)
	defer unsub()

	publishAgentStatus(b, "s1", "alex", "working", "")

	select {
	case msg := <-injectCh:
		t.Errorf("unexpected injection for working status: %q", msg.Content)
	case <-time.After(50 * time.Millisecond):
		// Correct: working status must not wake the engine.
	}
}

// TestTeammateNotification_IgnoresGrandchildren verifies that completion events from
// teammates spawned BY other teammates (grandchildren) are not injected into the main
// engine. Only direct children of the main session should wake it.
func TestTeammateNotification_IgnoresGrandchildren(t *testing.T) {
	b := bus.New()
	injectCh := make(chan attach.UserMsgPayload, 1)

	unsub := SubscribeTeammateNotifications(b, "s1", injectCh)
	defer unsub()

	// Grandchild: has a non-empty ParentAgentID pointing to its parent teammate.
	publishAgentStatusWithParent(b, "s1", "orion", "done", "exploration complete", "agent-alex-id")

	select {
	case msg := <-injectCh:
		t.Errorf("grandchild completion must not inject into main engine: %q", msg.Content)
	case <-time.After(50 * time.Millisecond):
		// Correct: grandchild result stays within parent teammate's tool context.
	}
}
