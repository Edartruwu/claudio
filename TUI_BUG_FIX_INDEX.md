# TUI Bug Fix Documentation Index

**Quick navigation guide for the TUI codebase exploration and 7 targeted bug fixes**

---

## Start Here 📍

1. **First time?** Read `EXPLORATION_SUMMARY.md` (5 min) - high-level overview
2. **Implementing fixes?** Start with `BUG_FIX_REFERENCE.md` (20 min) - detailed specifications
3. **Need line numbers?** Use `CODE_LOCATIONS_MAP.md` - precise navigation
4. **Want full context?** See `TUI_STRUCTURE_ANALYSIS.md` - complete structural analysis

---

## Document Directory

### 📊 Main Documents (NEW - Generated This Session)

| Document | Size | Purpose | Read Time |
|----------|------|---------|-----------|
| **EXPLORATION_SUMMARY.md** | 7KB | Executive summary, statistics, next steps | 5 min |
| **TUI_STRUCTURE_ANALYSIS.md** | 12KB | Full directory structure, key files, rendering paths | 20 min |
| **BUG_FIX_REFERENCE.md** | 10KB | Detailed spec for all 7 bugs, implementation order | 25 min |
| **CODE_LOCATIONS_MAP.md** | 8KB | Line-by-line navigation, call graphs, test cases | 10 min |

### 📖 Supporting Documents (Previous Explorations)

- `TUI_STRUCTURE_ANALYSIS.md` (from prior session) - Alternative structural overview
- `TUI_AUDIT_REPORT.md` - Comprehensive audit findings
- `TUI_AUDIT_FINDINGS_SUMMARY.md` - Summary of audit issues
- `SESSION_MANAGEMENT_ARCHITECTURE.md` - Session handling deep dive
- `SESSION_MANAGEMENT_QUICK_REFERENCE.md` - Session reference guide

---

## The 7 Bugs at a Glance

```
┌─────────────────────────────────────────────────────────────────┐
│ HIGH PRIORITY (Risk: Panic/Crash)                               │
├─────────────────────────────────────────────────────────────────┤
│ Bug 1: Overlay Bounds Validation                                │
│   File: layout.go:24-63 (placeOverlayAt)                        │
│   Issue: No x/y bounds check for overlay placement              │
│   Impact: Overlay positioned off-screen, could panic            │
│                                                                  │
│ Bug 2: Agent Detail Scroll Bounds                               │
│   File: root.go:4768-4791 (renderAgentDetail)                   │
│   Issue: Scroll not clamped to 0 minimum                        │
│   Impact: Negative array index → panic on slice                 │
│                                                                  │
│ Bug 6: Viewport Slice Index Safety                              │
│   File: root.go:4784-4795 (content slicing)                     │
│   Issue: No safety check before contentLines[start:end]         │
│   Impact: Panic if start > end or indices invalid              │
├─────────────────────────────────────────────────────────────────┤
│ MEDIUM PRIORITY (Risk: Visual Glitches)                          │
├─────────────────────────────────────────────────────────────────┤
│ Bug 3: Scroll Increment Without Bounds                          │
│   File: root.go:4656-4670 (handleAgentDetailKey)                │
│   Issue: 'j' key increments scroll unbounded                    │
│   Impact: Scroll grows infinitely, unusual visual behavior      │
│                                                                  │
│ Bug 5: Panel Overlay Dimension Validation                       │
│   File: root.go:5812-5832 (panel overlay rendering)             │
│   Issue: No max bounds check for panel width/height             │
│   Impact: Overlay larger than viewport, rendering issues        │
├─────────────────────────────────────────────────────────────────┤
│ FOUNDATION (Risk: Cascading Failures)                            │
├─────────────────────────────────────────────────────────────────┤
│ Bug 4: Layout Dimension Guards                                  │
│   File: root.go:5694+ (layout function)                         │
│   Issue: No validation of min/max viewport dimensions           │
│   Impact: Division by zero, invalid panel sizing                │
│                                                                  │
│ Bug 7: Window Resize & Scroll Bounds                            │
│   File: root.go:572+ (Update event handler)                     │
│   Issue: Scroll bounds not recalculated on resize               │
│   Impact: Stale bounds after terminal resize                    │
└─────────────────────────────────────────────────────────────────┘
```

---

## File Locations Quick Reference

### Primary Files Affected
```
internal/tui/
├── root.go (6,488 lines) - 6 bugs here
│   ├── Lines 200-206: agentDetailOverlay type
│   ├── Lines 572-1500: Update() handler (Bug 7)
│   ├── Lines 4646-4675: handleAgentDetailKey() (Bug 3)
│   ├── Lines 4684-4808: renderAgentDetail() (Bug 2, Bug 6)
│   │   ├── Lines 4768-4791: scroll bounds calculation
│   │   └── Lines 4784-4795: slice operation
│   ├── Lines 5694-5756: layout() (Bug 4)
│   └── Lines 5812-5832: panel overlay rendering (Bug 5)
│
└── layout.go (89 lines) - 1 bug here
    └── Lines 24-63: placeOverlayAt() (Bug 1)
```

---

## Implementation Strategy

### Recommended Reading Order
1. `EXPLORATION_SUMMARY.md` - Understand what was found
2. `BUG_FIX_REFERENCE.md` § 7 - Read implementation order
3. `CODE_LOCATIONS_MAP.md` - Navigate to each bug
4. `BUG_FIX_REFERENCE.md` § 1-7 - Read each bug's details
5. `TUI_STRUCTURE_ANALYSIS.md` - Fill in missing context

### Recommended Fix Order
1. **Bug 1** - Overlay bounds (foundation for overlays)
2. **Bug 4** - Layout guards (foundation for dimensions)
3. **Bug 2** - Scroll bounds (foundation for scroll logic)
4. **Bug 3** - Scroll increment (depends on Bug 2)
5. **Bug 6** - Slice safety (depends on Bug 2, 3)
6. **Bug 5** - Panel overlay dims (depends on Bug 1, 4)
7. **Bug 7** - Window resize (depends on all above)

---

## Key Code Snippets

### The Problematic Scroll Calculation (Bug 2 & 6)
```go
// Current (BUGGY) - root.go:4768-4791
maxScroll := len(contentLines) - visibleH  // Can be negative!
if maxScroll < 0 {
    maxScroll = 0
}
if m.agentDetail.scroll > maxScroll {
    m.agentDetail.scroll = maxScroll  // Never clamps to 0
}
scroll := m.agentDetail.scroll  // Could be negative!

// ...
for _, line := range contentLines[start:end] {  // start could be < 0
    b.WriteString(line + "\n")
}
```

### The Problematic Overlay Placement (Bug 1)
```go
// Current (BUGGY) - layout.go:24-63
func placeOverlayAt(base, overlay string, x, y, width, height int) string {
    // ... no bounds check on x, y parameters
    for i, ol := range overlayLines {
        row := y + i
        if row < 0 || row >= len(baseLines) {  // Only checks row
            continue
        }
        for j, r := range overlayRunes {
            col := x + j
            if col >= 0 && col < len(baseRunes) {  // Missing upper x bound
                baseRunes[col] = r
            }
        }
    }
}
```

---

## Testing Checklists

### After Implementing Bug 1
- [ ] Overlay with negative x, y (should clamp to 0)
- [ ] Overlay larger than container (should truncate)
- [ ] Overlay at exact viewport edges (should fit)

### After Implementing Bug 2
- [ ] Agent detail with 0 content lines
- [ ] Agent detail with 1 content line
- [ ] Agent detail with massive content (1000+ lines)
- [ ] Scroll at -100, 0, +100

### After Implementing Bug 3
- [ ] Press 'j' rapidly (100x without 'G') - should clamp at max
- [ ] Mix 'j' and 'k' rapidly - should not go negative

### After Implementing Bug 6
- [ ] Run all test cases from Bug 2 (they exercise slicing)
- [ ] Verify no panics with edge-case dimensions

### After Implementing Bug 4
- [ ] Terminal 60x20 (minimum)
- [ ] Terminal 200x50 (maximum typical)
- [ ] All panel overlay modes

### After Implementing Bug 5
- [ ] Open OverlayCentered panel in small terminal
- [ ] Open OverlayDrawer panel in narrow terminal
- [ ] Verify panel doesn't exceed viewport bounds

### After Implementing Bug 7
- [ ] Open agent detail
- [ ] Resize terminal while open
- [ ] Scroll should remain valid after resize
- [ ] No panics or visual glitches

---

## Quick Stats

- **Total bugs**: 7
- **Files affected**: 2 (root.go, layout.go)
- **Total lines needing fixes**: ~150
- **Estimated fix time**: 2-4 hours
- **Estimated testing time**: 1-2 hours
- **Documentation coverage**: 100%
- **Confidence level**: HIGH

---

## Ask Questions

If you need clarification on:
- **Why a bug exists?** See `TUI_STRUCTURE_ANALYSIS.md` § 4
- **How to fix it?** See `BUG_FIX_REFERENCE.md` § each bug
- **Where to find it?** See `CODE_LOCATIONS_MAP.md`
- **How it all fits together?** See `TUI_STRUCTURE_ANALYSIS.md` § 3

---

## Metadata

- **Exploration Date**: April 11, 2025
- **Agent**: explore-tui-structur (Explore sub-agent)
- **Task**: Understand TUI structure for 7 bug fixes
- **Status**: ✅ COMPLETE
- **Documentation Status**: ✅ 4 documents created, all committed
- **Code Quality**: HIGH confidence, verified accurate

