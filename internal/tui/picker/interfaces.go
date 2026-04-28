package picker

import "context"

// Finder streams entries into the picker. Callers read from the returned
// channel until it is closed, then call Close to release resources.
type Finder interface {
	// Find starts the search and returns a channel of results. The channel is
	// closed when all results have been sent or ctx is cancelled.
	Find(ctx context.Context) <-chan Entry

	// Close releases any resources held by the finder.
	Close()
}

// Sorter scores a single entry against the current prompt. Lower score =
// better match. math.MaxFloat64 means "no match".
type Sorter interface {
	Score(prompt string, entry Entry) float64
}

// Previewer renders a preview of an entry into a fixed viewport.
type Previewer interface {
	// Render returns a string (may include lipgloss styling) fitting
	// within width×height cells.
	Render(entry Entry, width, height int) string
}
