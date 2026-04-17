// Package tasks provides background task execution including cron scheduling.
package tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// CronEntry defines a scheduled recurring task.
type CronEntry struct {
	ID        string    `json:"id"`
	Schedule  string    `json:"schedule"` // simplified: "@every 1h", "@daily", or "HH:MM"
	Prompt    string    `json:"prompt"`   // what to execute
	Agent     string    `json:"agent,omitempty"`
	Type      string    `json:"type,omitempty"`       // "inline" (default) or "background"
	SessionID string    `json:"session_id,omitempty"` // owning session for inject/store
	LastRun   time.Time `json:"last_run,omitempty"`
	NextRun   time.Time `json:"next_run"`
	Enabled   bool      `json:"enabled"`
}

// CronStore manages persisted cron entries.
type CronStore struct {
	path    string
	entries []CronEntry
	nextID  int
}

// NewCronStore creates a cron store at the given path.
func NewCronStore(path string) *CronStore {
	return &CronStore{path: path}
}

// Load reads cron entries from disk.
func (cs *CronStore) Load() error {
	data, err := os.ReadFile(cs.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := json.Unmarshal(data, &cs.entries); err != nil {
		return err
	}
	// Find max ID for incrementing
	for _, e := range cs.entries {
		if id := extractIDNum(e.ID); id >= cs.nextID {
			cs.nextID = id + 1
		}
	}
	return nil
}

// Save writes cron entries to disk.
func (cs *CronStore) Save() error {
	data, err := json.MarshalIndent(cs.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cs.path, data, 0644)
}

// Add creates a new cron entry with the given schedule and prompt.
func (cs *CronStore) Add(schedule, prompt, agent, entryType, sessionID string) (*CronEntry, error) {
	nextRun, err := computeNextRun(schedule, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid schedule %q: %w", schedule, err)
	}

	if entryType == "" {
		entryType = "inline"
	}

	cs.nextID++
	entry := CronEntry{
		ID:        fmt.Sprintf("cron-%d", cs.nextID),
		Schedule:  schedule,
		Prompt:    prompt,
		Agent:     agent,
		Type:      entryType,
		SessionID: sessionID,
		NextRun:   nextRun,
		Enabled:   true,
	}

	cs.entries = append(cs.entries, entry)
	return &entry, cs.Save()
}

// UpdatePrompt replaces the Prompt field of a cron entry in-memory and saves.
func (cs *CronStore) UpdatePrompt(id, prompt string) error {
	for i := range cs.entries {
		if cs.entries[i].ID == id {
			cs.entries[i].Prompt = prompt
			return cs.Save()
		}
	}
	return fmt.Errorf("cron entry %q not found", id)
}

// Remove deletes a cron entry by ID.
func (cs *CronStore) Remove(id string) error {
	for i, e := range cs.entries {
		if e.ID == id {
			cs.entries = append(cs.entries[:i], cs.entries[i+1:]...)
			return cs.Save()
		}
	}
	return fmt.Errorf("cron entry %q not found", id)
}

// All returns all cron entries.
func (cs *CronStore) All() []CronEntry {
	result := make([]CronEntry, len(cs.entries))
	copy(result, cs.entries)
	return result
}

// Due returns all enabled entries whose NextRun has passed.
func (cs *CronStore) Due() []CronEntry {
	now := time.Now()
	var due []CronEntry
	for _, e := range cs.entries {
		if e.Enabled && now.After(e.NextRun) {
			due = append(due, e)
		}
	}
	return due
}

// MarkRun records that a cron entry was executed and computes its next run time.
func (cs *CronStore) MarkRun(id string) error {
	for i := range cs.entries {
		if cs.entries[i].ID == id {
			cs.entries[i].LastRun = time.Now()
			next, err := computeNextRun(cs.entries[i].Schedule, time.Now())
			if err != nil {
				cs.entries[i].Enabled = false // disable on bad schedule
			} else {
				cs.entries[i].NextRun = next
			}
			return cs.Save()
		}
	}
	return fmt.Errorf("cron entry %q not found", id)
}

// FormatList returns a human-readable list of cron entries.
func (cs *CronStore) FormatList() string {
	if len(cs.entries) == 0 {
		return "No scheduled tasks."
	}
	var lines []string
	lines = append(lines, "Scheduled tasks:")
	for _, e := range cs.entries {
		status := "enabled"
		if !e.Enabled {
			status = "disabled"
		}
		cronType := e.Type
		if cronType == "" {
			cronType = "inline"
		}
		line := fmt.Sprintf("  %s  %s  [%s]  type:%s", e.ID, e.Schedule, status, cronType)
		if e.SessionID != "" {
			line += fmt.Sprintf("  session:%s", e.SessionID)
		}
		line += fmt.Sprintf("  %s", truncateStr(e.Prompt, 60))
		if !e.LastRun.IsZero() {
			line += fmt.Sprintf("  (last: %s)", e.LastRun.Format("2006-01-02 15:04"))
		}
		line += fmt.Sprintf("  (next: %s)", e.NextRun.Format("2006-01-02 15:04"))
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// computeNextRun parses a simplified schedule string and returns the next run time.
// Supports: "@every 1h", "@every 30m", "@daily", "@hourly", "HH:MM"
func computeNextRun(schedule string, from time.Time) (time.Time, error) {
	schedule = strings.TrimSpace(strings.ToLower(schedule))

	switch {
	case schedule == "@daily":
		next := from.Add(24 * time.Hour)
		return time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, next.Location()), nil

	case schedule == "@hourly":
		return from.Add(time.Hour), nil

	case strings.HasPrefix(schedule, "@every "):
		durStr := strings.TrimPrefix(schedule, "@every ")
		dur, err := time.ParseDuration(durStr)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid duration %q: %w", durStr, err)
		}
		return from.Add(dur), nil

	default:
		// Try HH:MM format
		parts := strings.SplitN(schedule, ":", 2)
		if len(parts) == 2 {
			h, err1 := strconv.Atoi(parts[0])
			m, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil && h >= 0 && h < 24 && m >= 0 && m < 60 {
				next := time.Date(from.Year(), from.Month(), from.Day(), h, m, 0, 0, from.Location())
				if next.Before(from) {
					next = next.Add(24 * time.Hour)
				}
				return next, nil
			}
		}
		return time.Time{}, fmt.Errorf("unsupported schedule format: %s", schedule)
	}
}

func extractIDNum(id string) int {
	parts := strings.SplitN(id, "-", 2)
	if len(parts) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(parts[1])
	return n
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
