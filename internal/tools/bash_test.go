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

// TestBashTool_FastCommand verifies that a command finishing within the overall
// timeout returns output synchronously with no background task.
func TestBashTool_FastCommand(t *testing.T) {
	rt, dir := newTestRuntime(t)
	defer os.RemoveAll(dir)

	tool := &BashTool{TaskRuntime: rt}
	input, _ := json.Marshal(bashInput{Command: "echo hello"})

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

// TestBashTool_Timeout_ReturnsError verifies that a slow command exceeding the
// overall timeout returns a timeout error — not a background task ID.
func TestBashTool_Timeout_ReturnsError(t *testing.T) {
	rt, dir := newTestRuntime(t)
	defer os.RemoveAll(dir)

	tool := &BashTool{TaskRuntime: rt}
	input, _ := json.Marshal(bashInput{
		Command: "sleep 5",
		Timeout: 300, // 300ms overall — sleep 5 will exceed it
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected timeout error result, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "Task ID:") {
		t.Errorf("should NOT promote to background on timeout, got: %q", result.Content)
	}
	if !strings.Contains(result.Content, "timed out") {
		t.Errorf("expected 'timed out' in result, got: %q", result.Content)
	}
	if !strings.Contains(result.Content, "run_in_background") {
		t.Errorf("expected hint about run_in_background in result, got: %q", result.Content)
	}
}

// TestBashTool_RunInBackground_NilRuntime verifies that RunInBackground=true
// with a nil TaskRuntime returns an error.
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
	if !result.IsError {
		t.Fatalf("expected error result for nil runtime with RunInBackground, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Cannot run in background: no task runtime available") {
		t.Errorf("expected error message about no runtime, got: %q", result.Content)
	}
}

// TestBashTool_RunInBackground_SpawnsTask verifies that RunInBackground=true
// with a valid runtime returns a task ID immediately without blocking.
func TestBashTool_RunInBackground_SpawnsTask(t *testing.T) {
	rt, dir := newTestRuntime(t)
	defer os.RemoveAll(dir)

	tool := &BashTool{TaskRuntime: rt}
	input, _ := json.Marshal(bashInput{
		Command:         "sleep 60",
		RunInBackground: true,
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Task ID:") {
		t.Errorf("expected task ID in result, got: %q", result.Content)
	}
}

// TestBashTool_CommandExitsNonZero verifies that a command exiting non-zero
// returns an error result containing "Exit code".
func TestBashTool_CommandExitsNonZero(t *testing.T) {
	tool := &BashTool{TaskRuntime: nil}
	input, _ := json.Marshal(bashInput{Command: "false"})

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
	input, _ := json.Marshal(bashInput{
		Command: "yes a | head -c 40001",
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

// min is a helper for older Go versions that lack the built-in.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
