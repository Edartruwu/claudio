package cli

import (
	"encoding/json"
	"fmt"

	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/bus"
)

// SubscribeTeammateNotifications subscribes to the bus and injects a notification
// message into injectCh whenever a teammate with the matching sessionID finishes
// (status == "done"). This wakes the engine immediately — no user message required.
//
// The returned unsubscribe function should be called when the session ends.
func SubscribeTeammateNotifications(b *bus.Bus, sessionID string, injectCh chan<- attach.UserMsgPayload) func() {
	return b.Subscribe(attach.EventAgentStatus, func(event bus.Event) {
		// Filter: only handle events for the principal session.
		if event.SessionID != sessionID {
			return
		}

		var p attach.AgentStatusPayload
		if err := json.Unmarshal(event.Payload, &p); err != nil {
			return
		}

		// Only inject on terminal "done" status — not working/waiting/idle.
		if p.Status != "done" {
			return
		}
		// Grandchildren (spawned by a teammate, not directly by the main session)
		// report back via their parent's tool result — do not inject into the main engine.
		if p.ParentAgentID != "" {
			return
		}

		msg := fmt.Sprintf(
			"[Teammate %s completed]\n\nResult:\n%s",
			p.Name, p.Result,
		)

		// Non-blocking send: if injectCh is full the engine is already busy,
		// so this notification will be picked up on the next available slot.
		select {
		case injectCh <- attach.UserMsgPayload{Content: msg}:
		default:
		}
	})
}
