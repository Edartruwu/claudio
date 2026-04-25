package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

var (
	welcomeLogoStyle     = lipgloss.NewStyle().Bold(true).Foreground(styles.Primary)
	welcomeSubtitleStyle = lipgloss.NewStyle().Foreground(styles.Text).Italic(true)
	welcomeKeyStyle      = lipgloss.NewStyle().Foreground(styles.Warning).Bold(true)
	welcomeLabelStyle    = lipgloss.NewStyle().Foreground(styles.Text)
	welcomeMutedStyle    = lipgloss.NewStyle().Foreground(styles.Muted)
	welcomeCwdStyle      = lipgloss.NewStyle().Foreground(styles.Subtle)
	welcomeVerStyle      = lipgloss.NewStyle().Foreground(styles.Dim)
)

// renderWelcomeScreen renders a centered welcome screen.
func (m *Model) renderWelcomeScreen() string {
	w := m.viewport.Width
	h := m.viewport.Height

	// Box width — constrained so Place() has room to center it
	boxW := 60
	if w < 68 {
		boxW = w - 8
	}
	if boxW < 32 {
		boxW = 32
	}

	// ── Logo ──────────────────────────────────────────────
	logo := welcomeLogoStyle.Render("claudio")
	subtitle := welcomeSubtitleStyle.Render("AI coding assistant")

	// ── Hints row ─────────────────────────────────────────
	kb := func(key, label string) string {
		return welcomeKeyStyle.Render(key) + " " + welcomeLabelStyle.Render(label)
	}
	tagline := welcomeMutedStyle.Render("Type your message below, or use a shortcut:")
	hints := kb("/", "commands") + "   " +
		kb("@", "files") + "   " +
		kb("ctrl+p", "palette")

	// ── Recent sessions ────────────────────────────────────
	var recentParts []string
	if m.session != nil {
		recent, _ := m.session.RecentForProject(3)
		if len(recent) == 0 {
			recent, _ = m.session.Search("", 3)
		}
		if len(recent) > 0 {
			headerLine := welcomeKeyStyle.Render("Recent") +
				" " + welcomeMutedStyle.Render(strings.Repeat("─", boxW-7))
			recentParts = append(recentParts, headerLine)
			for i, s := range recent {
				num := welcomeKeyStyle.Render(fmt.Sprintf("%d", i+1))
				title := s.Title
				if title == "" {
					title = s.ID
				}
				if len(title) > boxW-12 {
					title = title[:boxW-15] + "…"
				}
				titleS := welcomeLabelStyle.Render(title)
				recentParts = append(recentParts, "  "+num+"  "+titleS)
			}
			recentParts = append(recentParts, welcomeMutedStyle.Render("[1-3] resume · <Space>. browse · type to chat"))
		}
	}

	// ── Assemble narrow block ──────────────────────────────
	// Each element is individually centered at boxW so the block has
	// a consistent width that Place() can center in the terminal.
	center := func(s string) string {
		return lipgloss.NewStyle().Width(boxW + 4).Align(lipgloss.Center).Render(s)
	}

	var lines []string
	lines = append(lines, center(logo))
	lines = append(lines, center(subtitle))
	lines = append(lines, "")
	lines = append(lines, center(tagline))
	lines = append(lines, "")
	lines = append(lines, center(hints))

	if len(recentParts) > 0 {
		lines = append(lines, "")
		for _, rp := range recentParts {
			lines = append(lines, center(rp))
		}
	}

	block := strings.Join(lines, "\n")

	// ── Bottom row: cwd ← → version ───────────────────────
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(cwd, home) {
		cwd = "~" + cwd[len(home):]
	}
	if len(cwd) > 40 {
		parts := strings.Split(cwd, string(filepath.Separator))
		if len(parts) > 3 {
			cwd = "…/" + strings.Join(parts[len(parts)-2:], "/")
		}
	}
	version := "claudio"
	cwdS := welcomeCwdStyle.Render(cwd)
	verS := welcomeVerStyle.Render(version)
	gap := w - lipgloss.Width(cwdS) - lipgloss.Width(verS) - 2
	if gap < 1 {
		gap = 1
	}
	bottomRow := " " + cwdS + strings.Repeat(" ", gap) + verS

	placed := lipgloss.Place(w, h-1, lipgloss.Center, lipgloss.Center, block)
	return placed + "\n" + bottomRow
}
