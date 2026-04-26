package attach

import (
	"encoding/json"
	"testing"
)

func TestNewEnvelope_HelloPayload(t *testing.T) {
	payload := HelloPayload{
		Name:   "session-1",
		Path:   "/tmp/project",
		Model:  "claude-opus",
		Master: true,
	}

	env, err := NewEnvelope(EventSessionHello, payload)
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}

	if env.Type != EventSessionHello {
		t.Errorf("Type = %s, want %s", env.Type, EventSessionHello)
	}

	jsonData, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var unmarshaled Envelope
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	var got HelloPayload
	if err := unmarshaled.UnmarshalPayload(&got); err != nil {
		t.Fatalf("UnmarshalPayload failed: %v", err)
	}

	if got.Name != payload.Name || got.Path != payload.Path || got.Model != payload.Model || got.Master != payload.Master {
		t.Errorf("Payload mismatch: got %+v, want %+v", got, payload)
	}
}

func TestNewEnvelope_AssistantMsgPayload(t *testing.T) {
	payload := AssistantMsgPayload{
		Content:   "This is a message",
		AgentName: "agent-1",
	}

	env, err := NewEnvelope(EventMsgAssistant, payload)
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}

	if env.Type != EventMsgAssistant {
		t.Errorf("Type = %s, want %s", env.Type, EventMsgAssistant)
	}

	jsonData, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var unmarshaled Envelope
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	var got AssistantMsgPayload
	if err := unmarshaled.UnmarshalPayload(&got); err != nil {
		t.Fatalf("UnmarshalPayload failed: %v", err)
	}

	if got.Content != payload.Content || got.AgentName != payload.AgentName {
		t.Errorf("Payload mismatch: got %+v, want %+v", got, payload)
	}
}

func TestNewEnvelope_ToolUsePayload(t *testing.T) {
	input := json.RawMessage(`{"key":"value"}`)
	payload := ToolUsePayload{
		Tool:      "bash",
		Input:     input,
		AgentName: "agent-2",
	}

	env, err := NewEnvelope(EventMsgToolUse, payload)
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}

	if env.Type != EventMsgToolUse {
		t.Errorf("Type = %s, want %s", env.Type, EventMsgToolUse)
	}

	jsonData, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var unmarshaled Envelope
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	var got ToolUsePayload
	if err := unmarshaled.UnmarshalPayload(&got); err != nil {
		t.Fatalf("UnmarshalPayload failed: %v", err)
	}

	if got.Tool != payload.Tool || string(got.Input) != string(payload.Input) || got.AgentName != payload.AgentName {
		t.Errorf("Payload mismatch: got %+v, want %+v", got, payload)
	}
}

func TestNewEnvelope_TaskCreatedPayload(t *testing.T) {
	payload := TaskCreatedPayload{
		ID:         "task-123",
		Subject:    "Fix bug",
		AssignedTo: "alice",
		Status:     "in_progress",
	}

	env, err := NewEnvelope(EventTaskCreated, payload)
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}

	if env.Type != EventTaskCreated {
		t.Errorf("Type = %s, want %s", env.Type, EventTaskCreated)
	}

	jsonData, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var unmarshaled Envelope
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	var got TaskCreatedPayload
	if err := unmarshaled.UnmarshalPayload(&got); err != nil {
		t.Fatalf("UnmarshalPayload failed: %v", err)
	}

	if got.ID != payload.ID || got.Subject != payload.Subject || got.AssignedTo != payload.AssignedTo || got.Status != payload.Status {
		t.Errorf("Payload mismatch: got %+v, want %+v", got, payload)
	}
}

func TestNewEnvelope_TaskUpdatedPayload(t *testing.T) {
	payload := TaskUpdatedPayload{
		ID:     "task-123",
		Status: "done",
		Output: "Fixed successfully",
	}

	env, err := NewEnvelope(EventTaskUpdated, payload)
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}

	if env.Type != EventTaskUpdated {
		t.Errorf("Type = %s, want %s", env.Type, EventTaskUpdated)
	}

	jsonData, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var unmarshaled Envelope
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	var got TaskUpdatedPayload
	if err := unmarshaled.UnmarshalPayload(&got); err != nil {
		t.Fatalf("UnmarshalPayload failed: %v", err)
	}

	if got.ID != payload.ID || got.Status != payload.Status || got.Output != payload.Output {
		t.Errorf("Payload mismatch: got %+v, want %+v", got, payload)
	}
}

func TestNewEnvelope_AgentStatusPayload(t *testing.T) {
	payload := AgentStatusPayload{
		Name:        "researcher-1",
		Status:      "working",
		CurrentTask: "task-456",
	}

	env, err := NewEnvelope(EventAgentStatus, payload)
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}

	if env.Type != EventAgentStatus {
		t.Errorf("Type = %s, want %s", env.Type, EventAgentStatus)
	}

	jsonData, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var unmarshaled Envelope
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	var got AgentStatusPayload
	if err := unmarshaled.UnmarshalPayload(&got); err != nil {
		t.Fatalf("UnmarshalPayload failed: %v", err)
	}

	if got.Name != payload.Name || got.Status != payload.Status || got.CurrentTask != payload.CurrentTask {
		t.Errorf("Payload mismatch: got %+v, want %+v", got, payload)
	}
}

func TestNewEnvelope_UserMsgPayload(t *testing.T) {
	payload := UserMsgPayload{
		Content: "Hello from user",
		Attachments: []Attachment{
			{FilePath: "/tmp/file1.txt", MimeType: "text/plain"},
			{FilePath: "/tmp/file2.json", MimeType: "application/json"},
		},
		FromSession: "session-1",
	}

	env, err := NewEnvelope(EventMsgUser, payload)
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}

	if env.Type != EventMsgUser {
		t.Errorf("Type = %s, want %s", env.Type, EventMsgUser)
	}

	jsonData, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var unmarshaled Envelope
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	var got UserMsgPayload
	if err := unmarshaled.UnmarshalPayload(&got); err != nil {
		t.Fatalf("UnmarshalPayload failed: %v", err)
	}

	if got.Content != payload.Content || got.FromSession != payload.FromSession {
		t.Errorf("Payload mismatch: got %+v, want %+v", got, payload)
	}

	if len(got.Attachments) != len(payload.Attachments) {
		t.Errorf("Attachments count: got %d, want %d", len(got.Attachments), len(payload.Attachments))
	}

	for i, att := range got.Attachments {
		if att.FilePath != payload.Attachments[i].FilePath || att.MimeType != payload.Attachments[i].MimeType {
			t.Errorf("Attachment %d mismatch: got %+v, want %+v", i, att, payload.Attachments[i])
		}
	}
}

func TestNewEnvelope_EmptyPayload(t *testing.T) {
	env, err := NewEnvelope(EventSessionBye, nil)
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}

	if env.Type != EventSessionBye {
		t.Errorf("Type = %s, want %s", env.Type, EventSessionBye)
	}

	if len(env.Payload) != 0 {
		t.Errorf("Payload should be empty, got %s", string(env.Payload))
	}

	jsonData, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var unmarshaled Envelope
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if len(unmarshaled.Payload) != 0 {
		t.Errorf("Unmarshaled Payload should be empty, got %s", string(unmarshaled.Payload))
	}
}

func TestEnvelope_UnmarshalPayload_HelloPayload(t *testing.T) {
	payload := HelloPayload{
		Name:   "session-2",
		Path:   "/home/user/project",
		Model:  "claude-sonnet",
		Master: false,
	}

	env, err := NewEnvelope(EventSessionHello, payload)
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}

	var got HelloPayload
	if err := env.UnmarshalPayload(&got); err != nil {
		t.Fatalf("UnmarshalPayload failed: %v", err)
	}

	if got.Name != payload.Name || got.Path != payload.Path || got.Model != payload.Model || got.Master != payload.Master {
		t.Errorf("Payload mismatch: got %+v, want %+v", got, payload)
	}
}

func TestNewEnvelope_ClearHistoryPayload(t *testing.T) {
	payload := ClearHistoryPayload{SessionID: "sess-abc"}

	env, err := NewEnvelope(EventClearHistory, payload)
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}
	if env.Type != EventClearHistory {
		t.Errorf("Type = %s, want %s", env.Type, EventClearHistory)
	}

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var unmarshaled Envelope
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	var got ClearHistoryPayload
	if err := unmarshaled.UnmarshalPayload(&got); err != nil {
		t.Fatalf("UnmarshalPayload failed: %v", err)
	}
	if got.SessionID != payload.SessionID {
		t.Errorf("SessionID = %q, want %q", got.SessionID, payload.SessionID)
	}
}

func TestNewEnvelope_ConfigChangedPayload(t *testing.T) {
	payload := ConfigChangedPayload{SessionID: "sess-abc", Model: "claude-opus-4-6"}

	env, err := NewEnvelope(EventConfigChanged, payload)
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}
	if env.Type != EventConfigChanged {
		t.Errorf("Type = %s, want %s", env.Type, EventConfigChanged)
	}

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var unmarshaled Envelope
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	var got ConfigChangedPayload
	if err := unmarshaled.UnmarshalPayload(&got); err != nil {
		t.Fatalf("UnmarshalPayload failed: %v", err)
	}
	if got.SessionID != payload.SessionID || got.Model != payload.Model {
		t.Errorf("Payload mismatch: got %+v, want %+v", got, payload)
	}
}

func TestNewEnvelope_AgentChangedPayload(t *testing.T) {
	payload := AgentChangedPayload{SessionID: "sess-abc", AgentType: "prab"}

	env, err := NewEnvelope(EventAgentChanged, payload)
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}
	if env.Type != EventAgentChanged {
		t.Errorf("Type = %s, want %s", env.Type, EventAgentChanged)
	}

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var unmarshaled Envelope
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	var got AgentChangedPayload
	if err := unmarshaled.UnmarshalPayload(&got); err != nil {
		t.Fatalf("UnmarshalPayload failed: %v", err)
	}
	if got.SessionID != payload.SessionID || got.AgentType != payload.AgentType {
		t.Errorf("Payload mismatch: got %+v, want %+v", got, payload)
	}
}

func TestNewEnvelope_TeamChangedPayload(t *testing.T) {
	payload := TeamChangedPayload{SessionID: "sess-abc", TeamTemplate: "backend-team"}

	env, err := NewEnvelope(EventTeamChanged, payload)
	if err != nil {
		t.Fatalf("NewEnvelope failed: %v", err)
	}
	if env.Type != EventTeamChanged {
		t.Errorf("Type = %s, want %s", env.Type, EventTeamChanged)
	}

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var unmarshaled Envelope
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	var got TeamChangedPayload
	if err := unmarshaled.UnmarshalPayload(&got); err != nil {
		t.Fatalf("UnmarshalPayload failed: %v", err)
	}
	if got.SessionID != payload.SessionID || got.TeamTemplate != payload.TeamTemplate {
		t.Errorf("Payload mismatch: got %+v, want %+v", got, payload)
	}
}

// TestAgentStatusPayload_ResultField verifies that the Result string field
// on AgentStatusPayload survives a JSON marshal → unmarshal roundtrip.
func TestAgentStatusPayload_ResultField(t *testing.T) {
	want := AgentStatusPayload{
		Name:   "alex",
		Status: "done",
		Result: "merged cleanly into main",
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if !json.Valid(data) {
		t.Fatal("marshaled data is not valid JSON")
	}

	var got AgentStatusPayload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got.Result != want.Result {
		t.Errorf("Result = %q, want %q", got.Result, want.Result)
	}
	if got.Name != want.Name {
		t.Errorf("Name = %q, want %q", got.Name, want.Name)
	}
	if got.Status != want.Status {
		t.Errorf("Status = %q, want %q", got.Status, want.Status)
	}
}
