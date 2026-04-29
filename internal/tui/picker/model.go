// Package picker provides a reusable Telescope-style fuzzy picker for Claudio TUI.
// It renders a prompt + results list + optional preview pane using one of four
// layout strategies (horizontal, vertical, dropdown, ivy).
package picker

import (
	"context"
	"math"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ── Messages ──────────────────────────────────────────────────────────────────

// EntryMsg carries a single entry that arrived from the Finder's channel.
type EntryMsg Entry

// finderDoneMsg is sent internally when the Finder's channel is closed.
type finderDoneMsg struct{}

// tickMsg is sent on each preview-refresh tick.
type tickMsg struct{}

// PickerClosedMsg is emitted when the user cancels the picker (Esc / q).
type PickerClosedMsg struct{}

// PickerDoneMsg is emitted when the user confirms a selection.
type PickerDoneMsg struct {
	// Entry is the highlighted item at time of confirmation.
	Entry Entry
	// Entries holds all Tab-selected items (non-nil only when multi-select was used).
	Entries []Entry
}

// ── Model ─────────────────────────────────────────────────────────────────────

// Model is the BubbleTea component for the picker.
// Embed it in a parent model and forward tea.Msg to its Update method.
type Model struct {
	cfg Config

	// prompt state
	prompt string

	// entry state
	allEntries []Entry
	filtered   []Entry
	// selectedIdx is the index into filtered for the currently highlighted row.
	selectedIdx int
	// multiSelect keys by entry Ordinal; survives filter changes.
	multiSelect map[string]bool

	// preview scroll position (lines from top)
	previewScroll int

	// async
	loading bool
	entryCh <-chan Entry
	cancel  context.CancelFunc

	// terminal dimensions (set via SetSize or tea.WindowSizeMsg)
	width  int
	height int
}

// New creates a picker Model from cfg. The Finder is started immediately;
// Init() returns the first wait-for-entry command.
//
// Defaults applied:
//   - Sorter → FuzzySorter  (substring match)
//   - Layout → LayoutHorizontal
func New(cfg Config) Model {
	if cfg.Sorter == nil {
		cfg.Sorter = NewFuzzySorter()
	}
	if cfg.Layout == "" {
		cfg.Layout = LayoutHorizontal
	}

	var (
		ch     <-chan Entry
		cancel context.CancelFunc
	)
	if cfg.Finder != nil {
		var ctx context.Context
		ctx, cancel = context.WithCancel(context.Background())
		ch = cfg.Finder.Find(ctx)
	} else {
		// nil finder: closed channel so finderDoneMsg fires immediately
		empty := make(chan Entry)
		close(empty)
		ch = empty
		cancel = func() {}
	}

	return Model{
		cfg:         cfg,
		multiSelect: make(map[string]bool),
		loading:     true,
		entryCh:     ch,
		cancel:      cancel,
	}
}

// SetSize informs the model of the available terminal area.
// Call this when the parent receives a tea.WindowSizeMsg.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Cancel cancels the underlying finder context and calls Finder.Close().
// Safe to call multiple times. Called by the host model when the picker is
// closed programmatically (e.g. replaced by a new picker) rather than via Esc.
func (m Model) Cancel() {
	cancelPicker(m)
}

// ── BubbleTea interface ───────────────────────────────────────────────────────

// tickCmd schedules a preview-refresh tick 250 ms from now.
func tickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// Init starts waiting for the first entry from the Finder.
// When a Previewer is configured, also arms the first preview-refresh tick.
func (m Model) Init() tea.Cmd {
	if m.cfg.Previewer != nil {
		return tea.Batch(waitForEntry(m.entryCh), tickCmd())
	}
	return waitForEntry(m.entryCh)
}

// Update handles all incoming messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case EntryMsg:
		m.allEntries = append(m.allEntries, Entry(msg))
		m = refilter(m)
		return m, waitForEntry(m.entryCh)

	case finderDoneMsg:
		m.loading = false
		return m, nil

	case tickMsg:
		// Re-arm only when previewer active — zero overhead otherwise.
		if m.cfg.Previewer != nil {
			return m, tickCmd()
		}
		return m, nil

	case tea.KeyMsg:
		return handleKey(m, msg)
	}

	return m, nil
}

// View renders the picker using the configured layout strategy.
func (m Model) View() string {
	switch m.cfg.Layout {
	case LayoutVertical:
		return renderVertical(m, m.width, m.height)
	case LayoutDropdown:
		return renderDropdown(m, m.width, m.height)
	case LayoutIvy:
		return renderIvy(m, m.width, m.height)
	default:
		return renderHorizontal(m, m.width, m.height)
	}
}

// ── Key handling ──────────────────────────────────────────────────────────────

func handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {

	case tea.KeyUp, tea.KeyCtrlP:
		if m.selectedIdx > 0 {
			m.selectedIdx--
			m.previewScroll = 0
		}

	case tea.KeyDown, tea.KeyCtrlN:
		if m.selectedIdx < len(m.filtered)-1 {
			m.selectedIdx++
			m.previewScroll = 0
		}

	case tea.KeyTab:
		// Toggle multi-select on current item, then advance cursor.
		if len(m.filtered) > 0 {
			key := m.filtered[m.selectedIdx].Ordinal
			if m.multiSelect[key] {
				delete(m.multiSelect, key)
			} else {
				m.multiSelect[key] = true
			}
			if m.selectedIdx < len(m.filtered)-1 {
				m.selectedIdx++
			}
		}

	case tea.KeyEnter:
		if len(m.filtered) == 0 {
			return m, nil
		}
		entry := m.filtered[m.selectedIdx]
		entries := collectMultiSelected(m)
		cancelPicker(m)
		if len(entries) > 0 {
			if m.cfg.OnMultiSelect != nil {
				m.cfg.OnMultiSelect(entries)
			}
		} else {
			if m.cfg.OnSelect != nil {
				m.cfg.OnSelect(entry)
			}
		}
		done := PickerDoneMsg{Entry: entry, Entries: entries}
		return m, func() tea.Msg { return done }

	case tea.KeyEsc:
		cancelPicker(m)
		return m, func() tea.Msg { return PickerClosedMsg{} }

	case tea.KeyCtrlU:
		if m.previewScroll >= 5 {
			m.previewScroll -= 5
		} else {
			m.previewScroll = 0
		}

	case tea.KeyCtrlD:
		m.previewScroll += 5

	case tea.KeyBackspace, tea.KeyCtrlH:
		if len(m.prompt) > 0 {
			// trim one rune from end (safe for ASCII; fine for picker prompts)
			runes := []rune(m.prompt)
			m.prompt = string(runes[:len(runes)-1])
			m = refilter(m)
		}

	default:
		if len(msg.Runes) > 0 {
			m.prompt += string(msg.Runes)
			m = refilter(m)
		}
	}

	return m, nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// waitForEntry returns a Cmd that blocks until the next entry from ch.
func waitForEntry(ch <-chan Entry) tea.Cmd {
	return func() tea.Msg {
		entry, ok := <-ch
		if !ok {
			return finderDoneMsg{}
		}
		return EntryMsg(entry)
	}
}

// refilter re-scores and re-sorts m.filtered from m.allEntries using current prompt.
// Returns a new Model (value copy) with filtered/selectedIdx updated.
func refilter(m Model) Model {
	if len(m.allEntries) == 0 {
		m.filtered = nil
		m.selectedIdx = 0
		return m
	}

	type scored struct {
		entry Entry
		score float64
	}

	results := make([]scored, 0, len(m.allEntries))
	for _, e := range m.allEntries {
		s := m.cfg.Sorter.Score(m.prompt, e)
		if s < math.MaxFloat64 {
			results = append(results, scored{e, s})
		}
	}

	// Stable sort preserves original Finder order for equal scores.
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].score < results[j].score
	})

	m.filtered = make([]Entry, len(results))
	for i, r := range results {
		m.filtered[i] = r.entry
	}

	// Clamp cursor.
	switch {
	case len(m.filtered) == 0:
		m.selectedIdx = 0
	case m.selectedIdx >= len(m.filtered):
		m.selectedIdx = len(m.filtered) - 1
	}

	return m
}

// collectMultiSelected returns the subset of m.filtered whose Ordinal keys are
// in m.multiSelect. Order follows filtered order.
func collectMultiSelected(m Model) []Entry {
	if len(m.multiSelect) == 0 {
		return nil
	}
	result := make([]Entry, 0, len(m.multiSelect))
	for _, e := range m.filtered {
		if m.multiSelect[e.Ordinal] {
			result = append(result, e)
		}
	}
	return result
}

// cancelPicker cancels the finder context and signals the Finder to stop.
func cancelPicker(m Model) {
	if m.cancel != nil {
		m.cancel()
	}
	if m.cfg.Finder != nil {
		m.cfg.Finder.Close()
	}
}
