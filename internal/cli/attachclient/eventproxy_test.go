package attachclient

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"

	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// capturingClient wraps a Client and captures SendEvent calls.
type capturingClient struct {
	*Client
	events []struct {
		eventType string
		payload   any
	}
	mu sync.Mutex
}

func (c *capturingClient) SendEvent(eventType string, payload any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, struct {
		eventType string
		payload   any
	}{eventType, payload})
	// Don't call underlying (it's nil in test)
	return nil
}

// mockHandler tracks calls to inner handler.
type mockHandler struct {
	textDeltaCalls    int
	thinkingDeltaCall int
	toolUseStartCalls []tools.ToolUse
	toolUseEndCalls   []struct {
		toolUse tools.ToolUse
		result  *tools.Result
	}
	turnCompleteCall bool
	errorCall        bool
	retryCalls       [][]tools.ToolUse
	mu               sync.Mutex
}

func (m *mockHandler) OnTextDelta(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.textDeltaCalls++
}

func (m *mockHandler) OnThinkingDelta(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.thinkingDeltaCall++
}

func (m *mockHandler) OnToolUseStart(toolUse tools.ToolUse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolUseStartCalls = append(m.toolUseStartCalls, toolUse)
}

func (m *mockHandler) OnToolUseEnd(toolUse tools.ToolUse, result *tools.Result) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolUseEndCalls = append(m.toolUseEndCalls, struct {
		toolUse tools.ToolUse
		result  *tools.Result
	}{toolUse, result})
}

func (m *mockHandler) OnTurnComplete(usage api.Usage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.turnCompleteCall = true
}

func (m *mockHandler) OnError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorCall = true
}

func (m *mockHandler) OnRetry(toolUses []tools.ToolUse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retryCalls = append(m.retryCalls, toolUses)
}

func (m *mockHandler) OnToolApprovalNeeded(toolUse tools.ToolUse) bool {
	return true
}

func (m *mockHandler) OnCostConfirmNeeded(currentCost, threshold float64) bool {
	return true
}

// TestEventProxy_TextDelta_AccumulatesAndFlushes: multiple OnTextDelta → single AssistantMsg.
func TestEventProxy_TextDelta_AccumulatesAndFlushes(t *testing.T) {
	capture := &capturingClient{Client: nil}
	proxy := NewEventProxy(nil, capture, "")

	proxy.OnTextDelta("hello ")
	proxy.OnTextDelta("world")

	// No message sent yet
	if len(capture.events) != 0 {
		t.Errorf("expected 0 events before flush, got %d", len(capture.events))
	}

	// Flush on TurnComplete
	proxy.OnTurnComplete(api.Usage{})

	if len(capture.events) != 1 {
		t.Errorf("expected 1 event after flush, got %d", len(capture.events))
	}

	if capture.events[0].eventType != attach.EventMsgAssistant {
		t.Errorf("expected EventMsgAssistant, got %s", capture.events[0].eventType)
	}

	payload, ok := capture.events[0].payload.(attach.AssistantMsgPayload)
	if !ok {
		t.Fatalf("payload not AssistantMsgPayload: %T", capture.events[0].payload)
	}

	if payload.Content != "hello world" {
		t.Errorf("expected 'hello world', got %q", payload.Content)
	}
}

// TestEventProxy_OnToolUseStart_Forwards: OnToolUseStart → EventMsgToolUse.
func TestEventProxy_OnToolUseStart_Forwards(t *testing.T) {
	capture := &capturingClient{Client: nil}
	proxy := NewEventProxy(nil, capture, "")

	input := json.RawMessage(`{"param":"value"}`)
	tu := tools.ToolUse{
		ID:    "tool-123",
		Name:  "grep",
		Input: input,
	}

	proxy.OnToolUseStart(tu)

	if len(capture.events) != 1 {
		t.Errorf("expected 1 event, got %d", len(capture.events))
	}

	if capture.events[0].eventType != attach.EventMsgToolUse {
		t.Errorf("expected EventMsgToolUse, got %s", capture.events[0].eventType)
	}

	payload, ok := capture.events[0].payload.(attach.ToolUsePayload)
	if !ok {
		t.Fatalf("payload not ToolUsePayload: %T", capture.events[0].payload)
	}

	if payload.Tool != "grep" {
		t.Errorf("expected tool 'grep', got %q", payload.Tool)
	}

	if !bytes.Equal(payload.Input, input) {
		t.Errorf("input mismatch: %v vs %v", payload.Input, input)
	}
}

// TestEventProxy_NilClient_NoOp: nil client + nil inner → no panic.
func TestEventProxy_NilClient_NoOp(t *testing.T) {
	proxy := NewEventProxy(nil, nil, "")

	// Should not panic
	proxy.OnTextDelta("test")
	proxy.OnThinkingDelta("thinking")
	proxy.OnToolUseStart(tools.ToolUse{Name: "test"})
	proxy.OnToolUseEnd(tools.ToolUse{}, &tools.Result{})
	proxy.OnTurnComplete(api.Usage{})
	proxy.OnError(nil)
	proxy.OnRetry(nil)
	proxy.OnToolApprovalNeeded(tools.ToolUse{})
	proxy.OnCostConfirmNeeded(0, 0)
}

// TestEventProxy_DelegatesTo_Inner: verify inner handler called.
func TestEventProxy_DelegatesTo_Inner(t *testing.T) {
	inner := &mockHandler{}
	proxy := NewEventProxy(inner, nil, "")

	proxy.OnTextDelta("text")
	if inner.textDeltaCalls != 1 {
		t.Errorf("expected 1 OnTextDelta call, got %d", inner.textDeltaCalls)
	}

	proxy.OnThinkingDelta("thinking")
	if inner.thinkingDeltaCall != 1 {
		t.Errorf("expected 1 OnThinkingDelta call, got %d", inner.thinkingDeltaCall)
	}

	tu := tools.ToolUse{ID: "1", Name: "test"}
	proxy.OnToolUseStart(tu)
	if len(inner.toolUseStartCalls) != 1 {
		t.Errorf("expected 1 OnToolUseStart call, got %d", len(inner.toolUseStartCalls))
	}
	if inner.toolUseStartCalls[0].Name != "test" {
		t.Errorf("tool use name mismatch: %q", inner.toolUseStartCalls[0].Name)
	}

	result := &tools.Result{Content: "done"}
	proxy.OnToolUseEnd(tu, result)
	if len(inner.toolUseEndCalls) != 1 {
		t.Errorf("expected 1 OnToolUseEnd call, got %d", len(inner.toolUseEndCalls))
	}

	proxy.OnTurnComplete(api.Usage{})
	if !inner.turnCompleteCall {
		t.Error("expected OnTurnComplete called")
	}

	proxy.OnError(nil)
	if !inner.errorCall {
		t.Error("expected OnError called")
	}

	proxy.OnRetry([]tools.ToolUse{tu})
	if len(inner.retryCalls) != 1 {
		t.Errorf("expected 1 OnRetry call, got %d", len(inner.retryCalls))
	}

	if !proxy.OnToolApprovalNeeded(tu) {
		t.Error("expected OnToolApprovalNeeded returned true")
	}

	if !proxy.OnCostConfirmNeeded(10, 100) {
		t.Error("expected OnCostConfirmNeeded returned true")
	}
}

// TestEventProxy_TextDelta_FlushedOnError: text flushed when OnError called.
func TestEventProxy_TextDelta_FlushedOnError(t *testing.T) {
	capture := &capturingClient{Client: nil}
	proxy := NewEventProxy(nil, capture, "")

	proxy.OnTextDelta("error text")
	proxy.OnError(nil)

	if len(capture.events) != 1 {
		t.Errorf("expected 1 event on error, got %d", len(capture.events))
	}

	if capture.events[0].eventType != attach.EventMsgAssistant {
		t.Errorf("expected EventMsgAssistant, got %s", capture.events[0].eventType)
	}

	payload := capture.events[0].payload.(attach.AssistantMsgPayload)
	if payload.Content != "error text" {
		t.Errorf("expected 'error text', got %q", payload.Content)
	}
}
