package attach

import (
	"encoding/json"
	"fmt"
)

// Events: Claudio → ComandCenter
const (
	EventSessionHello = "session.hello"
	EventMsgAssistant = "message.assistant"
	EventMsgToolUse   = "message.tool_use"
	EventTaskCreated  = "task.created"
	EventTaskUpdated  = "task.updated"
	EventAgentStatus  = "agent.status"
	EventSessionBye   = "session.bye"
)

// Events: ComandCenter → Claudio
const (
	EventMsgUser   = "message.user"
	EventInterrupt = "session.interrupt"
	EventSetAgent  = "set_agent"
	EventSetTeam   = "set_team"
)

// SetAgentPayload for EventSetAgent.
type SetAgentPayload struct {
	AgentType string `json:"agent_type"`
}

// SetTeamPayload for EventSetTeam.
type SetTeamPayload struct {
	TeamName string `json:"team_name"`
}

// Envelope wraps event type + payload.
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// HelloPayload for EventSessionHello.
type HelloPayload struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Model  string `json:"model,omitempty"`
	Master bool   `json:"master,omitempty"`
}

// AssistantMsgPayload for EventMsgAssistant.
type AssistantMsgPayload struct {
	Content   string `json:"content"`
	AgentName string `json:"agent_name,omitempty"`
}

// ToolUsePayload for EventMsgToolUse.
type ToolUsePayload struct {
	Tool      string          `json:"tool"`
	Input     json.RawMessage `json:"input,omitempty"`
	AgentName string          `json:"agent_name,omitempty"`
}

// TaskCreatedPayload for EventTaskCreated.
type TaskCreatedPayload struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	AssignedTo string `json:"assigned_to,omitempty"`
	Status     string `json:"status"`
}

// TaskUpdatedPayload for EventTaskUpdated.
type TaskUpdatedPayload struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Output string `json:"output,omitempty"`
}

// AgentStatusPayload for EventAgentStatus.
type AgentStatusPayload struct {
	Name        string `json:"name"`
	Status      string `json:"status"` // idle|working|done|waiting
	CurrentTask string `json:"current_task,omitempty"`
}

// UserMsgPayload for EventMsgUser.
type UserMsgPayload struct {
	Content       string       `json:"content"`
	Attachments   []Attachment `json:"attachments,omitempty"`
	FromSession   string       `json:"from_session,omitempty"`
	ModelOverride string       `json:"model_override,omitempty"`
}

// Attachment in UserMsgPayload.
type Attachment struct {
	FilePath string `json:"file_path"`
	MimeType string `json:"mime_type"`
}

// NewEnvelope marshals payload into an Envelope.
func NewEnvelope(eventType string, payload any) (Envelope, error) {
	env := Envelope{Type: eventType}

	if payload == nil {
		return env, nil
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshal payload: %w", err)
	}

	env.Payload = data
	return env, nil
}

// UnmarshalPayload unmarshals e.Payload into dst.
func (e Envelope) UnmarshalPayload(dst any) error {
	if len(e.Payload) == 0 {
		return nil
	}

	if err := json.Unmarshal(e.Payload, dst); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	return nil
}
