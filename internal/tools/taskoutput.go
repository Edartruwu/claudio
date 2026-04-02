package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Abraxas-365/claudio/internal/tasks"
)

// TaskOutputTool retrieves output from a background task.
type TaskOutputTool struct {
	deferrable
	Runtime *tasks.Runtime
}

type taskOutputInput struct {
	TaskID  string `json:"task_id"`
	Timeout int    `json:"timeout,omitempty"` // seconds to wait for completion (0 = non-blocking)
}

func (t *TaskOutputTool) Name() string { return "TaskOutput" }

func (t *TaskOutputTool) Description() string {
	return `Retrieves output from a background task (shell command or agent). By default waits up to 30 seconds for the task to complete. Set timeout to 0 for non-blocking mode (returns current status immediately). Returns the task's stdout/stderr output.`
}

func (t *TaskOutputTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_id": {
				"type": "string",
				"description": "The ID of the background task"
			},
			"timeout": {
				"type": "number",
				"description": "Seconds to wait for completion (default: 30, max: 600, 0 = non-blocking)"
			}
		},
		"required": ["task_id"]
	}`)
}

func (t *TaskOutputTool) IsReadOnly() bool                        { return true }
func (t *TaskOutputTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *TaskOutputTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in taskOutputInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.TaskID == "" {
		return &Result{Content: "task_id required", IsError: true}, nil
	}

	if t.Runtime == nil {
		return &Result{Content: "Task runtime not available", IsError: true}, nil
	}

	state, ok := t.Runtime.Get(in.TaskID)
	if !ok {
		return &Result{Content: fmt.Sprintf("Task %s not found", in.TaskID), IsError: true}, nil
	}

	// Determine timeout
	timeout := 30 * time.Second
	if in.Timeout > 0 {
		timeout = time.Duration(in.Timeout) * time.Second
		if timeout > 600*time.Second {
			timeout = 600 * time.Second
		}
	}
	blocking := in.Timeout != 0

	// Wait for completion if blocking
	if blocking && !state.Status.IsTerminal() {
		deadline := time.Now().Add(timeout)
		for !state.Status.IsTerminal() && time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return &Result{Content: "Interrupted", IsError: true}, nil
			case <-time.After(100 * time.Millisecond):
				// Re-fetch state
				state, _ = t.Runtime.Get(in.TaskID)
			}
		}
	}

	// Build result
	var result string

	// Read output
	if state.OutputFile != "" {
		content, _, err := tasks.ReadDelta(state.OutputFile, 0, 160*1024) // 160KB max
		if err == nil && content != "" {
			result = content
		}
	}

	// Truncate if needed
	const maxOutput = 32 * 1024
	if len(result) > maxOutput {
		result = result[:maxOutput] + "\n... (output truncated, " + fmt.Sprintf("%d bytes total", len(result)) + ")"
	}

	// Add status header
	status := fmt.Sprintf("[Task %s: %s (%s)]", state.ID, state.Status, state.Type)
	if state.Error != "" {
		status += fmt.Sprintf("\nError: %s", state.Error)
	}
	if state.ExitCode != nil {
		status += fmt.Sprintf("\nExit code: %d", *state.ExitCode)
	}
	duration := time.Since(state.StartTime).Round(time.Second)
	if state.EndTime != nil {
		duration = state.EndTime.Sub(state.StartTime).Round(time.Second)
	}
	status += fmt.Sprintf("\nDuration: %s", duration)

	if result != "" {
		result = status + "\n\n" + result
	} else {
		result = status + "\n\n(no output)"
	}

	retrieval := "success"
	if !state.Status.IsTerminal() {
		retrieval = "not_ready"
	}

	result = fmt.Sprintf("retrieval_status: %s\n%s", retrieval, result)

	return &Result{Content: result, IsError: state.Status == tasks.StatusFailed}, nil
}
