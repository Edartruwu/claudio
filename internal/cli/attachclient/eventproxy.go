package attachclient

import (
	"strings"
	"sync"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/query"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// ClientSender sends events to ComandCenter.
type ClientSender interface {
	SendEvent(eventType string, payload any) error
}

// EventProxy wraps a query.EventHandler and forwards events to a ComandCenter Client.
// If client is nil, behaves exactly like the wrapped handler (zero overhead).
type EventProxy struct {
	inner  query.EventHandler
	client ClientSender
	mu     sync.Mutex
	buf    strings.Builder
}

// NewEventProxy creates EventProxy wrapping inner handler and optionally forwarding to client.
func NewEventProxy(inner query.EventHandler, client ClientSender) *EventProxy {
	return &EventProxy{
		inner:  inner,
		client: client,
	}
}

// OnTextDelta accumulates text delta, calls inner if not nil.
func (e *EventProxy) OnTextDelta(text string) {
	e.mu.Lock()
	e.buf.WriteString(text)
	e.mu.Unlock()

	if e.inner != nil {
		e.inner.OnTextDelta(text)
	}
}

// OnThinkingDelta forwards to inner only (not buffered for ComandCenter).
func (e *EventProxy) OnThinkingDelta(text string) {
	if e.inner != nil {
		e.inner.OnThinkingDelta(text)
	}
}

// OnToolUseStart forwards tool use event to client, calls inner if not nil.
func (e *EventProxy) OnToolUseStart(toolUse tools.ToolUse) {
	// Flush any accumulated text first
	e.flushBuffer()

	if e.client != nil {
		payload := attach.ToolUsePayload{
			Tool:  toolUse.Name,
			Input: toolUse.Input,
		}
		_ = e.client.SendEvent(attach.EventMsgToolUse, payload)
	}

	if e.inner != nil {
		e.inner.OnToolUseStart(toolUse)
	}
}

// OnToolUseEnd delegates to inner only.
func (e *EventProxy) OnToolUseEnd(toolUse tools.ToolUse, result *tools.Result) {
	if e.inner != nil {
		e.inner.OnToolUseEnd(toolUse, result)
	}
}

// OnTurnComplete flushes accumulated text, calls inner if not nil.
func (e *EventProxy) OnTurnComplete(usage api.Usage) {
	e.flushBuffer()

	if e.inner != nil {
		e.inner.OnTurnComplete(usage)
	}
}

// OnError flushes accumulated text, calls inner if not nil.
func (e *EventProxy) OnError(err error) {
	e.flushBuffer()

	if e.inner != nil {
		e.inner.OnError(err)
	}
}

// OnRetry delegates to inner only.
func (e *EventProxy) OnRetry(toolUses []tools.ToolUse) {
	if e.inner != nil {
		e.inner.OnRetry(toolUses)
	}
}

// OnToolApprovalNeeded delegates to inner, returns false if inner is nil.
func (e *EventProxy) OnToolApprovalNeeded(toolUse tools.ToolUse) bool {
	if e.inner != nil {
		return e.inner.OnToolApprovalNeeded(toolUse)
	}
	return false
}

// OnCostConfirmNeeded delegates to inner, returns true if inner is nil.
func (e *EventProxy) OnCostConfirmNeeded(currentCost, threshold float64) bool {
	if e.inner != nil {
		return e.inner.OnCostConfirmNeeded(currentCost, threshold)
	}
	return true
}

// flushBuffer sends accumulated text as AssistantMsgPayload to client, resets buffer.
func (e *EventProxy) flushBuffer() {
	e.mu.Lock()
	defer e.mu.Unlock()

	content := e.buf.String()
	e.buf.Reset()

	if content == "" || e.client == nil {
		return
	}

	payload := attach.AssistantMsgPayload{
		Content: content,
	}
	_ = e.client.SendEvent(attach.EventMsgAssistant, payload)
}
