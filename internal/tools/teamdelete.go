package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Abraxas-365/claudio/internal/teams"
)

// TeamDeleteTool removes a team and cleans up.
type TeamDeleteTool struct {
	deferrable
	Manager *teams.Manager
	Runner  *teams.TeammateRunner
}

type teamDeleteInput struct {
	TeamName string `json:"team_name"`
}

func (t *TeamDeleteTool) Name() string { return "TeamDelete" }

func (t *TeamDeleteTool) Description() string {
	return `Delete a team and clean up all resources. Cancels any still-running members,
waits briefly for them to drain, then removes the team config, mailboxes, and
in-memory state.`
}

func (t *TeamDeleteTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"team_name": {
				"type": "string",
				"description": "Name of the team to delete"
			}
		},
		"required": ["team_name"]
	}`)
}

func (t *TeamDeleteTool) IsReadOnly() bool                        { return false }
func (t *TeamDeleteTool) RequiresApproval(_ json.RawMessage) bool { return true }

func (t *TeamDeleteTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in teamDeleteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.TeamName == "" {
		return &Result{Content: "team_name is required", IsError: true}, nil
	}

	if t.Manager == nil {
		return &Result{Content: "Team system not available", IsError: true}, nil
	}

	// Drain any still-running members so DeleteTeam doesn't refuse the delete.
	drainTimedOut := false
	if t.Runner != nil {
		t.Runner.KillTeam(in.TeamName)
		if !t.Runner.WaitForTeam(in.TeamName, 5*time.Second) {
			drainTimedOut = true
		}
	}

	if err := t.Manager.DeleteTeam(in.TeamName); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to delete team: %v", err), IsError: true}, nil
	}

	// Clean up runner-side maps so deleted teams don't leak across create/delete cycles.
	if t.Runner != nil {
		t.Runner.CleanupTeam(in.TeamName)
	}

	msg := fmt.Sprintf("Team %q deleted", in.TeamName)
	if drainTimedOut {
		msg += " (warning: some members did not drain within 5s before cleanup)"
	}
	return &Result{Content: msg}, nil
}
