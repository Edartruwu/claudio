package filtersavings

import (
	"context"
	"fmt"
)

// Discover returns commands that currently have no filter savings (bytes_in == bytes_out)
// ranked by opportunity size (avg bytes * frequency). These are candidates for new filters.
func (s *Service) Discover(ctx context.Context, limit int) ([]DiscoverySuggestion, error) {
	rows, err := s.db.Conn().QueryContext(ctx,
		`SELECT command,
		        CAST(AVG(bytes_in) AS INTEGER) AS avg_bytes_in,
		        COUNT(*) AS occurrences
		 FROM filter_savings
		 WHERE bytes_in = bytes_out
		 GROUP BY command
		 ORDER BY AVG(bytes_in) * COUNT(*) DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("filtersavings: discover: %w", err)
	}
	defer rows.Close()

	var result []DiscoverySuggestion
	for rows.Next() {
		var ds DiscoverySuggestion
		if err := rows.Scan(&ds.Command, &ds.AvgBytesIn, &ds.Occurrences); err != nil {
			return nil, fmt.Errorf("filtersavings: scan discovery: %w", err)
		}
		result = append(result, ds)
	}
	return result, rows.Err()
}
