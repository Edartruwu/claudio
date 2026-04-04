package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateSessionID_Format(t *testing.T) {
	id := GenerateSessionID()
	if len(id) != 36 {
		t.Errorf("GenerateSessionID() length = %d, want 36 (UUID format)", len(id))
	}
	// UUID v4 has dashes at positions 8, 13, 18, 23
	for _, pos := range []int{8, 13, 18, 23} {
		if id[pos] != '-' {
			t.Errorf("GenerateSessionID() missing dash at position %d: %q", pos, id)
		}
	}
}

func TestGenerateSessionID_Unique(t *testing.T) {
	ids := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		id := GenerateSessionID()
		if _, exists := ids[id]; exists {
			t.Fatalf("GenerateSessionID() returned duplicate: %q", id)
		}
		ids[id] = struct{}{}
	}
}

func TestSaveSessionFile_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	session := &SessionFile{
		SessionID:  "abcd1234-0000-0000-0000-000000000000",
		ProjectDir: "/some/project",
		Model:      "claude-3",
		StartTime:  now,
		Title:      "test session",
	}

	if err := SaveSessionFile(dir, session); err != nil {
		t.Fatalf("SaveSessionFile() error: %v", err)
	}

	// Check that at least one .json file was created
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	var found bool
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			found = true
			break
		}
	}
	if !found {
		t.Error("SaveSessionFile() did not create a .json file")
	}
}

func TestSaveSessionFile_CreatesDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nested", "sessions")

	session := &SessionFile{
		SessionID: "abcd1234-0000-0000-0000-000000000000",
		StartTime: time.Now(),
	}

	if err := SaveSessionFile(dir, session); err != nil {
		t.Fatalf("SaveSessionFile() with missing dir: %v", err)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("SaveSessionFile() did not create directory")
	}
}

func TestSaveAndLoadRecentSessions(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	session := &SessionFile{
		SessionID:  "abcd1234-ef00-0000-0000-000000000000",
		ProjectDir: "/project",
		Model:      "claude-3",
		StartTime:  now,
		Title:      "roundtrip",
	}

	if err := SaveSessionFile(dir, session); err != nil {
		t.Fatalf("SaveSessionFile() error: %v", err)
	}

	loaded := LoadRecentSessions(dir, 30)
	if len(loaded) != 1 {
		t.Fatalf("LoadRecentSessions(): got %d sessions, want 1", len(loaded))
	}
	if loaded[0].SessionID != session.SessionID {
		t.Errorf("SessionID = %q, want %q", loaded[0].SessionID, session.SessionID)
	}
	if loaded[0].Model != session.Model {
		t.Errorf("Model = %q, want %q", loaded[0].Model, session.Model)
	}
	if loaded[0].Title != session.Title {
		t.Errorf("Title = %q, want %q", loaded[0].Title, session.Title)
	}
}

func TestLoadRecentSessions_NonExistentDir(t *testing.T) {
	sessions := LoadRecentSessions("/nonexistent/dir/xyz", 7)
	if sessions != nil {
		t.Errorf("LoadRecentSessions nonexistent dir: expected nil, got %v", sessions)
	}
}

func TestLoadRecentSessions_OldFiles(t *testing.T) {
	dir := t.TempDir()

	// Write a valid JSON session file with an old modification time
	session := &SessionFile{
		SessionID: "abcd1234-0000-0000-0000-000000000000",
		StartTime: time.Now().AddDate(0, 0, -60),
		Model:     "claude",
	}
	if err := SaveSessionFile(dir, session); err != nil {
		t.Fatalf("SaveSessionFile() error: %v", err)
	}

	// Backdate the file's modification time
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		oldTime := time.Now().AddDate(0, 0, -60)
		_ = os.Chtimes(path, oldTime, oldTime)
	}

	// With maxAgeDays=30, files older than 30 days should be excluded
	sessions := LoadRecentSessions(dir, 30)
	if len(sessions) != 0 {
		t.Errorf("LoadRecentSessions with old files: expected 0, got %d", len(sessions))
	}
}

func TestLoadRecentSessions_InvalidJSON(t *testing.T) {
	dir := t.TempDir()

	// Write a .json file with invalid JSON content — exercises the json.Unmarshal error branch
	badFile := filepath.Join(dir, "2024-01-01-badjson.json")
	if err := os.WriteFile(badFile, []byte("{not valid json"), 0600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	sessions := LoadRecentSessions(dir, 30)
	// Invalid JSON should be silently skipped → empty result
	if len(sessions) != 0 {
		t.Errorf("LoadRecentSessions with invalid JSON: expected 0 sessions, got %d", len(sessions))
	}
}

func TestLoadRecentSessions_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()

	// Create a subdirectory named "subdir.json" — should be skipped because IsDir()
	subdir := filepath.Join(dir, "subdir.json")
	if err := os.MkdirAll(subdir, 0700); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}

	sessions := LoadRecentSessions(dir, 30)
	if len(sessions) != 0 {
		t.Errorf("LoadRecentSessions with dir-only: expected 0 sessions, got %d", len(sessions))
	}
}

func TestLoadRecentSessions_SkipsNonJSON(t *testing.T) {
	dir := t.TempDir()

	// Write a non-JSON file — should be skipped
	if err := os.WriteFile(filepath.Join(dir, "session.txt"), []byte("hello"), 0600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	sessions := LoadRecentSessions(dir, 30)
	if len(sessions) != 0 {
		t.Errorf("LoadRecentSessions with .txt file: expected 0 sessions, got %d", len(sessions))
	}
}

func TestSessionTmpFile_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	sessionID := "abcd1234-0000-0000-0000-000000000000"
	summary := "all tests passing"

	if err := SessionTmpFile(dir, sessionID, summary); err != nil {
		t.Fatalf("SessionTmpFile() error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}

	var found bool
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			found = true

			// Check content
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatalf("ReadFile error: %v", err)
			}
			content := string(data)
			if !strings.Contains(content, summary) {
				t.Errorf("tmp file does not contain summary %q, got:\n%s", summary, content)
			}
			if !strings.Contains(content, sessionID[:8]) {
				t.Errorf("tmp file does not contain session ID prefix, got:\n%s", content)
			}
			break
		}
	}

	if !found {
		t.Error("SessionTmpFile() did not create a .tmp file")
	}
}

func TestSessionTmpFile_CreatesDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nested", "tmp")

	sessionID := "abcd1234-0000-0000-0000-000000000000"
	if err := SessionTmpFile(dir, sessionID, "summary"); err != nil {
		t.Fatalf("SessionTmpFile() with missing dir: %v", err)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("SessionTmpFile() did not create directory")
	}
}
