// Package filtersavings provides analytics for output filter savings,
// tracking how much data is saved by filtering command output and
// identifying commands that could benefit from filtering.
package filtersavings

import "github.com/Abraxas-365/claudio/internal/storage"

// Service provides filter savings analytics backed by SQLite.
type Service struct {
	db *storage.DB
}

// NewService creates a new filter savings analytics service.
func NewService(db *storage.DB) *Service {
	return &Service{db: db}
}

// Stats holds aggregate filter savings statistics.
type Stats struct {
	TotalBytesIn  int64
	TotalBytesOut int64
	TotalSaved    int64
	SavingsPct    float64
	RecordCount   int64
}

// CommandStat holds per-command filter savings statistics.
type CommandStat struct {
	Command    string
	BytesIn    int64
	BytesOut   int64
	Saved      int64
	SavingsPct float64
	Count      int64
}

// DiscoverySuggestion represents a command that currently has no filter
// savings applied but could benefit from one, ranked by opportunity size.
type DiscoverySuggestion struct {
	Command     string
	AvgBytesIn  int64
	Occurrences int64
}
