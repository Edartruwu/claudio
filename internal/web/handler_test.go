package web

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/tools"
)

func TestNewWebHandler(t *testing.T) {
	h := NewWebHandler()
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.events == nil {
		t.Fatal("expected non-nil events channel")
	}
	if h.approvalCh == nil {
		t.Fatal("expected non-nil approval channel")
	}
	if h.done == nil {
		t.Fatal("expected non-nil done channel")
	}
	if h.PendingTool() != "" {
		t.Errorf("expected empty pending tool, got %q", h.PendingTool())
	}
}

func TestWebHandler_OnTextDelta(t *testing.T) {
	h := NewWebHandler()
	h.OnTextDelta("hello ")
	h.OnTextDelta("world")

	evt1 := readEvent(t, h, 100*time.Millisecond)
	if evt1.Event != "text" || evt1.Data != "hello " {
		t.Errorf("unexpected first event: %+v", evt1)
	}
	evt2 := readEvent(t, h, 100*time.Millisecond)
	if evt2.Event != "text" || evt2.Data != "world" {
		t.Errorf("unexpected second event: %+v", evt2)
	}
}

func TestWebHandler_OnThinkingDelta(t *testing.T) {
	h := NewWebHandler()
	h.OnThinkingDelta("hmm...")

	evt := readEvent(t, h, 100*time.Millisecond)
	if evt.Event != "thinking" || evt.Data != "hmm..." {
		t.Errorf("unexpected event: %+v", evt)
	}
}

func TestWebHandler_OnToolUseStartEnd(t *testing.T) {
	h := NewWebHandler()
	tu := tools.ToolUse{
		ID:    "tool_1",
		Name:  "Bash",
		Input: json.RawMessage(`{"command":"ls"}`),
	}

	h.OnToolUseStart(tu)
	evt := readEvent(t, h, 100*time.Millisecond)
	if evt.Event != "tool_start" {
		t.Fatalf("expected tool_start, got %s", evt.Event)
	}
	var startData map[string]interface{}
	if err := json.Unmarshal([]byte(evt.Data), &startData); err != nil {
		t.Fatalf("failed to parse tool_start data: %v", err)
	}
	if startData["id"] != "tool_1" {
		t.Errorf("expected id=tool_1, got %v", startData["id"])
	}
	if startData["name"] != "Bash" {
		t.Errorf("expected name=Bash, got %v", startData["name"])
	}

	result := &tools.Result{Content: "file1.txt\nfile2.txt", IsError: false}
	h.OnToolUseEnd(tu, result)
	evt = readEvent(t, h, 100*time.Millisecond)
	if evt.Event != "tool_end" {
		t.Fatalf("expected tool_end, got %s", evt.Event)
	}
	var endData map[string]interface{}
	if err := json.Unmarshal([]byte(evt.Data), &endData); err != nil {
		t.Fatalf("failed to parse tool_end data: %v", err)
	}
	if endData["content"] != "file1.txt\nfile2.txt" {
		t.Errorf("unexpected content: %v", endData["content"])
	}
	if endData["is_error"] != false {
		t.Errorf("expected is_error=false, got %v", endData["is_error"])
	}
}

func TestWebHandler_OnToolUseEnd_NilResult(t *testing.T) {
	h := NewWebHandler()
	tu := tools.ToolUse{ID: "tool_2", Name: "Read"}

	h.OnToolUseEnd(tu, nil)
	evt := readEvent(t, h, 100*time.Millisecond)
	var data map[string]interface{}
	json.Unmarshal([]byte(evt.Data), &data)
	if data["content"] != "" {
		t.Errorf("expected empty content for nil result, got %v", data["content"])
	}
}

func TestWebHandler_OnToolUseEnd_ErrorResult(t *testing.T) {
	h := NewWebHandler()
	tu := tools.ToolUse{ID: "tool_3", Name: "Bash"}
	result := &tools.Result{Content: "exit status 1", IsError: true}

	h.OnToolUseEnd(tu, result)
	evt := readEvent(t, h, 100*time.Millisecond)
	var data map[string]interface{}
	json.Unmarshal([]byte(evt.Data), &data)
	if data["is_error"] != true {
		t.Errorf("expected is_error=true, got %v", data["is_error"])
	}
}

func TestWebHandler_OnTurnComplete(t *testing.T) {
	h := NewWebHandler()
	usage := api.Usage{InputTokens: 100, OutputTokens: 50}

	h.OnTurnComplete(usage)

	evt := readEvent(t, h, 100*time.Millisecond)
	if evt.Event != "done" {
		t.Fatalf("expected done event, got %s", evt.Event)
	}
	var data map[string]interface{}
	json.Unmarshal([]byte(evt.Data), &data)
	if int(data["input_tokens"].(float64)) != 100 {
		t.Errorf("expected input_tokens=100, got %v", data["input_tokens"])
	}
	if int(data["output_tokens"].(float64)) != 50 {
		t.Errorf("expected output_tokens=50, got %v", data["output_tokens"])
	}

	// Done channel should be closed
	select {
	case <-h.Done():
	default:
		t.Error("expected Done channel to be closed after OnTurnComplete")
	}
}

func TestWebHandler_OnError(t *testing.T) {
	h := NewWebHandler()
	h.OnError(fmt.Errorf("something broke"))

	evt := readEvent(t, h, 100*time.Millisecond)
	if evt.Event != "error" || evt.Data != "something broke" {
		t.Errorf("unexpected error event: %+v", evt)
	}

	// Done channel should be closed
	select {
	case <-h.Done():
	default:
		t.Error("expected Done channel to be closed after OnError")
	}
}

func TestWebHandler_OnError_DoubleClose(t *testing.T) {
	h := NewWebHandler()
	// Calling OnError twice should not panic (done already closed)
	h.OnError(fmt.Errorf("first"))
	h.OnError(fmt.Errorf("second"))

	evt1 := readEvent(t, h, 100*time.Millisecond)
	if evt1.Data != "first" {
		t.Errorf("expected first error, got %q", evt1.Data)
	}
	evt2 := readEvent(t, h, 100*time.Millisecond)
	if evt2.Data != "second" {
		t.Errorf("expected second error, got %q", evt2.Data)
	}
}

func TestWebHandler_OnToolApprovalNeeded_Approve(t *testing.T) {
	h := NewWebHandler()
	tu := tools.ToolUse{
		ID:    "tool_a",
		Name:  "Bash",
		Input: json.RawMessage(`{"command":"rm -rf /"}`),
	}

	var approved bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		approved = h.OnToolApprovalNeeded(tu)
	}()

	// Wait for approval_needed event
	evt := readEvent(t, h, time.Second)
	if evt.Event != "approval_needed" {
		t.Fatalf("expected approval_needed, got %s", evt.Event)
	}
	var data map[string]interface{}
	json.Unmarshal([]byte(evt.Data), &data)
	if data["name"] != "Bash" {
		t.Errorf("expected name=Bash, got %v", data["name"])
	}

	// PendingTool should be set
	if pt := h.PendingTool(); pt != "Bash" {
		t.Errorf("expected PendingTool=Bash, got %q", pt)
	}

	// Approve the tool
	h.Approve(true)
	wg.Wait()

	if !approved {
		t.Error("expected approval to be true")
	}

	// PendingTool should be cleared
	if pt := h.PendingTool(); pt != "" {
		t.Errorf("expected PendingTool empty, got %q", pt)
	}

	// Should have approval_result event
	evt = readEvent(t, h, 100*time.Millisecond)
	if evt.Event != "approval_result" {
		t.Fatalf("expected approval_result, got %s", evt.Event)
	}
}

func TestWebHandler_OnToolApprovalNeeded_Deny(t *testing.T) {
	h := NewWebHandler()
	tu := tools.ToolUse{ID: "tool_d", Name: "Write", Input: json.RawMessage(`{}`)}

	var approved bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		approved = h.OnToolApprovalNeeded(tu)
	}()

	readEvent(t, h, time.Second) // approval_needed
	h.Approve(false)
	wg.Wait()

	if approved {
		t.Error("expected approval to be false")
	}
}

func TestWebHandler_OnCostConfirmNeeded(t *testing.T) {
	h := NewWebHandler()
	// Should always auto-approve
	if !h.OnCostConfirmNeeded(10.0, 5.0) {
		t.Error("expected OnCostConfirmNeeded to return true")
	}
}

func TestWebHandler_EventsChannelCapacity(t *testing.T) {
	h := NewWebHandler()
	// Fill the channel to capacity (512)
	for i := 0; i < 512; i++ {
		h.OnTextDelta("x")
	}
	// 513th should not block — it gets dropped
	done := make(chan struct{})
	go func() {
		h.OnTextDelta("overflow")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("OnTextDelta blocked when channel was full")
	}
}

func TestWebHandler_FullStreamSequence(t *testing.T) {
	// Simulates a full conversation turn: thinking → tool → text → done
	h := NewWebHandler()

	go func() {
		h.OnThinkingDelta("Let me think...")
		h.OnToolUseStart(tools.ToolUse{ID: "t1", Name: "Bash", Input: json.RawMessage(`{"cmd":"ls"}`)})
		h.OnToolUseEnd(tools.ToolUse{ID: "t1", Name: "Bash"}, &tools.Result{Content: "main.go"})
		h.OnTextDelta("Here are your files: ")
		h.OnTextDelta("main.go")
		h.OnTurnComplete(api.Usage{InputTokens: 200, OutputTokens: 100})
	}()

	events := drainEvents(t, h, 2*time.Second)

	expectedOrder := []string{"thinking", "tool_start", "tool_end", "text", "text", "done"}
	if len(events) != len(expectedOrder) {
		t.Fatalf("expected %d events, got %d: %v", len(expectedOrder), len(events), eventNames(events))
	}
	for i, expected := range expectedOrder {
		if events[i].Event != expected {
			t.Errorf("event[%d]: expected %s, got %s", i, expected, events[i].Event)
		}
	}
}

// --- helpers ---

// readEvent reads a single event from the handler with a timeout.
func readEvent(t *testing.T, h *WebHandler, timeout time.Duration) SSEEvent {
	t.Helper()
	select {
	case evt := <-h.Events():
		return evt
	case <-time.After(timeout):
		t.Fatal("timeout waiting for event")
		return SSEEvent{}
	}
}

// drainEvents reads all events until Done channel is closed.
func drainEvents(t *testing.T, h *WebHandler, timeout time.Duration) []SSEEvent {
	t.Helper()
	var events []SSEEvent
	timer := time.After(timeout)
	for {
		select {
		case evt := <-h.Events():
			events = append(events, evt)
			if evt.Event == "done" {
				return events
			}
		case <-h.Done():
			// Drain remaining events
			for {
				select {
				case evt := <-h.Events():
					events = append(events, evt)
				default:
					return events
				}
			}
		case <-timer:
			t.Fatalf("timeout draining events, got %d so far", len(events))
			return events
		}
	}
}

func eventNames(events []SSEEvent) []string {
	names := make([]string, len(events))
	for i, e := range events {
		names[i] = e.Event
	}
	return names
}
