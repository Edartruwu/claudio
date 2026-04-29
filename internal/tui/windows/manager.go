package windows

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Manager is the window registry and z-ordered stack.
// Thread-safe for register/open/close; Update() must be called from the
// BubbleTea event loop (single-threaded).
type Manager struct {
	windows  map[string]*Window
	liveBufs map[string]*LiveBuffer // name → LiveBuffer for agent:// windows
	stack    []*Window              // open windows in z-order (lowest first)
	mu       sync.RWMutex
}

// New returns an initialized Manager.
func New() *Manager {
	return &Manager{
		windows:  make(map[string]*Window),
		liveBufs: make(map[string]*LiveBuffer),
	}
}

// Register adds a window to the registry. Panics on duplicate name.
func (m *Manager) Register(w *Window) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.windows[w.Name]; exists {
		panic(fmt.Sprintf("windows: duplicate window name %q", w.Name))
	}
	m.windows[w.Name] = w
}

// RegisterLiveBuffer creates a Window backed by lb and registers both.
// Panics on duplicate name (same rule as Register).
func (m *Manager) RegisterLiveBuffer(lb *LiveBuffer, agentName, title string) {
	w := &Window{
		Name:      lb.name,
		Title:     title,
		AgentName: agentName,
		Buffer:    lb.Buffer(),
		Layout:    LayoutSidebar,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.windows[w.Name]; exists {
		panic(fmt.Sprintf("windows: duplicate window name %q", w.Name))
	}
	m.windows[w.Name] = w
	m.liveBufs[w.Name] = lb
}

// GetLiveBuffer returns the LiveBuffer registered under name, or (nil, false).
func (m *Manager) GetLiveBuffer(name string) (*LiveBuffer, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lb, ok := m.liveBufs[name]
	return lb, ok
}

// AllWindows returns all registered windows (open or closed).
func (m *Manager) AllWindows() []*Window {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Window, 0, len(m.windows))
	for _, w := range m.windows {
		out = append(out, w)
	}
	return out
}

// Get returns the named window or nil.
func (m *Manager) Get(name string) *Window {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.windows[name]
}

// Open makes the named window visible and pushes it to top of z-stack.
// Returns error if name not registered.
func (m *Manager) Open(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.windows[name]
	if !ok {
		return fmt.Errorf("windows: unknown window %q", name)
	}
	if w.open {
		// Already open — move to top.
		m.removeFromStack(w)
	}
	w.open = true
	m.stack = append(m.stack, w)
	m.updateFocus()
	return nil
}

// Close hides the named window. No-op if not open or not registered.
func (m *Manager) Close(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.windows[name]
	if !ok || !w.open {
		return
	}
	w.open = false
	w.Focused = false
	m.removeFromStack(w)
	m.updateFocus()
}

// Toggle opens a closed window or closes an open one.
func (m *Manager) Toggle(name string) {
	m.mu.RLock()
	w, ok := m.windows[name]
	m.mu.RUnlock()
	if !ok {
		return
	}
	if w.open {
		m.Close(name)
	} else {
		_ = m.Open(name)
	}
}

// FocusedFloat returns the topmost open Float window, or nil.
func (m *Manager) FocusedFloat() *Window {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := len(m.stack) - 1; i >= 0; i-- {
		w := m.stack[i]
		if w.Layout == LayoutFloat && w.open {
			return w
		}
	}
	return nil
}

// OpenFloats returns all open Float windows sorted by z-order (lowest first).
func (m *Manager) OpenFloats() []*Window {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*Window
	for _, w := range m.stack {
		if w.Layout == LayoutFloat && w.open {
			out = append(out, w)
		}
	}
	return out
}

// OpenWindows returns all open windows of any layout, z-ordered.
func (m *Manager) OpenWindows() []*Window {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Window, len(m.stack))
	copy(out, m.stack)
	return out
}

// Update routes tea.KeyMsg to the focused float window (if any).
// Returns the updated manager and any command.
func (m *Manager) Update(msg tea.Msg) (*Manager, tea.Cmd) {
	// Currently a passthrough — individual window key handling will be
	// wired when windows gain their own Update methods.
	return m, nil
}

// --- overlay rendering ---

var (
	floatBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	floatTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("62"))
)

// RenderOverlay composites open float windows on top of base content.
// Each float is centered in the terminal viewport.
func (m *Manager) RenderOverlay(base string, totalWidth, totalHeight int) string {
	floats := m.OpenFloats()
	if len(floats) == 0 {
		return base
	}

	result := base
	for _, w := range floats {
		// Resolve window dimensions.
		ww := w.Width
		if ww <= 0 {
			ww = totalWidth * 3 / 4
		}
		wh := w.Height
		if wh <= 0 {
			wh = totalHeight * 3 / 4
		}
		// Clamp to terminal size (leave room for border).
		if ww > totalWidth-4 {
			ww = totalWidth - 4
		}
		if wh > totalHeight-4 {
			wh = totalHeight - 4
		}
		if ww < 1 {
			ww = 1
		}
		if wh < 1 {
			wh = 1
		}

		// Render buffer content inside border.
		content := w.View(ww-2, wh-2) // account for border+padding
		title := ""
		if w.Title != "" {
			title = floatTitle.Render(" " + w.Title + " ")
		}
		box := floatBorder.
			Width(ww).
			Height(wh).
			Render(content)

		// Prepend title into top border if present.
		if title != "" {
			lines := strings.Split(box, "\n")
			if len(lines) > 0 {
				border := lines[0]
				if len(border) > 4 {
					// Insert title after first border char.
					titleRendered := floatTitle.Render(" " + w.Title + " ")
					runes := []rune(border)
					insertion := []rune(titleRendered)
					if len(insertion)+2 < len(runes) {
						merged := make([]rune, 0, len(runes)+len(insertion))
						merged = append(merged, runes[0])
						merged = append(merged, insertion...)
						merged = append(merged, runes[1+len(insertion):]...)
						lines[0] = string(merged)
					}
				}
				box = strings.Join(lines, "\n")
			}
		}

		// Center the box over the base using lipgloss.Place.
		result = lipgloss.Place(
			totalWidth, totalHeight,
			lipgloss.Center, lipgloss.Center,
			box,
			lipgloss.WithWhitespaceChars(" "),
		)
	}

	return result
}

// --- internal helpers ---

func (m *Manager) removeFromStack(w *Window) {
	for i, s := range m.stack {
		if s == w {
			m.stack = append(m.stack[:i], m.stack[i+1:]...)
			return
		}
	}
}

// updateFocus sets Focused on topmost float, clears others.
func (m *Manager) updateFocus() {
	for _, w := range m.stack {
		w.Focused = false
	}
	for i := len(m.stack) - 1; i >= 0; i-- {
		if m.stack[i].Layout == LayoutFloat {
			m.stack[i].Focused = true
			return
		}
	}
}

// SortByZIndex re-sorts the open stack by ZIndex (stable).
func (m *Manager) SortByZIndex() {
	m.mu.Lock()
	defer m.mu.Unlock()
	sort.SliceStable(m.stack, func(i, j int) bool {
		return m.stack[i].ZIndex < m.stack[j].ZIndex
	})
	m.updateFocus()
}
