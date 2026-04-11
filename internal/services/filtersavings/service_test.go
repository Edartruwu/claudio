package filtersavings

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Abraxas-365/claudio/internal/storage"
)

// openTestDB creates a temporary SQLite database for testing.
// The database is automatically cleaned up when the test finishes.
func openTestDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRecord_And_GetStats_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Record three entries with varying savings.
	if err := svc.Record(ctx, "git diff --stat", 1000, 400); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := svc.Record(ctx, "git log --oneline", 2000, 500); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := svc.Record(ctx, "ls -la /tmp", 500, 500); err != nil {
		t.Fatalf("Record: %v", err)
	}

	stats, err := svc.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}

	if stats.RecordCount != 3 {
		t.Errorf("RecordCount: want 3, got %d", stats.RecordCount)
	}
	if stats.TotalBytesIn != 3500 {
		t.Errorf("TotalBytesIn: want 3500, got %d", stats.TotalBytesIn)
	}
	if stats.TotalBytesOut != 1400 {
		t.Errorf("TotalBytesOut: want 1400, got %d", stats.TotalBytesOut)
	}
	if stats.TotalSaved != 2100 {
		t.Errorf("TotalSaved: want 2100, got %d", stats.TotalSaved)
	}
	// 2100 / 3500 * 100 = 60%
	if stats.SavingsPct < 59.9 || stats.SavingsPct > 60.1 {
		t.Errorf("SavingsPct: want ~60%%, got %.1f%%", stats.SavingsPct)
	}
}

func TestGetStats_Empty(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	stats, err := svc.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.RecordCount != 0 || stats.TotalBytesIn != 0 || stats.TotalSaved != 0 {
		t.Errorf("expected zero stats, got %+v", stats)
	}
	if stats.SavingsPct != 0 {
		t.Errorf("SavingsPct: want 0, got %f", stats.SavingsPct)
	}
}

func TestGetTopCommands_OrderBySaved(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// "git log" saves more bytes total than "git diff".
	_ = svc.Record(ctx, "git diff", 1000, 800)  // saved: 200
	_ = svc.Record(ctx, "git log", 3000, 500)   // saved: 2500
	_ = svc.Record(ctx, "cat README", 200, 200)  // saved: 0

	top, err := svc.GetTopCommands(ctx, 10)
	if err != nil {
		t.Fatalf("GetTopCommands: %v", err)
	}
	if len(top) != 3 {
		t.Fatalf("want 3 commands, got %d", len(top))
	}
	if top[0].Command != "git log" {
		t.Errorf("top[0]: want 'git log', got %q", top[0].Command)
	}
	if top[0].Saved != 2500 {
		t.Errorf("top[0].Saved: want 2500, got %d", top[0].Saved)
	}
	if top[1].Command != "git diff" {
		t.Errorf("top[1]: want 'git diff', got %q", top[1].Command)
	}
	if top[1].Saved != 200 {
		t.Errorf("top[1].Saved: want 200, got %d", top[1].Saved)
	}
}

func TestGetTopCommands_Limit(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	_ = svc.Record(ctx, "cmd-a", 1000, 100)
	_ = svc.Record(ctx, "cmd-b", 2000, 100)
	_ = svc.Record(ctx, "cmd-c", 3000, 100)

	top, err := svc.GetTopCommands(ctx, 2)
	if err != nil {
		t.Fatalf("GetTopCommands: %v", err)
	}
	if len(top) != 2 {
		t.Errorf("want 2 commands, got %d", len(top))
	}
}

func TestGetTopCommands_AggregatesSameCommand(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	_ = svc.Record(ctx, "git diff", 1000, 400)
	_ = svc.Record(ctx, "git diff", 2000, 600)

	top, err := svc.GetTopCommands(ctx, 10)
	if err != nil {
		t.Fatalf("GetTopCommands: %v", err)
	}
	if len(top) != 1 {
		t.Fatalf("want 1 aggregated command, got %d", len(top))
	}
	if top[0].BytesIn != 3000 {
		t.Errorf("BytesIn: want 3000, got %d", top[0].BytesIn)
	}
	if top[0].Count != 2 {
		t.Errorf("Count: want 2, got %d", top[0].Count)
	}
}

func TestDiscover_ReturnsOnlyUnfilteredCommands(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Filtered (bytes_in != bytes_out) — should NOT appear.
	_ = svc.Record(ctx, "git diff", 5000, 1000)

	// Unfiltered (bytes_in == bytes_out) — should appear.
	_ = svc.Record(ctx, "cat bigfile.log", 10000, 10000)
	_ = svc.Record(ctx, "cat bigfile.log", 12000, 12000)
	_ = svc.Record(ctx, "ps aux", 500, 500)

	suggestions, err := svc.Discover(ctx, 10)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// Should contain "cat bigfile.log" and "ps aux", not "git diff".
	if len(suggestions) != 2 {
		t.Fatalf("want 2 suggestions, got %d", len(suggestions))
	}

	// "cat bigfile.log" should rank first (larger opportunity: avg ~11000 * 2 = 22000).
	if suggestions[0].Command != "cat bigfile.log" {
		t.Errorf("suggestions[0]: want 'cat bigfile.log', got %q", suggestions[0].Command)
	}
	if suggestions[0].Occurrences != 2 {
		t.Errorf("suggestions[0].Occurrences: want 2, got %d", suggestions[0].Occurrences)
	}
	if suggestions[0].AvgBytesIn != 11000 {
		t.Errorf("suggestions[0].AvgBytesIn: want 11000, got %d", suggestions[0].AvgBytesIn)
	}

	if suggestions[1].Command != "ps aux" {
		t.Errorf("suggestions[1]: want 'ps aux', got %q", suggestions[1].Command)
	}
}

func TestDiscover_Empty(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// All records have savings — discover should return nothing.
	_ = svc.Record(ctx, "git diff", 1000, 200)

	suggestions, err := svc.Discover(ctx, 10)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(suggestions) != 0 {
		t.Errorf("want 0 suggestions, got %d", len(suggestions))
	}
}

func TestNormalizeCommand(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"git diff --stat HEAD~3", "git diff"},
		{"/usr/bin/ls -la /tmp", "ls"},
		{"cat README.md", "cat README.md"},
		{"docker compose up -d", "docker compose"},
		{"  git  log  ", "git log"},
		{"", ""},
		{"single", "single"},
		{"git -C /repo diff", "git"},
	}
	for _, tt := range tests {
		got := normalizeCommand(tt.input)
		if got != tt.want {
			t.Errorf("normalizeCommand(%q): want %q, got %q", tt.input, tt.want, got)
		}
	}
}

