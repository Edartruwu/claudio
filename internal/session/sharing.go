package session

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
)

// SharedSession is the portable format for sharing sessions across machines.
type SharedSession struct {
	Version    int               `json:"version"`
	ExportedAt time.Time         `json:"exported_at"`
	Model      string            `json:"model"`
	Summary    string            `json:"summary,omitempty"`
	Messages   []api.Message     `json:"messages"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// ExportSession creates a SharedSession from the current conversation state.
func ExportSession(messages []api.Message, model, summary string) *SharedSession {
	return &SharedSession{
		Version:    1,
		ExportedAt: time.Now(),
		Model:      model,
		Summary:    summary,
		Messages:   messages,
		Metadata:   map[string]string{},
	}
}

// MarshalSession serializes a shared session to JSON.
func MarshalSession(s *SharedSession) ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// ImportSession deserializes a shared session from JSON.
func ImportSession(data []byte) (*SharedSession, error) {
	var s SharedSession
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("invalid session file: %w", err)
	}
	if s.Version == 0 {
		return nil, fmt.Errorf("invalid session file: missing version")
	}
	if len(s.Messages) == 0 {
		return nil, fmt.Errorf("session file has no messages")
	}
	return &s, nil
}
