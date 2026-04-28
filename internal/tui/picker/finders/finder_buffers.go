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

// panelWindowNames lists windows that back TUI panels, not real content
// buffers. These are registered in the Manager but should not appear in the
// buffer picker because the user cannot meaningfully "open" them.
var panelWindowNames = map[string]bool{
	"AgentGUI":    true,
	"SessionTree": true,
	"Tasks":       true,
	"Memory":      true,
	"Skills":      true,
	"Analytics":   true,
	"Tools":       true,
	"Conversation": true,
}

// isPanel returns true when name corresponds to a TUI panel window (not a
// real content buffer that belongs in the buffer picker).
func isPanel(name string) bool {
	return panelWindowNames[name]
}

func (f *bufferFinder) Find(ctx context.Context) <-chan picker.Entry {
	wins := f.mgr.AllWindows()
	ch := make(chan picker.Entry, len(wins))
	go func() {
		defer close(ch)
		for _, w := range wins {
			// Skip panel windows — only real content buffers belong here.
			if isPanel(w.Name) {
				continue
			}
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
