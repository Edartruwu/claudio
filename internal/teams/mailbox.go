package teams

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// Message represents an inter-agent message.
type Message struct {
	From      string    `json:"from"`      // sender agent name
	To        string    `json:"to"`        // recipient agent name or "*"
	Text      string    `json:"text"`      // plain text or JSON for structured messages
	Timestamp time.Time `json:"timestamp"`
	Read      bool      `json:"read"`
	Color     string    `json:"color,omitempty"`  // sender's color
	Summary   string    `json:"summary,omitempty"` // 5-10 word preview
}

// StructuredMessage is the JSON payload for control messages.
type StructuredMessage struct {
	Type      string `json:"type"`                 // "shutdown_request", "shutdown_response", "plan_approval_response", "permission_request"
	RequestID string `json:"request_id,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Approve   *bool  `json:"approve,omitempty"`
}

// Mailbox handles reading and writing messages for a teammate.
type Mailbox struct {
	inboxDir string
}

// NewMailbox creates a mailbox for the given team.
func NewMailbox(teamsDir, teamName string) *Mailbox {
	dir := filepath.Join(teamsDir, teamName, "inboxes")
	os.MkdirAll(dir, 0700)
	return &Mailbox{inboxDir: dir}
}

// Send writes a message to a recipient's inbox (file-locked).
func (mb *Mailbox) Send(from, to string, msg Message) error {
	msg.From = from
	msg.To = to
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	msg.Read = false

	if msg.Summary == "" {
		msg.Summary = truncateForSummary(msg.Text, 50)
	}

	return mb.appendToInbox(to, msg)
}

// Broadcast sends a message to all inboxes in the team (except sender).
func (mb *Mailbox) Broadcast(from string, msg Message) error {
	entries, err := os.ReadDir(mb.inboxDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		recipient := entry.Name()[:len(entry.Name())-5] // strip .json
		if recipient == from {
			continue // don't send to self
		}
		if err := mb.Send(from, recipient, msg); err != nil {
			// Log but continue
			continue
		}
	}
	return nil
}

// ReadAll reads all messages from an agent's inbox.
func (mb *Mailbox) ReadAll(agentName string) ([]Message, error) {
	path := mb.inboxPath(agentName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var messages []Message
	json.Unmarshal(data, &messages)
	return messages, nil
}

// ReadUnread reads only unread messages and marks them as read.
func (mb *Mailbox) ReadUnread(agentName string) ([]Message, error) {
	lock := flock.New(mb.inboxPath(agentName) + ".lock")
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	defer lock.Unlock()

	messages, err := mb.ReadAll(agentName)
	if err != nil {
		return nil, err
	}

	var unread []Message
	changed := false
	for i, msg := range messages {
		if !msg.Read {
			unread = append(unread, msg)
			messages[i].Read = true
			changed = true
		}
	}

	if changed {
		data, _ := json.MarshalIndent(messages, "", "  ")
		os.WriteFile(mb.inboxPath(agentName), data, 0600)
	}

	return unread, nil
}

// ClearInbox empties an agent's inbox.
func (mb *Mailbox) ClearInbox(agentName string) error {
	return os.Remove(mb.inboxPath(agentName))
}

// UnreadCount returns the number of unread messages.
func (mb *Mailbox) UnreadCount(agentName string) int {
	messages, err := mb.ReadAll(agentName)
	if err != nil {
		return 0
	}
	count := 0
	for _, msg := range messages {
		if !msg.Read {
			count++
		}
	}
	return count
}

// SendStructured sends a structured control message.
func (mb *Mailbox) SendStructured(from, to string, structured StructuredMessage) error {
	payload, err := json.Marshal(structured)
	if err != nil {
		return err
	}
	return mb.Send(from, to, Message{
		Text:    string(payload),
		Summary: fmt.Sprintf("[%s]", structured.Type),
	})
}

// ParseStructured attempts to parse a message as a structured message.
// Returns nil if the message is plain text.
func ParseStructured(msg Message) *StructuredMessage {
	var s StructuredMessage
	if err := json.Unmarshal([]byte(msg.Text), &s); err != nil {
		return nil
	}
	if s.Type == "" {
		return nil
	}
	return &s
}

func (mb *Mailbox) appendToInbox(agentName string, msg Message) error {
	path := mb.inboxPath(agentName)

	lock := flock.New(path + ".lock")
	if err := lock.Lock(); err != nil {
		// Best effort without lock
		return mb.appendToInboxNoLock(path, msg)
	}
	defer lock.Unlock()

	return mb.appendToInboxNoLock(path, msg)
}

func (mb *Mailbox) appendToInboxNoLock(path string, msg Message) error {
	var messages []Message
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &messages)
	}
	messages = append(messages, msg)

	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (mb *Mailbox) inboxPath(agentName string) string {
	return filepath.Join(mb.inboxDir, agentName+".json")
}

func truncateForSummary(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	// Take first line only
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

