package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Abraxas-365/claudio/internal/api"
)

// FileAttachment represents a resolved @file mention.
type FileAttachment struct {
	OrigMatch   string // original text matched (e.g., `@src/main.go#L10-20`)
	Path        string // resolved absolute path
	DisplayPath string // short display path
	Content     string // file content (or directory listing)
	IsDir       bool
	LineStart   int // 0 = from beginning
	LineEnd     int // 0 = to end
	TotalLines  int
	Truncated   bool
}

// Regex for @mentions:
//   @"quoted path"           — quoted form
//   @"quoted path"#L10-20    — quoted with line range
//   @path/to/file            — unquoted (ends at whitespace)
//   @path/to/file#L10-20     — unquoted with line range
//   @path/to/file#L10        — single line
var atMentionRe = regexp.MustCompile(`@(?:"([^"]+)"|([\w./_\-~][^\s#]*))(?:#L(\d+)(?:-(\d+))?)?`)

const (
	maxFileSize  = 200_000 // ~200KB max per file
	maxDirEntries = 500
)

// ExtractFileAttachments parses @file mentions from text, reads their contents,
// and returns the attachments and the cleaned text (with @mentions replaced by filenames).
func ExtractFileAttachments(text, cwd string) ([]FileAttachment, string) {
	matches := atMentionRe.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil, text
	}

	var attachments []FileAttachment
	// Process matches in reverse so indices stay valid as we modify text
	cleaned := text
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		fullMatch := text[m[0]:m[1]]

		// Extract path (quoted or unquoted)
		var rawPath string
		if m[2] >= 0 && m[3] >= 0 {
			rawPath = text[m[2]:m[3]] // quoted
		} else if m[4] >= 0 && m[5] >= 0 {
			rawPath = text[m[4]:m[5]] // unquoted
		} else {
			continue
		}

		// Extract line range
		var lineStart, lineEnd int
		if m[6] >= 0 && m[7] >= 0 {
			lineStart, _ = strconv.Atoi(text[m[6]:m[7]])
		}
		if m[8] >= 0 && m[9] >= 0 {
			lineEnd, _ = strconv.Atoi(text[m[8]:m[9]])
		}
		if lineStart > 0 && lineEnd == 0 {
			lineEnd = lineStart // single line
		}

		// Resolve path
		absPath := expandPath(rawPath, cwd)

		att, err := readAttachment(absPath, rawPath, fullMatch, lineStart, lineEnd)
		if err != nil {
			// If file doesn't exist, leave the @mention as-is (might be a word like @param)
			continue
		}

		attachments = append([]FileAttachment{att}, attachments...) // prepend to maintain order

		// Replace @mention with just the display name in cleaned text
		cleaned = cleaned[:m[0]] + att.DisplayPath + cleaned[m[1]:]
	}

	return attachments, cleaned
}

// BuildContentBlocks creates API content blocks from file attachments + user text.
// File contents are prepended as separate text blocks so Claude sees them as context.
func BuildContentBlocks(text string, attachments []FileAttachment, imageBlocks []api.UserContentBlock) []api.UserContentBlock {
	var blocks []api.UserContentBlock

	// Images first
	blocks = append(blocks, imageBlocks...)

	// File attachments as context blocks
	for _, att := range attachments {
		var header string
		if att.IsDir {
			header = fmt.Sprintf("Directory listing of %s:", att.DisplayPath)
		} else if att.LineStart > 0 {
			header = fmt.Sprintf("Contents of %s (lines %d-%d):", att.DisplayPath, att.LineStart, att.LineEnd)
		} else {
			header = fmt.Sprintf("Contents of %s:", att.DisplayPath)
		}
		if att.Truncated {
			header += " [truncated]"
		}

		content := fmt.Sprintf("<file path=\"%s\">\n%s\n%s\n</file>",
			att.DisplayPath, header, att.Content)
		blocks = append(blocks, api.NewTextBlock(content))
	}

	// User message last
	blocks = append(blocks, api.NewTextBlock(text))

	return blocks
}

func readAttachment(absPath, rawPath, fullMatch string, lineStart, lineEnd int) (FileAttachment, error) {
	info, err := os.Stat(absPath)
	if err != nil {
		return FileAttachment{}, err
	}

	displayPath := rawPath
	if displayPath == "" {
		displayPath = filepath.Base(absPath)
	}

	att := FileAttachment{
		OrigMatch:   fullMatch,
		Path:        absPath,
		DisplayPath: displayPath,
		LineStart:   lineStart,
		LineEnd:     lineEnd,
	}

	if info.IsDir() {
		att.IsDir = true
		content, err := listDirectory(absPath)
		if err != nil {
			return FileAttachment{}, err
		}
		att.Content = content
		return att, nil
	}

	// Check size before reading
	if info.Size() > maxFileSize*2 {
		att.Truncated = true
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return FileAttachment{}, err
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	att.TotalLines = len(lines)

	// Apply line range
	if lineStart > 0 {
		start := lineStart - 1 // 1-indexed to 0-indexed
		end := lineEnd
		if start < 0 {
			start = 0
		}
		if end > len(lines) {
			end = len(lines)
		}
		if start >= len(lines) {
			start = len(lines) - 1
		}
		lines = lines[start:end]
		content = strings.Join(lines, "\n")
	}

	// Truncate if too large
	if len(content) > maxFileSize {
		content = content[:maxFileSize] + "\n... [truncated]"
		att.Truncated = true
	}

	att.Content = content
	return att, nil
}

func listDirectory(dirPath string) (string, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return "", err
	}

	var lines []string
	count := 0
	for _, e := range entries {
		if count >= maxDirEntries {
			lines = append(lines, fmt.Sprintf("... and %d more entries", len(entries)-maxDirEntries))
			break
		}
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
		count++
	}

	return strings.Join(lines, "\n"), nil
}

func expandPath(path, cwd string) string {
	path = strings.TrimSpace(path)

	// Tilde expansion
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}

	// Absolute path
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	// Relative path — resolve from cwd
	return filepath.Clean(filepath.Join(cwd, path))
}
