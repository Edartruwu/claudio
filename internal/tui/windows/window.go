package windows

// Layout describes how a window is positioned in the TUI.
type Layout int

const (
	LayoutFloat   Layout = iota // floating overlay, centered
	LayoutSidebar               // docked to right sidebar
	LayoutSplitH                // horizontal split (bottom pane)
	LayoutSplitV                // vertical split (right pane)
)

// Window is a viewport into a Buffer with layout type, size hints, and
// z-ordering for floating windows.
type Window struct {
	Name    string
	Title   string
	Buffer  *Buffer
	Layout  Layout
	Width   int // hint; 0 = auto
	Height  int // hint; 0 = auto
	ZIndex  int
	Focused       bool
	AgentName     string // routing name for >>agentName messages
	AllowEscClose bool   // if true, ESC closes this buffer (default: false = nvim-style)
	open          bool
}

// IsOpen reports whether the window is currently visible.
func (w *Window) IsOpen() bool { return w.open }

// View renders the window's buffer content at the given viewport size.
// Returns empty string if buffer or render func is nil.
func (w *Window) View(width, height int) string {
	if w.Buffer == nil || w.Buffer.Render == nil {
		return ""
	}
	return w.Buffer.Render(width, height)
}
