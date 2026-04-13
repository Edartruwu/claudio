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
	MemoryDir      string // target memory directory for writing memories

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

	prompt := buildDreamPrompt(input.SessionSummary, input.ProjectDir, input.MemoryDir)

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

func buildDreamPrompt(sessionSummary, projectDir, memoryDir string) string {
	return fmt.Sprintf(`You are a memory consolidation agent. Your job is to review the session activity and extract valuable patterns into persistent memory using the Memory tool.

## Session Summary
%s

## Project Directory
%s

## How to Use the Memory Tool

The Memory tool manages facts across sessions. Each memory entry contains:
- **name** — unique kebab-case identifier (e.g., "jwt-expiration", "user-prefers-table-tests")
- **description** — one-sentence summary
- **facts** — array of discrete one-sentence facts (not prose paragraphs)
- **tags** — labels for categorization
- **type** — one of: user, feedback, project, reference
- **scope** — ask: "would this be true in a completely different project?" → yes: "global" (user preferences, style). No: "project" (this repo's conventions, decisions). Persona-specific: "agent"

Key actions:
- **Memory(action="list")** — See all existing memories
- **Memory(action="read", name="...")** — Load a memory's full facts (shows fact indices 0, 1, 2, ...)
- **Memory(action="save", name="...", description="...", facts=[...], tags=[...], type="...", scope="project")** — Create new memory
- **Memory(action="append", name="...", fact="one discrete sentence")** — Add a fact to an existing memory
- **Memory(action="replace-fact", name="...", fact_index=N, fact="new text")** — Update a specific fact
- **Memory(action="delete-fact", name="...", fact_index=N)** — Remove a specific fact
- **Memory(action="delete", name="...")** — Delete entire memory entry

## Memory Types

- **user** — User preferences, corrections, working style (e.g., "user prefers table-driven tests")
- **feedback** — What to avoid, what worked/failed (e.g., "gorm caused performance issues, use raw SQL")
- **project** — Architecture decisions, constraints, current work (e.g., "JWT expires in 24h", "schema migration in progress")
- **reference** — Patterns, workarounds, project-specific conventions (e.g., "error wrapping convention uses pkg/errors")

## Step-by-Step Consolidation Process

### Step 1: Survey Existing Memories
Call Memory(action="list") to see what already exists. Review the session summary and identify which existing memories might be relevant. For those entries, call Memory(action="read", name="...") to load their full facts.

### Step 2: Identify Contradictions (Critical)
Compare what the session says against what existing memories claim. Examples:
- Memory says "always use X" but user said "never use X again" → contradiction
- Memory says "pattern Y is broken" but session fixed it → might need update
- Memory contains outdated guidance → should be deleted or updated

When you find a contradiction:
- Delete the entire memory: Memory(action="delete", name="...")
- OR remove just the contradicting fact: Memory(action="delete-fact", name="...", fact_index=N)
- Stale wrong memories are WORSE than no memories — be aggressive about cleanup.

### Step 3: Update Existing Memories with New Facts
If the session adds NEW information to an existing memory topic, use append:
Memory(action="append", name="...", fact="one new discrete sentence")

Never rewrite the whole memory. Only add facts that are:
- Genuinely new (not already in the facts)
- Discrete (one sentence, one idea)
- Specific and testable

### Step 4: Create New Memories for Genuinely New Learning
For patterns not covered by existing memories:
Memory(action="save", name="short-kebab-name", description="one sentence description", facts=["fact 1", "fact 2", "fact 3"], tags=["tag1", "tag2"], type="user|feedback|project|reference", scope="project")

Facts MUST be:
- One sentence each
- Discrete and specific
- NOT prose paragraphs

## What to Extract

Extract:
- User corrections and preferences
- Decisions made or changed this session
- Debugging techniques that worked
- Gotchas or workarounds discovered
- Architecture choices explained by the user
- Feedback on what to avoid or prefer

Do NOT extract:
- Things already visible in the code
- Standard best practices every developer knows
- Ephemeral decisions only relevant right now
- Anything already covered by existing memories

## Critical Rules

1. **NEVER use the Write tool for memories** — only use Memory tool actions
2. **NEVER update MEMORY.md directly** — it's auto-generated from memory entries
3. Each fact must be ONE discrete sentence, not a paragraph
4. Use append (update existing) over save (create new) whenever possible
5. If nothing worth saving → say "Nothing to consolidate." and stop
6. Contradiction detection is the MOST IMPORTANT step — wrong memories are worse than no memories

## Example Workflow

1. Memory(action="list") → See that "jwt-config" exists
2. Memory(action="read", name="jwt-config") → Reveals facts about JWT expiration
3. Session says "we removed JWT expiration limit" → Contradiction found
4. Memory(action="delete", name="jwt-config") → Delete stale memory
5. Session mentions "prefer arrow functions in tests" → New user preference
6. Memory(action="save", name="user-test-preferences", description="User's preferred testing patterns", facts=["User prefers arrow functions in test cases", "User likes table-driven test structure"], tags=["testing", "user-preference"], type="user", scope="project")

---

Now review the session summary above and consolidate memories. Start by listing existing memories.`, sessionSummary, projectDir)
}
