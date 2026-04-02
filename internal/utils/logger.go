package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Logger writes structured log entries to a file in the logs directory.
type Logger struct {
	logDir string
	file   *os.File
}

// NewLogger creates a logger that writes to the given directory.
// Creates a daily log file (e.g., 2026-04-01.log).
func NewLogger(logDir string) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, err
	}

	filename := time.Now().Format("2006-01-02") + ".log"
	path := filepath.Join(logDir, filename)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}

	return &Logger{logDir: logDir, file: f}, nil
}

// Log writes a timestamped message to the log file.
func (l *Logger) Log(level, message string) {
	if l == nil || l.file == nil {
		return
	}
	ts := time.Now().Format("2006-01-02T15:04:05.000Z07:00")
	fmt.Fprintf(l.file, "[%s] %s: %s\n", ts, level, message)
}

// Info logs an informational message.
func (l *Logger) Info(msg string) { l.Log("INFO", msg) }

// Warn logs a warning message.
func (l *Logger) Warn(msg string) { l.Log("WARN", msg) }

// Error logs an error message.
func (l *Logger) Error(msg string) { l.Log("ERROR", msg) }

// Debug logs a debug message.
func (l *Logger) Debug(msg string) { l.Log("DEBUG", msg) }

// Close flushes and closes the log file.
func (l *Logger) Close() {
	if l != nil && l.file != nil {
		l.file.Close()
	}
}

// CleanOldLogs removes log files older than the given number of days.
func CleanOldLogs(logDir string, maxAgeDays int) int {
	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)
	removed := 0

	entries, err := os.ReadDir(logDir)
	if err != nil {
		return 0
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(logDir, entry.Name()))
			removed++
		}
	}

	return removed
}
