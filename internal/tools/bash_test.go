package tools

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Abraxas-365/claudio/internal/tasks"
)

// newTestRuntime creates a tasks.Runtime backed by a temp directory.
// The caller must defer os.RemoveAll on the returned dir.
func newTestRuntime(t *testing.T) (*tasks.Runtime, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "claudio-bash-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	return tasks.NewRuntime(dir), dir
}

// TestBashTool_NoPromote_FastCommand verifies that a command finishing within
// the foreground budget returns output synchronously with no background task.
func TestBashTool_NoPromote_FastCommand(t *testing.T) {
	rt, dir := newTestRuntime(t)
	defer os.RemoveAll(dir)

	tool := &BashTool{TaskRuntime: rt}

	input, _ := json.Marshal(bashInput{
		Command:             "echo hello",
		ForegroundTimeoutMs: 5_000, // 5 s budget — echo is instant
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}
	if !strings.Contains(result.Content, "hello") {
		t.Errorf("expected 'hello' in output, got: %q", result.Content)
	}
	if strings.Contains(result.Content, "Task ID:") {
		t.Errorf("fast command should NOT be promoted to background, got: %q", result.Content)
	}
}

// TestBashTool_AutoPromote_OnTimeout verifies that a slow command exceeding the
// foreground budget is promoted to background and returns a task-ID message
// immediately (without blocking until the command finishes).
func TestBashTool_AutoPromote_OnTimeout(t *testing.T) {
	rt, dir := newTestRuntime(t)
	defer os.RemoveAll(dir)

	tool := &BashTool{TaskRuntime: rt}

	input, _ := json.Marshal(bashInput{
		Command:             "sleep 60",
		ForegroundTimeoutMs: 100, // 100 ms budget — sleep 60 will exceed it
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("auto-promote should not set IsError, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Command is running in background. Task ID:") {
		t.Errorf("expected background task message, got: %q", result.Content)
	}
	if !strings.Contains(result.Content, "Result will be injected when complete.") {
		t.Errorf("expected inject message, got: %q", result.Content)
	}
}

func TestIsCatFileCommand_SedLineRange(t *testing.T) {
	cases := []string{
		"sed -n '10,20p' somefile.go",
		"sed -n '1,50p' /abs/path/to/file",
	}
	for _, cmd := range cases {
		if !isCatFileCommand(cmd) {
			t.Errorf("expected isCatFileCommand(%q) = true, got false", cmd)
		}
	}
}

func TestIsCatFileCommand_SedInPipeline(t *testing.T) {
	cases := []string{
		"cat file | sed -n '10,20p'",
		"grep foo file | sed -n '1,5p'",
	}
	for _, cmd := range cases {
		if isCatFileCommand(cmd) {
			t.Errorf("expected isCatFileCommand(%q) = false, got true", cmd)
		}
	}
}

// TestBashTool_RunInBackground_NilRuntime verifies that RunInBackground=true
// with a nil TaskRuntime silently falls through to synchronous execution
// (no panic, no "Task ID" in result).
func TestBashTool_RunInBackground_NilRuntime(t *testing.T) {
	tool := &BashTool{TaskRuntime: nil}

	input, _ := json.Marshal(bashInput{
		Command:         "echo nilruntime",
		RunInBackground: true,
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}
	if !strings.Contains(result.Content, "nilruntime") {
		t.Errorf("expected sync output 'nilruntime', got: %q", result.Content)
	}
	if strings.Contains(result.Content, "Task ID:") {
		t.Errorf("nil runtime should not spawn bg task, got: %q", result.Content)
	}
}

// TestBashTool_CommandExitsNonZero verifies that a command exiting non-zero
// returns an error result containing "Exit code".
func TestBashTool_CommandExitsNonZero(t *testing.T) {
	tool := &BashTool{TaskRuntime: nil}

	input, _ := json.Marshal(bashInput{
		Command:             "false",
		ForegroundTimeoutMs: 5_000,
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for non-zero exit, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Exit code:") {
		t.Errorf("expected 'Exit code:' in result, got: %q", result.Content)
	}
}

// TestBashTool_EmptyCommand verifies that an empty command returns an error
// result without panicking.
func TestBashTool_EmptyCommand(t *testing.T) {
	tool := &BashTool{TaskRuntime: nil}

	input, _ := json.Marshal(bashInput{Command: ""})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for empty command, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "No command provided") {
		t.Errorf("expected 'No command provided', got: %q", result.Content)
	}
}

// TestBashTool_InvalidJSON verifies that malformed JSON input returns an error
// result without panicking.
func TestBashTool_InvalidJSON(t *testing.T) {
	tool := &BashTool{TaskRuntime: nil}

	result, err := tool.Execute(context.Background(), []byte("{bad json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for invalid JSON, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Invalid input:") {
		t.Errorf("expected 'Invalid input:', got: %q", result.Content)
	}
}

// TestBashTool_LargeOutputTruncated verifies that output exceeding 30KB is
// truncated with an explicit marker.
func TestBashTool_LargeOutputTruncated(t *testing.T) {
	rt, dir := newTestRuntime(t)
	defer os.RemoveAll(dir)

	tool := &BashTool{TaskRuntime: rt}

	// yes produces 40001 bytes of 'y\n' — well over the 30KB cap.
	input, _ := json.Marshal(bashInput{
		Command:             "yes a | head -c 40001",
		ForegroundTimeoutMs: 10_000,
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}
	if !strings.Contains(result.Content, "[Bash output truncated") {
		t.Errorf("expected truncation marker, got: %q", result.Content[:min(200, len(result.Content))])
	}
}

// TestBashTool_ForegroundTimeoutMs_Zero_UsesDefault verifies that
// ForegroundTimeoutMs=0 applies the 30s default — fast commands still complete
// synchronously with no background promotion.
func TestBashTool_ForegroundTimeoutMs_Zero_UsesDefault(t *testing.T) {
	rt, dir := newTestRuntime(t)
	defer os.RemoveAll(dir)

	tool := &BashTool{TaskRuntime: rt}

	input, _ := json.Marshal(bashInput{
		Command:             "echo defaultbudget",
		ForegroundTimeoutMs: 0, // triggers defaultFgBudgetMs = 30_000
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}
	if !strings.Contains(result.Content, "defaultbudget") {
		t.Errorf("expected sync output, got: %q", result.Content)
	}
	if strings.Contains(result.Content, "Task ID:") {
		t.Errorf("fast cmd with default budget should not promote, got: %q", result.Content)
	}
}

// TestBashTool_AutoPromote_Disabled_FastCmd verifies that a very high
// ForegroundTimeoutMs disables auto-promote (fgBudget >= overallTimeout)
// so fast commands return synchronously.
func TestBashTool_AutoPromote_Disabled_FastCmd(t *testing.T) {
	rt, dir := newTestRuntime(t)
	defer os.RemoveAll(dir)

	tool := &BashTool{TaskRuntime: rt}

	// fgBudget = 999_000_000 ms >> overallTimeout (120s) → autoPromote=false
	input, _ := json.Marshal(bashInput{
		Command:             "echo nopromote",
		ForegroundTimeoutMs: 999_000_000,
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}
	if !strings.Contains(result.Content, "nopromote") {
		t.Errorf("expected sync output 'nopromote', got: %q", result.Content)
	}
	if strings.Contains(result.Content, "Task ID:") {
		t.Errorf("auto-promote should be disabled, got: %q", result.Content)
	}
}

// TestBashTool_NoAutoPromote_SlowCmd_Timeout verifies that with auto-promote
// disabled (fgBudget >= overallTimeout), a slow command that exceeds the overall
// timeout returns a timeout error — not a background task ID.
func TestBashTool_NoAutoPromote_SlowCmd_Timeout(t *testing.T) {
	rt, dir := newTestRuntime(t)
	defer os.RemoveAll(dir)

	tool := &BashTool{TaskRuntime: rt}

	// fgBudget >> overallTimeout → autoPromote=false; overall timeout fires instead.
	input, _ := json.Marshal(bashInput{
		Command:             "sleep 5",
		Timeout:             300,         // 300ms overall
		ForegroundTimeoutMs: 999_000_000, // disabled
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected timeout error, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "Task ID:") {
		t.Errorf("should NOT promote to background, got: %q", result.Content)
	}
	if !strings.Contains(result.Content, "timed out") {
		t.Errorf("expected 'timed out' in result, got: %q", result.Content)
	}
}

// min is a helper for older Go versions that lack the built-in.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
