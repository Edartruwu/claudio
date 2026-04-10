package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/teams"
)

const teammateResultInstruction = `

## Result format (IMPORTANT)
When you finish, your LAST message must be a concise summary of ≤15 lines structured exactly as:

### Result
- **What was done**: one-line summary of the outcome
- **Files changed**: list changed files (or "none")
- **Issues / blockers**: anything unexpected, or "none"

This section is extracted as the notification shown to the orchestrator. Keep it tight — no narration of tool calls, no repeating the task description.`

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
	Name         string          `json:"name"`
	SubagentType string          `json:"subagent_type"`
	Prompt       string          `json:"prompt"`
	Model        string          `json:"model,omitempty"`
	Isolation    string          `json:"isolation,omitempty"`
	TaskIDs      json.RawMessage `json:"task_ids,omitempty"`
	MaxTurns     int             `json:"max_turns,omitempty"`
	Background   json.RawMessage `json:"run_in_background,omitempty"`
}

// backgroundBool coerces run_in_background from bool, string "true"/"false", or absent (defaults true).
func (in *spawnTeammateInput) backgroundBool() bool {
	if len(in.Background) == 0 {
		return true // default
	}
	// Try bool directly.
	var b bool
	if err := json.Unmarshal(in.Background, &b); err == nil {
		return b
	}
	// Fallback: model sent a quoted string like "true" or "false".
	var s string
	if err := json.Unmarshal(in.Background, &s); err == nil {
		return s != "false" && s != "0"
	}
	return true
}

// taskIDStrings coerces TaskIDs from either a JSON array or a JSON-encoded string.
func (in *spawnTeammateInput) taskIDStrings() []string {
	if len(in.TaskIDs) == 0 {
		return nil
	}
	// Try array first.
	var ids []string
	if err := json.Unmarshal(in.TaskIDs, &ids); err == nil {
		return ids
	}
	// Fallback: the model sent a JSON-encoded string like "[\"1\",\"2\"]".
	var encoded string
	if err := json.Unmarshal(in.TaskIDs, &encoded); err == nil {
		var ids2 []string
		if err2 := json.Unmarshal([]byte(encoded), &ids2); err2 == nil {
			return ids2
		}
	}
	return nil
}

func (t *SpawnTeammateTool) Name() string { return "SpawnTeammate" }

func (t *SpawnTeammateTool) Description() string {
	return `Spawn a named teammate agent that appears in the Agents panel.

Unlike the generic Agent tool, SpawnTeammate:
- Always routes through the team runner (visible in TUI, cancellable)
- Auto-creates a default team if none is active
- Accepts a name so you can reference this agent later via SendMessage
- Links task IDs that are auto-completed when the agent finishes

Use this whenever you want to delegate work to a named collaborator you can track, message, or wait on. Use Agent for anonymous one-off sub-tasks.

## Naming and parallel instances

The name you provide is a display label. You can spawn multiple agents with the same role
by using the same base name — if that name is already running, the system auto-suffixes it:
  maya → already running → new agent spawns as maya-2, maya-3, etc.

The RESOLVED name is always shown in the result. Use that resolved name (not the base name)
when calling SendMessage to reach a specific instance.

Example — two parallel frontend-mid agents:
  SpawnTeammate(name="maya", ...) → spawns "maya"
  SpawnTeammate(name="maya", ...) → spawns "maya-2"  ← use "maya-2" for SendMessage

## Name reuse for finished agents

If the name matches an agent that has already FINISHED (not currently running), SpawnTeammate
replaces it with a completely fresh agent — clean context, no history. This is the upsert path.
To continue an idle agent's conversation instead, use SendMessage (which triggers Revive and
preserves the full history).

## Workflow
1. (Optional) TeamCreate to name the team
2. SpawnTeammate — spawn each worker; note the resolved name in the result
3. SendMessage — coordinate using the resolved name
4. SpawnTeammate with run_in_background:false — wait for a specific agent to finish

## Handling questions from teammates

Sub-agents are instructed to stop and ask when they hit decisions they cannot resolve. When a teammate finishes with a line like:

    QUESTION: <their question>

they have gone idle with their full conversation history preserved. To answer:

1. Read the question and decide the answer (consult the user via AskUser if needed)
2. Call SendMessage(<resolved name>, <your answer>)
3. The idle teammate automatically resumes with full history intact and continues from where it left off

Do NOT re-spawn them with SpawnTeammate — that creates a fresh agent with no history. SendMessage is the correct path; it triggers Revive which preserves the conversation.`
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

// resolveAgentName returns a name that is safe to spawn under.
// If baseName is free (not found, or found but finished), it is returned as-is.
// If baseName is already running, we try baseName-2, baseName-3, … up to -99.
func resolveAgentName(runner *teams.TeammateRunner, baseName string) (string, bool) {
	existing, ok := runner.GetStateByName(baseName)
	if !ok || existing.Status != teams.StatusWorking {
		return baseName, true
	}
	for i := 2; i <= 99; i++ {
		candidate := fmt.Sprintf("%s-%d", baseName, i)
		if ex, ok2 := runner.GetStateByName(candidate); !ok2 || ex.Status != teams.StatusWorking {
			return candidate, true
		}
	}
	return "", false
}

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

	background := in.backgroundBool()

	// Resolve the actual name to use.
	// If the requested name is already running, auto-suffix (maya → maya-2, maya-3, …).
	resolvedName, ok := resolveAgentName(t.Runner, in.Name)
	if !ok {
		return &Result{Content: fmt.Sprintf(
			"Cannot spawn %q: all 99 parallel slots for this name are already running.",
			in.Name,
		), IsError: true}, nil
	}

	// If task_ids were provided, prepend their subject+description to the prompt
	// so the sub-agent has full context without the orchestrator having to repeat it.
	taskIDs := in.taskIDStrings()
	prompt := in.Prompt
	if len(taskIDs) > 0 {
		var block strings.Builder
		block.WriteString("--- Assigned Tasks ---\n")
		for _, id := range taskIDs {
			if task, ok := GlobalTaskStore.Get(id); ok {
				block.WriteString(fmt.Sprintf("[Task #%s] %s\n", task.ID, task.Subject))
				if task.Description != "" {
					block.WriteString(fmt.Sprintf("Description: %s\n", task.Description))
				}
				block.WriteString("\n")
			}
		}
		block.WriteString("---\n\n")
		prompt = block.String() + prompt
	}

	agentDef.SystemPrompt += teammateResultInstruction

	// Upsert semantics: whether the name is new or previously finished, always spawn fresh.
	// Spawn → AddMember removes the old terminal entry and creates a clean one;
	// r.teammates[agentID] is overwritten with a brand-new state (no history).
	parentAgentID := teams.TeammateAgentIDFromContext(ctx)
	state, err := t.Runner.Spawn(teams.SpawnConfig{
		TeamName:      teamName,
		AgentName:     resolvedName,
		Prompt:        prompt,
		System:        agentDef.SystemPrompt,
		Model:         modelOverride,
		SubagentType:  in.SubagentType,
		MaxTurns:      maxTurns,
		Isolation:     in.Isolation,
		MemoryDir:     agentDef.MemoryDir,
		Foreground:    !background,
		TaskIDs:       taskIDs,
		ParentAgentID: parentAgentID,
	})
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to spawn teammate %q: %v", resolvedName, err), IsError: true}, nil
	}

	if background {
		msg := fmt.Sprintf("Teammate %q spawned in team %q\nRole: %s\nOpen the Agents panel (space a) to monitor progress.",
			resolvedName, teamName, agentDef.Type)
		if resolvedName != in.Name {
			msg += fmt.Sprintf("\nNote: %q was already running — spawned as %q instead. Use %q for SendMessage.", in.Name, resolvedName, resolvedName)
		}
		if state.WorktreePath != "" {
			msg += fmt.Sprintf("\nWorktree: %s\nBranch: %s", state.WorktreePath, state.WorktreeBranch)
		}
		return &Result{Content: msg}, nil
	}

	// Foreground: block until done.
	done := t.Runner.WaitForOne(state.Identity.AgentID, 30*time.Minute)
	if !done {
		return &Result{Content: fmt.Sprintf("Teammate %q timed out after 30 minutes", resolvedName), IsError: true}, nil
	}
	if state.Error != "" {
		return &Result{Content: fmt.Sprintf("Teammate %q error: %s", resolvedName, state.Error), IsError: true}, nil
	}
	result := state.Result
	const maxBytes = 50_000
	if len(result) > maxBytes {
		result = result[:maxBytes] + fmt.Sprintf("\n[Output truncated at %d bytes]", maxBytes)
	}
	return &Result{Content: result}, nil
}
