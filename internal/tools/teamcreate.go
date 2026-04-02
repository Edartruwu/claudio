package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Abraxas-365/claudio/internal/teams"
)

// TeamCreateTool creates a new agent team.
type TeamCreateTool struct {
	deferrable
	Manager   *teams.Manager
	SessionID string
}

type teamCreateInput struct {
	TeamName    string `json:"team_name"`
	Description string `json:"description,omitempty"`
}

func (t *TeamCreateTool) Name() string { return "TeamCreate" }

func (t *TeamCreateTool) Description() string {
	return `Create a new agent team for parallel multi-agent work.

## When to Use
- Complex tasks that benefit from parallel execution
- Tasks that can be decomposed into independent subtasks
- When you need multiple specialized agents working simultaneously

## How It Works
1. Create a team with a name and optional description
2. You become the team lead
3. Use the Agent tool to spawn teammates with team context
4. Teammates communicate via the SendMessage tool
5. Monitor progress and collect results
6. Delete the team when done

## Team Workflow
1. TeamCreate — initialize team
2. Agent (with subagent_type) — spawn workers
3. SendMessage — coordinate and collect results
4. TaskCreate — track shared task list
5. TeamDelete — cleanup when done`
}

func (t *TeamCreateTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"team_name": {
				"type": "string",
				"description": "Name for the team (lowercase, no spaces)"
			},
			"description": {
				"type": "string",
				"description": "What this team will accomplish"
			}
		},
		"required": ["team_name"]
	}`)
}

func (t *TeamCreateTool) IsReadOnly() bool                        { return false }
func (t *TeamCreateTool) RequiresApproval(_ json.RawMessage) bool { return true }

func (t *TeamCreateTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in teamCreateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.TeamName == "" {
		return &Result{Content: "team_name is required", IsError: true}, nil
	}

	if t.Manager == nil {
		return &Result{Content: "Team system not available", IsError: true}, nil
	}

	team, err := t.Manager.CreateTeam(in.TeamName, in.Description, t.SessionID)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create team: %v", err), IsError: true}, nil
	}

	return &Result{Content: fmt.Sprintf("Team created: %s\nLead: %s\nYou are the team lead. Use Agent tool to spawn teammates.", team.Name, team.LeadAgent)}, nil
}
