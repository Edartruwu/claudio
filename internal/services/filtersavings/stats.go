package filtersavings

import (
	"context"
	"fmt"
)

// GetStats returns aggregate filter savings statistics across all recorded commands.
func (s *Service) GetStats(ctx context.Context) (Stats, error) {
	var st Stats
	err := s.db.Conn().QueryRowContext(ctx,
		`SELECT COALESCE(SUM(bytes_in), 0),
		        COALESCE(SUM(bytes_out), 0),
		        COALESCE(SUM(bytes_in - bytes_out), 0),
		        COUNT(*)
		 FROM filter_savings`,
	).Scan(&st.TotalBytesIn, &st.TotalBytesOut, &st.TotalSaved, &st.RecordCount)
	if err != nil {
		return Stats{}, fmt.Errorf("filtersavings: get stats: %w", err)
	}
	if st.TotalBytesIn > 0 {
		st.SavingsPct = float64(st.TotalSaved) / float64(st.TotalBytesIn) * 100
	}
	return st, nil
}

// GetTopCommands returns the top N commands ranked by total bytes saved (descending).
func (s *Service) GetTopCommands(ctx context.Context, limit int) ([]CommandStat, error) {
	rows, err := s.db.Conn().QueryContext(ctx,
		`SELECT command,
		        SUM(bytes_in) AS total_in,
		        SUM(bytes_out) AS total_out,
		        SUM(bytes_in - bytes_out) AS total_saved,
		        COUNT(*) AS cnt
		 FROM filter_savings
		 GROUP BY command
		 ORDER BY total_saved DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("filtersavings: get top commands: %w", err)
	}
	defer rows.Close()

	var result []CommandStat
	for rows.Next() {
		var cs CommandStat
		if err := rows.Scan(&cs.Command, &cs.BytesIn, &cs.BytesOut, &cs.Saved, &cs.Count); err != nil {
			return nil, fmt.Errorf("filtersavings: scan top command: %w", err)
		}
		if cs.BytesIn > 0 {
			cs.SavingsPct = float64(cs.Saved) / float64(cs.BytesIn) * 100
		}
		result = append(result, cs)
	}
	return result, rows.Err()
}
