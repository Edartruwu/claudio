package web

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// SSEEvent represents a server-sent event to push to the browser.
type SSEEvent struct {
	Seq   int    `json:"seq"`
	Event string `json:"event"`
	Data  string `json:"data"`
}

const eventBufferSize = 2048

// WebHandler implements query.EventHandler and pushes events to an SSE channel.
// It also maintains a ring buffer of all events for replay after reconnection.
type WebHandler struct {
	events     chan SSEEvent
	approvalCh chan bool // receives true=approve, false=deny
	mu         sync.Mutex
	pendingTool string // tool ID awaiting approval
	done       chan struct{} // closed when turn completes
	closeOnce  sync.Once
	running    bool // true while engine.Run is active

	// Event buffer for replay on reconnect
	bufMu    sync.RWMutex
	buffer   []SSEEvent
	seqCount int
}

// NewWebHandler creates a new web event handler.
func NewWebHandler() *WebHandler {
	return &WebHandler{
		events:     make(chan SSEEvent, 512),
		approvalCh: make(chan bool, 1),
		done:       make(chan struct{}),
		buffer:     make([]SSEEvent, 0, eventBufferSize),
		running:    true,
	}
}

// Events returns the SSE event channel for reading by the HTTP handler.
func (h *WebHandler) Events() <-chan SSEEvent {
	return h.events
}

// Done returns a channel that's closed when the turn is complete.
func (h *WebHandler) Done() <-chan struct{} {
	return h.done
}

// IsRunning returns whether the handler is still processing.
func (h *WebHandler) IsRunning() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.running
}

// EventCount returns the total number of events emitted so far.
func (h *WebHandler) EventCount() int {
	h.bufMu.RLock()
	defer h.bufMu.RUnlock()
	return h.seqCount
}

// EventsSince returns all buffered events with Seq > since.
func (h *WebHandler) EventsSince(since int) []SSEEvent {
	h.bufMu.RLock()
	defer h.bufMu.RUnlock()
	var result []SSEEvent
	for _, evt := range h.buffer {
		if evt.Seq > since {
			result = append(result, evt)
		}
	}
	return result
}

// Approve sends an approval decision for the pending tool.
func (h *WebHandler) Approve(approved bool) {
	select {
	case h.approvalCh <- approved:
	default:
	}
}

// PendingTool returns the name of the tool awaiting approval, or empty string.
func (h *WebHandler) PendingTool() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pendingTool
}

func (h *WebHandler) send(event, data string) {
	h.bufMu.Lock()
	h.seqCount++
	evt := SSEEvent{Seq: h.seqCount, Event: event, Data: data}
	// Ring buffer: keep last eventBufferSize events
	if len(h.buffer) >= eventBufferSize {
		h.buffer = append(h.buffer[1:], evt)
	} else {
		h.buffer = append(h.buffer, evt)
	}
	h.bufMu.Unlock()

	select {
	case h.events <- evt:
	default:
		// Channel full, drop from live channel (still in buffer for replay)
	}
}

func (h *WebHandler) sendJSON(event string, v interface{}) {
	data, _ := json.Marshal(v)
	h.send(event, string(data))
}

func (h *WebHandler) markDone() {
	h.mu.Lock()
	h.running = false
	h.mu.Unlock()
	h.closeOnce.Do(func() { close(h.done) })
}

// --- query.EventHandler implementation ---

func (h *WebHandler) OnTextDelta(text string) {
	h.send("text", text)
}

func (h *WebHandler) OnThinkingDelta(text string) {
	h.send("thinking", text)
}

func (h *WebHandler) OnToolUseStart(toolUse tools.ToolUse) {
	h.sendJSON("tool_start", map[string]interface{}{
		"id":    toolUse.ID,
		"name":  toolUse.Name,
		"input": string(toolUse.Input),
	})
}

func (h *WebHandler) OnToolUseEnd(toolUse tools.ToolUse, result *tools.Result) {
	content := ""
	if result != nil {
		content = result.Content
	}
	isErr := false
	if result != nil {
		isErr = result.IsError
	}
	h.sendJSON("tool_end", map[string]interface{}{
		"id":       toolUse.ID,
		"name":     toolUse.Name,
		"content":  content,
		"is_error": isErr,
	})
}

func (h *WebHandler) OnTurnComplete(usage api.Usage) {
	h.sendJSON("done", map[string]interface{}{
		"input_tokens":  usage.InputTokens,
		"output_tokens": usage.OutputTokens,
	})
	h.markDone()
}

func (h *WebHandler) OnError(err error) {
	h.send("error", err.Error())
	h.markDone()
}

func (h *WebHandler) OnToolApprovalNeeded(toolUse tools.ToolUse) bool {
	h.mu.Lock()
	h.pendingTool = toolUse.Name
	h.mu.Unlock()

	// Send approval request to browser
	h.sendJSON("approval_needed", map[string]interface{}{
		"id":    toolUse.ID,
		"name":  toolUse.Name,
		"input": string(toolUse.Input),
	})

	// Block until user approves/denies
	approved := <-h.approvalCh

	h.mu.Lock()
	h.pendingTool = ""
	h.mu.Unlock()

	if approved {
		h.send("approval_result", fmt.Sprintf("Approved: %s", toolUse.Name))
	} else {
		h.send("approval_result", fmt.Sprintf("Denied: %s", toolUse.Name))
	}

	return approved
}

func (h *WebHandler) OnCostConfirmNeeded(currentCost, threshold float64) bool {
	// Auto-approve cost in web mode
	return true
}
