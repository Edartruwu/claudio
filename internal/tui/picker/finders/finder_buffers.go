package finders

import (
	"context"
	"strings"

	"github.com/Abraxas-365/claudio/internal/tui/picker"
	"github.com/Abraxas-365/claudio/internal/tui/windows"
)

// bufferFinder emits entries for every window registered in a Manager.
// For agent:// windows backed by a LiveBuffer, Meta["live"]=true and
// Meta["status"] holds the LiveBuffer's current status string.
type bufferFinder struct {
	mgr *windows.Manager
}

// NewBufferFinder returns a Finder over all windows registered in mgr.
func NewBufferFinder(mgr *windows.Manager) picker.Finder {
	return &bufferFinder{mgr: mgr}
}

func (f *bufferFinder) Find(ctx context.Context) <-chan picker.Entry {
	wins := f.mgr.AllWindows()
	ch := make(chan picker.Entry, len(wins))
	go func() {
		defer close(ch)
		for _, w := range wins {
			meta := map[string]any{
				"name": w.Name,
			}
			display := w.Name
			isLive := strings.HasPrefix(w.Name, "agent://")
			if isLive {
				meta["live"] = true
				if lb, ok := f.mgr.GetLiveBuffer(w.Name); ok {
					status := lb.Status()
					meta["status"] = status
					indicator := "⟳"
					switch status {
					case "done":
						indicator = "✓"
					case "error":
						indicator = "✗"
					}
					display = w.Name + " [" + indicator + "]"
				}
			}
			e := picker.Entry{
				Value:   w,
				Display: display,
				Ordinal: w.Name,
				Meta:    meta,
			}
			select {
			case <-ctx.Done():
				return
			case ch <- e:
			}
		}
	}()
	return ch
}

func (f *bufferFinder) Close() {}
