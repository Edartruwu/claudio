package comandcenter

import (
	"crypto/rand"
	"fmt"
	"time"
)

// Session represents a connected Claudio session.
type Session struct {
	ID           string
	Name         string
	Path         string
	Model        string
	Master       bool
	Status       string // active|inactive
	CreatedAt    time.Time
	LastActiveAt time.Time
}

// Message is a stored conversation message for a session.
type Message struct {
	ID              string
	SessionID       string
	Role            string // assistant|user|tool_use
	Content         string
	AgentName       string
	CreatedAt       time.Time
	ReplyToSession  string // name of target session for @mention routing (empty if none)
	QuotedContent   string // first 80 chars of content stored as reply-quote metadata
}

// Task is a tracked task associated with a session.
type Task struct {
	ID         string
	SessionID  string
	Title      string
	Status     string
	AssignedTo string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Agent tracks an agent within a session.
type Agent struct {
	ID            string
	SessionID     string
	Name          string
	Status        string
	CurrentTaskID string
	UpdatedAt     time.Time
}

// Attachment is a file uploaded by the user and linked to a session (optionally a message).
type Attachment struct {
	ID           string
	SessionID    string
	MessageID    string // empty if not linked to a specific message
	Filename     string // stored filename on disk (unique, e.g. "<id>.<ext>")
	OriginalName string // original filename from the upload
	MimeType     string
	Size         int64
	CreatedAt    time.Time
}

// NewID generates a random hex ID (16 bytes → 32 hex chars).
func NewID() string {
	return newID()
}

// newID generates a random hex ID (16 bytes → 32 hex chars).
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("comandcenter: crypto/rand failed: %v", err))
	}
	return fmt.Sprintf("%x", b)
}
