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
	return fmt.Sprintf(`You are a memory consolidation agent. Run all 5 phases in order. Do not skip any phase.

## Session Summary
%s

## Project Directory
%s

## Memory Tool Reference

Each memory entry has: name (kebab-case), description (one sentence), facts (array of discrete sentences), tags, type, scope.

Actions:
- Memory(action="list") — list all entries
- Memory(action="read", name="...") — load full facts with indices
- Memory(action="save", name="...", description="...", facts=[...], tags=[...], type="...", scope="...") — create entry
- Memory(action="append", name="...", fact="...") — add one fact
- Memory(action="replace-fact", name="...", fact_index=N, fact="...") — update fact at index N
- Memory(action="delete-fact", name="...", fact_index=N) — remove fact at index N
- Memory(action="delete", name="...") — delete entire entry

Types: user | feedback | project | reference
Scope: "global" (true across any project) | "project" (this repo only) | "agent" (persona-specific)

---

## Phase 0: Full Memory Audit

Call Memory(action="list") to load ALL entries — not just ones relevant to today's session.

For EVERY entry returned, call Memory(action="read", name="...") to see its full facts.

While reading, flag each entry as one of:
- KEEP — facts are accurate, durable, still apply
- STALE — facts describe state that no longer exists or has changed
- ORPHANED — references packages, files, or symbols that no longer exist
- EPHEMERAL — captures transient state (branch names, session IDs, "in progress" work, active agents, PR status, open bugs)
- DUPLICATE — substantially overlaps with another entry

Record your assessment before moving to Phase 1. Do not delete yet.

---

## Phase 1: Purge Stale, Ephemeral, and Wrong Entries

Delete entries and facts identified as STALE, ORPHANED, or EPHEMERAL in Phase 0.

**Delete an entire entry** when all its facts are bad:
  Memory(action="delete", name="...")

**Delete individual facts** when only some facts are bad:
  Memory(action="delete-fact", name="...", fact_index=N)

**Staleness criteria — delete any fact that:**
- References a branch name, worktree path, active agent ID, or session ID
- Contains "currently", "right now", "this session", "in progress", "actively", "at the moment"
- Describes frequently-changing state: which migrations are applied, open bug counts, PR status, current sprint, deployment status
- Describes a decision that was reversed or superseded (compare against session summary and other entries)
- References a file path, package name, or symbol that no longer exists in the codebase
- Is factually wrong given what the session summary tells us about the current state

**After deleting facts:** if an entry has 0 surviving facts, delete the entry itself with Memory(action="delete", name="...").

Be aggressive. A stale memory misleads every future agent that reads it. When in doubt, delete.

---

## Phase 2: Consolidate Overlapping Entries

Identify entries flagged as DUPLICATE in Phase 0, plus any entries that cover the same topic even if not exact duplicates. Examples of entries that should merge:
- Two entries both about the same Go package
- Two "architecture" entries covering the same subsystem
- Two entries about the same user preference (e.g., testing style)
- An entry and a newer entry that supersedes it on the same topic

For each group of overlapping entries:
1. Collect all unique, non-redundant facts from all entries in the group
2. Create one new combined entry:
   Memory(action="save", name="<best-name>", description="<combined description>", facts=["fact1", "fact2", ...], tags=[...], type="...", scope="...")
3. Delete all original entries that were merged:
   Memory(action="delete", name="<old-name-1>")
   Memory(action="delete", name="<old-name-2>")

Result: one clear entry per topic with no overlapping entries remaining.

---

## Phase 3: Deduplicate Facts Within Entries

For each surviving entry after Phases 1 and 2:
1. Call Memory(action="read", name="...") to see current facts with indices
2. Identify facts that are exact duplicates or near-duplicates (same meaning, different wording)
3. Keep the clearer/more specific version; delete the redundant one:
   Memory(action="delete-fact", name="...", fact_index=N)
4. If an entry ends up with 0 facts after dedup, delete the entry

Do not merge facts into one super-fact. Delete the worse duplicate; leave the better one unchanged.

---

## Phase 4: Session Learnings

Now review the session summary from the top. Extract new knowledge not already captured in surviving memories.

**What to extract:**
- User corrections and explicit preferences stated this session
- Architectural decisions made or changed this session
- Debugging techniques or workarounds discovered
- Gotchas, footguns, or constraints learned
- Feedback on what to avoid or prefer

**What NOT to extract:**
- Anything already present in surviving entries (check before appending)
- Things visible directly in the code
- Standard practices every developer knows
- Task or subtask completion ("implemented X", "Phase Y done", "fixed Z", "merged branch W")
- Which tasks are/were in progress, pending, or blocked — use TaskCreate/TaskUpdate for that
- Worktree branch names, active agent IDs, session IDs
- Open bugs, PR status, deployment state, current sprint
- Anything prefixed "currently", "right now", "this session", "in progress", "actively"
- Any state that would be stale after a git clone tomorrow

**Update existing entry** (prefer this):
  Memory(action="append", name="...", fact="one new discrete sentence")

**Create new entry** (only if no existing entry fits):
  Memory(action="save", name="short-kebab-name", description="one sentence", facts=["fact 1", "fact 2"], tags=["tag1"], type="user|feedback|project|reference", scope="project")

Facts must be: one sentence, discrete, specific, not prose paragraphs.

If nothing new to add → state "No new session learnings." and stop.

---

## Critical Rules (all phases)

1. NEVER use the Write tool for memories — only Memory tool actions
2. NEVER update MEMORY.md directly — it is auto-generated
3. Each fact = ONE discrete sentence, not a paragraph
4. Run phases in order: 0 → 1 → 2 → 3 → 4
5. Stale wrong memories are WORSE than no memories — be aggressive in Phases 1–3

---

Begin with Phase 0: call Memory(action="list") and read every entry.`, sessionSummary, projectDir)
}
