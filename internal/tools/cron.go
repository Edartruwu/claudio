package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Abraxas-365/claudio/internal/tasks"
)

// CronCreateTool creates a new scheduled task.
type CronCreateTool struct {
	Store     *tasks.CronStore
	SessionID string // injected at runtime — current active session ID
}

func (t *CronCreateTool) Name() string        { return "CronCreate" }
func (t *CronCreateTool) Description() string  { return "Create a recurring scheduled task" }
func (t *CronCreateTool) IsReadOnly() bool     { return false }
func (t *CronCreateTool) ShouldDefer() bool    { return true }
func (t *CronCreateTool) SearchHint() string   { return "cron schedule recurring task timer" }
func (t *CronCreateTool) RequiresApproval(_ json.RawMessage) bool { return true }

func (t *CronCreateTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"schedule": {"type": "string", "description": "Schedule: @every 1h, @daily, @hourly, or HH:MM"},
			"prompt": {"type": "string", "description": "The prompt/task to execute on schedule"},
			"agent": {"type": "string", "description": "Optional agent type to use"},
			"type": {"type": "string", "enum": ["inline", "background"], "description": "Execution type: inline (inject as user msg) or background (spawn isolated engine). Default: inline"}
		},
		"required": ["schedule", "prompt"]
	}`)
}

func (t *CronCreateTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var params struct {
		Schedule string `json:"schedule"`
		Prompt   string `json:"prompt"`
		Agent    string `json:"agent"`
		Type     string `json:"type"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return errResult("invalid input: " + err.Error()), nil
	}
	if t.Store == nil {
		return errResult("cron store not configured"), nil
	}

	entry, err := t.Store.Add(params.Schedule, params.Prompt, params.Agent, params.Type, t.SessionID)
	if err != nil {
		return errResult("failed to create cron entry: " + err.Error()), nil
	}

	// Embed cron ID in inline prompts so the AI can self-delete when done.
	entryType := entry.Type
	if entryType == "" {
		entryType = "inline"
	}
	if entryType == "inline" {
		updated := entry.Prompt + fmt.Sprintf("\n\n[cron_id: %s] If your task is complete or a condition is met that should stop future runs, call the CronDelete tool with this ID.", entry.ID)
		if err := t.Store.UpdatePrompt(entry.ID, updated); err != nil {
			// Non-fatal: log but don't fail the create.
			_ = err
		}
	}

	return &Result{Content: fmt.Sprintf("Created cron entry %s (type: %s, schedule: %s, next: %s)",
		entry.ID, entry.Type, entry.Schedule, entry.NextRun.Format("2006-01-02 15:04"))}, nil
}

// CronDeleteTool removes a scheduled task.
type CronDeleteTool struct {
	Store *tasks.CronStore
}

func (t *CronDeleteTool) Name() string        { return "CronDelete" }
func (t *CronDeleteTool) Description() string  { return "Delete a scheduled task by ID" }
func (t *CronDeleteTool) IsReadOnly() bool     { return false }
func (t *CronDeleteTool) ShouldDefer() bool    { return true }
func (t *CronDeleteTool) SearchHint() string   { return "cron delete remove schedule" }
func (t *CronDeleteTool) RequiresApproval(_ json.RawMessage) bool { return true }

func (t *CronDeleteTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "The cron entry ID to delete"}
		},
		"required": ["id"]
	}`)
}

func (t *CronDeleteTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return errResult("invalid input: " + err.Error()), nil
	}
	if t.Store == nil {
		return errResult("cron store not configured"), nil
	}

	if err := t.Store.Remove(params.ID); err != nil {
		return errResult(err.Error()), nil
	}
	return &Result{Content: fmt.Sprintf("Deleted cron entry %s", params.ID)}, nil
}

// CronListTool lists all scheduled tasks.
type CronListTool struct {
	Store *tasks.CronStore
}

func (t *CronListTool) Name() string        { return "CronList" }
func (t *CronListTool) Description() string  { return "List all scheduled tasks" }
func (t *CronListTool) IsReadOnly() bool     { return true }
func (t *CronListTool) ShouldDefer() bool    { return true }
func (t *CronListTool) SearchHint() string   { return "cron list schedule recurring" }
func (t *CronListTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *CronListTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}

func (t *CronListTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	if t.Store == nil {
		return errResult("cron store not configured"), nil
	}
	return &Result{Content: t.Store.FormatList()}, nil
}

func errResult(msg string) *Result {
	return &Result{Content: msg, IsError: true}
}
