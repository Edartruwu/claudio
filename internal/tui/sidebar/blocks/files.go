// Package blocks contains sidebar block implementations.
package blocks

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/panels/filespanel"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// FilesBlock shows files touched during the session.
type FilesBlock struct {
	entries []filespanel.FileEntry
}

// NewFilesBlock creates a new files block.
func NewFilesBlock() *FilesBlock { return &FilesBlock{} }

// Refresh updates the file list from the given ops.
func (b *FilesBlock) Refresh(ops []filespanel.FileOp) {
	byPath := make(map[string]*filespanel.FileEntry)
	for _, op := range ops {
		e, ok := byPath[op.Path]
		if !ok {
			e = &filespanel.FileEntry{Path: op.Path}
			byPath[op.Path] = e
		}
		e.Count++
		switch op.Operation {
		case "write":
			if e.Status < filespanel.FileAdded {
				e.Status = filespanel.FileAdded
			}
		case "edit", "multiedit":
			if e.Status < filespanel.FileModified {
				e.Status = filespanel.FileModified
			}
		}
	}
	b.entries = b.entries[:0]
	for _, e := range byPath {
		if e.Status > filespanel.FileRead {
			b.entries = append(b.entries, *e)
		}
	}
}

func (b *FilesBlock) Title() string     { return fmt.Sprintf("Files (%d)", len(b.entries)) }
func (b *FilesBlock) MinHeight() int    { return 1 }
func (b *FilesBlock) Weight() int       { return 3 }

func (b *FilesBlock) Render(width, maxHeight int) string {
	addStyle := lipgloss.NewStyle().Foreground(styles.Success)
	modStyle := lipgloss.NewStyle().Foreground(styles.Warning)
	readStyle := lipgloss.NewStyle().Foreground(styles.Muted)
	dimStyle := lipgloss.NewStyle().Foreground(styles.Muted)

	if len(b.entries) == 0 {
		return dimStyle.Render("  No files yet")
	}

	maxPathW := width - 5
	if maxPathW < 8 {
		maxPathW = 8
	}

	var lines []string
	for i, e := range b.entries {
		if i >= maxHeight {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  +%d more", len(b.entries)-i)))
			break
		}
		var icon string
		var iconSt lipgloss.Style
		switch e.Status {
		case filespanel.FileAdded:
			icon = "✚"
			iconSt = addStyle
		case filespanel.FileModified:
			icon = "✎"
			iconSt = modStyle
		default:
			icon = "○"
			iconSt = readStyle
		}
		path := e.Path
		if len(path) > maxPathW {
			path = "…" + path[len(path)-maxPathW+1:]
		}
		count := ""
		if e.Count > 1 {
			count = dimStyle.Render(fmt.Sprintf(" (%d)", e.Count))
		}
		lines = append(lines, " "+iconSt.Render(icon)+" "+path+count)
	}
	return strings.Join(lines, "\n")
}
