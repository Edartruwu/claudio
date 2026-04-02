package tasks

import (
	"context"
	"fmt"
	"time"
)

// DreamTaskInput defines parameters for the auto-memory consolidation task.
type DreamTaskInput struct {
	SessionSummary string // summary of what happened in the session
	ProjectDir     string // current project directory

	// RunDream is the callback that executes the dream agent.
	// It receives context and a prompt describing what to consolidate.
	// The agent should extract patterns, update memory files, etc.
	RunDream func(ctx context.Context, prompt string) (string, error)
}

// SpawnDreamTask starts a background memory consolidation agent.
func SpawnDreamTask(rt *Runtime, input DreamTaskInput) (*TaskState, error) {
	if input.RunDream == nil {
		return nil, fmt.Errorf("RunDream callback required")
	}

	id := rt.GenerateID(TypeDream)

	output, err := NewTaskOutput(rt.outputDir, id)
	if err != nil {
		return nil, fmt.Errorf("create output: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	state := &TaskState{
		ID:          id,
		Type:        TypeDream,
		Status:      StatusRunning,
		Description: "Memory consolidation",
		OutputFile:  output.Path(),
		StartTime:   time.Now(),
		cancel:      cancel,
	}

	rt.Register(state)

	go runDreamTask(ctx, rt, state, output, input)

	return state, nil
}

func runDreamTask(ctx context.Context, rt *Runtime, state *TaskState, output *TaskOutput, input DreamTaskInput) {
	defer output.Close()

	output.Write([]byte("[Dream] Starting memory consolidation...\n\n"))

	prompt := buildDreamPrompt(input.SessionSummary, input.ProjectDir)

	result, err := input.RunDream(ctx, prompt)
	if err != nil {
		if ctx.Err() == context.Canceled {
			rt.SetStatus(state.ID, StatusKilled, "")
			output.Write([]byte("\n[Dream cancelled]\n"))
			return
		}
		rt.SetStatus(state.ID, StatusFailed, err.Error())
		output.Write([]byte(fmt.Sprintf("\n[Dream failed] %v\n", err)))
		return
	}

	output.Write([]byte(fmt.Sprintf("\n[Dream complete]\n%s\n", result)))
	rt.SetStatus(state.ID, StatusCompleted, "")
}

func buildDreamPrompt(sessionSummary, projectDir string) string {
	return fmt.Sprintf(`You are a memory consolidation agent. Your job is to review the session activity and extract valuable patterns into persistent memory.

## Session Summary
%s

## Project Directory
%s

## Instructions

Review what happened in this session and extract:

1. **User Preferences** (type: user)
   - Any corrections the user made to your approach
   - Preferred coding style or patterns
   - Tools or workflows they prefer

2. **Feedback** (type: feedback)
   - Things the user said not to do
   - Approaches that worked well
   - Approaches that failed

3. **Project Context** (type: project)
   - Key decisions made during this session
   - Current state of work in progress
   - Important deadlines or constraints mentioned

4. **Learned Patterns** (type: reference)
   - Debugging techniques that worked
   - Workarounds discovered
   - Project-specific conventions

For each finding, save it as a memory file using the Write tool:
- Path: memory/<type>_<descriptive_name>.md
- Include YAML frontmatter with name, description, and type fields
- Keep each memory focused and under 50 lines

Also update the MEMORY.md index with a one-line entry for each new memory.

Only extract NON-OBVIOUS patterns. Do not save things that are:
- Already in the code (conventions visible in the codebase)
- Standard best practices (obvious to any developer)
- Ephemeral (only relevant to this exact moment)

If there's nothing worth extracting, say so and exit.`, sessionSummary, projectDir)
}
