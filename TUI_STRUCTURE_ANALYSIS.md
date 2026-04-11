# TUI Codebase Structure Analysis

## Overview
The TUI codebase is a comprehensive terminal user interface built with Bubble Tea framework. It's a large, feature-rich application with 6,488 lines in `root.go` and supporting layout logic in `layout.go` (89 lines).

---

## 1. Directory Structure

### Root Level Files (`internal/tui/`)
```
internal/tui/
├── root.go                  (6,488 lines) - Main Bubble Tea Model & Update logic
├── layout.go                (89 lines)   - Layout helper functions
├── messages.go              (37,410 bytes) - Message types & rendering
├── context.go               (1,462 bytes) - App context setup
├── editor.go                (6,265 bytes) - Text editor component
├── focus.go                 (1,736 bytes) - Focus state management
├── images.go                (7,702 bytes) - Image rendering
├── sessionrt.go             (5,067 bytes) - Session runtime management
├── toast.go                 (1,278 bytes) - Toast notifications
├── welcome.go               (4,062 bytes) - Welcome screen
├── attachments.go           (6,216 bytes) - File attachments handling
└── *_test.go files          - Test files
```

### Subdirectories
```
panels/                      - Main UI panels
  ├── panel.go              - Base Panel interface
  ├── conversationpanel/    - Conversation display
  ├── taskspanel/           - Task management
  ├── skillspanel/          - Skill display
  ├── config/               - Configuration panel
  ├── memorypanel/          - Memory management
  ├── filespanel/           - File browsing
  ├── sessions/             - Session picker
  ├── analyticspanel/       - Analytics display
  ├── stree/                - Source tree panel
  ├── toolspanel/           - Tools display
  ├── agui/                 - AI GUI panel
  ├── whichkey/             - Key binding help
  └── ...

agentselector/              - Agent selection overlay
teamselector/              - Team selection overlay
teampanel/                 - Team panel display
modelselector/             - Model selection overlay
filepicker/                - File picker overlay
commandpalette/            - Command palette overlay
prompt/                    - Prompt input component
sidebar/                   - Sidebar rendering
  └── blocks/             - Sidebar content blocks
docks/                    - Dock components
  └── todo_dock.go        - Todo dock widget
permissions/              - Permission dialog
keymap/                   - Key binding definitions
styles/                   - Theme & styling
vim/                      - Vim mode support
components/               - Shared components
notifications/            - Notification queue
```

---

## 2. Key Files Deep Dive

### `root.go` (6,488 lines) - The Main Model

**Main Type: `Model`**
```go
type Model struct {
    // Components
    viewport viewport.Model
    prompt prompt.Model
    spinner components.SpinnerModel
    permission permissions.Model
    palette commandpalette.Model
    filePicker filepicker.Model
    modelSelector modelselector.Model
    agentSelector agentselector.Model
    teamSelector teamselector.Model
    agentDetail *agentDetailOverlay    // ← Key overlay state
    
    // Panels
    activePanel panels.Panel
    activePanelID PanelID
    panelPool map[PanelID]panels.Panel
    
    // State
    messages []ChatMessage
    focus Focus
    width, height int
    streaming bool
    // ... many more fields (200+ lines for type definition)
}
```

**Key Nested Type: `agentDetailOverlay`**
```go
type agentDetailOverlay struct {
    state teams.State              // Agent state
    scroll int                      // ← SCROLL OFFSET (relative)
    toolCalls []ToolCallEntry       // Tool call feed
}
```

**Major Methods by Category:**

1. **Initialization & Setup**
   - `New()` - Create Model with options
   - `WithSkills()`, `WithEngineConfig()`, `WithDB()` - Configuration options
   - `Init()` - Bubble Tea initialization
   - `layout()` - Compute viewport & panel dimensions

2. **Core Update Logic**
   - `Update(msg tea.Msg)` (line 572) - **Main event handler (1,000+ lines)**
   - `waitForEvent()` - Listen for engine events
   - `updatePaletteState()` - Sync command palette

3. **Overlay Rendering**
   - `renderAgentDetail(width, height)` (line 4684) - **Full-screen agent overlay**
     - Handles scroll bounds (lines 4774-4791)
     - Renders conversation + tool calls
     - **Line 4775**: `maxScroll := len(contentLines) - visibleH` (POTENTIAL BUG AREA)
   - `renderPlanApprovalDialog(width)` - Plan approval dialog
   - `renderAskUserDialog(width)` - User input dialog

4. **Scroll Management**
   - `scrollToSection(idx)` (line 5477) - Scroll viewport to section
   - Agent detail scroll: lines 4656-4670 (j/k keys)
     - **Line 4779**: `if m.agentDetail.scroll > maxScroll` - **Bounds check**
     - **Lines 4784-4791**: Slice calculation with edge cases

5. **Session & Panel Management**
   - `switchSessionRelative(dir)` - Navigate between sessions
   - `switchToAlternateSession()` - Toggle last session
   - `togglePanel(id)` - Open/close panels
   - `openPanel(id)` - Open specific panel
   - `createPanel(id)` - Instantiate panel from pool

6. **View Rendering**
   - `View()` (line 5757) - **Main render method (200+ lines)**
     - Calls `layout()` to compute dimensions
     - Overlays multiple components:
       - Plan approval dialog
       - Ask user dialog
       - Selectors (model, agent, team)
       - Which key panel
       - Session picker
       - **Agent detail** (line 5803-5805)
       - Active panels
       - Files panel / sidebar
     - Stacks sections: viewport → palette → dock → statusline → prompt → mode line → help → status bar

### `layout.go` (89 lines) - Layout Utilities

**Key Functions:**

1. **`buildSeparator(height int) string`**
   - Creates vertical line for sidebar divider
   - One character (`│`) per line, repeated `height` times

2. **`placeOverlayAt(base, overlay string, x, y, width, height int) string`** (CRITICAL)
   ```go
   // Lines 24-63
   // Places overlay at (x, y) position within container
   // Risk areas:
   // - Line 29: Pads base to full height
   // - Line 35: Bounds check: if row < 0 || row >= len(baseLines)
   // - Line 42-43: Expands base line to full width with runes
   // - Line 50: Places overlay: if col >= 0 && col < len(baseRunes)
   // - Line 58-60: Truncates to container height
   ```
   - **OVERLAY BOUNDS ISSUES** - This is where overlay positioning bugs occur

3. **`renderPanelWithHelp(panel panels.Panel, w, h int) string`**
   - Reserves space for panel help footer
   - Calls `panel.SetSize()` before rendering
   - Joins panel view + footer vertically

---

## 3. Key Rendering Paths

### Agent Detail Overlay Flow
```
View() (line 5757)
  ↓
layout() (line 5694) - Compute viewport dimensions
  ↓
Check Focus == FocusAgentDetail (line 5803)
  ↓
renderAgentDetail(width, height) (line 4684)
  ├── Build header (lines 4694-4728)
  ├── Render conversation entries (lines 4757-4760)
  ├── Render tool call feed (lines 4750-4756)
  ├── Calculate scroll bounds (lines 4768-4791) ← BUG AREA
  ├── Slice content by scroll (lines 4784-4795)
  ├── Pad remaining height (lines 4798-4801)
  └── Add hint bar (lines 4804-4805)
```

### Dialog Overlay Flow
```
View() → Check activePanel (line 5809)
  ├── OverlayCentered mode → placeOverlay() (line 5822)
  ├── OverlayDrawer mode → placeOverlayAt() (line 5829)
  └── OverlayFullscreen mode → replace viewport (line 5831)
```

---

## 4. Identified Issues & Bug-Fix Areas

### Issue 1: Overlay Bounds in `placeOverlayAt()`
**File**: `layout.go` lines 24-63
- Line 35: `if row < 0 || row >= len(baseLines)` - doesn't check negative bounds carefully
- Line 50: Overlay placement checks `col >= 0 && col < len(baseRunes)` - could be overly restrictive
- Missing: Validation that overlay doesn't exceed right/bottom bounds

### Issue 2: Agent Detail Scroll Edge Cases
**File**: `root.go` lines 4684-4808
- Line 4775: `maxScroll := len(contentLines) - visibleH` - if `visibleH > len(contentLines)`, maxScroll could be negative
- Line 4779: `if m.agentDetail.scroll > maxScroll` - doesn't clamp negative scroll
- Line 4784-4791: Slice logic assumes `start <= end` but doesn't validate

### Issue 3: Layout Guards
**File**: `root.go` line 5694 onwards
- `layout()` function computes `viewport.Width`, `viewport.Height`
- Multiple callers assume these are set correctly
- No validation that `width >= 60` and `height >= 20` (per line 5759)

### Issue 4: Scroll Clamping in Update
**File**: `root.go` lines 4656-4670
- j/k keys increment/decrement scroll without bounds checks
- `m.agentDetail.scroll++` (line 4656) - could overflow
- `m.agentDetail.scroll = 999999` (line 4665) - arbitrary large number
- `m.agentDetail.scroll = 0` (line 4669) - only clamps on 'g'

---

## 5. Critical Paths for Bug Fixes

### 1. Overlay Bounds Validation
- **Location**: `layout.go:24-63` (`placeOverlayAt`)
- **Changes Needed**:
  - Add guard: check overlay width + x < container width
  - Add guard: check overlay height + y < container height
  - Validate all indices before rune operations
  - Test with negative x, y and overlays larger than container

### 2. Scroll Bounds Guards
- **Location**: `root.go:4768-4791` (agent detail scroll)
- **Changes Needed**:
  - Add guard: `if visibleH <= 0 { visibleH = 1 }`
  - Add guard: `maxScroll = max(0, maxScroll)`
  - Clamp scroll: `scroll = clamp(scroll, 0, maxScroll)`
  - Validate slice indices: `start >= 0 && end <= len(contentLines)`

### 3. Scroll Update in Update()
- **Location**: `root.go:4656-4670` (j/k key handlers)
- **Changes Needed**:
  - After each scroll change, clamp to [0, maxScroll]
  - Recalculate maxScroll each frame
  - Never allow scroll < 0

### 4. Layout Guards
- **Location**: `root.go:5694` onwards (`layout()`)
- **Changes Needed**:
  - Validate minimum dimensions before computing splits
  - Guard against divide-by-zero in percentage calculations
  - Ensure sidebar width, files panel width don't exceed total width

### 5. Panel Overlay Placement
- **Location**: `root.go:5812-5832` (panel overlay modes)
- **Changes Needed**:
  - In OverlayCentered: validate w >= 40, h >= 10
  - In OverlayDrawer: validate drawerW >= 30
  - Pass correct dimensions to `placeOverlay` / `placeOverlayAt`

### 6. Viewport Slice Safety
- **Location**: `root.go:5784-4795` (content slicing)
- **Changes Needed**:
  - Guard all array slices: `contentLines[start:end]`
  - Validate: `start >= 0`, `end <= len(contentLines)`, `start <= end`

### 7. Window Resize Handling
- **Location**: `root.go` (throughout View & Update)
- **Changes Needed**:
  - When `width` or `height` changes, recalculate scroll bounds
  - Don't assume viewport dimensions are stable across frames

---

## 6. File Organization Summary

| File | Lines | Purpose | Risk Level |
|------|-------|---------|-----------|
| `root.go` | 6,488 | Core model, update, rendering | **HIGH** |
| `layout.go` | 89 | Overlay placement, separator | **HIGH** |
| `messages.go` | ~2,000 | Message types & rendering | Medium |
| `prompt/prompt.go` | ~1,500 | Input handling | Medium |
| `panels/panel.go` | ~500 | Panel interface | Low |
| Other panels | ~1,500 ea | Domain-specific UI | Low |

---

## 7. Testing Considerations

When applying bug fixes, test:
1. **Overlay placement** with various terminal widths (60-300+)
2. **Agent detail scroll** with 0, 1, 10, 100+ content lines
3. **Negative scroll values** - ensure they're clamped to 0
4. **Small windows** - ensure no panic on division/slicing
5. **Rapid key presses** - stress test scroll increment/decrement
6. **Window resizing** - terminal shrink/grow during operation

