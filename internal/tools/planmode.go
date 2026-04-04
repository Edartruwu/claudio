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
	// Do NOT say "approved" here — the user hasn't approved yet.
	// The TUI will show an approval dialog and send the actual approval
	// as a user message via handleSubmit. We just signal that the plan
	// is ready for review.
	return &Result{Content: "Plan submitted for user review. Do NOT start implementation until you receive explicit user approval in the next message."}, nil
}
