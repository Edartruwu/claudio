package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Abraxas-365/claudio/internal/prompts"
)

// --- EnterPlanModeTool ---

type EnterPlanModeTool struct{}

func (t *EnterPlanModeTool) Name() string { return "EnterPlanMode" }
func (t *EnterPlanModeTool) Description() string {
	return prompts.EnterPlanModeDescription()
}
func (t *EnterPlanModeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}
func (t *EnterPlanModeTool) IsReadOnly() bool                        { return true }
func (t *EnterPlanModeTool) RequiresApproval(_ json.RawMessage) bool { return true }
func (t *EnterPlanModeTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	// Create plan file
	home, _ := os.UserHomeDir()
	planDir := filepath.Join(home, ".claudio", "plans")
	os.MkdirAll(planDir, 0700)

	planFile := filepath.Join(planDir, fmt.Sprintf("plan-%d.md", time.Now().Unix()))
	initial := "# Plan\n\n## Context\n\n## Steps\n\n## Verification\n"
	os.WriteFile(planFile, []byte(initial), 0644)

	// Return workflow instructions in the tool result, matching Claude Code's
	// mapToolResultToToolResultBlockParam behavior.
	instructions := fmt.Sprintf(`Entered plan mode. You should now focus on exploring the codebase and designing an implementation approach.

In plan mode, you should:
1. Thoroughly explore the codebase to understand existing patterns
2. Identify similar features and architectural approaches
3. Consider multiple approaches and their trade-offs
4. Use AskUser if you need to clarify the approach
5. Design a concrete implementation strategy
6. Write your plan to the plan file: %s
7. When ready, use ExitPlanMode to present your plan for approval

Remember: DO NOT write or edit any files except the plan file. This is a read-only exploration and planning phase.`, planFile)

	return &Result{Content: instructions}, nil
}

// --- ExitPlanModeTool ---

type ExitPlanModeTool struct{}

func (t *ExitPlanModeTool) Name() string { return "ExitPlanMode" }
func (t *ExitPlanModeTool) Description() string {
	return prompts.ExitPlanModeDescription()
}
func (t *ExitPlanModeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}
func (t *ExitPlanModeTool) IsReadOnly() bool                        { return false }
func (t *ExitPlanModeTool) RequiresApproval(_ json.RawMessage) bool { return false }
func (t *ExitPlanModeTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	// Read the plan content from the most recent plan file and include it
	// in the tool result, matching Claude Code's mapToolResultToToolResultBlockParam.
	home, _ := os.UserHomeDir()
	planDir := filepath.Join(home, ".claudio", "plans")
	entries, err := os.ReadDir(planDir)
	if err == nil && len(entries) > 0 {
		// Pick the last entry (lexicographic = chronological since filenames use unix timestamps)
		latest := entries[len(entries)-1]
		planPath := filepath.Join(planDir, latest.Name())
		if data, err := os.ReadFile(planPath); err == nil && len(data) > 0 {
			plan := string(data)
			return &Result{Content: fmt.Sprintf(`User has approved your plan. You can now start coding. Start with updating your todo list if applicable.

Your plan has been saved to: %s
You can refer back to it if needed during implementation.

## Approved Plan:
%s`, planPath, plan)}, nil
		}
	}
	return &Result{Content: "User has approved exiting plan mode. You can now proceed."}, nil
}
