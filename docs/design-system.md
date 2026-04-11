# Claudio TUI — Design System

**Version:** 1.0
**Status:** Draft — For Implementation
**Stack:** Go · Bubbletea · Bubbles · Lipgloss · Glamour
**Constraint:** Terminal monospace grid. No subpixel. No font choice beyond user's terminal font.

---

## Table of Contents

1. [Design Principles](#1-design-principles)
2. [Color System](#2-color-system)
3. [Typography](#3-typography)
4. [Spacing & Layout](#4-spacing--layout)
5. [Component Library](#5-component-library)
6. [Interaction Patterns](#6-interaction-patterns)
7. [Lipgloss Implementation Notes](#7-lipgloss-implementation-notes)

---

## 1. Design Principles

1. **Density over decoration** — Every cell earns its place. No ornamental borders, no double-spacing for aesthetics. Information first.
2. **Calm hierarchy** — Use color weight (bright → dim) and bold to guide the eye. Avoid competing accents in the same region.
3. **Predictable focus** — One focused region at a time. Focus is always visible via border color shift or cursor glyph. No guessing where input goes.
4. **Terminal-native** — Design for 80–200 column terminals. Degrade gracefully below 80. Never assume mouse.
5. **Gruvbox everywhere** — All color comes from the Gruvbox Dark palette. No ad-hoc hex values outside the system.

---

## 2. Color System

### 2.1 Background Layers

Use darker → lighter to express depth. Deeper = further back.

| Token          | Hex       | Lipgloss Var   | Use                                    |
|----------------|-----------|----------------|----------------------------------------|
| `bg-hard`      | `#1d2021` | `BgHard`       | App background, deepest layer          |
| `bg`           | `#282828` | `Surface`      | Main content area background           |
| `bg-soft`      | `#32302f` | `BgSoft`       | Elevated surfaces (modals, overlays)   |
| `bg1`          | `#3c3836` | `SurfaceAlt`   | Borders, pills, input backgrounds      |
| `bg2`          | `#504945` | `Subtle`        | Inactive borders, tree lines, dividers |
| `bg3`          | `#665c54` | `Bg3`          | Hover state backgrounds, scrollbar     |
| `bg4`          | `#7c6f64` | `Bg4`          | Disabled element backgrounds           |

### 2.2 Foreground Levels

Brightest = most important. Use sparingly at top levels.

| Token    | Hex       | Lipgloss Var | Use                              |
|----------|-----------|--------------|----------------------------------|
| `fg`     | `#ebdbb2` | `Text`       | Primary text, headings           |
| `fg1`    | `#ebdbb2` | `Text`       | (alias — same as fg in gruvbox)  |
| `fg2`    | `#d5c4a1` | `Fg2`        | Secondary body text              |
| `fg3`    | `#bdae93` | `Dim`        | Tertiary text, descriptions      |
| `fg4`    | `#a89984` | `Fg4`        | Timestamps, metadata, captions   |
| `gray`   | `#928374` | `Muted`      | Hints, placeholders, disabled    |

### 2.3 Accent Colors

Two variants per hue. Bright = emphasis/active. Normal = body/passive.

| Name       | Normal    | Bright    | Lipgloss Var (normal / bright)  |
|------------|-----------|-----------|----------------------------------|
| Red        | `#cc241d` | `#fb4934` | `RedNorm` / `Error`             |
| Green      | `#98971a` | `#b8bb26` | `GreenNorm` / `Success`         |
| Yellow     | `#d79921` | `#fabd2f` | `YellowNorm` / `Warning`        |
| Blue       | `#458588` | `#83a598` | `BlueNorm` / `Secondary`        |
| Purple     | `#b16286` | `#d3869b` | `PurpleNorm` / `Primary`        |
| Aqua       | `#689d6a` | `#8ec07c` | `AquaNorm` / `Aqua`             |
| Orange     | `#d65d0e` | `#fe8019` | `OrangeNorm` / `Orange`         |

### 2.4 Semantic Aliases

Map intentions to palette colors. All UI code uses semantic names, never raw hues.

| Semantic    | Maps To          | Hex       | Use                                        |
|-------------|------------------|-----------|--------------------------------------------|
| `primary`   | Bright Purple    | `#d3869b` | Focus rings, active tab, assistant prefix   |
| `secondary` | Bright Blue      | `#83a598` | User input accent, links, selected items    |
| `success`   | Bright Green     | `#b8bb26` | Tool success, allow button, completed state |
| `warning`   | Bright Yellow    | `#fabd2f` | Tool calls, permissions, running spinner    |
| `error`     | Bright Red       | `#fb4934` | Errors, deny button, failed state           |
| `info`      | Bright Aqua      | `#8ec07c` | Headings, file paths, decorators            |
| `accent`    | Bright Orange    | `#fe8019` | Inline code, badges, highlights             |
| `muted`     | Gray             | `#928374` | Secondary text, hints, inactive elements    |
| `subtle`    | Bg2              | `#504945` | Borders, separators, tree connectors        |

### 2.5 Contrast Rules

- Primary text (`fg` on `bg`): 10.5:1 ✓
- Muted text (`gray` on `bg`): 4.6:1 ✓ (meets WCAG AA)
- All bright accents on `bg`: ≥4.5:1 ✓
- `Subtle` (`bg2`) for borders only — never for text on `bg`
- Normal-variant accents reserved for backgrounds (badges). Never for text on `bg` — contrast too low.

---

## 3. Typography

### 3.1 Terminal Constraints

No font selection — user's terminal font applies. Design assumes monospace grid.
All sizing is in "cells" (1 cell = 1 character width × 1 line height).

### 3.2 Heading Hierarchy

Headings use color + bold + optional prefix glyph. No size variation (terminal).

| Level | Bold | Color        | Prefix     | Use                          |
|-------|------|--------------|------------|------------------------------|
| H1    | Yes  | `bg` on `warning` bg | ` TEXT `  | Glamour doc titles (inverted badge) |
| H2    | Yes  | `success`    | `## `      | Section headers in markdown   |
| H3    | Yes  | `info`       | `### `     | Subsection headers            |
| H4    | No   | `secondary`  | `#### `    | Minor headers                 |

In TUI chrome (non-markdown):

| Element        | Bold | Color      | Use                              |
|----------------|------|------------|----------------------------------|
| Panel Title    | Yes  | `primary`  | Panel header text                |
| Section Label  | Yes  | `info`     | Sidebar block titles             |
| Status Label   | Yes  | `warning`  | Active status indicators         |

### 3.3 Body Text Treatments

| Role         | Bold | Color    | Italic | Use                                |
|--------------|------|----------|--------|------------------------------------|
| Body         | No   | `fg`     | No     | Primary content, messages          |
| Body-dim     | No   | `fg3`    | No     | Descriptions, summaries            |
| Caption      | No   | `fg4`    | No     | Timestamps, durations, metadata    |
| Hint         | No   | `muted`  | Yes    | Keybinding hints, placeholders     |
| Code inline  | No   | `accent` | No     | Inline code (orange on bg1 background) |
| Code block   | No   | `fg`     | No     | Fenced code (glamour handles)      |
| Error        | Yes  | `error`  | No     | Error messages                     |
| Link         | No   | `secondary` | No  | Underlined                         |

### 3.4 Line Width Rules

| Context           | Max Width | Rationale                         |
|-------------------|-----------|-----------------------------------|
| Markdown body     | 80 chars  | Readability sweet spot            |
| Glamour word-wrap | panel width - 4 | 2 margin each side       |
| Status bar        | terminal width | Full width, single line    |
| Panel content     | panel width - 4 | 2 padding each side       |
| Prompt input      | terminal width - 6 | Room for border + padding |

---

## 4. Spacing & Layout

### 4.1 Spacing Scale

Terminal spacing is in cells. Use this scale consistently.

| Token | Cells | Use                                    |
|-------|-------|----------------------------------------|
| `xs`  | 1     | Inline gaps, icon-to-text              |
| `sm`  | 2     | Padding inside components, list indent |
| `md`  | 4     | Margin between sections                |
| `lg`  | 8     | Padding inside modals/dialogs          |

No spacing beyond 8. Terminal real estate is scarce.

### 4.2 Application Layout

Three-zone layout, left to right:

```
┌─────────────┬──────────────────────────────┬─────────────────┐
│  Sidebar    │  Main Content                │  Panel          │
│  (16–22ch)  │  (flex — takes remaining)    │  (30–45ch)      │
│             │                              │  (toggle)       │
│  · tokens   │  · conversation viewport     │  · tasks        │
│  · files    │  · markdown rendering        │  · tools        │
│  · todos    │                              │  · files        │
│             │                              │  · sessions     │
├─────────────┴──────────────────────────────┴─────────────────┤
│  Prompt Input                                                │
├──────────────────────────────────────────────────────────────┤
│  Status Bar                                                  │
└──────────────────────────────────────────────────────────────┘
```

**Width allocation rules:**
- Below 80 cols: sidebar hidden, panel hidden. Main only + prompt + status.
- 80–120 cols: sidebar (18ch) + main (flex). Panel overlays main when open.
- 120+ cols: sidebar (20ch) + main (flex) + panel (40ch) when open.

**Vertical allocation:**
- Status bar: 1 line (always)
- Prompt: 3–8 lines (dynamic, grows with input)
- Main viewport: remaining height
- Dock (todo): 0–3 lines above prompt when active

### 4.3 Border Styles

| Border          | Chars                      | Use                                  |
|-----------------|----------------------------|--------------------------------------|
| Rounded         | `╭╮╰╯│─`                   | Panels, dialogs, modals, cards       |
| Left-bar focus  | `▌` (U+258C)               | Focused prompt, user message block   |
| Left-bar blur   | `▎` (U+258E)               | Unfocused prompt                     |
| Separator       | `─` repeated               | Horizontal dividers within panels    |
| Vertical sep    | `│`                        | Between sidebar and main             |
| None            | (space)                    | Status bar, inline content           |

**Border color rules:**
- Focused panel/dialog: `primary` (#d3869b)
- Unfocused panel: `surfaceAlt` (#3c3836)
- Warning dialog (permissions): `warning` (#fabd2f)
- Separators: `surfaceAlt` (#3c3836) or `muted` (#928374)

### 4.4 Panel Padding Convention

```
╭─ Panel Title ────────────────╮   ← border row
│                              │   ← 0 top padding (title IS the top)
│  Content starts here         │   ← 2ch left padding
│  More content                │
│                              │
│  hint: j/k nav · esc close  │   ← hint row at bottom
╰──────────────────────────────╯   ← border row
```

- Content left padding: 2 cells
- No right padding beyond border
- Separator between sections: 1 blank line or `─` line

---

## 5. Component Library

### 5.1 Panel / Card

Bordered container for side panels, dialogs, overlays.

| Property     | Value                                    |
|--------------|------------------------------------------|
| Border       | Rounded (`╭╮╰╯│─`)                      |
| Border color | `surfaceAlt` default, `primary` focused  |
| Title        | Bold `primary`, 1ch padding each side    |
| Content pad  | 2ch left                                 |
| Help footer  | Italic `muted`, below separator line     |

**States:**
- **Active/focused**: border `primary`, title rendered
- **Inactive**: border `surfaceAlt`, title `muted`
- **Hidden**: not rendered (0 width)

### 5.2 List Item

Used in panels (tasks, tools, files, sessions, skills).

| State      | Prefix   | Text Color | Text Bold | Notes                    |
|------------|----------|------------|-----------|--------------------------|
| Unselected | 2ch pad  | `fg3`      | No        | Dim, blends into bg      |
| Selected   | `▸ `     | `fg`       | Yes       | Cursor glyph in `warning`|
| Disabled   | 2ch pad  | `muted`    | No        | Grayed out               |

**Row anatomy (task example):**
```
▸ ◐ #3 Implement caching  @agent-1  12s
  ○ #4 Write tests
```
- Prefix (cursor or pad) → status icon → ID badge → subject → assignee badge → duration

### 5.3 Input Field (Prompt)

| State    | Left Border | Border Color | Placeholder      |
|----------|-------------|--------------|------------------|
| Focused  | `▌`         | `primary`    | Hidden           |
| Blurred  | `▎`         | `muted`      | Italic `subtle`  |

- Textarea from bubbles. Full-width minus border.
- Vim mode indicator shown in status bar, not in prompt.
- Paste content > 200 chars collapsed to pill.
- Image attachments shown as pills above input.

### 5.4 Button / Action Item

Terminal buttons use background color fills.

| Variant       | Background | Foreground | Bold | Use              |
|---------------|------------|------------|------|------------------|
| Allow         | `success`  | `bg`       | Yes  | Permission allow |
| Deny          | `error`    | `fg`       | Yes  | Permission deny  |
| Primary       | `primary`  | `fg`       | Yes  | Allow-always     |
| Inactive      | none       | `fg3`      | No   | Unselected btn   |

- Padding: 0 vertical, 1 horizontal
- Buttons laid out inline with 2ch gap
- Active button visually distinct via background fill; inactive = text only

### 5.5 Status Badge

Inline colored labels for states.

| Badge        | Color      | Icon | Use                     |
|--------------|------------|------|-------------------------|
| Success      | `success`  | `●`  | Completed task/tool     |
| Warning      | `warning`  | `◐`  | In-progress, running    |
| Error        | `error`    | `✗`  | Failed task/tool        |
| Info         | `info`     | `◆`  | File paths, decorators  |
| Pending      | `fg3`      | `○`  | Not started             |
| Killed       | `muted`    | `⊘`  | Cancelled/killed        |

**Type badges** (background tasks):
- `[bash]` in `info`
- `[agent]` in `primary`
- `[dream]` in `accent`
- `[task]` in `fg3`

### 5.6 Progress Indicator / Spinner

| Property     | Value                         |
|--------------|-------------------------------|
| Frames       | `◐ ◓ ● ◑` (4-frame cycle)    |
| Color        | `primary` (default spinner)   |
| Color        | `warning` (running task)      |
| Text beside  | `fg3` italic                  |
| Timer        | `muted`, appended right       |
| Tick rate    | 150ms (component), 500ms (panel refresh) |

### 5.7 Header Bar

No dedicated header bar — the application title is not shown during active sessions.

**Welcome screen header:** Large ASCII art logo in `primary`, centered. Shown only on empty state / first launch.

### 5.8 Footer / Status Bar

Single line, full width. Background = `bg` (no fill — blends with terminal).

```
model-name │ tokens: 12.3k/100k │ cost: $0.04 │ session: abc │ vim:N │ ?:help
```

| Segment     | Style                     |
|-------------|---------------------------|
| Model name  | Bold `fg`                 |
| Separators  | `subtle` ` │ `            |
| Values      | `fg3`                     |
| Active flag | `warning` (e.g. streaming)|
| Hints       | `fg3`                     |

### 5.9 Modal / Overlay

Used for: permission dialog, plan mode, command palette, file picker, model selector.

| Property       | Value                                   |
|----------------|-----------------------------------------|
| Border         | Rounded                                 |
| Border color   | `warning` (permission), `primary` (other)|
| Background     | `bg` (rendered on top of dimmed base)    |
| Padding        | 1 vertical, 2 horizontal                |
| Title          | Bold, colored per context                |
| Placement      | Centered horizontally, top-third vertically |
| Dismiss        | `esc` always closes                      |

**Permission dialog anatomy:**
```
╭─ Permission Required ─────────────╮
│                                    │
│  Tool: Write                       │
│  Path: /src/main.go                │
│                                    │
│  [Allow]  [Deny]  [Allow Always]   │
│                                    │
╰────────────────────────────────────╯
```

### 5.10 Tab Bar

Used within panels (e.g., Tasks panel: Planning | Background).

| State    | Style                          |
|----------|--------------------------------|
| Active   | Bold `primary`, underlined     |
| Inactive | `muted`, no underline          |

Tabs are numbered: `1 Planning  2 Background`
Number prefix aids keyboard access.
Rendered inline after panel title, separated by 2ch.

### 5.11 Divider / Separator

| Type         | Char     | Color        | Use                        |
|--------------|----------|--------------|----------------------------|
| Horizontal   | `─`×width| `surfaceAlt` | Between sections in panels |
| Vertical     | `│`      | `muted`      | Between sidebar and main   |
| Blank line   | (empty)  | —            | Between sidebar blocks     |

### 5.12 Agent Card

Shown in team panel and agent detail overlay.

```
 agent-1  [sonnet]  ◐ working
   Task: Implement login flow
   Duration: 45s
```

| Element     | Style                           |
|-------------|---------------------------------|
| Agent name  | Bold, color from `AgentColor()` |
| Model badge | `fg3` in brackets               |
| Status icon | Per status badge (§5.5)         |
| Task line   | `fg3`, 3ch indent               |
| Duration    | `muted`                         |

**Detail overlay** (expanded): full-panel takeover with scrollable conversation log.
- Agent messages: colored by `AgentColor(name)`
- Tool calls: `warning`
- Text content: `fg3` with 3ch indent
- Done line: `success`
- Error line: `error`

### 5.13 Task Item

Two contexts: planning tasks and background tasks.

**Planning task row:**
```
▸ ◐ #3 Implement caching  @agent-1  12s
```
See §5.2 for list item states. Additional elements:
- Status icon: `○` pending (`fg3`), `◐` in-progress (`warning`), `●` done (`success`)
- ID badge: `muted` `#N`
- Subject: per list item state
- Assignee: `info` `@name`

**Background task row:**
```
▸ ◐ [bash] running build  45s
```
- Type badge: colored per §5.5
- Exit code on failure: `error` `(exit 1)`
- Error detail on selected: next line, `error` bold, 4ch indent

### 5.14 Tool Call Row

Rendered in conversation viewport.

```
⚡ Write  src/main.go  "Add error handling"
  ├─ ✓ success (0.3s)
```

| Element       | Style                    |
|---------------|--------------------------|
| Icon `⚡`     | `warning`                |
| Tool name     | Bold `warning`           |
| File path     | `info`                   |
| Description   | Italic `muted`           |
| Connector `├─`| `subtle`                 |
| Success `✓`   | `success`                |
| Error `✗`     | Bold `error`             |
| Result preview| `muted`, truncated       |
| Expand hint   | Italic `subtle`          |
| Diff old      | `error`                  |
| Diff new      | `success`                |
| Badge         | Bold `accent`            |

### 5.15 Notification / Toast

Auto-dismissing overlay. 1.5s duration.

| Property      | Value                      |
|---------------|----------------------------|
| Border        | Rounded                    |
| Border color  | `surfaceAlt`               |
| Background    | `bg`                       |
| Text color    | `fg`                       |
| Padding       | 0 vertical, 2 horizontal   |
| Position      | Centered, top of viewport  |
| Animation     | Appear/disappear (no transition — terminal) |

---

## 6. Interaction Patterns

### 6.1 Focus Management

**Rules:**
1. One focused region at a time: prompt OR panel OR overlay.
2. Focus indicated by border color shift to `primary`.
3. `esc` moves focus back toward prompt (panel → prompt, overlay → previous).
4. `tab` cycles focus: prompt → panel → prompt (when panel open).
5. Overlays are modal — capture all input until dismissed.

**Focus ring:**
- Prompt: left border `▌` changes `muted` → `primary`
- Panel: border changes `surfaceAlt` → `primary`
- Overlay: always `primary` or `warning` border (inherently focused)

### 6.2 Keyboard Shortcut Display

**Which-key popup:**
- Triggered by `?` or prefix timeout
- Rounded border, `surfaceAlt` border color
- Keys: bold `warning`
- Descriptions: `fg3`
- Separators: `subtle` `│`
- Title: bold `primary`
- Dismiss: any key or `esc`

**Status bar hints:**
- Contextual hints in `fg3` at right edge of status bar
- Format: `key:action` separated by ` · `

**In-panel hints:**
- Footer line below separator
- Italic `muted`
- Format: `j/k nav · enter select · esc close`

### 6.3 Loading States

| Context              | Indicator                          |
|----------------------|------------------------------------|
| AI streaming         | Spinner `◐◓●◑` in `primary` + italic `fg3` label + timer in `muted` |
| Panel data refresh   | No indicator (instant — local data)|
| Background task      | Spinner in task row, `warning`     |
| Tool execution       | Tool row shows `◐` until result    |

**Rule:** Never freeze the UI. Prompt remains interactive during AI streaming. User can type while waiting.

### 6.4 Empty States

Every list must handle zero items gracefully.

| Context           | Empty Message                     | Style              |
|-------------------|-----------------------------------|---------------------|
| Tasks (planning)  | `No planning tasks`               | Italic `muted`, 2ch indent |
| Tasks (background)| `No background tasks`             | Italic `muted`, 2ch indent |
| Files panel       | `No files attached`               | Italic `muted`      |
| Tools panel       | `No tools available`              | Italic `muted`      |
| Sessions          | `No saved sessions`               | Italic `muted`      |
| Conversation      | Welcome screen with logo + hints  | `primary` logo, `fg3` hints |
| Search results    | `No matches`                      | Italic `muted`      |

### 6.5 Error States

| Context          | Display                                            |
|------------------|----------------------------------------------------|
| Tool failure     | `✗` icon + bold `error` message inline in tool row |
| API error        | Bold `error` text in conversation viewport         |
| Background task  | `✗` icon + `(exit N)` + error detail on select     |
| Permission denied| Inline `error` text after deny action              |
| Network error    | Toast with error message, auto-dismiss 3s          |

**Rule:** Errors appear at point of cause, not in a global error region. No error modals — always inline or toast.

---

## 7. Lipgloss Implementation Notes

### 7.1 Color Variable Naming

Expand current `styles/theme.go` with missing palette entries:

```go
// Gruvbox Dark — Full palette
var (
    // Backgrounds (dark → light)
    BgHard     = lipgloss.Color("#1d2021")
    Surface    = lipgloss.Color("#282828") // existing
    BgSoft     = lipgloss.Color("#32302f")
    SurfaceAlt = lipgloss.Color("#3c3836") // existing
    Subtle     = lipgloss.Color("#504945") // existing
    Bg3        = lipgloss.Color("#665c54")
    Bg4        = lipgloss.Color("#7c6f64")

    // Foregrounds (bright → dim)
    Text  = lipgloss.Color("#ebdbb2") // existing
    Fg2   = lipgloss.Color("#d5c4a1")
    Dim   = lipgloss.Color("#bdae93") // existing
    Fg4   = lipgloss.Color("#a89984")
    Muted = lipgloss.Color("#928374") // existing

    // Accents — bright (existing)
    Primary   = lipgloss.Color("#d3869b")
    Secondary = lipgloss.Color("#83a598")
    Success   = lipgloss.Color("#b8bb26")
    Warning   = lipgloss.Color("#fabd2f")
    Error     = lipgloss.Color("#fb4934")
    Aqua      = lipgloss.Color("#8ec07c")
    Orange    = lipgloss.Color("#fe8019")

    // Accents — normal (new)
    RedNorm    = lipgloss.Color("#cc241d")
    GreenNorm  = lipgloss.Color("#98971a")
    YellowNorm = lipgloss.Color("#d79921")
    BlueNorm   = lipgloss.Color("#458588")
    PurpleNorm = lipgloss.Color("#b16286")
    AquaNorm   = lipgloss.Color("#689d6a")
    OrangeNorm = lipgloss.Color("#d65d0e")
)
```

### 7.2 Style Composition Patterns

**Principle:** Define base styles, compose via `.Inherit()` or direct field override. Never duplicate full style definitions.

```go
// Base styles — compose from these
var (
    basePanel = lipgloss.NewStyle().
        Border(lipgloss.RoundedBorder()).
        BorderForeground(SurfaceAlt)

    basePanelFocused = basePanel.
        BorderForeground(Primary)

    baseText = lipgloss.NewStyle().
        Foreground(Text)

    baseDim = lipgloss.NewStyle().
        Foreground(Dim)

    baseHint = lipgloss.NewStyle().
        Foreground(Muted).
        Italic(true)
)
```

**Width/height:** Always set `.Width()` and `.Height()` at render time via `SetSize()`, never in style constants.

**Padding pattern:**
```go
// Consistent panel content padding
func PanelContent(content string, width int) string {
    return lipgloss.NewStyle().
        PaddingLeft(2).
        Width(width).
        Render(content)
}
```

### 7.3 Border Rendering

Use custom `lipgloss.Border` for the rounded panel (already in codebase):

```go
var RoundedBorder = lipgloss.Border{
    Top:         "─",
    Bottom:      "─",
    Left:        "│",
    Right:       "│",
    TopLeft:     "╭",
    TopRight:    "╮",
    BottomLeft:  "╰",
    BottomRight: "╯",
}
```

**Title-in-border pattern:**
```go
// Render title inline with top border
titleStr := PanelTitle.Render(title)
style := basePanel.
    BorderTop(true).
    Width(width)
// Place title after TopLeft char via string manipulation
```

### 7.4 Agent Color System

Current `AgentColor()` via FNV hash is good. Keep it. Palette order:
`Primary, Secondary, Aqua, Orange, Warning, Success`

This gives maximum visual distinction between concurrent agents.

### 7.5 Glamour Theme Alignment

Current `glamour.go` gruvbox JSON is well-aligned with this system. Key mappings:
- H1: `warning` background badge (inverted)
- H2: `success`
- H3: `info` (aqua)
- H4: `secondary` (blue)
- Code inline: `accent` (orange) on `surfaceAlt` bg
- Code block: `fg` on `bg`, chroma syntax colors from gruvbox
- Blockquote: `muted` with `│` indent

No changes needed to glamour JSON.

### 7.6 Anti-Patterns to Avoid

1. **No hardcoded hex** — every `lipgloss.Color("#...")` must use a named var from `styles/theme.go`. Existing violations (e.g. `PlanPreviewStyle` using `#aaaaaa`) must be migrated.
2. **No inline style creation in View()** — define styles as package vars or compose from base styles. Hot-path `NewStyle()` allocates.
3. **No competing bright accents** — a single row should have at most 2 accent colors. Use `fg3`/`muted` for less important elements.
4. **No borders on everything** — use borders for containers (panels, dialogs). Use indentation and color for hierarchy within containers.
5. **No emoji in chrome** — use Unicode box-drawing and geometric shapes (`●○◐✗⊘▸`). Emoji for content only (task status in glamour-rendered markdown is acceptable).

---

## Appendix A: Current vs New Token Mapping

| Current Code                    | Issue                          | New Token        |
|---------------------------------|--------------------------------|------------------|
| `lipgloss.Color("#aaaaaa")`     | Hardcoded, not gruvbox         | `Fg4` (#a89984)  |
| `lipgloss.Color("#666666")`     | Hardcoded, not gruvbox         | `Bg3` (#665c54)  |
| `lipgloss.Color("#ebdbb2")` raw | Should use var                 | `Text`           |
| `lipgloss.Color("#83a598")` raw | Should use var                 | `Secondary`      |

## Appendix B: Component → File Mapping

| Component          | Current File(s)                       |
|--------------------|---------------------------------------|
| Theme/colors       | `styles/theme.go`                     |
| Agent colors       | `styles/agent_colors.go`              |
| Glamour theme      | `styles/glamour.go`                   |
| Panel interface    | `panels/panel.go`                     |
| Task panel         | `panels/taskspanel/tasks.go`          |
| Tool panel         | `panels/toolspanel/tools.go`          |
| File panel         | `panels/filespanel/files.go`          |
| Session panel      | `panels/sessions/sessions.go`         |
| Skills panel       | `panels/skillspanel/skills.go`        |
| Config panel       | `panels/config/config.go`             |
| Analytics panel    | `panels/analyticspanel/analytics.go`  |
| Memory panel       | `panels/memorypanel/memory.go`        |
| Team panel         | `teampanel/panel.go`                  |
| Conversation       | `panels/conversationpanel/panel.go`   |
| Prompt             | `prompt/prompt.go`                    |
| Sidebar            | `sidebar/sidebar.go`                  |
| Sidebar blocks     | `sidebar/blocks/`                     |
| Toast              | `toast.go`                            |
| Command palette    | `commandpalette/palette.go`           |
| Permission dialog  | `permissions/dialog.go`               |
| Which-key          | `panels/whichkey/whichkey.go`         |
| Spinner            | `components/spinner.go`               |
| File picker        | `filepicker/picker.go`                |
| Model selector     | `modelselector/selector.go`           |
| Agent selector     | `agentselector/selector.go`           |
| Layout             | `layout.go`, `root.go`               |
| Docks              | `docks/dock.go`, `docks/todo_dock.go` |
