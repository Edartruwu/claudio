package styles

import (
	"hash/fnv"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// agentPalette is the set of colors cycled through for agent name coloring.
// Ordered from most to least visually distinct.
var agentPalette = []lipgloss.Color{
	Primary, Secondary, Aqua, Orange, Warning, Success,
}

// AgentColor returns a stable color for the given agent name using FNV hash.
// The same name always maps to the same color. Different names get different colors.
func AgentColor(name string) lipgloss.Color {
	h := fnv.New32a()
	h.Write([]byte(name))
	idx := h.Sum32() % uint32(len(agentPalette))
	return agentPalette[idx]
}

var (
	agentStyleCache   = make(map[string]lipgloss.Style)
	agentStyleCacheMu sync.RWMutex
)

// AgentStyle returns a cached lipgloss style for rendering an agent name badge.
func AgentStyle(name string) lipgloss.Style {
	agentStyleCacheMu.RLock()
	s, ok := agentStyleCache[name]
	agentStyleCacheMu.RUnlock()
	if ok {
		return s
	}

	s = lipgloss.NewStyle().
		Foreground(AgentColor(name)).
		Bold(true)
	agentStyleCacheMu.Lock()
	agentStyleCache[name] = s
	agentStyleCacheMu.Unlock()
	return s
}
