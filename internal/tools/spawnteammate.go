package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/teams"
)

// SpawnTeammateTool spawns a named team member through the TeammateRunner,
// auto-creating a default team when none is active.
type SpawnTeammateTool struct {
	deferrable
	Runner          *teams.TeammateRunner
	Manager         *teams.Manager
	SessionID       string
	AvailableModels []string
}

type spawnTeammateInput struct {
	Name         string   `json:"name"`             // short identifier (e.g. "a1", "backend-1")
	SubagentType string   `json:"subagent_type"`    // e.g. "backend-mid", "backend-senior"
	Prompt       string   `json:"prompt"`           // task description
	Model        string   `json:"model,omitempty"`  // optional model override
	Isolation    string   `json:"isolation,omitempty"` // "worktree"
	TaskIDs      []string `json:"task_ids,omitempty"`  // task IDs to auto-complete
	MaxTurns     int      `json:"max_turns,omitempty"`
	Background   bool     `json:"run_in_background,omitempty"`
}

func (t *SpawnTeammateTool) Name() string { return "SpawnTeammate" }

func (t *SpawnTeammateTool) Description() string {
	return `Spawn a named teammate agent that appears in the Agents panel.

Unlike the generic Agent tool, SpawnTeammate:
- Always routes through the team runner (visible in TUI, cancellable)
- Auto-creates a default team if none is active
- Accepts an explicit name so you can reference this agent later via SendMessage
- Links task IDs that are auto-completed when the agent finishes

Use this whenever you want to delegate work to a named collaborator you can track, message, or wait on. Use Agent for anonymous one-off sub-tasks.

## Workflow
1. (Optional) TeamCreate to name the team
2. SpawnTeammate — spawn each worker with a name and task
3. SendMessage — coordinate (use the name you gave)
4. SpawnTeammate with run_in_background:false — wait for a specific agent to finish`
}

func (t *SpawnTeammateTool) InputSchema() json.RawMessage {
	modelEnum := buildModelEnum(t.AvailableModels)
	return json.RawMessage(fmt.Sprintf(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Short identifier for this agent (e.g. \"a1\", \"tester\", \"migrator\"). Used to address it via SendMessage."
			},
			"subagent_type": {
				"type": "string",
				"description": "Agent role: backend-mid, backend-senior, general-purpose, Explore, Plan, verification"
			},
			"prompt": {
				"type": "string",
				"description": "The task for this agent to perform"
			},
			"model": {
				"type": "string",
				"description": "Optional model override",
				"enum": %s
			},
			"max_turns": {
				"type": "number",
				"description": "Maximum agentic turns before the agent stops (0 = unlimited)"
			},
			"run_in_background": {
				"type": "boolean",
				"description": "If true (default), agent runs in the background; if false, blocks until the agent finishes and returns its output"
			},
			"task_ids": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Task IDs to auto-complete when this agent finishes"
			},
			"isolation": {
				"type": "string",
				"description": "\"worktree\" to run in an isolated git worktree",
				"enum": ["worktree"]
			}
		},
		"required": ["name", "subagent_type", "prompt"]
	}`, modelEnum))
}

func (t *SpawnTeammateTool) IsReadOnly() bool                        { return false }
func (t *SpawnTeammateTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *SpawnTeammateTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in spawnTeammateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}
	if in.Name == "" {
		return &Result{Content: "name is required", IsError: true}, nil
	}
	if in.Prompt == "" {
		return &Result{Content: "prompt is required", IsError: true}, nil
	}
	if t.Runner == nil {
		return &Result{Content: "Team system not available", IsError: true}, nil
	}

	// Auto-create a default team if none is active.
	teamName := t.Runner.ActiveTeamName()
	if teamName == "" {
		if t.Manager == nil {
			return &Result{Content: "Team manager not available — call TeamCreate first", IsError: true}, nil
		}
		teamName = "default-team"
		if _, err := t.Manager.CreateTeam(teamName, "Auto-created default team", t.SessionID, ""); err != nil {
			// Team may already exist; proceed anyway.
			_ = err
		}
		t.Runner.SetActiveTeam(teamName)
	}

	agentDef := agents.GetAgent(in.SubagentType)

	maxTurns := in.MaxTurns
	if maxTurns <= 0 {
		maxTurns = agentDef.MaxTurns
	}

	modelOverride := in.Model
	if modelOverride == "" {
		modelOverride = agentDef.Model
	}

	background := in.Background
	if !background {
		// default to background=true unless explicitly set to false
		background = true
	}

	state, err := t.Runner.Spawn(teams.SpawnConfig{
		TeamName:     teamName,
		AgentName:    in.Name,
		Prompt:       in.Prompt,
		System:       agentDef.SystemPrompt,
		Model:        modelOverride,
		SubagentType: in.SubagentType,
		MaxTurns:     maxTurns,
		Isolation:    in.Isolation,
		MemoryDir:    agentDef.MemoryDir,
		Foreground:   !background,
		TaskIDs:      in.TaskIDs,
	})
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to spawn teammate %q: %v", in.Name, err), IsError: true}, nil
	}

	if background {
		msg := fmt.Sprintf("Teammate %q spawned in team %q (agent ID: %s)\nRole: %s\nOpen the Agents panel (space a) to monitor progress.",
			in.Name, teamName, state.Identity.AgentID, agentDef.Type)
		if state.WorktreePath != "" {
			msg += fmt.Sprintf("\nWorktree: %s", state.WorktreePath)
		}
		return &Result{Content: msg}, nil
	}

	// Foreground: block until done.
	done := t.Runner.WaitForOne(state.Identity.AgentID, 30*time.Minute)
	if !done {
		return &Result{Content: fmt.Sprintf("Teammate %q timed out after 30 minutes", in.Name), IsError: true}, nil
	}
	if state.Error != "" {
		return &Result{Content: fmt.Sprintf("Teammate %q error: %s", in.Name, state.Error), IsError: true}, nil
	}
	result := state.Result
	const maxBytes = 50_000
	if len(result) > maxBytes {
		result = result[:maxBytes] + fmt.Sprintf("\n[Output truncated at %d bytes]", maxBytes)
	}
	return &Result{Content: result}, nil
}
