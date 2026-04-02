// Package difftracker tracks file changes per conversation turn.
package difftracker

import (
	"os/exec"
	"strings"
	"sync"
)

// TurnDiff records what changed during a single turn.
type TurnDiff struct {
	Turn          int
	FilesModified []string
	Summary       string // short stat summary
	Patch         string // full diff output
}

// Tracker captures before/after snapshots of git state per turn.
type Tracker struct {
	mu       sync.Mutex
	diffs    []TurnDiff
	turnNum  int
	baseline string // git diff output before turn
}

// New creates a new diff tracker.
func New() *Tracker {
	return &Tracker{}
}

// BeforeTurn captures the current git diff state before tools execute.
func (t *Tracker) BeforeTurn() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.turnNum++
	t.baseline = gitDiff()
}

// AfterTurn computes what changed during this turn.
func (t *Tracker) AfterTurn() *TurnDiff {
	t.mu.Lock()
	defer t.mu.Unlock()

	after := gitDiff()

	// Compute new changes (diff of diffs)
	// Simple approach: if baseline was empty and after is not, all changes are new
	// If baseline had content, the new content is the delta
	patch := after
	if t.baseline != "" && after == t.baseline {
		return nil // no changes
	}

	stat := gitDiffStat()
	files := parseModifiedFiles(stat)

	if len(files) == 0 && patch == t.baseline {
		return nil
	}

	diff := &TurnDiff{
		Turn:          t.turnNum,
		FilesModified: files,
		Summary:       stat,
		Patch:         patch,
	}
	t.diffs = append(t.diffs, *diff)
	return diff
}

// GetTurn returns the diff for a specific turn number.
func (t *Tracker) GetTurn(n int) *TurnDiff {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i := range t.diffs {
		if t.diffs[i].Turn == n {
			return &t.diffs[i]
		}
	}
	return nil
}

// All returns all recorded turn diffs.
func (t *Tracker) All() []TurnDiff {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := make([]TurnDiff, len(t.diffs))
	copy(result, t.diffs)
	return result
}

// Count returns the number of recorded turn diffs.
func (t *Tracker) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.diffs)
}

func gitDiff() string {
	cmd := exec.Command("git", "diff")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func gitDiffStat() string {
	cmd := exec.Command("git", "diff", "--stat")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func parseModifiedFiles(stat string) []string {
	if stat == "" {
		return nil
	}
	var files []string
	for _, line := range strings.Split(stat, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "changed") {
			continue // skip summary line
		}
		// Each line looks like: " path/to/file | 5 ++---"
		parts := strings.SplitN(line, "|", 2)
		if len(parts) >= 1 {
			file := strings.TrimSpace(parts[0])
			if file != "" {
				files = append(files, file)
			}
		}
	}
	return files
}
