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
