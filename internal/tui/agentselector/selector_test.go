package agentselector

import "testing"

// ---------------------------------------------------------------------------
// AgentSelectedMsg — Capabilities field (compile-time + runtime check)
// ---------------------------------------------------------------------------

func TestAgentSelectedMsg_HasCapabilitiesField(t *testing.T) {
	msg := AgentSelectedMsg{
		AgentType:    "design",
		DisplayName:  "Design Agent",
		Capabilities: []string{"design"},
	}
	if len(msg.Capabilities) != 1 {
		t.Errorf("expected 1 capability, got %d", len(msg.Capabilities))
	}
	if msg.Capabilities[0] != "design" {
		t.Errorf("expected Capabilities[0]=%q, got %q", "design", msg.Capabilities[0])
	}
}

func TestAgentSelectedMsg_EmptyCapabilities(t *testing.T) {
	msg := AgentSelectedMsg{
		AgentType: "general-purpose",
	}
	if msg.Capabilities != nil && len(msg.Capabilities) != 0 {
		t.Errorf("expected nil/empty Capabilities for general-purpose agent, got %v", msg.Capabilities)
	}
}

func TestAgentSelectedMsg_MultipleCapabilities(t *testing.T) {
	msg := AgentSelectedMsg{
		AgentType:    "custom",
		Capabilities: []string{"design", "extra"},
	}
	if len(msg.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(msg.Capabilities))
	}
}
