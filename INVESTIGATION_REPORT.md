# Investigation Report: TUI Codebase Structure & Layout Analysis

## Subject
Investigation of the TUI codebase (`internal/tui/`) to identify layout structure, spacing/padding patterns, viewport handling, and potential rendering issues causing content to disappear at the top of the terminal (especially in Alacritty).

## Codebase Overview

### Top-Level Architecture
The TUI is built using **Bubbletea + Lipgloss** with an **Elm-style Model/Update/View pattern**. The codebase consists of **56 Go files** organized in a modular structure:

- **Root coordinator**: `root.go` (214 symbols, ~6,400 lines) — main Model, Update, View, and layout logic
- **Message rendering**: `messages.go` (86 symbols) — ChatMessage formatting, viewport content generation
- **Components**: Reusable UI blocks (prompt, spinner, sidebar, panels, docks)
- **Panels**: Modal/drawer overlays for different views (conversation, config, tasks, skills, memory, etc.)
- **Subsidiary systems**: Docks (todo, permissions), sidebars, keymaps, vim mode, notifications

### View Hierarchy
The final rendered screen (line 5810 in root.go) is built as a vertical stack using `lipgloss.JoinVertical()`:

```
1. "" (empty line - TOP PADDING)
2. topArea (viewport + overlays/sidebars)
3. [Command palette or file picker] (optional)
4. [Dock - permissions or todo] (optional)
5. Status line (renderStatusLine)
6. Separator line
7. Prompt (prompt.View())
8. Mode line (renderModeLine or renderSearchBar)
9. Help footer (renderHelpFooter)
10. Status bar (renderStatusBar)
```

## Key Findings

### 1. **Top Padding Mechanism - Intentional but Fragile**
- **Location**: `root.go:5740` and layout `root.go:5587-5588`
- **Description**: 
  - A single empty string `""` is prepended to sections to prevent "content from being clipped at terminal edge"
  - This is accounted for in the viewport height calculation: `const topPadding = 1`
  - **vpHeight calculation** (line 5588):
    ```go
    vpHeight := m.height - statusH - promptH - paletteH - modeLineH - helpFooterH - statusLineH - 1 - topPadding
    ```
  - This reserves exactly 1 line for the empty padding
- **Risk**: The comment suggests this is a workaround for a clipping issue. If the empty line doesn't render correctly or if content is being positioned incorrectly, the viewport content could be pushed down or clipped.

### 2. **Viewport Height Calculation - Potential Off-by-One Issues**
- **Location**: `root.go:5576-5590`
- **Description**:
  ```
  statusH = 1          (status bar at top - NOT RENDERED IN MAIN OUTPUT)
  promptH = dynamic    (1-10 lines, from textarea height)
  paletteH = 0 or 10   (command palette when active)
  modeLineH = 1        (vim mode / mode indicator line)
  helpFooterH = 1      (keyboard hints)
  statusLineH = 1      (above prompt, nvim-style)
  topPadding = 1       (empty line)
  + 1 for "separator" (not explicitly named)
  ```
- **Issue Identified**: The `statusH = 1` variable is set but appears to be unused in the calculation! This suggests the calculation was refactored but the variable wasn't removed. The actual layout items stacked are:
  - topArea (viewport height)
  - [palette]
  - [dock]
  - statusline (renderStatusLine)
  - separator
  - prompt
  - modeline/searchbar
  - helpfooter
  - statusbar (renderStatusBar)
  
  But `vpHeight` is calculated as: `m.height - statusH - promptH - paletteH - modeLineH - helpFooterH - statusLineH - 1 - topPadding`
  
  **The unused `statusH` is being subtracted even though it's not in the layout!**

### 3. **Dynamic Prompt Height Issue**
- **Location**: `prompt/prompt.go:684-705`
- **Description**:
  - Prompt height is **dynamic**: `Height() = lipgloss.Height(m.View())`
  - The View() method renders text + pills row + divider + actual textarea
  - If pills (images, pastes) are present, they add `2 + pills_height` to the rendered height
  - Layout recalculation happens every frame in `View()` (line 5646): `m.layout()`
- **Risk**: If the prompt height changes (e.g., when images are added or removed), the viewport height will shrink/expand, but the GotoTop/GotoBottom logic (lines 5331-5338) only triggers based on content lines, not on layout changes.

### 4. **Window Size Message Handler - Viewport Refresh**
- **Location**: `root.go:565-576`
- **Description**:
  ```go
  case tea.WindowSizeMsg:
      m.width = msg.Width
      m.height = msg.Height
      m.tooSmall = msg.Width < 60 || msg.Height < 20
      m.palette.SetWidth(m.width)
      m.filePicker.SetWidth(m.width)
      m.modelSelector.SetWidth(m.width)
      m.agentSelector.SetWidth(m.height)  // NOTE: height passed to agentSelector, not width!
      m.layout()
      m.refreshViewport()
      return m, nil
  ```
- **Issue**: Line 573: `m.agentSelector.SetHeight(m.height)` — this should likely be `SetWidth` not `SetHeight`, or there's a missing SetWidth call.
- **More importantly**: `refreshViewport()` is called, which updates the viewport content **after** layout() has calculated the new viewport dimensions. This should work, but let's verify the flow.

### 5. **Viewport Content Refresh - GotoTop/GotoBottom Logic**
- **Location**: `root.go:5282-5339` (refreshViewport method)
- **Description**:
  - When focus is NOT on viewport (line 5331): `if m.focus != FocusViewport`
    - If content fits in viewport: `GotoTop()`
    - If content is larger: `GotoBottom()`
  - When focus IS on viewport: viewport YOffset is NOT changed (implicit)
- **Risk**: If the user has scrolled partway through content, then the content changes size (e.g., tool expands), the viewport will jump to top or bottom without warning. More critically, there's no mechanism to **revalidate YOffset when viewport height changes**. If `m.viewport.Height` is reduced after content is already set, and `YOffset` was calculated for the old height, it could cause content clipping.

### 6. **Section Tracking & Scroll Management**
- **Location**: `root.go:5304-5306` (renderMessages result), `5368-5377` (scrollToSection)
- **Description**:
  - `renderMessages()` returns `Sections` — metadata about line ranges for each message/tool
  - `vpSections` caches these for scroll-to-section logic
  - `scrollToSection()` adjusts `YOffset` based on `LineStart` and `LineCount`
- **Issue**: If `vpHeight` changes but `vpSections` is stale, the scroll-to calculations will use incorrect viewport heights. The sections don't have viewport-dependent state; they're computed once per render.

### 7. **Sidebar Width Interaction**
- **Location**: `root.go:5625-5637` (layout method, sidebar width reduction)
- **Description**:
  - Sidebar is persistent on the right side when no overlay panel is active
  - Sidebar width is calculated as percentage or fixed minimum
  - Affects `m.viewport.Width` but viewport content is already rendered
- **Issue**: If sidebar toggled on/off mid-stream, or if sidebar width calculation is wrong, the viewport might overflow its allocated width or content might be clipped.

### 8. **Overlay Placement - placeOverlay Function**
- **Location**: `root.go:6180-6187` and `layout.go:24-62`
- **Description**: Two overlay placement strategies:
  - `placeOverlay()` — centers overlay in viewport (uses lipgloss.Place)
  - `placeOverlayAt()` — places overlay at specific x,y coordinates
- **Issue**: Both functions render the **entire container height** to avoid clipping, but if base content is shorter than container height, they pad with spaces. This is correct, but if YOffset is wrong, the base content might be positioned incorrectly before the overlay is placed.

### 9. **Empty Top Padding - Rendering Details**
- **Location**: `root.go:5740`
- **Code**: `sections = append(sections, "")` followed by `sections = append(sections, topArea)`
- **Issue - CRITICAL**: The empty string is meant to be a blank line, but:
  - `lipgloss.JoinVertical()` joins with newlines
  - An empty string becomes a single newline character
  - If the terminal's cursor is already at line 1, an extra newline might push content down unexpectedly
  - In some terminals (like Alacritty), if the TUI doesn't explicitly place the cursor or if scrolling is off by one, this could cause the top line to be clipped by the terminal's rendering bounds

---

## Symbol Map

| Symbol | File | Role |
|--------|------|------|
| `Model` | `root.go:71-188` | Main TUI state container |
| `View()` | `root.go:5640-5811` | Renders the entire terminal screen |
| `Update()` | `root.go:561-1500+` | Processes input and events |
| `layout()` | `root.go:5576-5638` | Calculates viewport and component dimensions |
| `refreshViewport()` | `root.go:5282-5339` | Renders messages into viewport content |
| `renderMessages()` | `messages.go:85+` | Converts ChatMessages to formatted strings with sections |
| `placeOverlay()` | `root.go:6180-6187` | Centers modal overlays on viewport |
| `renderStatusLine()` | `root.go:5921-5990` | Renders mode/session/model status line |
| `renderHelpFooter()` | `root.go:5856-5864` | Renders keyboard shortcuts footer |
| `renderModeLine()` | `root.go:5866-5901` | Renders vim mode / permission mode line |
| `Sidebar.View()` | `sidebar/sidebar.go:33-100` | Stacks sidebar blocks vertically with title separators |
| `width/height` (Model fields) | `root.go:104` | Terminal dimensions from WindowSizeMsg |
| `viewport.Width/Height` | `root.go:73` | Viewport component dimensions (set in layout) |
| `vpHeight` (local var) | `root.go:5588` | Calculated height of viewport content area |
| `topPadding` (const) | `root.go:5587` | Constant = 1 line, reserved at top |

---

## Dependencies & Data Flow

### Layout Calculation Flow
1. **Update (WindowSizeMsg)** → sets `m.width`, `m.height`
2. **View()** called → calls `m.layout()` 
3. **layout()** computes:
   - `vpHeight` based on terminal height minus all UI elements
   - `mw` (main viewport width) considering sidebar/panels
   - Updates `m.viewport.Width` and `m.viewport.Height`
4. **refreshViewport()** called separately (when messages change or on WindowSizeMsg)
   - Renders messages into string content
   - Calls `m.viewport.SetContent(content)`
   - Auto-scrolls to top or bottom based on focus
5. **View()** renders sections with calculated viewport dimensions

### Viewport Content Flow
```
ChatMessage[] → renderMessages() → [Content string + Sections metadata] 
              → viewport.SetContent()
              → viewport.View() (reads YOffset, renders visible window)
              → placeOverlay() (if needed)
              → lipgloss.JoinVertical() (final stack)
```

### Height Budget
```
Terminal Height
├─ topPadding (1)
├─ topArea (variable, usually viewport + sidebar)
│  └─ viewport.View() (height = vpHeight)
├─ [palette] (0 or 10)
├─ [dock] (variable)
├─ statusline (1)
├─ separator (1)
├─ prompt (promptH, dynamic 1-10)
├─ modeline (1)
├─ helpfooter (1)
└─ statusbar (1)
```

---

## Risks & Observations

### 1. **CRITICAL: Unused statusH in vpHeight Calculation**
The variable `statusH = 1` is subtracted from vpHeight, but there is no status bar at the top being rendered in the sections stack. This causes the viewport to be allocated **1 line smaller than it should be**, effectively wasting 1 line and reducing available message space.

**Impact**: In a 24-line terminal, the viewport might be 2 lines shorter than expected, causing more aggressive scrolling or message cropping.

### 2. **Empty Top Padding — Rendering Artifact**
The leading empty string in sections (line 5740) is intended to prevent clipping, but:
- Its effectiveness depends on how lipgloss renders empty strings
- If the terminal cursor or scrolling position is not properly reset, this might not prevent clipping
- **In Alacritty specifically**: If the TUI doesn't disable alternate screen or if timing is off, the top line could be clipped by the terminal's native scrollback region

### 3. **YOffset Not Validated on Resize**
When viewport height changes (WindowSizeMsg → layout() → refreshViewport), the YOffset is **not revalidated**. If YOffset points beyond the content after a resize, or if it points to a stale section position, content could be clipped.

### 4. **Sidebar Width Calculation Complexity**
The sidebar width is computed with multiple fallbacks and minimums (lines 5625-5633). If any of these calculations are off-by-one, the viewport could be sized incorrectly and positioned off-screen.

### 5. **Dynamic Prompt Height Complicates Layout**
Since prompt height is computed from rendered content (including optional pills), and this happens **after** viewport dimensions are set, there's a potential for layout thrashing:
- If prompt grows, viewport shrinks
- This might cause content reflow in the viewport
- Which could change the number of lines, causing a re-render
- Which could change prompt height again

### 6. **Viewport.SetContent() with GotoTop/GotoBottom**
Lines 5333-5337 in refreshViewport():
```go
if contentLines <= m.viewport.Height {
    m.viewport.GotoTop()
} else {
    m.viewport.GotoBottom()
}
```
This logic uses `contentLines` from string counting, not from the actual rendered Sections. If `renderMessages()` produces sections with different line counts than `strings.Count()`, the scroll position could be wrong.

### 7. **Sidebar/Panel Rendering Order**
The viewport is rendered, then overlaid with panel/sidebar. If the sidebar's height differs from viewport height, there could be padding differences, causing misalignment between viewport and sidebar.

---

## All TUI Files & Roles

### Core Files (root, layout, messages)
- **root.go** (6,369 lines) — Main Model, Update, View, layout logic, event handling
- **layout.go** (89 lines) — Helper functions: buildSeparator, placeOverlayAt, renderPanelWithHelp
- **messages.go** (1,000+ lines) — Renders ChatMessages, builds Sections, formats tool calls, diffs, etc.

### Input & Prompt
- **prompt/prompt.go** — Textarea-based input with vim mode, history, image/paste attachments
- **prompt/pills.go** — Renders pills for @mentions, pastes, images
- **prompt/prompt_vim_test.go** — Vim mode tests
- **keymap/keymap.go** — Key binding registry and leader-key handling
- **keymap/actions.go** — Action definitions
- **vim/vim.go** — Vim mode state machine (Insert, Normal, Visual modes)
- **vim/vim_test.go** — Vim mode tests
- **attachments.go** — Image attachment handling
- **attachments_test.go** — Tests
- **editor.go** — External editor integration
- **context.go** — Context/state management
- **sessionrt.go** — Per-session runtime state

### Panels & Overlays
- **panels/panel.go** — Panel interface/base
- **panels/conversationpanel/panel.go** — Mirrored conversation view for right window
- **panels/config/config.go** — Configuration editor
- **panels/skillspanel/skills.go** — Skills browser
- **panels/taskspanel/tasks.go** — Task/todo browser
- **panels/toolspanel/tools.go** — Available tools browser
- **panels/analyticspanel/analytics.go** — Usage/cost/token analytics
- **panels/memorypanel/memory.go** — Memory/knowledge base browser
- **panels/whichkey/whichkey.go** — Vim which-key menu
- **panels/filespanel/files.go** — File picker/browser
- **panels/agui/panel.go** — Agent UI configuration
- **panels/stree/panel.go** — Syntax tree viewer
- **panels/sessions/sessions.go** — Session switcher
- **commandpalette/palette.go** — Command palette (fuzzy finder)
- **filepicker/picker.go** — File/directory picker
- **agentselector/selector.go** — Agent/persona selector
- **teamselector/selector.go** — Team switcher
- **modelselector/selector.go** — Model selector
- **permissions/dialog.go** — Permission approval dialog
- **teampanel/panel.go** — Team status display

### Docks & Sidebars
- **docks/dock.go** — Dock interface
- **docks/todo_dock.go** — Inline todo list
- **sidebar/sidebar.go** — Sidebar block stacking layout
- **sidebar/block.go** — Block interface
- **sidebar/blocks/files.go** — Recent files sidebar block
- **sidebar/blocks/todos.go** — Todos sidebar block
- **sidebar/blocks/tokens.go** — Token/cost usage block

### Styling & Utilities
- **styles/theme.go** — Lipgloss color palette and reusable styles
- **styles/agent_colors.go** — Agent-specific color assignments
- **styles/glamour.go** — Markdown rendering configuration
- **components/spinner.go** — Animated spinner component
- **notifications/queue.go** — Toast/notification queue
- **toast.go** — Toast overlay
- **welcome.go** — Welcome screen with animated logo
- **focus.go** — Focus mode enum and helpers
- **images.go** — Image attachment utilities
- **logo_test.go** — Logo animation test
- **sanitize_test.go** — HTML sanitization test
- **delete_interaction_test.go** — Message deletion test
- **planmode_test.go** — Plan mode interaction test

---

## Open Questions

1. **Why is `statusH` subtracted from vpHeight if it's not rendered?**
   - Is this a leftover from a refactor?
   - Should it be removed, or is there a hidden status bar at the top that should be rendered?

2. **Does the empty top padding line actually prevent clipping in Alacritty?**
   - Does lipgloss.JoinVertical() render an empty string as a true newline?
   - Is there a better way to ensure the top line is visible (e.g., alternate screen buffer management)?

3. **What causes the viewport height to be incorrect on certain terminal sizes?**
   - The calculation seems correct in theory, but practice suggests issues in Alacritty
   - Is it a rounding error in the dynamically-sized components (prompt, sidebar)?
   - Is it terminal-specific rendering behavior?

4. **Should YOffset be revalidated/clamped when viewport height changes?**
   - Currently, refreshViewport() only auto-scrolls on focus changes, not on layout changes
   - This could leave YOffset pointing to an invalid position

5. **How does the viewport content rendering interact with the sidebar?**
   - Is the sidebar height always exactly matched to viewport height?
   - Could padding differences cause visual misalignment?

---

## Conclusion

The TUI layout is well-structured overall, but there are **several calculation issues** that could cause content clipping or misalignment at the top of the terminal:

1. **Unused statusH** wastes 1 line of viewport space
2. **Empty top padding** is a potential workaround for a deeper terminal interaction issue
3. **YOffset is not revalidated** on resize, which could cause content to disappear
4. **Dynamic prompt height** adds layout complexity that could cause thrashing
5. **Viewport height calculation** includes multiple interdependent values (sidebar width, panel state) that could compound rounding errors

The most likely culprit for **content disappearing at the top in Alacritty** is either:
- The empty padding line not working as intended (Alacritty-specific behavior)
- YOffset being positioned incorrectly and not recalculated on resize
- The viewport height being 1-2 lines smaller than intended due to the unused statusH or rounding errors

---

**Investigation completed by**: orion-1  
**Date**: 2024  
**Codebase**: internal/tui/ (56 .go files, ~10,620 lines)
