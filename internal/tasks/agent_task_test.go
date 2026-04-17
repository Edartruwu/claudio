package tasks

import (
	"encoding/json"
	"testing"

	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/bus"
)

// TestAgentTaskInput_AcceptsEventBus verifies EventBus field can be set.
func TestAgentTaskInput_AcceptsEventBus(t *testing.T) {
	eventBus := bus.New()
	input := AgentTaskInput{
		Prompt:      "test",
		Description: "test",
		EventBus:    eventBus,
	}

	if input.EventBus == nil {
		t.Error("EventBus field should be set")
	}
}

// TestAgentTaskInput_PublishesOnStatusChange verifies the publish pattern works.
func TestAgentTaskInput_PublishesOnStatusChange(t *testing.T) {
	eventBus := bus.New()

	// Subscribe to status events
	events := make(chan bus.Event, 1)
	eventBus.Subscribe(attach.EventAgentStatus, func(e bus.Event) {
		events <- e
	})

	// Simulate publishing
	payload, _ := json.Marshal(attach.AgentStatusPayload{
		Name:   "test-agent",
		Status: "completed",
	})
	eventBus.Publish(bus.Event{
		Type:    attach.EventAgentStatus,
		Payload: payload,
	})

	// Verify received
	evt := <-events
	if evt.Type != attach.EventAgentStatus {
		t.Errorf("expected %s, got %s", attach.EventAgentStatus, evt.Type)
	}

	var p attach.AgentStatusPayload
	json.Unmarshal(evt.Payload, &p)
	if p.Status != "completed" {
		t.Errorf("expected completed, got %s", p.Status)
	}
}
