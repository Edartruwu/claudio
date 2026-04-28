// Package blocks contains sidebar block implementations.
package blocks

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/panels/filespanel"
	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

var (
	fileAddStyle  = lipgloss.NewStyle().Foreground(styles.Success)
	fileModStyle  = lipgloss.NewStyle().Foreground(styles.Warning)
	fileReadStyle = lipgloss.NewStyle().Foreground(styles.Muted)
	fileDimStyle  = lipgloss.NewStyle().Foreground(styles.Muted)
	fileNameStyle = lipgloss.NewStyle().Foreground(styles.Text).Bold(true)
	fileDirStyle  = lipgloss.NewStyle().Foreground(styles.Muted)
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

func (b *FilesBlock) Title() string  { return fmt.Sprintf("Files (%d)", len(b.entries)) }
func (b *FilesBlock) MinHeight() int { return 1 }
func (b *FilesBlock) Weight() int    { return 3 }

// Entries returns a snapshot of the current file entries for use by Lua providers.
func (b *FilesBlock) Entries() []filespanel.FileEntry {
	result := make([]filespanel.FileEntry, len(b.entries))
	copy(result, b.entries)
	return result
}

func (b *FilesBlock) Render(width, maxHeight int) string {
	if len(b.entries) == 0 {
		return fileDimStyle.Render("  No files yet")
	}

	// Reserve: 1 icon + 1 space + 1 space indent = 3 chars prefix
	maxLineW := width - 3
	if maxLineW < 8 {
		maxLineW = 8
	}

	var lines []string
	for i, e := range b.entries {
		if i >= maxHeight {
			lines = append(lines, fileDimStyle.Render(fmt.Sprintf("  +%d more", len(b.entries)-i)))
			break
		}

		var icon string
		var iconSt lipgloss.Style
		switch e.Status {
		case filespanel.FileAdded:
			icon = "✚"
			iconSt = fileAddStyle
		case filespanel.FileModified:
			icon = "✎"
			iconSt = fileModStyle
		default:
			icon = "○"
			iconSt = fileReadStyle
		}

		base := filepath.Base(e.Path)
		dir := filepath.Dir(e.Path)
		if dir == "." {
			dir = ""
		}

		count := ""
		if e.Count > 1 {
			count = fileDimStyle.Render(fmt.Sprintf("×%d", e.Count))
		}

		// Render basename bold + dir dimmed, truncating dir if needed
		baseRendered := fileNameStyle.Render(base)
		if dir != "" {
			// Total visible length: base + "/" + dir
			visibleLen := len(base) + 1 + len(dir)
			if visibleLen > maxLineW {
				// Truncate dir from the left
				keep := maxLineW - len(base) - 2 // 2 = "/" + "…"
				if keep > 0 {
					dir = "…" + dir[len(dir)-keep:]
				} else {
					dir = ""
				}
			}
			if dir != "" {
				dirRendered := fileDirStyle.Render(dir + "/")
				lines = append(lines, " "+iconSt.Render(icon)+" "+dirRendered+baseRendered+count)
			} else {
				lines = append(lines, " "+iconSt.Render(icon)+" "+baseRendered+count)
			}
		} else {
			// Truncate base if needed
			if len(base) > maxLineW {
				base = base[:maxLineW-1] + "…"
				baseRendered = fileNameStyle.Render(base)
			}
			lines = append(lines, " "+iconSt.Render(icon)+" "+baseRendered+count)
		}
	}
	return strings.Join(lines, "\n")
}
