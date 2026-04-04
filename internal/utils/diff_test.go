package utils

import (
	"strings"
	"testing"
)

func TestSimpleDiff(t *testing.T) {
	tests := []struct {
		name        string
		oldText     string
		newText     string
		wantAdded   int
		wantRemoved int
	}{
		{
			name:        "identical texts",
			oldText:     "line1\nline2\nline3",
			newText:     "line1\nline2\nline3",
			wantAdded:   0,
			wantRemoved: 0,
		},
		{
			// strings.Split("", "\n") = [""] so empty old gives 1 phantom line,
			// which gets replaced/removed; hence removed=1 and added depends on newText lines.
			name:        "empty old",
			oldText:     "",
			newText:     "line1\nline2",
			wantAdded:   2,
			wantRemoved: 1,
		},
		{
			name:        "empty new",
			oldText:     "line1\nline2",
			newText:     "",
			wantAdded:   1,
			wantRemoved: 2,
		},
		{
			name:        "both empty",
			oldText:     "",
			newText:     "",
			wantAdded:   0,
			wantRemoved: 0,
		},
		{
			name:        "added line",
			oldText:     "a\nb",
			newText:     "a\nb\nc",
			wantAdded:   1,
			wantRemoved: 0,
		},
		{
			name:        "removed line",
			oldText:     "a\nb\nc",
			newText:     "a\nc",
			wantAdded:   0,
			wantRemoved: 1,
		},
		{
			name:        "changed line",
			oldText:     "hello",
			newText:     "world",
			wantAdded:   1,
			wantRemoved: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lines := SimpleDiff(tc.oldText, tc.newText)
			added, removed, _ := DiffStats(lines)
			if added != tc.wantAdded {
				t.Errorf("added: got %d, want %d", added, tc.wantAdded)
			}
			if removed != tc.wantRemoved {
				t.Errorf("removed: got %d, want %d", removed, tc.wantRemoved)
			}
		})
	}
}

func TestSimpleDiff_LineTypes(t *testing.T) {
	lines := SimpleDiff("a\nb\nc", "a\nd\nc")
	// "b" removed, "d" added, "a" and "c" equal
	var added, removed, equal int
	for _, l := range lines {
		switch l.Type {
		case DiffAdded:
			added++
		case DiffRemoved:
			removed++
		case DiffEqual:
			equal++
		}
	}
	if added != 1 || removed != 1 || equal != 2 {
		t.Errorf("unexpected counts: added=%d removed=%d equal=%d", added, removed, equal)
	}
}

func TestFormatDiff(t *testing.T) {
	lines := []DiffLine{
		{Type: DiffEqual, Content: "same"},
		{Type: DiffAdded, Content: "new"},
		{Type: DiffRemoved, Content: "old"},
	}
	got := FormatDiff(lines)
	if !strings.Contains(got, " same\n") {
		t.Errorf("expected equal line with space prefix, got: %q", got)
	}
	if !strings.Contains(got, "+new\n") {
		t.Errorf("expected added line with + prefix, got: %q", got)
	}
	if !strings.Contains(got, "-old\n") {
		t.Errorf("expected removed line with - prefix, got: %q", got)
	}
}

func TestFormatDiff_Empty(t *testing.T) {
	got := FormatDiff(nil)
	if got != "" {
		t.Errorf("expected empty string for nil lines, got %q", got)
	}
	got = FormatDiff([]DiffLine{})
	if got != "" {
		t.Errorf("expected empty string for empty lines, got %q", got)
	}
}

func TestDiffStats(t *testing.T) {
	tests := []struct {
		name          string
		lines         []DiffLine
		wantAdded     int
		wantRemoved   int
		wantUnchanged int
	}{
		{
			name:          "empty",
			lines:         nil,
			wantAdded:     0,
			wantRemoved:   0,
			wantUnchanged: 0,
		},
		{
			name: "mixed",
			lines: []DiffLine{
				{Type: DiffAdded},
				{Type: DiffAdded},
				{Type: DiffRemoved},
				{Type: DiffEqual},
				{Type: DiffEqual},
				{Type: DiffEqual},
			},
			wantAdded:     2,
			wantRemoved:   1,
			wantUnchanged: 3,
		},
		{
			name: "all equal",
			lines: []DiffLine{
				{Type: DiffEqual},
				{Type: DiffEqual},
			},
			wantAdded:     0,
			wantRemoved:   0,
			wantUnchanged: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			added, removed, unchanged := DiffStats(tc.lines)
			if added != tc.wantAdded {
				t.Errorf("added: got %d, want %d", added, tc.wantAdded)
			}
			if removed != tc.wantRemoved {
				t.Errorf("removed: got %d, want %d", removed, tc.wantRemoved)
			}
			if unchanged != tc.wantUnchanged {
				t.Errorf("unchanged: got %d, want %d", unchanged, tc.wantUnchanged)
			}
		})
	}
}

func TestDiffSummary(t *testing.T) {
	tests := []struct {
		name    string
		added   int
		removed int
		want    string
	}{
		{"no changes", 0, 0, "no changes"},
		{"only added", 5, 0, "+5"},
		{"only removed", 0, 3, "-3"},
		{"both", 4, 2, "+4 -2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DiffSummary(tc.added, tc.removed)
			if got != tc.want {
				t.Errorf("DiffSummary(%d, %d) = %q, want %q", tc.added, tc.removed, got, tc.want)
			}
		})
	}
}
