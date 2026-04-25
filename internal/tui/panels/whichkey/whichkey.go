// Package whichkey implements a which-key style popup that shows available
// leader key sequences after a brief timeout.
package whichkey

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Abraxas-365/claudio/internal/tui/keymap"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// Timeout is the delay before showing the popup after the leader key is pressed.
const Timeout = 300 * time.Millisecond

// TimeoutMsg is sent when the leader key timeout elapses.
// Gen carries the generation counter so stale messages can be ignored.
type TimeoutMsg struct{ Gen int }

// Binding represents a single leader key binding.
type Binding struct {
	Key  string
	Desc string
}

// defaultLeft, defaultRight, defaultMulti define the two-column grid layout.
// Multi-key bindings (len > 1) appear full-width below the grid.
var (
	defaultLeft = []Binding{
		{Key: "p", Desc: "palette"},
		{Key: "f", Desc: "files"},
		{Key: "a", Desc: "agents"},
		{Key: "/", Desc: "search"},
		{Key: ";", Desc: "recent"},
		{Key: ".", Desc: "sessions"},
		{Key: "e", Desc: "edit prompt"},
	}
	defaultRight = []Binding{
		{Key: "w", Desc: "windows"},
		{Key: "b", Desc: "buffers"},
		{Key: "C", Desc: "config"},
		{Key: "K", Desc: "skills"},
		{Key: "M", Desc: "memory"},
		{Key: "T", Desc: "tasks"},
		{Key: "O", Desc: "tools"},
		{Key: "A", Desc: "analytics"},
	}
	defaultMulti = []Binding{
		{Key: "oa", Desc: "agent inspector"},
		{Key: "ev", Desc: "view section"},
	}
)

func DefaultBindings() []Binding {
	var all []Binding
	all = append(all, defaultLeft...)
	all = append(all, defaultRight...)
	all = append(all, defaultMulti...)
	return all
}

// WindowBindings returns bindings for the Space+W sub-menu.
func WindowBindings() []Binding {
	return []Binding{
		{Key: "w", Desc: "cycle"},
		{Key: "h", Desc: "←"},
		{Key: "j", Desc: "↓"},
		{Key: "k", Desc: "↑"},
		{Key: "l", Desc: "→"},
		{Key: "v", Desc: "mirror"},
		{Key: "q", Desc: "close"},
	}
}

// SessionBindings returns bindings for the Space+B sub-menu.
func SessionBindings() []Binding {
	return []Binding{
		{Key: "n", Desc: "next"},
		{Key: "p", Desc: "prev"},
		{Key: "c", Desc: "new"},
		{Key: "k", Desc: "kill"},
		{Key: "r", Desc: "rename"},
		{Key: "l", Desc: "list"},
	}
}

// PanelBindings returns bindings for the Space+I sub-menu.
func PanelBindings() []Binding {
	return []Binding{
		{Key: "c", Desc: "config"},
		{Key: "k", Desc: "skills"},
		{Key: "m", Desc: "memory/rules"},
		{Key: "a", Desc: "analytics"},
		{Key: "t", Desc: "tasks"},
		{Key: "o", Desc: "tools"},
	}
}

// OpenBindings returns bindings for the Space+O sub-menu (open panels).
func OpenBindings() []Binding {
	return []Binding{
		{Key: "p", Desc: "session tree"},
		{Key: "a", Desc: "agent inspector"},
	}
}

// Model is the which-key popup overlay.
type Model struct {
	active   bool
	bindings []Binding
	width    int
	km       *keymap.Keymap // optional keymap reference for dynamic bindings
}

// New creates a new which-key popup.
func New() Model {
	return Model{
		bindings: DefaultBindings(),
	}
}

// Show activates the popup with the given bindings.
func (m *Model) Show(bindings []Binding) {
	m.active = true
	m.bindings = bindings
}

// ShowDefault shows the default leader bindings.
func (m *Model) ShowDefault() {
	m.Show(DefaultBindings())
}

// ShowWindow shows the window sub-menu bindings.
func (m *Model) ShowWindow() {
	m.Show(WindowBindings())
}

// ShowSessions shows the session sub-menu bindings.
func (m *Model) ShowSessions() {
	m.Show(SessionBindings())
}

// ShowOpen shows the open-panel sub-menu bindings (Space+O).
func (m *Model) ShowOpen() {
	m.Show(OpenBindings())
}

// SetKeymap sets the keymap reference for dynamic binding generation.
func (m *Model) SetKeymap(km *keymap.Keymap) {
	m.km = km
}

// ShowFromKeymap shows bindings for the given prefix, reading from the keymap.
// Falls back to the legacy hardcoded bindings if no keymap is set.
func (m *Model) ShowFromKeymap(prefix string) {
	if m.km == nil {
		// Fallback to legacy behaviour
		switch prefix {
		case "":
			m.Show(DefaultBindings())
		case "w":
			m.Show(WindowBindings())
		case "b":
			m.Show(SessionBindings())
		case "i":
			m.Show(PanelBindings())
		case "o":
			m.Show(OpenBindings())
		}
		return
	}
	keymapBindings := m.km.BindingsForPrefix(prefix)
	bindings := make([]Binding, len(keymapBindings))
	for i, b := range keymapBindings {
		bindings[i] = Binding{Key: b.KeySeq, Desc: b.Action.Description}
	}
	if len(bindings) > 0 {
		m.Show(bindings)
	}
}

// Hide dismisses the popup.
func (m *Model) Hide() {
	m.active = false
}

// IsActive returns whether the popup is visible.
func (m Model) IsActive() bool {
	return m.active
}

// SetWidth sets the available width for rendering.
func (m *Model) SetWidth(w int) {
	m.width = w
}

// ScheduleTimeout returns a tea.Cmd that sends TimeoutMsg after the delay.
// gen must match the model's leaderSeqGen at receipt time or the message is discarded.
func ScheduleTimeout(gen int) tea.Cmd {
	return tea.Tick(Timeout, func(time.Time) tea.Msg {
		return TimeoutMsg{Gen: gen}
	})
}

// View renders the popup.
// wkColW is the fixed visible width of each two-column cell (key + desc).
const wkColW = 22

// wkFormatBinding formats a single binding into a fixed-width cell.
func wkFormatBinding(b Binding) string {
	key := fmt.Sprintf("%-3s", b.Key)
	desc := b.Desc
	styled := styles.WhichKeyKey.Render(key) + "  " + styles.WhichKeyDesc.Render(desc)
	visLen := 3 + 2 + len(desc)
	if pad := wkColW - visLen; pad > 0 {
		styled += strings.Repeat(" ", pad)
	}
	return styled
}

func (m Model) View() string {
	if !m.active || len(m.bindings) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, styles.WhichKeyTitle.Render(" <Space> "))
	lines = append(lines, "")

	isDefault := len(m.bindings) == len(defaultLeft)+len(defaultRight)+len(defaultMulti)
	if isDefault {
		left := defaultLeft
		right := defaultRight
		rows := len(left)
		if len(right) > rows {
			rows = len(right)
		}
		for i := 0; i < rows; i++ {
			var l, r string
			if i < len(left) {
				l = wkFormatBinding(left[i])
			} else {
				l = strings.Repeat(" ", wkColW)
			}
			if i < len(right) {
				r = wkFormatBinding(right[i])
			}
			lines = append(lines, "  "+l+"  "+r)
		}
		if len(defaultMulti) > 0 {
			lines = append(lines, "")
			for _, b := range defaultMulti {
				line := "  " + styles.WhichKeyKey.Render(b.Key) + "  " + styles.WhichKeyDesc.Render(b.Desc)
				lines = append(lines, line)
			}
		}
	} else {
		for _, b := range m.bindings {
			if b.Key == "" && b.Desc == "" {
				continue
			}
			line := "  " + styles.WhichKeyKey.Render(b.Key) + " " + styles.WhichKeySep.Render("→") + " " + styles.WhichKeyDesc.Render(b.Desc)
			lines = append(lines, line)
		}
	}

	lines = append(lines, "")
	lines = append(lines, styles.WhichKeySep.Render("  esc to close"))

	content := strings.Join(lines, "\n")
	box := styles.WhichKeyBorder.Padding(0, 1).Render(content)
	return box
}
