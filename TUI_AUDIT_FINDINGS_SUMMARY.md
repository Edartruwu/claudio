# TUI UX Audit Report Findings - Comprehensive Summary

## Documents Found

### 1. **TUI_AUDIT_REPORT.md** (23,046 bytes)
   - Location: `/Users/abraxas/Personal/claudio/TUI_AUDIT_REPORT.md`
   - Type: Comprehensive audit report
   - Date: 2024
   - Overall Score: **8.2/10** — Production-ready implementation
   - Auditor: Orion TUI Structure Investigation
   - **Key Focus:** 8-area Bubbletea + Lipgloss best practices audit

### 2. **AUDIT_REPORT.md** (30,802 bytes)
   - Location: `/Users/abraxas/Personal/claudio/AUDIT_REPORT.md`
   - Type: Detailed best practices audit
   - Scope: Bubbletea + Lipgloss review
   - Assessment: 8-area evaluation (8.2/10 overall)
   - **Key Focus:** Component composition, keybinding patterns, style usage

### 3. **tui-refresh-plan.md** (12+ KB)
   - Location: `/Users/abraxas/Personal/claudio/docs/tui-refresh-plan.md`
   - Type: Strategic UX refresh roadmap
   - Scope: Opencode-inspired UI improvements (Phases 1-4)
   - **Key Focus:** Visual polish, dock system, layout improvements, new features

---

## TUI Audit Findings - Executive Summary

### Overall Health: 8.2/10 (Production-Ready) ✅

**Strengths:**
- ✅ Perfect Elm Architecture (TEA) pattern implementation
- ✅ Zero blocking I/O; event-driven async throughout
- ✅ Sophisticated viewport navigation with Section caching
- ✅ Clean component encapsulation (10+ sub-models)
- ✅ Responsive layout with dynamic sizing
- ✅ Excellent modal/overlay management
- ✅ Well-organized color palette (12 base colors)

**Areas for Improvement:**
- ⚠️ Style object allocation (PRIMARY optimization opportunity)
- ⚠️ Light terminal color support (adaptive colors)
- ⚠️ Large Update function (939 lines)
- ⚠️ Debug file creation on every render
- ⚠️ Testing gaps in viewport navigation

---

## Key Audit Findings (Detailed)

### 1. Model/Update/View Separation: 9/10 ✅ EXCELLENT

**Finding:** The codebase follows the Elm Architecture perfectly. All 10 sub-models implement the pattern correctly.

**Sub-models identified:**
| Model | File | Role |
|-------|------|------|
| `prompt.Model` | `prompt/prompt.go:26` | Multi-line textarea with Vim mode, paste detection, image attachment support |
| `whichkey.Model` | `panels/whichkey/whichkey.go` | Custom help system with context-aware bindings |
| `ModelSelector` | `modelselector/selector.go` | LLM model picker overlay |
| `AgentSelector` | `agentselector/selector.go` | Agent persona picker overlay |
| `TeamSelector` | `teamselector/selector.go` | Team template picker overlay |
| `FilePicker` | `filepicker/picker.go` | File selection dialog |
| `CommandPalette` | `commandpalette/palette.go` | Slash-command explorer |
| `SessionPanel` | `panels/sessions/sessions.go` | Session manager (Telescope-style) |
| `SpinnerModel` | `components/spinner.go` | Animated loading indicator |
| Panel interface | `panels/panel.go:9` | Interface for all 8 side panels |

**Quality Assessment:**
- ✅ Clear Model/Update/View separation in all components
- ✅ Proper message passing between models
- ✅ Each component owns its state
- ✅ No God objects — responsibilities well-distributed
- ✅ All I/O happens in tea.Cmd callbacks
- ✅ All state mutations in Update()
- ✅ View() is pure: no side effects, no database calls

---

### 2. Lipgloss Usage: 6/10 ⚠️ GOOD BUT SUBOPTIMAL (PRIMARY OPTIMIZATION)

**CRITICAL FINDING:** Style objects are recreated on every render cycle.

**Current Problem:**
- **50+ `lipgloss.NewStyle()` calls in View methods** across:
  - `root.go` (28 calls in render functions)
  - `panels/whichkey/whichkey.go` (4 calls per frame)
  - `permissions/dialog.go` (3+ calls)
  - `prompt/prompt.go` (styles inlined in textarea styling)
  - And 8+ other files (sidebar/blocks/*, agentselector/*, etc.)
  
- **TOTAL COUNT:** 364 instances of `lipgloss.NewStyle()` found in codebase
- **Impact:** ~100+ style allocations per render cycle (typically 20–100 Hz depending on activity)
- **Memory Pressure:** Unnecessary GC activity during interactive use

**Example - Current (Bad):**
```go
func (m Model) renderAskUserDialog(width int) string {
    titleStyle := lipgloss.NewStyle().Foreground(styles.Aqua).Bold(true)  // RECREATED EVERY FRAME
    labelStyle := lipgloss.NewStyle().Foreground(styles.Text).Bold(true)  // RECREATED EVERY FRAME
    dimStyle := lipgloss.NewStyle().Foreground(styles.Dim)                 // RECREATED EVERY FRAME
    // ... more NewStyle() calls in render methods
}
```

**Recommendation:**
```go
// Extract to styles/theme.go (allocated ONCE at startup)
var AskUserTitle = lipgloss.NewStyle().Foreground(Aqua).Bold(true)
var AskUserLabel = lipgloss.NewStyle().Foreground(Text).Bold(true)
var AskUserDim = lipgloss.NewStyle().Foreground(Dim)
```

**Estimated Impact:** 
- **~30% reduction in render-time allocations**
- Effort: 2–3 hours
- Priority: **HIGH**
- Status: **Not yet implemented** — identified in audit but no in-code comments found

**Color Palette Status:**
- ✅ **Good:** Centralized in `styles/theme.go`
- ✅ **Good:** Uses 12 base colors + 40 pre-defined styles
- ⚠️ **Gap:** No `lipgloss.LightDark()` implementation for light terminals
- ⚠️ **Gap:** No `tea.RequestBackgroundColor` in Init() or handler in Update()
- **Impact:** Hardcoded to Gruvbox dark; unusable on light terminal backgrounds

---

### 3. Component Composition: 9/10 ✅ EXCELLENT

**Finding:** Proper encapsulation with clean message-based delegation. No monolithic component.

**Panel Architecture:**
- **Interface:** `panels/panel.go:9` with proper contract
- **8 Panel implementations** — all follow the same interface
- **Message-based delegation:** Clean ActionMsg structs
- **Proper sub-model routing** — all sized in root.Update()

**Panel System Excellence:**
- ✅ Custom `Panel` interface allows hot-swappable secondary UI areas
- ✅ 8 Panel implementations (Sessions, Analytics, Config, Files, Memory, Skills, Tasks, Tools)
- ✅ Clean focus management: `root.Update(tea.KeyMsg)` → routes to focused component
- ✅ No state leakage between panels
- ✅ Proper cleanup on panel close

---

### 4. Key Bindings: 7/10 ⚠️ CUSTOM BUT FUNCTIONAL

**Finding:** Using custom `msg.String()` pattern instead of `bubbles/key.Binding`.

**Current Pattern (30+ instances):**
```go
// root.go:568, 708, 725, 750+ and other files
switch msg.String() {
case "shift+tab":
    m.cyclePermissionMode()
case "ctrl+c":
    // cancel streaming
case "ctrl+o":
    // toggle tool group
case "ctrl+p":
    // toggle palette
case "/":
    // enter search mode
// ... 20+ more cases
}
```

**Why bubbles/key would be better:**
- Auto-generates help text from key.Binding definitions
- Enables dynamic keybinding configuration
- Consistent with Charmbracelet ecosystem standards

**Why this approach works for Claudio:**
- ✅ Custom `whichkey` system already generates help dynamically
- ✅ Keybindings are stable (not user-configurable)
- ✅ Less boilerplate than key.Binding setup
- ✅ Works well for this specific use case

**Assessment:** Document this architectural decision. No change required unless ecosystem consistency is a future goal.

---

### 5. Loading/Async Patterns: 10/10 ✅ PERFECT

**Finding:** Perfect async/event-driven architecture. ZERO blocking calls in Update().

**Event Channel Pattern:**
```go
// root.go:132
eventCh chan tuiEvent  // capacity 64 (buffer prevents blocking)

// root.go:1754–1763: Background engine execution
go func() {
    var err error
    if hasAttachments {
        blocks := BuildContentBlocks(apiText, fileAttachments, imageBlocks)
        err = m.engine.RunWithBlocks(ctx, blocks)  // blocking, in goroutine
    } else {
        err = m.engine.Run(ctx, apiText)  // blocking, in goroutine
    }
    m.eventCh <- tuiEvent{typ: "done", err: err}  // sends result via channel
}()

timerTick := tea.Tick(time.Second, func(time.Time) tea.Msg { return timerTickMsg{} })
return m, tea.Batch(m.spinner.Tick(), m.waitForEvent(), timerTick)  // non-blocking
```

**Handler Interfaces:**
- ✅ Engine events → `handleEngineEvent()` (root.go:1769)
- ✅ Tool execution → `handleEngineEvent()` for tool_use/tool_result messages
- ✅ Sub-agent events → `handleTeammateEvent()` (root.go:3686) for real-time team collaboration
- ✅ Permission prompts → approval channel with async response

**Result:** Event loop never blocks; responsive UI during streaming, API calls, sub-agent execution.

---

### 6. Layout System: 9/10 ✅ EXCELLENT

**Finding:** Sophisticated responsive layout using lipgloss.JoinVertical/Horizontal + Place. Handles dynamic width/height constraints well.

**Multi-section Vertical Layout (root.go:4720–4927):**
1. Viewport (messages + inline spinners)
2. Overlays (modals placed on top)
3. Sidebar (left) + Main pane (center) + Panel (right)
4. Status bar (bottom)
5. Prompt (bottom input area)

**Responsive Sizing Logic:**
- ✅ Window size → calculate main width accounting for panels
- ✅ Panel width = 35% of total (configurable via `panelSplitRatio`)
- ✅ Minimum panel width enforced: 30 chars
- ✅ Sidebar width dynamic: ~40% of main width

**Overlay Composition Pattern:**
```go
// root.go:4751–4784
if m.focus == FocusPlanApproval {
    overlay := m.renderPlanApprovalDialog(mw)
    vpView = placeOverlay(vpView, overlay, mw, m.viewport.Height)
}
// placeOverlay uses lipgloss.Place to center the modal
```

**Design Choice - No Alt Screen:**
- Claudio runs in inline mode, not full-screen alt-screen
- This is **intentional** design choice (allows inline output, session persistence)
- NOT an issue for the app's UX goals

---

### 7. Missing Features / Suboptimal Patterns: 8/10 ⚠️ MOSTLY GOOD

**Help bar / Status bar:**
- ✅ Dedicated `renderStatusBar()` (root.go:4929)
- ✅ Data-driven via `StatusBarState` struct
- ✅ Context-aware hints (varies by focus state)
- ✅ Model + tokens + cost display
- ⚠️ Not using `bubbles/help.Model` (uses custom `whichkey` instead — superior UX)

**Missing Standard Patterns:**
1. ❌ **No `tea.EnterAltScreen`** — By design; app runs inline
2. ❌ **No `bubbles/help.Model`** — Using custom `whichkey` instead (superior UX for this app)
3. ✅ **Spinner** — Implemented (components/spinner.go)
4. ✅ **Viewport scrolling** — Sophisticated section-based navigation
5. ✅ **Textarea** — bubbles/textarea used in prompt
6. ❌ **No bubbles/list or bubbles/table** — Custom implementations for sessions/skills/tasks
   - Reason: Need specialized rendering (Glamour, agent status, etc.)

**Viewport Search Feature (root.go:890–930):**
- ✅ Enter with "/"
- ✅ Match highlighting
- ✅ n/N for next/prev
- ✅ Esc to exit
- ⚠️ Search not persisted across sessions

**Section Metadata Caching (messages.go:69–74):**
```go
type Section struct {
    MsgIndex    int  // message array index
    IsToolGroup bool // is this a tool group?
    LineStart   int  // rendered line number
    LineCount   int  // height of this section
}
```
✅ Enables O(1) cursor navigation in large viewports

---

### 8. Modal Dialog Patterns: 9/10 ✅ EXCELLENT

**Finding:** Clean, non-destructive modal composition with proper focus routing.

**Modals Implemented:**
1. **Plan Approval Dialog** (root.go:2774–2852)
   - Displayed after `/plan-mode` EnterPlanMode call
   - Shows 4 options: Approve, Modify, Reject, Cancel
   - Cursor navigation with visual highlighting
   - FocusPlanApproval routes keys to `handlePlanApprovalKey()`

2. **AskUser Dialog** (root.go:2660–2772)
   - Multi-question flow with tracked answers
   - Single-select, multi-select, and free-text modes
   - Option cursor management
   - Sends response back to tool via `responseCh`

3. **Permission Approval** (permissions/dialog.go)
   - Tool execution confirmation
   - "Allow always" + "Deny" + "Cancel" options
   - Persistent rule saving

4. **Model/Agent/Team Selectors**
   - List-style pickers with arrow navigation
   - Filtered search capability

**State management:** ✅ Clean
- `m.focus` gates which modal receives input
- `m.askUserDialog` (non-nil when active), `m.agentDetail` (overlay state)
- Proper cleanup on close: focus restored, dialog nulled

---

## Risk Assessment Summary

### 🔴 HIGH PRIORITY ISSUES

1. **Style Object Allocations** (root.go, panels/whichkey/whichkey.go, permissions/dialog.go, +8 more files)
   - **Issue:** 50+ `lipgloss.NewStyle()` calls in View methods; 364 total in codebase
   - **Impact:** Memory pressure, render performance degradation
   - **Fix:** Extract to `styles/theme.go` (30 derived styles)
   - **Estimated Impact:** ~30% reduction in render-time allocations
   - **Effort:** 2–3 hours
   - **Status:** Identified in audit, not yet implemented
   - **Risk Level:** Low (isolated change, no behavior changes)

2. **No Adaptive Colors for Light Terminals**
   - **Issue:** Styles hardcoded to Gruvbox dark palette
   - **Impact:** TUI becomes unusable on light backgrounds
   - **Fix:** Implement `tea.RequestBackgroundColor` in Init() + handler in Update()
   - **Effort:** 1–2 hours
   - **Status:** Identified in audit, not yet implemented
   - **Risk Level:** Low (additive feature)

### 🟡 MEDIUM PRIORITY ISSUES

3. **Debug File Creation** (root.go, multiple locations)
   - **Issue:** `/tmp/claudio-viewport-debug.txt` written every render cycle
   - **Should be:** Conditional on `--debug` flag
   - **Impact:** Unnecessary disk I/O
   - **Effort:** 30 min
   - **Risk Level:** Very low

4. **Large Update Function** (root.go:521–1459, 939 lines)
   - **Status:** Readable with clear switch cases; not blocking
   - **Assessment:** Well-organized, clear intent
   - **Nice-to-have:** Extract sub-handlers to separate functions for further clarity
   - **Effort:** 1–2 hours

5. **Sidebar Rebuilding** (sidebar/sidebar.go)
   - **Issue:** Layout recalculated every frame
   - **Fix:** Cache block closures
   - **Impact:** Microseconds; very low ROI

### 🟢 LOW PRIORITY ISSUES

6. **Vim Mode Integration** (prompt/vim.go, prompt.go)
   - Complex state machine; well-tested
   - No issues found

7. **Sub-agent Integration** (teampanel, root.go:3686)
   - Clean event handling via `handleTeammateEvent()`
   - Proper state tracking
   - No issues found

---

## TUI Refresh Plan (Strategic Roadmap)

### Purpose
Comprehensive refresh of Claudio's TUI, taking design cues from opencode's web/Electron app for modern, polished UX.

### Four-Phase Implementation Plan

**Phase 1 — Quick wins (low risk, high visual impact)**
- 1.1 Fix thinking/message truncation (`messages.go`)
- 1.2 Collapsible thinking blocks
- 1.3 Agent color coding (FNV hash-based stable colors)
- 1.4 Polish inline command palette

**Phase 2 — Contextual docks**
- 2.1 Dock interface (`docks/dock.go`)
- 2.2 Permission dock (replaces modal)
- 2.3 Todo dock
- 2.4 Dock slot integration in View()

**Phase 3 — Layout & input polish**
- 3.1 Resizable side panel
- 3.2 Prompt context pills
- 3.3 Welcome screen refresh
- 3.4 `/` command trigger (already works)

**Phase 4 — New features**
- 4.1 File changes panel (`panels/filespanel/files.go`)
- 4.2 Message search polish
- 4.3 Message-level revert (largest item — storage migration)

### Recommended Implementation Order
| Order | Phase | Items | Priority |
|-------|-------|-------|----------|
| 1 | 1.1, 1.2 | Truncation fix, collapsible thinking | ⭐⭐⭐⭐⭐ High visual impact |
| 2 | 1.4 | Palette polish | ⭐⭐⭐⭐ Quick win |
| 3 | 1.3 | Agent colors | ⭐⭐⭐⭐ Low effort, big UX improvement |
| 4 | 3.3 | Welcome refresh | ⭐⭐⭐⭐ Brand polish, self-contained |
| 5 | 2.1, 2.2 | Dock interface + permission dock | ⭐⭐⭐ Foundational |
| 6 | 2.3, 2.4 | Todo dock + slot wiring | ⭐⭐⭐ Completes dock system |
| 7 | 3.1 | Resizable panel | ⭐⭐⭐ Config plumbing |
| 8 | 3.2 | Prompt pills | ⭐⭐ Polish |
| 9 | 4.1 | Files panel | ⭐⭐ New panel, isolated |
| 10 | 4.2 | Search audit | ⭐⭐ Cleanup |
| 11 | 4.3 | Revert | ⭐ Risky — storage migration |

---

## Testing Coverage Status

**✅ Tested areas:**
- Interaction deletion (`delete_interaction_test.go`)
- Plan approval dialog (`planmode_test.go`)
- Vim mode (`vim_test.go`, `prompt_vim_test.go`)
- Input sanitization (`sanitize_test.go`)
- Image attachment handling (`attachments_test.go`)
- Logo animation (`logo_test.go`)

**⚠️ Missing coverage:**
- Viewport cursor navigation (Section-based jumps)
- Modal overlay composition
- Layout size calculations (splitLayout, mainWidth)
- Focus routing (9 focus states)

**Recommendation:** Add tests for viewport navigation and modal focus routing (1–2 hours each, 80% coverage gain).

---

## Code Quality Scorecard

| Area | Score | Status | Notes |
|------|-------|--------|-------|
| Model/Update/View Separation | 9/10 | ✅ Excellent | Perfect TEA pattern |
| Lipgloss Usage | 6/10 | ⚠️ Fix style caching | 364 NewStyle() calls, many recreated |
| Component Composition | 9/10 | ✅ Excellent | 10 sub-models, clean delegation |
| Key Bindings | 7/10 | ⚠️ Functional | Custom pattern, not bubbles/key |
| Async/Blocking | 10/10 | ✅ Perfect | Zero blocking I/O |
| Layout System | 9/10 | ✅ Excellent | Responsive, sophisticated |
| Missing Features | 8/10 | ⚠️ Mostly good | No alt-screen, custom whichkey |
| Modal Patterns | 9/10 | ✅ Excellent | Clean, non-destructive |
| **OVERALL** | **8.2/10** | **Production-ready** | One optimization opportunity |

---

## Key Takeaways

### What's Working Well ✅
1. **Excellent architecture** — Perfect Elm Architecture implementation
2. **Non-blocking I/O** — Event-driven async throughout, zero blocking
3. **Clean components** — 10+ well-encapsulated sub-models
4. **Sophisticated layout** — Dynamic sizing, responsive design
5. **Modal management** — Clean, non-destructive overlays
6. **Performance** — Sophisticated caching (Section metadata for O(1) navigation)

### What Needs Attention ⚠️
1. **Style caching** — 364 NewStyle() calls, many recreated per frame
   - **Fix:** Extract 30 derived styles to theme.go
   - **Impact:** 30% render performance improvement
   - **Effort:** 2–3 hours
   - **Priority:** HIGH

2. **Light terminal support** — No adaptive color detection
   - **Fix:** Implement tea.RequestBackgroundColor
   - **Impact:** TUI usable on light backgrounds
   - **Effort:** 1–2 hours
   - **Priority:** HIGH

3. **Testing gaps** — Missing viewport navigation and layout tests
   - **Fix:** Add 1-2 hour tests for coverage
   - **Impact:** 80% coverage increase
   - **Priority:** MEDIUM

### Strategic Direction 🎯
The **TUI Refresh Plan** provides a clear 4-phase roadmap for modernizing the UI with design cues from opencode. Recommended implementation order prioritizes high-impact, low-risk items (thinking truncation, collapsible blocks, agent colors) before foundational dock system work.

---

## File References

### Main Audit Documents
- `/Users/abraxas/Personal/claudio/TUI_AUDIT_REPORT.md` (23 KB) — Comprehensive audit
- `/Users/abraxas/Personal/claudio/AUDIT_REPORT.md` (31 KB) — Detailed best practices
- `/Users/abraxas/Personal/claudio/docs/tui-refresh-plan.md` (12 KB) — Strategic roadmap

### Key Source Files
- `/Users/abraxas/Personal/claudio/internal/tui/root.go` — Main model (5,341 lines)
- `/Users/abraxas/Personal/claudio/internal/tui/styles/theme.go` — Color palette
- `/Users/abraxas/Personal/claudio/internal/tui/messages.go` — Message rendering
- `/Users/abraxas/Personal/claudio/internal/tui/layout.go` — Layout logic
- `/Users/abraxas/Personal/claudio/internal/tui/prompt/prompt.go` — Input component
- `/Users/abraxas/Personal/claudio/internal/tui/panels/panel.go` — Panel interface

### Code Instances
- **Style allocations:** 364 instances of `lipgloss.NewStyle()` across:
  - root.go (28 calls in render functions)
  - panels/whichkey/whichkey.go (4 calls per frame)
  - permissions/dialog.go (3+ calls)
  - prompt/prompt.go (inlined styles)
  - sidebar/blocks/* (todos.go, files.go, tokens.go)
  - agentselector/selector.go, modelselector/selector.go, etc.

---

## Open Questions from Audit

1. **Why no `tea.EnterAltScreen`?** — Is Claudio designed to run inline (non-full-screen)? If so, this is correct; if not, should be added for proper terminal isolation.

2. **Keybinding stability** — Are keybindings intentionally non-configurable? Custom `msg.String()` pattern works well, but bubbles/key would enable user customization.

3. **Light terminal support** — Is Gruvbox dark-only intentional, or an oversight? Would benefit from `tea.BackgroundColorMsg` handler.

4. **Viewport search persistence** — Should search query be saved in session for resumption? Currently resets between renders.

---

## Summary for Team

**Status:** Search complete — comprehensive TUI audit documentation exists.

**Key Findings:**
- Two main audit reports + strategic refresh plan found and analyzed
- Overall health: 8.2/10 (production-ready)
- One primary optimization opportunity: style caching (30% performance improvement)
- One key gap: light terminal support
- Strategic roadmap provided for UI modernization

**Next Steps:**
- Implement style caching (HIGH priority, 2-3 hours)
- Add light terminal support (HIGH priority, 1-2 hours)
- Consider Phase 1 of TUI refresh plan (thinking truncation, collapsible blocks)
- Add missing test coverage for viewport navigation

---

*Report compiled from project audit documents*
*TUI_AUDIT_REPORT.md, AUDIT_REPORT.md, tui-refresh-plan.md*
*Date: 2024-2025*
