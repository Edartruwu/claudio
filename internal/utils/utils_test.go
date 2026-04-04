package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- strings.go ---

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "he..."},
		{"hi", 2, "hi"},
		{"hello", 3, "hel"},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := Truncate(tt.input, tt.max)
		if got != tt.expected {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.expected)
		}
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello\nworld", "hello"},
		{"single", "single"},
		{"", ""},
		{"first\nsecond\nthird", "first"},
	}
	for _, tt := range tests {
		got := FirstLine(tt.input)
		if got != tt.expected {
			t.Errorf("FirstLine(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestIndentLines(t *testing.T) {
	got := IndentLines("a\nb\nc", "  ")
	want := "  a\n  b\n  c"
	if got != want {
		t.Errorf("IndentLines = %q, want %q", got, want)
	}
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"a", 1},
		{"a\nb", 2},
		{"a\nb\nc", 3},
	}
	for _, tt := range tests {
		got := CountLines(tt.input)
		if got != tt.want {
			t.Errorf("CountLines(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestContainsAny(t *testing.T) {
	if !ContainsAny("hello world", "foo", "world") {
		t.Error("expected true for matching substring")
	}
	if ContainsAny("hello world", "foo", "bar") {
		t.Error("expected false for no match")
	}
}

func TestSanitizeForLog(t *testing.T) {
	got := SanitizeForLog("token is sk-abc123xyz")
	if strings.Contains(got, "abc123xyz") {
		t.Errorf("SanitizeForLog should mask sk- secrets, got %q", got)
	}
	if !strings.Contains(got, "sk-****") {
		t.Errorf("SanitizeForLog should contain masked sk-****, got %q", got)
	}
}

// --- format.go ---

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0B"},
		{500, "500B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1048576, "1.0MB"},
		{1073741824, "1.0GB"},
	}
	for _, tt := range tests {
		got := FormatFileSize(tt.input)
		if got != tt.want {
			t.Errorf("FormatFileSize(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{500 * time.Millisecond, "500ms"},
		{1500 * time.Millisecond, "1.5s"},
		{90 * time.Second, "1m30s"},
		{3661 * time.Second, "1h1m"},
	}
	for _, tt := range tests {
		got := FormatDuration(tt.input)
		if got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{500, "500"},
		{1500, "1.5K"},
		{1500000, "1.5M"},
	}
	for _, tt := range tests {
		got := FormatTokenCount(tt.input)
		if got != tt.want {
			t.Errorf("FormatTokenCount(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0.001, "$0.0010"},
		{1.5, "$1.50"},
		{0.0, "$0.0000"},
	}
	for _, tt := range tests {
		got := FormatCost(tt.input)
		if got != tt.want {
			t.Errorf("FormatCost(%f) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatPercent(t *testing.T) {
	got := FormatPercent(756, 1000)
	if got != "76%" {
		t.Errorf("FormatPercent(756, 1000) = %q, want %q", got, "76%")
	}
}

func TestPluralS(t *testing.T) {
	if PluralS(1) != "" {
		t.Error("PluralS(1) should be empty")
	}
	if PluralS(2) != "s" {
		t.Error("PluralS(2) should be 's'")
	}
	if PluralS(0) != "s" {
		t.Error("PluralS(0) should be 's'")
	}
}

func TestTreeify(t *testing.T) {
	items := []string{"a", "b", "c"}
	got := Treeify(items)
	if !strings.Contains(got, "a") || !strings.Contains(got, "c") {
		t.Errorf("Treeify should contain all items, got %q", got)
	}
}

func TestFormatTable(t *testing.T) {
	headers := []string{"Name", "Age"}
	rows := [][]string{{"Alice", "30"}, {"Bob", "25"}}
	got := FormatTable(headers, rows)
	if !strings.Contains(got, "Alice") || !strings.Contains(got, "Name") {
		t.Errorf("FormatTable missing expected content, got %q", got)
	}
}

// --- hash.go ---

func TestSHA256String(t *testing.T) {
	h1 := SHA256String("hello")
	h2 := SHA256String("hello")
	h3 := SHA256String("world")

	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
	if len(h1) != 64 {
		t.Errorf("SHA256 hex should be 64 chars, got %d", len(h1))
	}
}

func TestShortHash(t *testing.T) {
	got := ShortHash("hello", 8)
	if len(got) != 8 {
		t.Errorf("ShortHash length = %d, want 8", len(got))
	}
}

func TestContentHash(t *testing.T) {
	h := ContentHash([]byte("test data"))
	if !strings.HasPrefix(h, "sha256:") {
		t.Errorf("ContentHash should start with 'sha256:', got %q", h)
	}
}

func TestSHA256File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	h, err := SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}
	if h != SHA256String("hello") {
		t.Error("SHA256File should match SHA256String for same content")
	}

	_, err = SHA256File(filepath.Join(dir, "nonexistent"))
	if err == nil {
		t.Error("SHA256File should fail for nonexistent file")
	}
}

// --- circular_buffer.go ---

func TestCircularBuffer(t *testing.T) {
	b := NewCircularBuffer(3)

	if b.Len() != 0 {
		t.Error("new buffer should be empty")
	}

	b.Push("a")
	b.Push("b")
	b.Push("c")
	if b.Len() != 3 {
		t.Errorf("Len = %d, want 3", b.Len())
	}

	got := b.All()
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("All() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("All()[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// Overflow: oldest should be dropped
	b.Push("d")
	got = b.All()
	want = []string{"b", "c", "d"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("after overflow All()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCircularBufferLast(t *testing.T) {
	b := NewCircularBuffer(5)
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		b.Push(s)
	}

	got := b.Last(2)
	if len(got) != 2 || got[0] != "d" || got[1] != "e" {
		t.Errorf("Last(2) = %v, want [d e]", got)
	}

	got = b.Last(10) // more than available
	if len(got) != 5 {
		t.Errorf("Last(10) len = %d, want 5", len(got))
	}
}

func TestCircularBufferClear(t *testing.T) {
	b := NewCircularBuffer(3)
	b.Push("a")
	b.Push("b")
	b.Clear()
	if b.Len() != 0 {
		t.Error("Clear should reset length to 0")
	}
	if len(b.All()) != 0 {
		t.Error("Clear should make All() return empty")
	}
}

// --- frontmatter.go ---

func TestParseFrontmatter(t *testing.T) {
	input := "---\ntitle: Hello\ntags: a, b\n---\nBody content here"
	fm, body := ParseFrontmatter(input)

	if fm.Get("title") != "Hello" {
		t.Errorf("Get('title') = %q, want 'Hello'", fm.Get("title"))
	}
	if fm.Get("tags") != "a, b" {
		t.Errorf("Get('tags') = %q, want 'a, b'", fm.Get("tags"))
	}
	if strings.TrimSpace(body) != "Body content here" {
		t.Errorf("body = %q, want 'Body content here'", body)
	}
}

func TestParseFrontmatterNoFrontmatter(t *testing.T) {
	input := "Just plain content"
	fm, body := ParseFrontmatter(input)
	if fm.Get("anything") != "" {
		t.Error("no frontmatter should return empty values")
	}
	if body != input {
		t.Errorf("body should be full input without frontmatter")
	}
}

// --- tokens.go ---

func TestEstimateTokens(t *testing.T) {
	tokens := EstimateTokens("hello world this is a test")
	if tokens <= 0 {
		t.Error("EstimateTokens should return positive value for non-empty input")
	}
	if EstimateTokens("") != 0 {
		t.Error("EstimateTokens should return 0 for empty input")
	}
}

// --- cron.go ---

func TestParseCron(t *testing.T) {
	tests := []struct {
		expr    string
		wantErr bool
	}{
		{"* * * * *", false},
		{"0 9 * * *", false},
		{"*/5 * * * *", false},
		{"0 9 * * 1-5", false},
		{"0 9,18 * * *", false},
		{"bad", true},
		{"* * *", true},
	}
	for _, tt := range tests {
		_, err := ParseCron(tt.expr)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseCron(%q) err = %v, wantErr = %v", tt.expr, err, tt.wantErr)
		}
	}
}

func TestCronMatches(t *testing.T) {
	// "0 9 * * *" = every day at 9:00
	expr, _ := ParseCron("0 9 * * *")
	match := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	noMatch := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	if !expr.Matches(match) {
		t.Error("should match 9:00")
	}
	if expr.Matches(noMatch) {
		t.Error("should not match 10:00")
	}
}

func TestCronNext(t *testing.T) {
	expr, _ := ParseCron("0 9 * * *")
	from := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	next := expr.Next(from)

	if next.Hour() != 9 || next.Minute() != 0 {
		t.Errorf("Next should be 9:00, got %v", next)
	}
	if !next.After(from) {
		t.Error("Next should be after from")
	}
}

func TestHumanToCron(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"hourly", "0 * * * *", false},
		{"daily", "0 9 * * *", false},
		{"5m", "*/5 * * * *", false},
		{"2h", "0 */2 * * *", false},
		{"invalid", "", true},
	}
	for _, tt := range tests {
		got, err := HumanToCron(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("HumanToCron(%q) err = %v, wantErr = %v", tt.input, err, tt.wantErr)
		}
		if got != tt.want {
			t.Errorf("HumanToCron(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- files.go ---

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.txt")
	os.WriteFile(path, []byte("hi"), 0644)

	if !FileExists(path) {
		t.Error("FileExists should return true for existing file")
	}
	if FileExists(filepath.Join(dir, "nope.txt")) {
		t.Error("FileExists should return false for missing file")
	}
}

func TestDirExists(t *testing.T) {
	dir := t.TempDir()
	if !DirExists(dir) {
		t.Error("DirExists should return true for existing dir")
	}
	if DirExists(filepath.Join(dir, "nope")) {
		t.Error("DirExists should return false for missing dir")
	}
}

func TestFindProjectRoot(t *testing.T) {
	// Create a nested dir structure with a .git marker
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".git"), 0755)
	nested := filepath.Join(root, "a", "b", "c")
	os.MkdirAll(nested, 0755)

	got := FindProjectRoot(nested)
	if got != root {
		t.Errorf("FindProjectRoot = %q, want %q", got, root)
	}
}

func TestCollectMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("md"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("txt"), 0644)
	os.WriteFile(filepath.Join(dir, "c.md"), []byte("md2"), 0644)

	files := CollectMarkdownFiles(dir)
	count := 0
	for _, f := range files {
		if strings.HasSuffix(f, ".md") {
			count++
		}
	}
	if count != 2 {
		t.Errorf("CollectMarkdownFiles found %d .md files, want 2", count)
	}
}
