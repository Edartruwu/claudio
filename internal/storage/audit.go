package storage

import "time"

// AuditEntry represents a logged tool invocation.
type AuditEntry struct {
	ID            int64
	SessionID     string
	Tool          string
	InputSummary  string
	OutputSummary string
	Approval      string // "allowed", "denied", "auto"
	TokensUsed    int
	DurationMs    int64
	CreatedAt     time.Time
}

// LogAudit writes a tool invocation to the audit log.
func (db *DB) LogAudit(entry AuditEntry) error {
	_, err := db.conn.Exec(
		`INSERT INTO audit_log (session_id, tool, input_summary, output_summary, approval, tokens_used, duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.SessionID, entry.Tool, entry.InputSummary, entry.OutputSummary,
		entry.Approval, entry.TokensUsed, entry.DurationMs,
	)
	return err
}

// GetRecentAudit returns recent audit entries.
func (db *DB) GetRecentAudit(limit int) ([]AuditEntry, error) {
	rows, err := db.conn.Query(
		`SELECT id, session_id, tool, input_summary, output_summary, approval, tokens_used, duration_ms, created_at
		 FROM audit_log ORDER BY created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Tool, &e.InputSummary, &e.OutputSummary,
			&e.Approval, &e.TokensUsed, &e.DurationMs, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetSessionAudit returns audit entries for a specific session.
func (db *DB) GetSessionAudit(sessionID string) ([]AuditEntry, error) {
	rows, err := db.conn.Query(
		`SELECT id, session_id, tool, input_summary, output_summary, approval, tokens_used, duration_ms, created_at
		 FROM audit_log WHERE session_id = ? ORDER BY created_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Tool, &e.InputSummary, &e.OutputSummary,
			&e.Approval, &e.TokensUsed, &e.DurationMs, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
