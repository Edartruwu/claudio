package tools

import (
	"context"
	"encoding/json"
	"fmt"

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
	return `Delete a team and clean up all resources. Fails if any members are still active — kill them first.`
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

	if t.Manager == nil {
		return &Result{Content: "Team system not available", IsError: true}, nil
	}

	if err := t.Manager.DeleteTeam(in.TeamName); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to delete team: %v", err), IsError: true}, nil
	}

	// Clear active team if this was the active one
	if t.Runner != nil && t.Runner.ActiveTeamName() == in.TeamName {
		t.Runner.SetActiveTeam("")
	}

	return &Result{Content: fmt.Sprintf("Team %q deleted", in.TeamName)}, nil
}
