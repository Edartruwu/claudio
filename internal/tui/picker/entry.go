// Package picker provides foundational types for the Telescope-style picker
// system: Entry, Finder, Sorter, and Previewer.
package picker

// Entry is a single item in the picker list.
//
//   - Display may include lipgloss ANSI styling; shown in the list.
//   - Ordinal is plain text used for fuzzy/scoring (no ANSI escapes).
//   - Meta carries arbitrary key/value metadata (agentID, path, …).
type Entry struct {
	Value   any
	Display string
	Ordinal string
	Meta    map[string]any
}
