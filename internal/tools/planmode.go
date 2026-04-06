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
	instructions := fmt.Sprintf(`Plan mode is active. The user indicated that they do not want you to execute yet -- you MUST NOT make any edits (with the exception of the plan file mentioned below), run any non-readonly tools (including changing configs or making commits), or otherwise make any changes to the system. This supersedes any other instructions you have received.

## Plan File
Plan file: %s
No plan file exists yet. You should create your plan at the path above using the Write tool.
You should build your plan incrementally by writing to or editing this file. NOTE that this is the ONLY file you are allowed to edit — other than this you are only allowed to take READ-ONLY actions.

## Plan Workflow

### Phase 1: Initial Understanding
Goal: Gain a comprehensive understanding of the user's request by reading through code and asking them questions.

1. Focus on understanding the user's request and the code associated with their request. Actively search for existing functions, utilities, and patterns that can be reused — avoid proposing new code when suitable implementations already exist.
2. Launch Explore agents IN PARALLEL (single message, multiple tool calls) to efficiently explore the codebase when the scope is broad or multiple areas are involved.
   - Use 1 agent when the task is isolated to known files or you're making a small targeted change.
   - Use multiple agents when: the scope is uncertain, multiple areas of the codebase are involved, or you need to understand existing patterns before planning.

### Phase 2: Design
Goal: Design an implementation approach.

Launch Plan agent(s) to design the implementation based on the user's intent and your exploration results from Phase 1.

Guidelines:
- Default: Launch at least 1 Plan agent for most tasks — it helps validate your understanding and consider alternatives
- Skip agents: Only for truly trivial tasks (typo fixes, single-line changes, simple renames)

In the agent prompt:
- Provide comprehensive background context from Phase 1 exploration including filenames and code path traces
- Describe requirements and constraints
- Request a detailed implementation plan

### Phase 3: Review
Goal: Review the plan(s) from Phase 2 and ensure alignment with the user's intentions.
1. Read the critical files identified by agents to deepen your understanding
2. Ensure that the plans align with the user's original request
3. Use AskUser to clarify any remaining questions with the user

### Phase 4: Final Plan
Goal: Write your final plan to the plan file (the only file you can edit).
- List the paths of files to be modified and what changes are needed in each
- Reference existing functions to reuse, with file:line where known
- Include a Context section explaining why this change is being made
- End with a verification step (test command or manual check)

### Phase 5: Call ExitPlanMode
At the very end of your turn, once you have asked the user questions and are happy with your final plan file — you should always call ExitPlanMode to indicate to the user that you are done planning.
This is critical — your turn should only end with either using the AskUser tool OR calling ExitPlanMode. Do not stop unless it's for these 2 reasons.

**Important:** Use AskUser ONLY to clarify requirements or choose between approaches. Use ExitPlanMode to request plan approval. Do NOT ask about plan approval in any other way — no text questions. Phrases like "Is this plan okay?", "Should I proceed?", "How does this plan look?", or similar MUST use ExitPlanMode.

NOTE: At any point through this workflow you should feel free to ask the user questions or clarifications using the AskUser tool. Don't make large assumptions about user intent. The goal is to present a well-researched plan to the user and tie any loose ends before implementation begins.`, planFile)

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
func (t *ExitPlanModeTool) IsReadOnly() bool                        { return true }
func (t *ExitPlanModeTool) RequiresApproval(_ json.RawMessage) bool { return false }
func (t *ExitPlanModeTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	// Do NOT say "approved" here — the user hasn't approved yet.
	// The TUI will show an approval dialog and send the actual approval
	// as a user message via handleSubmit. We just signal that the plan
	// is ready for review.
	return &Result{Content: "Plan submitted for user review. Do NOT start implementation until you receive explicit user approval in the next message."}, nil
}
