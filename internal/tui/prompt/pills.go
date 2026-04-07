// Package prompt — pills.go provides the context pills row rendered above the textarea.
package prompt

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// Pill represents a single context item shown above the prompt textarea.
type Pill struct {
	Icon     string
	Label    string
	PillType string // "image", "paste", "file", "memory"
}

// pillStyle returns the lipgloss style for a pill.
func pillStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(styles.Warning).
		Bold(true).
		Padding(0, 1).
		MarginRight(1)
}

// mentionIcon returns the appropriate icon for an @mention path.
func mentionIcon(name, cwd string) string {
	// Strip line-range suffix (e.g. file.go#L10-20)
	path := name
	if idx := strings.IndexByte(name, '#'); idx >= 0 {
		path = name[:idx]
	}
	// Resolve relative paths
	if !strings.HasPrefix(path, "/") {
		path = cwd + "/" + path
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return "📁"
	}
	return "📄"
}

// RenderPills renders a row of pills. Returns empty string if no pills.
func RenderPills(pills []Pill, width int) string {
	if len(pills) == 0 {
		return ""
	}
	s := pillStyle()
	var rendered []string
	totalW := 0
	for i, p := range pills {
		label := "[" + p.Icon + " " + p.Label + "]"
		w := lipgloss.Width(s.Render(label)) + 1
		if totalW+w > width-2 {
			rendered = append(rendered, lipgloss.NewStyle().Foreground(styles.Dim).Render(fmt.Sprintf("+%d more", len(pills)-i)))
			break
		}
		rendered = append(rendered, s.Render(label))
		totalW += w
	}
	return " " + strings.Join(rendered, "")
}

// BuildPills constructs the list of pills from prompt state.
func BuildPills(images []ImageAttachment, pastedContents map[int]string, promptText string) []Pill {
	var pills []Pill
	for _, img := range images {
		pills = append(pills, Pill{Icon: "📎", Label: img.FileName, PillType: "image"})
	}
	// Sort paste IDs for deterministic order
	for id, paste := range pastedContents {
		lines := strings.Count(paste, "\n") + 1
		pills = append(pills, Pill{Icon: "📋", Label: fmt.Sprintf("#%d +%d lines", id, lines), PillType: "paste"})
	}
	// Parse @mentions from prompt text
	cwd, _ := os.Getwd()
	words := strings.Fields(promptText)
	seen := make(map[string]bool)
	for _, w := range words {
		if strings.HasPrefix(w, "@") && len(w) > 1 {
			name := w[1:]
			if !seen[name] {
				seen[name] = true
				icon := mentionIcon(name, cwd)
				pills = append(pills, Pill{Icon: icon, Label: name, PillType: "file"})
			}
		}
	}
	return pills
}
