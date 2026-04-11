# TUI Bug Fixes Reference Guide

## Overview
This document outlines 7 targeted bug fixes for overlay bounds, layout guards, and scroll edge cases in the TUI codebase.

---

## Bug Fix 1: Overlay Bounds Validation in `placeOverlayAt()`

**File**: `internal/tui/layout.go`, lines 24-63

**Current Code**:
```go
func placeOverlayAt(base, overlay string, x, y, width, height int) string {
    baseLines := strings.Split(base, "\n")
    overlayLines := strings.Split(overlay, "\n")

    // Pad base to fill the container height
    for len(baseLines) < height {
        baseLines = append(baseLines, strings.Repeat(" ", width))
    }

    for i, ol := range overlayLines {
        row := y + i
        if row < 0 || row >= len(baseLines) {
            continue
        }
        // ... rune placement logic
    }
    // ...
}
```

**Issues**:
1. No validation that `x` is within `[0, width)`
2. No validation that overlay doesn't extend beyond `width + x`
3. Doesn't handle case where `x < 0` (negative x)
4. Doesn't check if `y` is reasonable relative to `height`
5. Rune logic assumes proper width padding but doesn't validate `width` parameter

**Fix**:
- Add pre-condition checks for x, y, width, height parameters
- Clamp x to `[0, width)` and y to `[0, height)`
- Validate overlay doesn't exceed bounds before rune placement
- Guard against negative or zero width/height

**Related Code**:
- Called from `root.go` line 5829 (OverlayDrawer mode)
- Called from `root.go` line 5822 (OverlayCentered - but uses `placeOverlay`, not `placeOverlayAt`)

---

## Bug Fix 2: Agent Detail Scroll Bounds in `renderAgentDetail()`

**File**: `internal/tui/root.go`, lines 4768-4791

**Current Code**:
```go
visibleH := height - 5 // header + task + hints
if visibleH < 3 {
    visibleH = 3
}

// Clamp scroll
maxScroll := len(contentLines) - visibleH
if maxScroll < 0 {
    maxScroll = 0
}
if m.agentDetail.scroll > maxScroll {
    m.agentDetail.scroll = maxScroll
}
scroll := m.agentDetail.scroll

end := scroll + visibleH
if end > len(contentLines) {
    end = len(contentLines)
}
start := scroll
if start > len(contentLines) {
    start = len(contentLines)
}

for _, line := range contentLines[start:end] {
    b.WriteString(line + "\n")
}
```

**Issues**:
1. Doesn't clamp `scroll` to minimum of 0 (could be negative)
2. Slice `contentLines[start:end]` has no guard if `start > end`
3. If `visibleH > len(contentLines)`, `maxScroll` becomes negative but scroll is still clamped to it
4. No validation that calculated indices are safe for slicing

**Fix**:
- Add guard: `scroll = max(0, scroll)` after checking bounds
- Recalculate maxScroll after any scroll operation
- Validate: `maxScroll = max(0, maxScroll)`
- Before slice: ensure `0 <= start <= end <= len(contentLines)`
- Consider: if content < visibleH, just show all (no scroll needed)

**Related Code**:
- Updated in `Update()` via j/k key handlers (lines 4656-4670)
- Used in `handleAgentDetailKey()` (line 4646)

---

## Bug Fix 3: Scroll Increment/Decrement Without Bounds

**File**: `internal/tui/root.go`, lines 4646-4675 (agent detail key handler)

**Current Code**:
```go
case 'j':
    m.agentDetail.scroll++
    return m, nil
case 'k':
    if m.agentDetail.scroll > 0 {
        m.agentDetail.scroll--
    }
    return m, nil
// ...
case 'G':
    m.agentDetail.scroll = 999999  // ← Arbitrary!
    return m, nil
case 'g':
    m.agentDetail.scroll = 0
    return m, nil
```

**Issues**:
1. `j` increment has no upper bound check
2. `G` (goto end) uses magic number `999999` instead of calculating actual max
3. No recalculation of maxScroll after scroll changes
4. Scroll value can grow without limit

**Fix**:
- After each scroll modification, recalculate `maxScroll` based on current content
- Clamp scroll: `scroll = max(0, min(scroll, maxScroll))`
- Replace `999999` with actual calculation of last line
- Extract scroll clamping to helper function

**Related Code**:
- Helper function needed: `clampAgentDetailScroll()`
- Called from `handleAgentDetailKey()` (line 4646)

---

## Bug Fix 4: Layout Dimension Guards

**File**: `internal/tui/root.go`, line 5694 onwards (`layout()`)

**Current Code**:
```go
func (m *Model) layout() {
    // Computes m.viewport.Width, m.viewport.Height
    // based on m.width, m.height, sidebar width, etc.
}
```

**Issues**:
1. No guard against `m.width < 60` or `m.height < 20`
2. Percentage calculations (e.g., `m.width * 70 / 100`) don't validate result
3. Sidebar width calculation doesn't prevent overflow
4. No check that computed viewport dimensions are positive

**Fix**:
- At start of `layout()`: validate `m.width >= 60 && m.height >= 20`
- Guard all percentage splits: ensure result >= minimum required width
- Validate final viewport dimensions before use
- Return early if terminal is too small (rely on `View()` check at line 5758)

**Related Code**:
- Called from `View()` at line 5763
- Terminal size check at line 5758-5760

---

## Bug Fix 5: Panel Overlay Dimension Validation

**File**: `internal/tui/root.go`, lines 5812-5832 (panel overlay rendering)

**Current Code**:
```go
case OverlayCentered:
    w := m.viewport.Width * 70 / 100
    h := m.viewport.Height * 70 / 100
    if w < 40 {
        w = 40
    }
    if h < 10 {
        h = 10
    }
    panelView := renderPanelWithHelp(m.activePanel, w, h)
    topArea = placeOverlay(topArea, panelView, m.viewport.Width, m.viewport.Height)

case OverlayDrawer:
    drawerW := m.width * 35 / 100
    if drawerW < 30 {
        drawerW = 30
    }
    panelView := renderPanelWithHelp(m.activePanel, drawerW, m.viewport.Height)
    topArea = placeOverlayAt(topArea, panelView, 0, 0, mw, m.viewport.Height)
```

**Issues**:
1. OverlayCentered: computes w, h but doesn't validate they don't exceed viewport
2. OverlayDrawer: drawerW could exceed `m.width` (no upper bound check)
3. OverlayCentered: if w > viewport.Width, overlay goes off-screen
4. OverlayDrawer: passes `mw` (might be wrong) instead of `m.width` to `placeOverlayAt()`

**Fix**:
- Add upper bounds: `w = min(w, m.viewport.Width)`, `h = min(h, m.viewport.Height)`
- For OverlayDrawer: `drawerW = min(drawerW, m.width)`
- Ensure min dimensions are met without exceeding max
- Pass correct width parameter to `placeOverlayAt()`

**Related Code**:
- Calls `placeOverlay()` (line 5822) and `placeOverlayAt()` (line 5829)
- Uses `renderPanelWithHelp()` (from `layout.go`)

---

## Bug Fix 6: Viewport Slice Index Safety

**File**: `internal/tui/root.go`, lines 4784-4795 (agent detail content slicing)

**Current Code**:
```go
end := scroll + visibleH
if end > len(contentLines) {
    end = len(contentLines)
}
start := scroll
if start > len(contentLines) {
    start = len(contentLines)
}

for _, line := range contentLines[start:end] {
    b.WriteString(line + "\n")
}
```

**Issues**:
1. Doesn't validate `start <= end` before slicing
2. If `start == end == len(contentLines)`, slice is empty (correct but no explicit check)
3. If `scroll < 0`, `start` becomes negative (Go allows but unintended)
4. No assertion that slice operation is safe

**Fix**:
- Ensure `start >= 0` (clamp scroll to 0)
- Ensure `end <= len(contentLines)`
- Ensure `start <= end`
- Add defensive slice guards before actual slice operation
- Consider: helper function `safeSlice(lines []string, start, end int) []string`

---

## Bug Fix 7: Window Resize & Scroll Bounds Invalidation

**File**: `internal/tui/root.go`, throughout `Update()` (line 572 onwards)

**Current Code**:
- When `tea.WindowSizeMsg` is received (terminal resize), viewport dimensions change
- Scroll bounds become stale if visibleH changes
- No recalculation of agent detail maxScroll on resize

**Issues**:
1. Agent detail scroll not recalculated on window resize
2. Panel overlay dimensions may change but overlay remains at old position
3. Sidebar width recalculation on resize doesn't validate new bounds

**Fix**:
- In `Update()` handler for `tea.WindowSizeMsg`:
  - Recalculate all scroll bounds
  - Clamp current scroll values to new bounds
  - Reset viewport search if active
- Add helper: `recalculateScrollBounds()` for agent detail
- Call from both initialization and window resize handlers

**Related Code**:
- Window size handler in `Update()` (search for `tea.WindowSizeMsg`)
- `layout()` function (line 5694)
- Agent detail overlay rendering (line 4684)

---

## Implementation Order

**Recommended order for applying fixes**:

1. **Bug Fix 1** (Overlay bounds): Base layer, no dependencies
2. **Bug Fix 2** (Scroll bounds): Foundation for scroll safety
3. **Bug Fix 3** (Scroll increment): Depends on Bug Fix 2
4. **Bug Fix 4** (Layout guards): Independent, foundational
5. **Bug Fix 5** (Panel overlay dims): Uses Bug Fix 1 & 4
6. **Bug Fix 6** (Slice safety): Depends on Bug Fix 2 & 3
7. **Bug Fix 7** (Window resize): Last, uses all previous fixes

---

## Testing Strategy

### Unit Tests to Add
- `TestPlaceOverlayAtBounds()` - Overlay with negative x/y, oversized overlay
- `TestAgentDetailScrollClamping()` - Scroll with 0, 1, 10, 1000+ lines
- `TestPanelOverlayDimensions()` - Small viewport, large panels
- `TestViewportSliceSafety()` - Edge cases in content slicing

### Integration Tests
- Resize terminal while agent detail is open
- Rapidly press j/k with minimal content
- Open panel overlay with < 60px width
- Scroll to end, then trigger content change (new tool call)

### Manual Tests
- Terminal 60x20 (minimum)
- Terminal 200x50 (maximum typical)
- Agent detail with 0 lines, 1 line, 5 lines, 100+ lines
- Scrolling: j, jjj, G, gg, resize, add content

---

## Key Helper Functions to Extract

```go
// Clamp a value between min and max
func clamp(val, min, max int) int {
    if val < min {
        return min
    }
    if val > max {
        return max
    }
    return val
}

// Calculate max scroll given content size and visible height
func calculateMaxScroll(contentLen, visibleH int) int {
    maxScroll := contentLen - visibleH
    if maxScroll < 0 {
        return 0
    }
    return maxScroll
}

// Safely slice with bounds checking
func safeSlice(lines []string, start, end int) []string {
    if start < 0 {
        start = 0
    }
    if end > len(lines) {
        end = len(lines)
    }
    if start > end {
        start = end
    }
    return lines[start:end]
}

// Recalculate and clamp agent detail scroll
func (m *Model) recalculateAgentDetailScroll(contentLen, visibleH int) {
    if m.agentDetail == nil {
        return
    }
    maxScroll := calculateMaxScroll(contentLen, visibleH)
    m.agentDetail.scroll = clamp(m.agentDetail.scroll, 0, maxScroll)
}
```

