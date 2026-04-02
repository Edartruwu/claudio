package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Abraxas-365/claudio/internal/tasks"
)

// TaskStopTool stops a running background task.
type TaskStopTool struct {
	deferrable
	Runtime *tasks.Runtime
}

type taskStopInput struct {
	TaskID string `json:"task_id"`
}

func (t *TaskStopTool) Name() string { return "TaskStop" }

func (t *TaskStopTool) Description() string {
	return `Stops a running background task by ID. Returns success or failure status. Use this when a background command or agent needs to be terminated.`
}

func (t *TaskStopTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_id": {
				"type": "string",
				"description": "The ID of the background task to stop"
			}
		},
		"required": ["task_id"]
	}`)
}

func (t *TaskStopTool) IsReadOnly() bool                        { return false }
func (t *TaskStopTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *TaskStopTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in taskStopInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.TaskID == "" {
		return &Result{Content: "task_id required", IsError: true}, nil
	}

	if t.Runtime == nil {
		return &Result{Content: "Task runtime not available", IsError: true}, nil
	}

	if err := t.Runtime.Kill(in.TaskID); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to stop task: %v", err), IsError: true}, nil
	}

	return &Result{Content: fmt.Sprintf("Task %s stopped", in.TaskID)}, nil
}
