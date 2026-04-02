package utils

import (
	"fmt"
	"strings"
)

// DiffLine represents a single line in a diff.
type DiffLine struct {
	Type    DiffType
	Content string
	OldNum  int // line number in old text (0 if added)
	NewNum  int // line number in new text (0 if removed)
}

// DiffType is the type of a diff line.
type DiffType int

const (
	DiffEqual   DiffType = iota // unchanged line
	DiffAdded                    // added line
	DiffRemoved                  // removed line
)

// SimpleDiff computes a simple line-by-line diff between two strings.
// Returns the diff lines. For production use, consider a proper diff library.
func SimpleDiff(oldText, newText string) []DiffLine {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")

	// Simple LCS-based diff
	return computeDiff(oldLines, newLines)
}

// FormatDiff formats diff lines as a unified diff string.
func FormatDiff(lines []DiffLine) string {
	var sb strings.Builder
	for _, line := range lines {
		switch line.Type {
		case DiffAdded:
			sb.WriteString(fmt.Sprintf("+%s\n", line.Content))
		case DiffRemoved:
			sb.WriteString(fmt.Sprintf("-%s\n", line.Content))
		case DiffEqual:
			sb.WriteString(fmt.Sprintf(" %s\n", line.Content))
		}
	}
	return sb.String()
}

// DiffStats returns counts of added, removed, and changed lines.
func DiffStats(lines []DiffLine) (added, removed, unchanged int) {
	for _, line := range lines {
		switch line.Type {
		case DiffAdded:
			added++
		case DiffRemoved:
			removed++
		case DiffEqual:
			unchanged++
		}
	}
	return
}

// DiffSummary returns a one-line summary like "+10 -5 ~2 files".
func DiffSummary(added, removed int) string {
	parts := []string{}
	if added > 0 {
		parts = append(parts, fmt.Sprintf("+%d", added))
	}
	if removed > 0 {
		parts = append(parts, fmt.Sprintf("-%d", removed))
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, " ")
}

func computeDiff(old, new []string) []DiffLine {
	// Simple O(nm) LCS diff
	m, n := len(old), len(new)

	// Build LCS table
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if old[i-1] == new[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] > dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// Backtrack to produce diff
	var result []DiffLine
	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && old[i-1] == new[j-1] {
			result = append([]DiffLine{{Type: DiffEqual, Content: old[i-1], OldNum: i, NewNum: j}}, result...)
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			result = append([]DiffLine{{Type: DiffAdded, Content: new[j-1], NewNum: j}}, result...)
			j--
		} else if i > 0 {
			result = append([]DiffLine{{Type: DiffRemoved, Content: old[i-1], OldNum: i}}, result...)
			i--
		}
	}

	return result
}
