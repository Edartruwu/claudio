# Keybindings Feature Codebase Analysis

## Overview
This document maps the exact code architecture for integrating a keybindings picker overlay into Claudio's TUI, paralleling existing picker patterns (model selector, command palette, file picker).

---

## 1. Picker Architecture

### 1.1 Core Picker Component
**Location:** `internal/tui/picker/`

**Files:**
- `model.go` — BubbleTea Model implementing Update() + View()
  - Handles key events: Up/Down (nav), Tab (multi-select), Enter (confirm), Esc (cancel)
  - Manages prompt filtering via Sorter (fuzzy score)
  - Async entry loading from Finder via channel
  - Multi-select tracking via map[string]bool keyed by Entry.Ordinal
  
- `config.go` — Configuration struct
  ```go
  type Config struct {
    Title string
    Finder Finder           // async entry provider
    Sorter Sorter           // filters/scores entries (default: FuzzySorter)
    Previewer Previewer     // optional preview pane (can be nil)
    Layout LayoutStrategy   // horizontal|vertical|dropdown|ivy
    OnSelect func(Entry)           // single-select callback
    OnMultiSelect func([]Entry)    // multi-select callback
  }
  ```

- `interfaces.go` — Abstract interfaces
  - **Finder** — async entry provider
    ```go
    type Finder interface {
      Find(ctx context.Context) <-chan Entry
      Close()
    }
    ```
  - **Sorter** — entry filter/scorer
    ```go
    type Sorter interface {
      Score(prompt string, e Entry) float64
    }
    ```
  - **Previewer** — optional preview renderer (not required for keybindings)

- `entry.go` — Entry struct
  ```go
  type Entry struct {
    Value any              // arbitrary data (binding object)
    Display string         // user-visible string
    Ordinal string         // unique key for multi-select
    Meta map[string]any    // arbitrary metadata
  }
  ```

- `fuzzy_sorter.go` — Default Sorter implementation (substring matching)

- `layout.go` — Render functions for each layout strategy

### 1.2 Key Behaviors in model.go
- **handleKey()** (line 158-236):
  - Up/Down, Ctrl+P/N — cursor movement
  - Tab — toggle multi-select on highlighted entry
  - Enter — confirm (single or multi)
  - Esc — cancel
  - Ctrl+U/D — preview scroll
  - Backspace/Ctrl+H — backspace from prompt
  - Other runes — append to prompt, refilter
  - **Returns (Model, Cmd)** — model is copy with updated state

- **Update()** message handling:
  - `tea.WindowSizeMsg` — set width/height
  - `EntryMsg` — new entry from Finder, append to allEntries, refilter
  - `finderDoneMsg` — async load complete, set loading=false
  - `tea.KeyMsg` — delegate to handleKey()

### 1.3 Finder Implementations
**Location:** `internal/tui/picker/finders/`

Pattern for new Finder:
1. Implement `Find(ctx) <-chan Entry` — spawns goroutine, yields entries to channel, closes on done
2. Implement `Close()` — stub is fine
3. Return Entry with Display, Ordinal, Meta set

**Existing Finders:**
- **AgentFinder** (`finder_agents.go`) — emits all agents from TeammateRunner
- **BufferFinder** (`finder_buffers.go`) — emits all windows from WindowManager
- **CommandFinder** (`finder_commands.go`) — emits all commands from command registry

---

## 2. Root Model Integration Pattern

**Location:** `internal/tui/root.go`

### 2.1 Model Field Definition (lines 78-109)
```go
type Model struct {
  // ... other fields ...
  palette             commandpalette.Model
  cmdline             cmdline.Model
  filePicker          filepicker.Model
  modelSelector       modelselector.Model    // <-- OVERLAY MODEL
  agentSelector       agentselector.Model
  teamSelector        teamselector.Model
  // ...
}
```

**For keybindings picker:** Will add a new field like `keybindingsPicker  picker.Model` (or wrapped in a custom selector like modelselector).

### 2.2 Key Handling Cascade (lines 1448-1517)
Order matters — first match wins, later overlays don't run:

```go
// 1. Model selector gets priority
if m.focus == FocusModelSelector {
  m.modelSelector, cmd = m.modelSelector.Update(msg)
  return m, tea.Batch(cmds...)
}

// 2. Agent selector
if m.focus == FocusAgentSelector {
  // ...
}

// 3. Permission dialog
if m.focus == FocusPermission {
  // ...
}

// 4. Files panel
if m.focus == FocusFiles && m.filesPanel != nil {
  cmd, consumed := m.filesPanel.Update(msg)
  if !consumed || !m.filesPanel.IsActive() {
    m.filesPanel.SetFocused(false)
    m.focus = FocusPrompt
    m.prompt.Focus()
    m.refreshViewport()
  }
  return m, cmd
}

// 5. Panel focus mode
if m.focus == FocusPanel && m.activePanel != nil {
  // ...
}

// 6. Command palette (secondary overlay, only if FocusPrompt)
if m.focus == FocusPrompt && !m.streaming && m.palette.IsActive() {
  if cmd, consumed := m.palette.Update(msg); consumed {
    return m, tea.Batch(cmds...)
  }
}

// 7. File picker (secondary overlay, only if FocusPrompt)
if m.focus == FocusPrompt && !m.streaming && m.filePicker.IsActive() {
  if cmd, consumed := m.filePicker.Update(msg); consumed {
    return m, tea.Batch(cmds...)
  }
}
```

**Key insight:** Primary overlays (FocusModelSelector, FocusAgentSelector) consume entire focus. Secondary overlays (palette, filePicker) only active when FocusPrompt + IsActive().

### 2.3 Window Size Sync (lines 859-869)
```go
case tea.WindowSizeMsg:
  m.width = msg.Width
  m.height = msg.Height
  // ... set tooSmall ...
  m.palette.SetWidth(m.width)
  m.cmdline.SetWidth(m.width)
  m.filePicker.SetWidth(m.width)
  m.modelSelector.SetWidth(m.width)
  m.agentSelector.SetWidth(m.width)
  m.agentSelector.SetHeight(m.height)
  m.layout()
```

Every overlay model must implement `SetWidth(width)` and optionally `SetHeight(height)`.

### 2.4 View Rendering (lines 6801-6850)
```go
// Line 6801: Model selector overlay
if m.modelSelector.IsActive() {
  overlay := m.modelSelector.View()
  vpView = placeOverlay(vpView, overlay, mw, m.viewport.Height)
}

// Line 6805: Agent selector overlay
if m.agentSelector.IsActive() {
  overlay := m.agentSelector.View()
  vpView = placeOverlay(vpView, overlay, mw, m.viewport.Height)
}

// Line 6807: Team selector overlay
if m.teamSelector.IsActive() {
  overlay := m.teamSelector.View()
  vpView = placeOverlay(vpView, overlay, mw, m.viewport.Height)
}

// ... permission, window float ...

// Line 6900: File picker overlay (rendered as is, not placeOverlay)
if pickerView := m.filePicker.View(); pickerView != "" {
  sections = append(sections, pickerView)
}
```

**Pattern:** Call `IsActive()` + `View()` on each overlay, stack onto vpView or sections.

### 2.5 Focus & Activation
**Location:** `internal/tui/focus.go` (lines 1-18)

```go
const (
  FocusPrompt        Focus = iota
  FocusViewport
  FocusPermission
  FocusModelSelector          // <-- PRIMARY OVERLAY
  FocusAgentSelector          // <-- PRIMARY OVERLAY
  FocusTeamSelector           // <-- PRIMARY OVERLAY
  FocusPanel
  FocusPlanApproval
  FocusAskUser
  FocusAgentDetail
  FocusFiles                  // <-- SPECIAL: FILES PANEL
)
```

**Add for keybindings:** `FocusKeybindingsPicker Focus = iota` (new constant)

---

## 3. Model Selector as Reference Pattern

**Location:** `internal/tui/modelselector/selector.go`

Model selector = **primary overlay** (gets focus, consumes all keys):

```go
type Model struct {
  models   []ModelOption
  thinking []ThinkingOption
  budgets  []BudgetOption
  
  modelIdx, thinkingIdx, budgetIdx int
  
  section Section        // which sub-section focused
  active  bool
  width   int
}

// Activate/Deactivate
func (m *Model) Activate() { m.active = true }
func (m *Model) Deactivate() { m.active = false }
func (m *Model) IsActive() bool { return m.active }

// Update(msg tea.Msg) tea.Cmd
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)

// View() string
func (m *Model) View() string

// SetWidth(w int) / SetHeight(h int)
func (m *Model) SetWidth(w int) { m.width = w }
func (m *Model) SetHeight(h int) { /* optional */ }
```

Emits `ModelSelectedMsg` on Enter, `DismissMsg` on Esc.

---

## 4. Existing Overlay Interaction Points

### 4.1 Opening Overlays (Search for "OpenModel", "OpenAgent", etc.)

**Model Selector:** Line 3063-3067 in root.go
```go
case keymap.ActionChangeModel:
  m.modelSelector = modelselector.NewWithModels(...)
  m.modelSelector.SetWidth(m.width)
  m.focus = FocusModelSelector
  m.prompt.Blur()
  return m, nil
```

### 4.2 Closing/Message Handling

**Model Selector Message:** Root.go Search for `modelselector.ModelSelectedMsg`
- User pressed Enter → Model gets updated, model selector deactivates
- Focus resets to FocusPrompt

**Files Panel Message:** Line 1058-1067 in root.go
```go
if m.focus == FocusFiles && m.filesPanel != nil {
  cmd, consumed := m.filesPanel.Update(msg)
  if !consumed || !m.filesPanel.IsActive() {
    m.filesPanel.SetFocused(false)
    m.focus = FocusPrompt
    m.prompt.Focus()
    m.refreshViewport()
  }
  return m, cmd
}
```

**Pattern:** On close (Esc or action complete), set `m.focus = FocusPrompt`, call `m.prompt.Focus()`, call `m.refreshViewport()`.

---

## 5. Keybindings-Specific Data

### 5.1 Data Models Needed
1. **KeyBinding struct** (minimal)
   ```go
   type KeyBinding struct {
     Action string        // "PanelFiles", "ChangeModel", etc.
     Key    string        // "space f", "ctrl+m", etc.
     Description string   // user-facing description
   }
   ```

2. **Display format** for picker Entry
   - **Display:** `"space f  -  Toggle Files Panel"`
   - **Ordinal:** action + key (e.g., "PanelFiles:space f")
   - **Value:** the KeyBinding struct itself

### 5.2 Finder for Keybindings
1. Read from keymap (internal/tui/keymap package)
2. Convert each action→key mapping to Entry
3. Emit via channel
4. Support search/filter (FuzzySorter handles display string matching)

### 5.3 Keybindings Picker Integration Points

**New Focus Constant:**
```go
// internal/tui/focus.go
FocusKeybindingsPicker Focus = iota
```

**New Root Model Field:**
```go
// internal/tui/root.go
keybindingsPicker picker.Model
```

**Initialization:**
```go
// In New() or init func
finder := /* NewKeybindingsFinder(keymap) */
m.keybindingsPicker = picker.New(picker.Config{
  Title: "Keybindings",
  Finder: finder,
  Sorter: picker.NewFuzzySorter(),
  Layout: picker.LayoutVertical, // or other
  OnSelect: func(entry picker.Entry) {
    // Handle selection
    m.handleKeybindingSelected(entry)
  },
})
```

**Key Handling (insert into cascade at line 1448-1517):**
```go
// Add after FocusAgentSelector, before FocusPermission
if m.focus == FocusKeybindingsPicker {
  m.keybindingsPicker, cmd = m.keybindingsPicker.Update(msg)
  cmds = append(cmds, cmd)
  return m, tea.Batch(cmds...)
}
```

**Opening (e.g., `?` key):**
```go
case "?":
  if !m.streaming {
    m.keybindingsPicker = picker.New(picker.Config{
      // ...
    })
    m.focus = FocusKeybindingsPicker
    m.prompt.Blur()
  }
```

**View (in render section around line 6801):**
```go
if m.keybindingsPicker.IsActive() {
  overlay := m.keybindingsPicker.View()
  vpView = placeOverlay(vpView, overlay, mw, m.viewport.Height)
}
```

**Window Size Sync (in tea.WindowSizeMsg handler):**
```go
m.keybindingsPicker.SetWidth(m.width)
m.keybindingsPicker.SetHeight(m.height)
```

---

## 6. Code Locations Summary

| Component | Path | Lines |
|-----------|------|-------|
| Picker model.go | internal/tui/picker/model.go | 1-317 |
| Picker config | internal/tui/picker/config.go | 1-44 |
| Picker interfaces | internal/tui/picker/interfaces.go | All |
| Finder examples | internal/tui/picker/finders/finder_*.go | All |
| Root model def | internal/tui/root.go | 78-109 |
| Root key cascade | internal/tui/root.go | 1448-1517 |
| Root view overlay | internal/tui/root.go | 6801-6850 |
| Window size | internal/tui/root.go | 859-869 |
| Focus constants | internal/tui/focus.go | 1-18 |
| Model selector | internal/tui/modelselector/selector.go | All (reference) |

---

## 7. Implementation Checklist

- [ ] Create `internal/tui/keybindings/` directory
- [ ] Implement `KeyBinding` struct (data model)
- [ ] Implement `Finder` over keymap/bindings
- [ ] Create keybindings selector wrapper (optional, or use raw picker.Model)
- [ ] Add `FocusKeybindingsPicker` to focus.go
- [ ] Add `keybindingsPicker` field to root.Model
- [ ] Initialize in root.New()
- [ ] Add key cascade handler in Update() (line ~1456)
- [ ] Add View() rendering (line ~6805)
- [ ] Add window size sync (line ~865)
- [ ] Wire trigger key (e.g., "?" or "ctrl+h")
- [ ] Add help footer hint for keybindings mode

---

## 8. Files Touched Summary

```
✓ internal/tui/picker/model.go          (read only, reference)
✓ internal/tui/picker/config.go         (read only, reference)
✓ internal/tui/picker/interfaces.go     (read only, reference)
✓ internal/tui/picker/finders/          (read only, reference patterns)
✓ internal/tui/root.go                  (MODIFY: add field, cascade, view, size)
✓ internal/tui/focus.go                 (MODIFY: add FocusKeybindingsPicker)
• internal/tui/keybindings/             (CREATE: new package)
  • model.go                            (CREATE: KeyBinding struct)
  • finder.go                           (CREATE: Finder impl)
  • selector.go                         (CREATE: optional wrapper like modelselector)
```

---

## 9. Data Flow Example

1. User presses `?` (or trigger key)
   → root.Update() key handler
   → Create new picker.Model with keybindings finder
   → Set focus = FocusKeybindingsPicker
   → Finder spawns goroutine, emits entries

2. Picker renders async entries, user types to filter
   → Prompt text updated
   → refilter() rescores all entries
   → View shows top matches

3. User presses Enter on highlighted entry
   → OnSelect callback fires
   → root.handleKeybindingSelected() executes action
   → Deactivate picker, reset focus to FocusPrompt

4. User presses Esc
   → picker.PickerClosedMsg emitted
   → root handles msg, closes picker, resets focus
