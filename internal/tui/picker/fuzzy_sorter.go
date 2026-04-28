package picker

import (
	"math"
	"strings"
)

// fuzzySorter is a case-insensitive substring-match sorter.
// Scoring rules:
//   - empty prompt  → 0.0   (everything matches, preserve order)
//   - exact match   → 0.0   (best possible)
//   - prefix match  → position score near 0
//   - substring     → position / len(ordinal) in (0, 1)
//   - no match      → math.MaxFloat64
type fuzzySorter struct{}

// NewFuzzySorter returns a Sorter using simple case-insensitive substring
// matching. No external dependencies; no CGO.
func NewFuzzySorter() Sorter {
	return fuzzySorter{}
}

func (fuzzySorter) Score(prompt string, entry Entry) float64 {
	if prompt == "" {
		return 0.0
	}

	p := strings.ToLower(prompt)
	o := strings.ToLower(entry.Ordinal)

	if o == "" {
		return math.MaxFloat64
	}

	idx := strings.Index(o, p)
	if idx < 0 {
		return math.MaxFloat64
	}

	// Earlier match position = lower (better) score.
	// 0 → exact prefix → score 0.0; end of string → score approaches 1.0.
	return float64(idx) / float64(len(o))
}
