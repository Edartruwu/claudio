# TUI Code Locations Map

Quick reference for navigating the TUI codebase to understand and fix the 7 bugs.

---

## File Locations

### Primary Files
- **`internal/tui/root.go`** (6,488 lines)
  - Main Model type definition
  - Update() event handler
  - View() rendering
  - Agent detail rendering and key handling
  
- **`internal/tui/layout.go`** (89 lines)
  - buildSeparator()
  - placeOverlayAt() ← CRITICAL FOR BUG 1
  - renderPanelWithHelp()

---

## Bug Fix Location Index

### Bug 1: Overlay Bounds Validation
**File**: `layout.go`
```
Lines 24-63: func placeOverlayAt()
  - Line 35: row bounds check
  - Line 42-43: base line width padding
  - Line 50: overlay rune placement (col bounds)
  - Line 58-60: final truncation
```
**Function Call Sites**:
- `root.go:5829` - OverlayDrawer mode
- (Note: OverlayCentered at 5822 uses placeOverlay, not placeOverlayAt)

---

### Bug 2: Agent Detail Scroll Bounds
**File**: `root.go`
```
Lines 4684-4808: func (m Model) renderAgentDetail()
  - Lines 4768-4791: CRITICAL scroll calculation section
    - Line 4769: visibleH := height - 5
    - Line 4775: maxScroll := len(contentLines) - visibleH
    - Line 4779: if m.agentDetail.scroll > maxScroll
    - Line 4782: scroll := m.agentDetail.scroll
    - Line 4784: end := scroll + visibleH
    - Line 4793: contentLines[start:end] slice
```
**Key Nested Type**:
```
Lines 200-206: type agentDetailOverlay struct
  - Line 203: scroll int    ← BUG AREA
```

---

### Bug 3: Scroll Increment Without Bounds
**File**: `root.go`
```
Lines 4646-4675: func (m *Model) handleAgentDetailKey()
  - Line 4656: case 'j': m.agentDetail.scroll++      ← NO UPPER BOUND
  - Line 4659: if m.agentDetail.scroll > 0 { ... }  ← Lower bound check (asymmetric)
  - Line 4665: m.agentDetail.scroll = 999999        ← MAGIC NUMBER
  - Line 4669: m.agentDetail.scroll = 0             ← Reset to 0
```
**Caller**:
- `root.go:1145-1500` - Update() method handles FocusAgentDetail case

---

### Bug 4: Layout Dimension Guards
**File**: `root.go`
```
Lines 5694-5756: func (m *Model) layout()
  - No visible guards in first section
  - Computes viewport.Width, viewport.Height
  - Called from View() at line 5763
```
**Terminal Size Check**:
```
Lines 5758-5760: if m.tooSmall check (precondition)
  - min 60×20 required
```

---

### Bug 5: Panel Overlay Dimension Validation
**File**: `root.go`
```
Lines 5812-5832: Panel overlay rendering in View()
  - Lines 5812-5822: OverlayCentered mode
    - Line 5813: w := m.viewport.Width * 70 / 100
    - Line 5814: h := m.viewport.Height * 70 / 100
    - Line 5815-5820: Min guards only (no max)
    - Line 5822: placeOverlay(...) call
    
  - Lines 5823-5829: OverlayDrawer mode
    - Line 5824: drawerW := m.width * 35 / 100
    - Line 5825-5827: Min guard only (no max)
    - Line 5829: placeOverlayAt(topArea, panelView, 0, 0, mw, m.viewport.Height)
```

---

### Bug 6: Viewport Slice Index Safety
**File**: `root.go`
```
Lines 4784-4795: Content slicing in renderAgentDetail()
  - Line 4784: end := scroll + visibleH
  - Line 4785-4787: if end > len(contentLines) { end = len(contentLines) }
  - Line 4788: start := scroll
  - Line 4789-4791: if start > len(contentLines) { start = len(contentLines) }
  - Line 4793: for _, line := range contentLines[start:end]  ← SLICE CALL
```

---

### Bug 7: Window Resize & Scroll Bounds
**File**: `root.go`
```
Lines 572-1500: func (m Model) Update() event handler
  - Search for: tea.WindowSizeMsg  ← Window resize event
  - No explicit scroll recalculation on resize
  - layout() called from View() (line 5763), not from Update
```
**Related**:
- `layout()` at line 5694 (called every View())
- `renderAgentDetail()` at line 4684 (recalculates on every render)

---

## Function Call Graph

```
View() [5757]
  ├─ layout() [5763]
  │   └─ Computes viewport dimensions
  │
  ├─ viewport.View() [5770]
  │
  ├─ Overlay dialogs [5773-5805]
  │   ├─ renderPlanApprovalDialog()
  │   ├─ renderAskUserDialog()
  │   ├─ modelSelector.View()
  │   ├─ agentSelector.View()
  │   ├─ renderAgentDetail() [5804] ← FULL SCREEN OVERLAY
  │   └─ sessionPicker.View()
  │
  ├─ Panel overlay rendering [5808-5854]
  │   ├─ OverlayCentered → placeOverlay()
  │   ├─ OverlayDrawer → placeOverlayAt()
  │   └─ OverlayFullscreen → direct render
  │
  ├─ filesPanel / sidebar [5833-5854]
  │
  └─ Sections stacking [5856-5928]
      ├─ palette.View()
      ├─ filePicker.View()
      ├─ permission / todoDock
      ├─ renderStatusLine()
      ├─ prompt.View()
      ├─ renderSearchBar() / renderModeLine()
      ├─ renderHelpFooter()
      └─ renderStatusBar()

Update() [572]
  └─ Key handlers (1000+ lines)
      └─ handleAgentDetailKey() [4646]
          ├─ 'j' - scroll down
          ├─ 'k' - scroll up
          ├─ 'G' - goto end
          └─ 'g' - goto start
```

---

## Data Flow for Agent Detail Scroll

```
Update() event loop
  ↓
Key press 'j' / 'k' / 'G' / 'gg'
  ↓
handleAgentDetailKey() [4646]
  ├─ Modifies: m.agentDetail.scroll
  ├─ Issues: No bounds checking after increment/decrement
  └─ ← BUG 3 LOCATION
  
View() next frame [5757]
  ↓
layout() [5763]
  ├─ Computes viewport dimensions
  └─ ← BUG 4 LOCATION
  
renderAgentDetail() [4684]
  ├─ Reads: m.agentDetail.scroll
  ├─ Reads: agentDetail.state, toolCalls, etc.
  ├─ Calculates:
  │   ├─ visibleH [4769]
  │   ├─ maxScroll [4775] ← BUG 2
  │   ├─ start, end [4788, 4784] ← BUG 6
  │   └─ contentLines[start:end] [4793]
  ├─ Returns: rendered string
  └─ ← BUG 2 LOCATION
  
View() continues
  ├─ Panel overlay checks [5809-5854]
  │   └─ May call placeOverlay() or placeOverlayAt()
  │       └─ ← BUG 1, BUG 5 LOCATIONS
  └─ Returns combined layout string
```

---

## Critical Code Snippets

### Current (Buggy) Scroll Bound Calculation
```go
// root.go:4775-4791
maxScroll := len(contentLines) - visibleH  // Can be negative!
if maxScroll < 0 {
    maxScroll = 0
}
if m.agentDetail.scroll > maxScroll {
    m.agentDetail.scroll = maxScroll  // Never clamps to 0
}
scroll := m.agentDetail.scroll  // Could still be negative!

end := scroll + visibleH
if end > len(contentLines) {
    end = len(contentLines)
}
start := scroll
if start > len(contentLines) {
    start = len(contentLines)
}

for _, line := range contentLines[start:end] {  // start could be < 0!
```

### Current (Buggy) Overlay Bounds Check
```go
// layout.go:24-63
func placeOverlayAt(base, overlay string, x, y, width, height int) string {
    // ...
    for i, ol := range overlayLines {
        row := y + i
        if row < 0 || row >= len(baseLines) {  // Only checks row
            continue
        }
        // ...
        for j, r := range overlayRunes {
            col := x + j
            if col >= 0 && col < len(baseRunes) {  // Checks col but...
                baseRunes[col] = r
            }
        }
        // ... no upper bound check on x+len(overlayRunes)!
    }
}
```

---

## Test Entry Points

### For Bug 2 & 6 (Scroll bounds)
```go
// Create minimal test:
// m.agentDetail.scroll = -1000  // Negative scroll
// renderAgentDetail(100, 30)    // Should not panic
```

### For Bug 1 (Overlay bounds)
```go
// Test cases:
placeOverlayAt(base, overlay, -10, -10, 80, 24)  // Negative x, y
placeOverlayAt(base, overlay, 100, 10, 80, 24)   // x > width
placeOverlayAt(base, overlay, 10, 100, 80, 24)   // y > height
```

### For Bug 3 (Scroll increment)
```go
// Rapidly press 'j' 1000 times without 'G'
// Scroll should be clamped, not unbounded
```

---

## Summary Table

| Bug | File | Lines | Issue | Impact |
|-----|------|-------|-------|--------|
| 1 | layout.go | 24-63 | No x/y bounds check | Overlay off-screen |
| 2 | root.go | 4775-4791 | Scroll not clamped to 0 | Negative index panic |
| 3 | root.go | 4656-4670 | Unbounded increment | Infinite scroll |
| 4 | root.go | 5694+ | No dimension guards | Division by zero? |
| 5 | root.go | 5812-5832 | No max bounds for w/h | Overlay too large |
| 6 | root.go | 4793 | No slice safety checks | Panic on edge case |
| 7 | root.go | 572+ | No scroll recalc on resize | Stale bounds |

