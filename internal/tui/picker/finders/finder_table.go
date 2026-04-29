// Package finders provides built-in Finder implementations for the picker.
package finders

import (
	"context"

	"github.com/Abraxas-365/claudio/internal/tui/picker"
)

// tableFinder emits a static slice of entries.
type tableFinder struct {
	entries []picker.Entry
}

// NewTableFinder returns a Finder that streams entries from a static slice.
// Find sends all entries then closes the channel. Close is a no-op.
func NewTableFinder(entries []picker.Entry) picker.Finder {
	return &tableFinder{entries: entries}
}

func (f *tableFinder) Find(ctx context.Context) <-chan picker.Entry {
	ch := make(chan picker.Entry, len(f.entries))
	go func() {
		defer close(ch)
		for _, e := range f.entries {
			select {
			case <-ctx.Done():
				return
			case ch <- e:
			}
		}
	}()
	return ch
}

func (f *tableFinder) Close() {}
