package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// GenerateSessionID creates a new unique session ID.
func GenerateSessionID() string {
	return uuid.New().String()
}

// SessionFile represents a session transcript file.
type SessionFile struct {
	SessionID string    `json:"session_id"`
	ProjectDir string   `json:"project_dir"`
	Model     string    `json:"model"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time,omitempty"`
	Title     string    `json:"title,omitempty"`
}

// SaveSessionFile saves session metadata to a JSON file.
func SaveSessionFile(dir string, session *SessionFile) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	filename := fmt.Sprintf("%s-%s.json",
		session.StartTime.Format("2006-01-02"),
		session.SessionID[:8])

	path := filepath.Join(dir, filename)
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// LoadRecentSessions loads session files from the last N days.
func LoadRecentSessions(dir string, maxAgeDays int) []*SessionFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)
	var sessions []*SessionFile

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		info, err := entry.Info()
		if err != nil || info.ModTime().Before(cutoff) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var session SessionFile
		if err := json.Unmarshal(data, &session); err != nil {
			continue
		}

		sessions = append(sessions, &session)
	}

	return sessions
}

// SessionTmpFile creates a .tmp session context file for handoff between sessions.
// Format matches the everything-claude-code session pattern.
func SessionTmpFile(dir, sessionID, summary string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	filename := fmt.Sprintf("%s-%s.tmp",
		time.Now().Format("2006-01-02"),
		sessionID[:8])

	content := fmt.Sprintf(`# Session Context
Date: %s
Session: %s

## Current State
%s

## Next Steps
(to be filled by next session)
`, time.Now().Format("2006-01-02 15:04"), sessionID[:8], summary)

	return os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644)
}
