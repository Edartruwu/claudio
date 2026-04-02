package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Abraxas-365/claudio/internal/query"
)

// SessionRuntime holds the execution state for a single session.
// When the session is backgrounded (user switches to another session),
// a drain goroutine consumes engine events and accumulates messages.
type SessionRuntime struct {
	mu sync.Mutex

	SessionID string

	// Engine state
	Engine     *query.Engine
	CancelFunc context.CancelFunc
	EventCh    chan tuiEvent
	ApprovalCh chan bool

	// Conversation state
	Messages       []ChatMessage
	StreamText     strings.Builder
	Streaming      bool
	TotalTokens    int
	TotalCost      float64
	Turns          int
	ExpandedGroups map[int]bool
	LastToolGroup  int
	SpinText       string
	MessageQueue   []string

	// Background drain
	draining   bool
	drainStop  chan struct{}
}

// NewSessionRuntime creates a runtime for a session.
func NewSessionRuntime(sessionID string) *SessionRuntime {
	return &SessionRuntime{
		SessionID:      sessionID,
		EventCh:        make(chan tuiEvent, 64),
		ExpandedGroups: make(map[int]bool),
		LastToolGroup:  -1,
	}
}

// StartBackgroundDrain begins consuming events from EventCh in the background.
// Called when the user switches away from this session while it's still streaming.
func (sr *SessionRuntime) StartBackgroundDrain() {
	sr.mu.Lock()
	if sr.draining || !sr.Streaming {
		sr.mu.Unlock()
		return
	}
	sr.draining = true
	sr.drainStop = make(chan struct{})
	sr.mu.Unlock()

	go sr.drainLoop()
}

// StopBackgroundDrain stops the drain goroutine (called when foregrounding).
func (sr *SessionRuntime) StopBackgroundDrain() {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if !sr.draining {
		return
	}
	sr.draining = false
	close(sr.drainStop)
}

func (sr *SessionRuntime) drainLoop() {
	for {
		select {
		case <-sr.drainStop:
			return
		case event, ok := <-sr.EventCh:
			if !ok {
				return
			}
			sr.mu.Lock()
			sr.processEvent(event)
			sr.mu.Unlock()
		}
	}
}

// processEvent handles an engine event while backgrounded, accumulating state.
func (sr *SessionRuntime) processEvent(event tuiEvent) {
	switch event.typ {
	case "text_delta":
		sr.StreamText.WriteString(event.text)
		sr.SpinText = "Responding..."
		sr.updateStreamingMessage()

	case "thinking_delta":
		sr.SpinText = "Thinking deeply..."

	case "tool_start":
		sr.finalizeStreamingMessage()
		sr.SpinText = fmt.Sprintf("Using %s...", event.toolUse.Name)

		msgIdx := len(sr.Messages)
		prevType := MsgUser
		if msgIdx > 0 {
			prevType = sr.Messages[msgIdx-1].Type
		}
		if prevType != MsgToolUse && prevType != MsgToolResult {
			sr.LastToolGroup = msgIdx
		}

		sr.Messages = append(sr.Messages, ChatMessage{
			Type:         MsgToolUse,
			ToolName:     event.toolUse.Name,
			ToolInput:    formatToolSummary(event.toolUse),
			ToolInputRaw: event.toolUse.Input,
		})

	case "tool_end":
		if event.toolUse.Input != nil {
			for i := len(sr.Messages) - 1; i >= 0; i-- {
				if sr.Messages[i].Type == MsgToolUse && sr.Messages[i].ToolName == event.toolUse.Name && sr.Messages[i].ToolInputRaw == nil {
					sr.Messages[i].ToolInputRaw = event.toolUse.Input
					sr.Messages[i].ToolInput = formatToolSummary(event.toolUse)
					break
				}
			}
		}
		if event.result != nil {
			sr.Messages = append(sr.Messages, ChatMessage{
				Type:    MsgToolResult,
				Content: event.result.Content,
				IsError: event.result.IsError,
			})
		}

	case "approval_needed":
		// Auto-approve tools when backgrounded (can't show dialog)
		sr.finalizeStreamingMessage()
		if sr.ApprovalCh != nil {
			sr.ApprovalCh <- true
		}

	case "turn_complete":
		sr.finalizeStreamingMessage()
		sr.TotalTokens += event.usage.OutputTokens
		sr.TotalCost += float64(event.usage.InputTokens) * 3.0 / 1_000_000
		sr.TotalCost += float64(event.usage.OutputTokens) * 15.0 / 1_000_000
		sr.Turns++

	case "done":
		sr.finalizeStreamingMessage()
		sr.Streaming = false
		sr.SpinText = ""
		if event.err != nil && event.err.Error() != "context canceled" {
			sr.Messages = append(sr.Messages, ChatMessage{Type: MsgError, Content: event.err.Error()})
		}

	case "error":
		sr.Messages = append(sr.Messages, ChatMessage{Type: MsgError, Content: event.err.Error()})
	}
}

func (sr *SessionRuntime) updateStreamingMessage() {
	text := sr.StreamText.String()
	if text == "" {
		return
	}
	if len(sr.Messages) > 0 && sr.Messages[len(sr.Messages)-1].Type == MsgAssistant {
		sr.Messages[len(sr.Messages)-1].Content = text
	} else {
		sr.Messages = append(sr.Messages, ChatMessage{Type: MsgAssistant, Content: text})
	}
}

func (sr *SessionRuntime) finalizeStreamingMessage() {
	if sr.StreamText.Len() > 0 {
		sr.updateStreamingMessage()
		sr.StreamText.Reset()
	}
}

// formatToolSummary is defined in root.go (shared)
