package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/tasks"
)

// BgTaskListTool lists background shell/agent tasks for the current session.
type BgTaskListTool struct {
	deferrable
	Runtime   *tasks.Runtime
	SessionID string
}

type bgTaskListInput struct {
	OnlyRunning bool `json:"only_running,omitempty"`
}

func (t *BgTaskListTool) Name() string { return "BgTaskList" }

func (t *BgTaskListTool) Description() string {
	return `List background shell/bash tasks and Agent-tool tasks started in this session. Returns task ID, type, status, and duration. NOTE: this does NOT show SpawnTeammate agents — use ListTeammates for those.`
}

func (t *BgTaskListTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"only_running": {
				"type": "boolean",
				"description": "If true, only show running (non-terminal) tasks. Default: false (show all)."
			}
		}
	}`)
}

func (t *BgTaskListTool) IsReadOnly() bool                        { return true }
func (t *BgTaskListTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *BgTaskListTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in bgTaskListInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if t.Runtime == nil {
		return &Result{Content: "Task runtime not available", IsError: true}, nil
	}

	all := t.Runtime.List(in.OnlyRunning)

	// Filter to this session only.
	var filtered []*tasks.TaskState
	for _, ts := range all {
		if t.SessionID == "" || ts.SessionID == "" || ts.SessionID == t.SessionID {
			filtered = append(filtered, ts)
		}
	}

	if len(filtered) == 0 {
		if in.OnlyRunning {
			return &Result{Content: "No running background tasks."}, nil
		}
		return &Result{Content: "No background tasks."}, nil
	}

	var sb strings.Builder
	for _, ts := range filtered {
		duration := time.Since(ts.StartTime).Round(time.Second)
		if ts.EndTime != nil {
			duration = ts.EndTime.Sub(ts.StartTime).Round(time.Second)
		}
		sb.WriteString(fmt.Sprintf("ID: %s  type: %s  status: %s  duration: %s\n", ts.ID, ts.Type, ts.Status, duration))
		if ts.Description != "" {
			sb.WriteString(fmt.Sprintf("  desc: %s\n", ts.Description))
		}
		if ts.Error != "" {
			sb.WriteString(fmt.Sprintf("  error: %s\n", ts.Error))
		}
	}

	return &Result{Content: strings.TrimRight(sb.String(), "\n")}, nil
}
