# TUI Refresh Plan — Opencode-Inspired

This document plans a comprehensive refresh of Claudio's TUI, taking design cues from opencode's web/Electron app. The goal is a more polished, contextual, and scalable terminal interface.

## Scope

Implement all items from the original proposal **except** follow-up suggestions, plus three additional user requests:
- Opencode-style **welcome screen** (big centered logo, prompt in middle, "tab agents / ctrl+p commands" hints, cwd + version in corners)
- Polished **inline `/` command palette** triggered from the prompt
- **No truncation** for thinking blocks or messages — content must wrap and scroll, never get cut

## Current state (what already works)

Claudio's TUI is already more complete than expected. Key existing pieces:
- **Welcome screen** — animated "claudio" wave logo + recent sessions box (`root.go:3958`)
- **Command palette** — already inline above the prompt, activates on `/` prefix (`commandpalette/palette.go`, trigger at `root.go:1269`)
- **Viewport search** — `/` in VIEWPORT focus (`root.go:4283`, `vpSearchActive`)
- **Task side panel** — `panels/taskspanel/tasks.go`
- **Permission dialog** — currently a centered modal overlay (`permissions/dialog.go`)
- **Split layout** — fixed `panelSplitRatio = 0.65` (`layout.go:14`)

This means several items are **refreshes** rather than new builds.

## Phase 1 — Quick wins (low risk, high visual impact)

### 1.1 Fix thinking/message truncation  `messages.go`

**Bug:** `messages.go:862` truncates thinking content to 200 chars:
```go
case MsgThinking:
    return styles.ThinkingStyle.Width(maxW).Render("💭 " + truncate(msg.Content, 200))
```

**Fix:**
- Remove `truncate(...)` — render full content with `Width(maxW)` for wrapping
- Add a header line: `Thinking: <first sentence or first 60 chars>` in orange italic (opencode style), then the full body below in muted text
- Audit all other `truncate()` calls in `messages.go` — any that truncate user-visible content should be removed

**Files:** `internal/tui/messages.go`

### 1.2 Collapsible thinking blocks

Even with no truncation, long thinking blocks bloat the viewport. Add per-message expand state:

- Add `thinkingExpanded map[int]bool` to root model (keyed by message index)
- Collapsed (default): `💭 Thinking: <header>  (ctrl+o to expand)`
- Expanded: full content wrapped to `maxW`
- Toggle key: `ctrl+o` when viewport cursor is on a thinking block (reuse existing viewport cursor navigation)

**Files:** `internal/tui/root.go`, `internal/tui/messages.go`

### 1.3 Agent color coding

Stable color per agent name using FNV hash.

**New file:** `internal/tui/styles/agent_colors.go`
```go
package styles

import (
    "hash/fnv"
    "github.com/charmbracelet/lipgloss"
)

var agentPalette = []lipgloss.Color{
    Primary, Secondary, Aqua, Orange, Warning, Success,
}

func AgentColor(name string) lipgloss.Color {
    h := fnv.New32a()
    h.Write([]byte(name))
    return agentPalette[h.Sum32()%uint32(len(agentPalette))]
}
```

**Apply in:**
- `messages.go` — tool call headers for subagent calls (when tool is `Task` or agent name available)
- `teampanel/panel.go` — team member badges and mailbox entries

### 1.4 Polish inline command palette

**Now:** Already inline, works on `/` prefix — but visuals are sparse.

**Changes to match opencode screenshot:**
- Give the **selected row** a filled background (use `Orange` or `SurfaceAlt`), not just bold text
- Tighten padding — remove `Padding(0, 2)` wrapper; align to prompt's left edge
- Two-column layout: `/name` in `Text` bold (18-col wide), `description` in `Dim`
- Don't truncate descriptions — wrap to next line with indent if needed
- Show up to 10 items (was 8)

**Files:** `internal/tui/commandpalette/palette.go`, `internal/tui/styles/theme.go`

---

## Phase 2 — Contextual docks

**Concept:** A "dock" is a component that occupies the space between the viewport and the prompt. Only one dock is active at a time. Priority (high → low):
1. Permission request
2. Ask-user question
3. Plan approval
4. Todo list (passive — always shown when in-progress tasks exist)

Implements the "inline, non-modal" pattern from opencode's session-composer region.

### 2.1 Dock interface

**New package:** `internal/tui/docks/`

```go
// docks/dock.go
package docks

type Dock interface {
    IsActive() bool
    SetWidth(int)
    View() string
    Update(tea.Msg) (Dock, tea.Cmd)
}
```

### 2.2 Permission dock (replaces modal)

**Now:** Centered modal overlay (`permissions/dialog.go:160 lipgloss.Place center`) blocking viewport.

**Change:**
- Add `Inline bool` field to `permissions.Model`
- When `Inline=true`, render as a horizontal bar: left = warning icon + tool summary, right = buttons
- Single line for common cases (Bash), expand to 3-4 lines for Write/Edit with diff preview
- Remove `lipgloss.Place` wrapper in inline mode
- In `root.go` View(), place permission output in the dock slot instead of as an overlay on vpView

**Layout:**
```
┌─ viewport (full height up to prompt) ─┐
│ ...conversation...                    │
├───────────────────────────────────────┤
│ ⚠ Bash · $ rm -rf build/              │  ← dock
│ [Allow] [Deny] [Always] [All Bash]    │
├───────────────────────────────────────┤
│ > prompt                              │
└───────────────────────────────────────┘
```

**Files:** `internal/tui/permissions/dialog.go`, `internal/tui/root.go`

### 2.3 Todo dock

**New file:** `internal/tui/docks/todo_dock.go`

- Pulls active todos from the same data source as `taskspanel`
- Only shows when at least one task is in-progress
- Collapsed (default, 1 line): `Tasks: [✓] analyze  [◐] write tests  [ ] run lint   (ctrl+t expand)`
- Expanded (up to 6 lines): bullet list with statuses

Lower priority than permission/question docks — only claims the slot when nothing else does.

**Files:** `internal/tui/docks/todo_dock.go`, `internal/tui/root.go`

### 2.4 Dock slot integration in View()

In `root.go:4096 View()`, replace the current overlay approach for permission with:
```go
// After viewport, before palette/prompt
if dock := m.activeDock(); dock != nil {
    sections = append(sections, dock.View())
}
```

`activeDock()` returns the highest-priority active dock, or nil.

**Files:** `internal/tui/root.go`

---

## Phase 3 — Layout & input polish

### 3.1 Resizable side panel

**Now:** `layout.go:14` has `const panelSplitRatio = 0.65`.

**Change:**
- Move to `Model.panelSplitRatio float64` field (default 0.65)
- `<` and `>` keys in main area (not in prompt insert mode) nudge by 0.05
- Clamp to [0.4, 0.85]
- Respect existing `panelMinWidth = 30` guard
- Persist the ratio to config under `tui.panelSplitRatio` — load on startup

**Files:** `internal/tui/layout.go`, `internal/tui/root.go`, `internal/config/` (add field)

### 3.2 Prompt context pills

**Now:** Images show as `📎 2 image(s): file.png` below the textarea. Paste refs are inline text. No visibility for @file mentions or memory anchors.

**Change:** New pills row rendered **above** the textarea (inside the same bordered prompt box):

```
┌ prompt ─────────────────────────────────────────┐
│ 📎 report.pdf   📁 src/main.go   🧠 user prefs   │  ← pills
│ ─────────────────────────────────────────────── │
│ > implement the thing                            │  ← textarea
└──────────────────────────────────────────────────┘
```

**New file:** `internal/tui/prompt/pills.go`

Pill types:
- **Image** — from existing `m.images`
- **Paste** — from `m.pastedContents` (show `[#1 +42 lines]`)
- **File** — parse `@path/to/file` mentions from current textarea value
- **Memory** — detect references via memory service (if any match text)

Each pill is styled as `lipgloss.NewStyle().Background(SurfaceAlt).Foreground(Text).Padding(0,1).MarginRight(1)` with a leading icon.

Dismissal: when focused on pills row (future enhancement), `x` removes the selected pill.

**Files:** `internal/tui/prompt/prompt.go`, `internal/tui/prompt/pills.go`, `internal/tui/styles/theme.go`

### 3.3 Welcome screen refresh

**Now:** Animated wave logo + subtitle + hint line + recent sessions box.

**New design (matches opencode screenshot):**

```
                                                        
                  ██████  ██                            
                 ██       ██  █████  ██ ██ ██████       
                 ██       ██ ██   ██ ██ ██ ██  ██       
                  ██████  ██  █████  ██ ██ ██████       
                                                        
  ┌───────────────────────────────────────────────────┐ 
  │ Ask anything... "Fix the failing tests"           │ 
  │                                                   │ 
  │ Build · claude-opus-4-6 · Anthropic               │ 
  └───────────────────────────────────────────────────┘ 
                                                        
                          tab agents   ctrl+p commands  
                                                        
 /Users/abraxas/Personal/claudio              v0.5.2    
```

**Implementation:**
- Extract `welcomeScreen()` to a new file `internal/tui/welcome.go`
- Render order: top padding → big block logo → spacer → centered prompt (reuse real prompt widget with a placeholder) → hints row → spacer → bottom row (cwd left, version right)
- Keep the animated color wave but apply it to a block ASCII rendering of "claudio" (or use `charmbracelet/bubbletea` examples for block glyphs)
- Keep recent sessions accessible via `<Space>.` (don't clutter the welcome with them — simplified)
- Fall back to the compact layout when terminal height < 20 rows

**Files:** `internal/tui/welcome.go` (new), `internal/tui/root.go` (call into it), `internal/tui/logo_test.go` (update expectations)

### 3.4 `/` command trigger (already works)

Already implemented at `root.go:1269`. No changes needed beyond the visual polish in 1.4.

---

## Phase 4 — New features

### 4.1 File changes panel

**New file:** `internal/tui/panels/filespanel/files.go`

Implements the `panels.Panel` interface. Tracks files touched by tool calls in the current session:

**Data model:**
```go
type FileEntry struct {
    Path   string
    Status FileStatus  // Added | Modified | Read
    Count  int         // number of operations
}
type Model struct {
    entries []FileEntry
    ...
}
```

**Source:** Walk `m.messages` for `MsgToolCall` entries and inspect `ToolInputRaw`:
- `Write` → Added (if file didn't exist) or Modified
- `Edit`, `MultiEdit` → Modified
- `Read`, `Grep`, `Glob` → Read

Update on every new message or recompute lazily on panel open.

**Rendering:** Tree-style with colored indicators:
```
 Files (12)
 ├ + internal/tui/docks/
 │  ├ + dock.go
 │  └ + todo_dock.go
 ├ ~ internal/tui/root.go      (4)
 ├ ~ internal/tui/messages.go  (2)
 └ r internal/api/stream.go
```

Colors: `+` green, `~` yellow, `r` dim.

**Activation:** Add to command palette as `/files` or hotkey `Ctrl+F`.

**Files:** `internal/tui/panels/filespanel/files.go`, `internal/tui/root.go` (register panel)

### 4.2 Message search polish

**Now:** Already exists (`vpSearchActive`, `renderSearchBar`).

**Audit:**
- Verify `/` enters search mode from VIEWPORT focus
- `n`/`N` navigate matches
- Highlight matches in viewport content
- `Esc` exits search

If these all work, just document. If not, fix gaps.

**Files:** `internal/tui/root.go` (audit/fixes)

### 4.3 Message-level revert (largest item)

**Scope:** Allow reverting file changes made during a specific assistant turn.

**Prerequisites:**
- Git checkpoint metadata in session storage: store `git rev-parse HEAD` before each user message in `storage.Message` (new column via migration)
- If the repo is dirty, stash the state (or fail gracefully)

**New migration:** `internal/storage/migrations/00XX_add_checkpoint.sql`
```sql
ALTER TABLE messages ADD COLUMN checkpoint_sha TEXT;
```

**UI:**
- In viewport navigation mode, `r` on a user message triggers a revert dialog
- Dialog shows: "Revert to state before this message? (n files changed)"
- Options: [Revert] [Cancel]
- On confirm: checkout files touched by tool calls in the window `[user_msg, next_user_msg)` to the checkpoint SHA
- Show toast confirming the revert

**Files:** `internal/storage/migrations/`, `internal/session/`, `internal/tui/root.go`, new dock or dialog for confirm

**Risks:** Touches persistence layer; users with uncommitted work need safe handling. Gate behind `tui.enableRevert = true` in config.

---

## Recommended implementation order

| Order | Phase | Items | Rationale |
|-------|-------|-------|-----------|
| 1 | 1.1, 1.2 | Truncation fix, collapsible thinking | One-line fix + small addition; biggest user-visible win |
| 2 | 1.4 | Palette polish | Quick visual improvement |
| 3 | 1.3 | Agent colors | Low effort, helps multi-agent UX |
| 4 | 3.3 | Welcome refresh | Brand polish, self-contained |
| 5 | 2.1, 2.2 | Dock interface + permission dock | Foundational for docks |
| 6 | 2.3, 2.4 | Todo dock + slot wiring | Completes the dock system |
| 7 | 3.1 | Resizable panel | Config plumbing |
| 8 | 3.2 | Prompt pills | Polishes input area |
| 9 | 4.1 | Files panel | New panel, isolated |
| 10 | 4.2 | Search audit | Cleanup |
| 11 | 4.3 | Revert | Riskiest — storage migration |

## Testing strategy

Each phase should land with:
- Unit tests for new components (dock view output, pill rendering, agent color stability)
- Golden-file tests for welcome screen at several widths
- Update `logo_test.go` for new logo rendering
- Integration test in `delete_interaction_test.go` pattern for dock priority
- Manual verification: `make build` then run against a real session

## Non-goals

- Follow-up suggestions (explicitly excluded)
- Re-theming beyond adding agent colors
- Changing the underlying bubbletea/lipgloss stack
- Rewriting the existing vim mode or paste handling
