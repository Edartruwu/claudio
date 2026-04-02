// Package teampanel provides a TUI component for managing agent teams.
// Shows active teammates with status, progress, and controls.
package teampanel

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/teams"
)

// Model is the Bubble Tea model for the team panel.
type Model struct {
	manager   *teams.Manager
	runner    *teams.TeammateRunner
	teamName  string
	width     int
	height    int
	visible   bool
	selected  int // selected member index for actions
}

// New creates a new team panel.
func New(manager *teams.Manager, runner *teams.TeammateRunner) Model {
	return Model{
		manager: manager,
		runner:  runner,
	}
}

// SetTeam sets the active team to display.
func (m *Model) SetTeam(name string) {
	m.teamName = name
}

// SetSize updates the panel dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Toggle shows/hides the panel.
func (m *Model) Toggle() {
	m.visible = !m.visible
}

// IsVisible returns whether the panel is shown.
func (m Model) IsVisible() bool {
	return m.visible
}

// View renders the team panel.
func (m Model) View() string {
	if !m.visible || m.manager == nil || m.teamName == "" {
		return ""
	}

	team, ok := m.manager.GetTeam(m.teamName)
	if !ok {
		return ""
	}

	// Panel styles
	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#83a598")).
		Padding(0, 1).
		Width(m.width - 2)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#83a598"))

	// Header
	var sb strings.Builder
	sb.WriteString(headerStyle.Render(fmt.Sprintf("Team: %s", team.Name)))
	if team.Description != "" {
		sb.WriteString(fmt.Sprintf(" — %s", team.Description))
	}
	sb.WriteString("\n")

	// Members
	for i, mem := range team.Members {
		if mem.Identity.IsLead {
			continue // don't show lead in panel
		}

		icon := statusIcon(mem.Status)
		color := lipgloss.Color(mem.Identity.Color)
		nameStyle := lipgloss.NewStyle().Foreground(color).Bold(true)
		statusStyle := lipgloss.NewStyle().Foreground(statusColor(mem.Status))

		// Selection indicator
		selector := "  "
		if i-1 == m.selected { // -1 because lead is skipped
			selector = "> "
		}

		line := fmt.Sprintf("%s%s %s %s",
			selector,
			icon,
			nameStyle.Render(mem.Identity.AgentName),
			statusStyle.Render(string(mem.Status)))

		// Add progress info from runner
		if m.runner != nil {
			if state, ok := m.runner.GetState(mem.Identity.AgentID); ok {
				duration := time.Since(state.StartedAt).Round(time.Second)
				line += fmt.Sprintf(" (%s)", duration)
				if state.Progress.ToolCalls > 0 {
					line += fmt.Sprintf(" [%d tools]", state.Progress.ToolCalls)
				}
			}
		}

		sb.WriteString(line + "\n")
	}

	// Footer with hints
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#928374")).Italic(true)
	sb.WriteString(hintStyle.Render("ctrl+t toggle | /team status | @name message"))

	return panelStyle.Render(sb.String())
}

// TeamSummary returns a one-line summary for the status bar.
func (m Model) TeamSummary() string {
	if m.manager == nil || m.teamName == "" {
		return ""
	}

	team, ok := m.manager.GetTeam(m.teamName)
	if !ok {
		return ""
	}

	working := 0
	total := 0
	for _, mem := range team.Members {
		if !mem.Identity.IsLead {
			total++
			if mem.Status == teams.StatusWorking {
				working++
			}
		}
	}

	if total == 0 {
		return ""
	}

	return fmt.Sprintf("team:%s %d/%d working", m.teamName, working, total)
}

func statusIcon(s teams.MemberStatus) string {
	switch s {
	case teams.StatusWorking:
		return "◐"
	case teams.StatusComplete:
		return "●"
	case teams.StatusFailed:
		return "✗"
	case teams.StatusShutdown:
		return "⊘"
	default:
		return "○"
	}
}

func statusColor(s teams.MemberStatus) lipgloss.Color {
	switch s {
	case teams.StatusWorking:
		return lipgloss.Color("#fabd2f") // yellow
	case teams.StatusComplete:
		return lipgloss.Color("#b8bb26") // green
	case teams.StatusFailed:
		return lipgloss.Color("#fb4934") // red
	case teams.StatusShutdown:
		return lipgloss.Color("#928374") // gray
	default:
		return lipgloss.Color("#a89984") // light gray
	}
}
