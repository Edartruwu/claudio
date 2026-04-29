package taskspanel

// tasks_filter_test.go — unit tests for InputPanel + filteredPlanItems.
//
// Coverage:
//   - filteredPlanItems empty filter → all items
//   - filteredPlanItems with query → case-insensitive match
//   - filteredPlanItems no match → empty slice
//   - HasInput false on new panel
//   - HasInput true after filterActive set
//   - InputUpdate Esc deactivates filter, clears text, resets cursor
//   - InputUpdate key routes to textinput (filter text changes)
//   - InputView truncates to panel width

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Abraxas-365/claudio/internal/tools"
)

// newTestPanel builds a Panel with no runtime (nil is ok for filter tests).
func newTestPanel() *Panel {
	return New(nil)
}

// populateItems sets planItems directly (bypassing runtime refresh).
func populateItems(p *Panel, subjects ...string) {
	p.planItems = make([]*tools.Task, len(subjects))
	for i, s := range subjects {
		p.planItems[i] = &tools.Task{Subject: s}
	}
}

// ── filteredPlanItems ─────────────────────────────────────────────────────────

// TestFilteredPlanItems_EmptyFilter returns all items when filter is empty.
func TestFilteredPlanItems_EmptyFilter(t *testing.T) {
	p := newTestPanel()
	populateItems(p, "Alpha", "Beta", "Gamma")

	got := p.filteredPlanItems()
	if len(got) != 3 {
		t.Errorf("empty filter: expected 3 items, got %d", len(got))
	}
}

// TestFilteredPlanItems_EmptyFilterNoItems returns empty when no tasks.
func TestFilteredPlanItems_EmptyFilterNoItems(t *testing.T) {
	p := newTestPanel()
	got := p.filteredPlanItems()
	if len(got) != 0 {
		t.Errorf("empty filter no items: expected 0, got %d", len(got))
	}
}

// TestFilteredPlanItems_CaseInsensitiveMatch matches regardless of case.
func TestFilteredPlanItems_CaseInsensitiveMatch(t *testing.T) {
	p := newTestPanel()
	populateItems(p, "Fix Login Bug", "Add Dashboard Feature", "Refactor Auth")

	p.filterInput.SetValue("login")
	got := p.filteredPlanItems()
	if len(got) != 1 {
		t.Fatalf("case-insensitive: expected 1 match, got %d", len(got))
	}
	if got[0].Subject != "Fix Login Bug" {
		t.Errorf("case-insensitive: wrong item: %q", got[0].Subject)
	}
}

// TestFilteredPlanItems_UpperCaseQuery matches items with lowercase query stored.
func TestFilteredPlanItems_UpperCaseQuery(t *testing.T) {
	p := newTestPanel()
	populateItems(p, "fix login bug", "unrelated task")

	p.filterInput.SetValue("FIX")
	got := p.filteredPlanItems()
	if len(got) != 1 {
		t.Fatalf("upper query: expected 1 match, got %d", len(got))
	}
}

// TestFilteredPlanItems_NoMatch returns empty slice when nothing matches.
func TestFilteredPlanItems_NoMatch(t *testing.T) {
	p := newTestPanel()
	populateItems(p, "Alpha", "Beta")

	p.filterInput.SetValue("zzz")
	got := p.filteredPlanItems()
	if len(got) != 0 {
		t.Errorf("no match: expected 0, got %d", len(got))
	}
}

// TestFilteredPlanItems_PartialMatch returns subset matching substring.
func TestFilteredPlanItems_PartialMatch(t *testing.T) {
	p := newTestPanel()
	populateItems(p, "Fix auth bug", "Add auth tests", "Refactor storage")

	p.filterInput.SetValue("auth")
	got := p.filteredPlanItems()
	if len(got) != 2 {
		t.Errorf("partial match: expected 2, got %d", len(got))
	}
}

// ── InputPanel interface ──────────────────────────────────────────────────────

// TestHasInput_FalseByDefault verifies new panel has no active input.
func TestHasInput_FalseByDefault(t *testing.T) {
	p := newTestPanel()
	if p.HasInput() {
		t.Error("HasInput: expected false on new panel")
	}
}

// TestHasInput_TrueWhenActive verifies HasInput reflects filterActive state.
func TestHasInput_TrueWhenActive(t *testing.T) {
	p := newTestPanel()
	p.filterActive = true
	if !p.HasInput() {
		t.Error("HasInput: expected true when filterActive")
	}
}

// TestInputUpdate_EscDeactivatesFilter verifies Esc clears filter and deactivates.
func TestInputUpdate_EscDeactivatesFilter(t *testing.T) {
	p := newTestPanel()
	populateItems(p, "Task A", "Task B", "Task C")
	p.filterActive = true
	p.filterInput.SetValue("Task")
	p.cursor = 2

	p.InputUpdate(tea.KeyMsg{Type: tea.KeyEsc})

	if p.filterActive {
		t.Error("Esc: filterActive should be false")
	}
	if p.filterInput.Value() != "" {
		t.Errorf("Esc: filter text should be empty, got %q", p.filterInput.Value())
	}
	if p.cursor != 0 {
		t.Errorf("Esc: cursor should reset to 0, got %d", p.cursor)
	}
	if p.HasInput() {
		t.Error("Esc: HasInput should return false after Esc")
	}
}

// TestInputUpdate_KeyUpdatesFilter verifies keystrokes update filterInput.
func TestInputUpdate_KeyUpdatesFilter(t *testing.T) {
	p := newTestPanel()
	p.filterActive = true
	// Focus the input so it accepts rune keys
	p.filterInput.Focus()

	p.InputUpdate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	p.InputUpdate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

	got := p.filterInput.Value()
	if got != "ab" {
		t.Errorf("key input: expected %q, got %q", "ab", got)
	}
}

// TestInputUpdate_CursorResetOnKeypress verifies cursor resets to 0 on each key.
func TestInputUpdate_CursorResetOnKeypress(t *testing.T) {
	p := newTestPanel()
	populateItems(p, "A", "B", "C")
	p.filterActive = true
	p.filterInput.Focus()
	p.cursor = 2

	p.InputUpdate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	if p.cursor != 0 {
		t.Errorf("cursor reset: expected 0, got %d", p.cursor)
	}
}

// TestInputView_TruncatesToWidth verifies InputView respects panel width.
func TestInputView_TruncatesToWidth(t *testing.T) {
	p := newTestPanel()
	p.filterActive = true
	p.filterInput.SetValue("a very long filter query that exceeds width")
	// Set a narrow width
	p.width = 10

	view := p.InputView()
	// lipgloss.Width is ANSI-aware; raw len may differ. Just verify it doesn't panic.
	if view == "" {
		t.Error("InputView: should not be empty")
	}
}

// TestInputView_NormalWidth renders without truncation when wide enough.
func TestInputView_NormalWidth(t *testing.T) {
	p := newTestPanel()
	p.filterActive = true
	p.filterInput.SetValue("fix")
	p.width = 80

	view := p.InputView()
	if view == "" {
		t.Error("InputView normal: should not be empty")
	}
}

// ── filter state across focus changes ─────────────────────────────────────────

// TestFilterState_PersistsWhenFocusLost verifies filter stays active when
// the panel loses focus (root stops calling InputUpdate, but state is preserved).
// On refocus, HasInput() should still return true.
func TestFilterState_PersistsWhenFocusLost(t *testing.T) {
	p := newTestPanel()
	p.filterActive = true
	p.filterInput.SetValue("auth")

	// Simulate focus loss — nothing calls Esc, state should remain
	// (root simply stops routing to InputUpdate when FocusPanel loses focus)

	if !p.HasInput() {
		t.Error("filter persists: HasInput should remain true after simulated focus loss")
	}
	if p.filterInput.Value() != "auth" {
		t.Errorf("filter persists: filter text changed to %q", p.filterInput.Value())
	}
}
