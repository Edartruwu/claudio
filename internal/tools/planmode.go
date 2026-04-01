package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// --- EnterPlanModeTool ---

type EnterPlanModeTool struct{}

func (t *EnterPlanModeTool) Name() string { return "EnterPlanMode" }
func (t *EnterPlanModeTool) Description() string {
	return `Enters plan mode for structured thinking. Creates a plan file that can be edited. In plan mode, only read-only tools are available until the plan is approved.`
}
func (t *EnterPlanModeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}
func (t *EnterPlanModeTool) IsReadOnly() bool                        { return false }
func (t *EnterPlanModeTool) RequiresApproval(_ json.RawMessage) bool { return false }
func (t *EnterPlanModeTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	// Create plan file
	home, _ := os.UserHomeDir()
	planDir := filepath.Join(home, ".claudio", "plans")
	os.MkdirAll(planDir, 0700)

	planFile := filepath.Join(planDir, fmt.Sprintf("plan-%d.md", time.Now().Unix()))
	initial := "# Plan\n\n## Context\n\n## Steps\n\n## Verification\n"
	os.WriteFile(planFile, []byte(initial), 0644)

	return &Result{Content: fmt.Sprintf("Plan mode active. Plan file: %s", planFile)}, nil
}

// --- ExitPlanModeTool ---

type ExitPlanModeTool struct{}

func (t *ExitPlanModeTool) Name() string { return "ExitPlanMode" }
func (t *ExitPlanModeTool) Description() string {
	return `Exits plan mode and requests user approval of the plan.`
}
func (t *ExitPlanModeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}
func (t *ExitPlanModeTool) IsReadOnly() bool                        { return false }
func (t *ExitPlanModeTool) RequiresApproval(_ json.RawMessage) bool { return false }
func (t *ExitPlanModeTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	return &Result{Content: "Plan mode exited. Awaiting user approval."}, nil
}
