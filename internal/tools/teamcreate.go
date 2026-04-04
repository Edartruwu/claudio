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
	Manager         *teams.Manager
	Runner          *teams.TeammateRunner
	SessionID       string
	AvailableModels []string // dynamic model list from configured providers
}

type teamCreateInput struct {
	TeamName    string `json:"team_name"`
	Description string `json:"description,omitempty"`
	Model       string `json:"model,omitempty"`
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
	modelEnum := buildModelEnum(t.AvailableModels)
	return json.RawMessage(fmt.Sprintf(`{
		"type": "object",
		"properties": {
			"team_name": {
				"type": "string",
				"description": "Name for the team (lowercase, no spaces)"
			},
			"description": {
				"type": "string",
				"description": "What this team will accomplish"
			},
			"model": {
				"type": "string",
				"description": "Default model for all agents in this team. Per-agent model overrides this.",
				"enum": %s
			}
		},
		"required": ["team_name"]
	}`, modelEnum))
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

	team, err := t.Manager.CreateTeam(in.TeamName, in.Description, t.SessionID, in.Model)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create team: %v", err), IsError: true}, nil
	}

	// Set this as the active team so Agent tool routes through the runner
	if t.Runner != nil {
		t.Runner.SetActiveTeam(team.Name)
	}

	msg := fmt.Sprintf("Team created: %s\nLead: %s", team.Name, team.LeadAgent)
	if team.Model != "" {
		msg += fmt.Sprintf("\nDefault model: %s", team.Model)
	}
	msg += "\nYou are the team lead. Use Agent tool to spawn teammates."
	return &Result{Content: msg}, nil
}
