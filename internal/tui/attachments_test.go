package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractFileAttachments_NoMentions(t *testing.T) {
	atts, cleaned := ExtractFileAttachments("hello world", "/tmp")
	if len(atts) != 0 {
		t.Errorf("expected 0 attachments, got %d", len(atts))
	}
	if cleaned != "hello world" {
		t.Errorf("expected unchanged text, got %q", cleaned)
	}
}

func TestExtractFileAttachments_SingleFile(t *testing.T) {
	// Create a temp file
	dir := t.TempDir()
	f := filepath.Join(dir, "test.go")
	os.WriteFile(f, []byte("package main\n\nfunc main() {}\n"), 0644)

	text := "please review @test.go"
	atts, cleaned := ExtractFileAttachments(text, dir)

	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	if atts[0].DisplayPath != "test.go" {
		t.Errorf("expected display path 'test.go', got %q", atts[0].DisplayPath)
	}
	if !strings.Contains(atts[0].Content, "package main") {
		t.Errorf("expected file content, got %q", atts[0].Content)
	}
	if cleaned != "please review test.go" {
		t.Errorf("expected cleaned text 'please review test.go', got %q", cleaned)
	}
}

func TestExtractFileAttachments_LineRange(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "lines.txt")
	content := "line1\nline2\nline3\nline4\nline5\n"
	os.WriteFile(f, []byte(content), 0644)

	text := "check @lines.txt#L2-4"
	atts, _ := ExtractFileAttachments(text, dir)

	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	if atts[0].LineStart != 2 || atts[0].LineEnd != 4 {
		t.Errorf("expected lines 2-4, got %d-%d", atts[0].LineStart, atts[0].LineEnd)
	}
	if !strings.Contains(atts[0].Content, "line2") {
		t.Errorf("expected line2 in content, got %q", atts[0].Content)
	}
	if strings.Contains(atts[0].Content, "line1") {
		t.Errorf("should not contain line1, got %q", atts[0].Content)
	}
	if strings.Contains(atts[0].Content, "line5") {
		t.Errorf("should not contain line5, got %q", atts[0].Content)
	}
}

func TestExtractFileAttachments_SingleLine(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	os.WriteFile(f, []byte("a\nb\nc\nd\n"), 0644)

	text := "check @file.txt#L3"
	atts, _ := ExtractFileAttachments(text, dir)

	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	if atts[0].Content != "c" {
		t.Errorf("expected 'c', got %q", atts[0].Content)
	}
}

func TestExtractFileAttachments_Directory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte(""), 0644)
	os.Mkdir(filepath.Join(dir, "sub"), 0755)

	// Create a subdir to reference
	subdir := filepath.Join(dir, "mydir")
	os.Mkdir(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "x.txt"), []byte(""), 0644)

	text := "list @mydir"
	atts, _ := ExtractFileAttachments(text, dir)

	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	if !atts[0].IsDir {
		t.Error("expected IsDir=true")
	}
	if !strings.Contains(atts[0].Content, "x.txt") {
		t.Errorf("expected x.txt in dir listing, got %q", atts[0].Content)
	}
}

func TestExtractFileAttachments_RelativePath(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0755)
	os.WriteFile(filepath.Join(dir, "root.txt"), []byte("root content"), 0644)

	text := "check @../root.txt"
	atts, _ := ExtractFileAttachments(text, sub)

	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	if !strings.Contains(atts[0].Content, "root content") {
		t.Errorf("expected root content, got %q", atts[0].Content)
	}
}

func TestExtractFileAttachments_QuotedPath(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "my file.txt")
	os.WriteFile(f, []byte("quoted content"), 0644)

	text := `check @"my file.txt" please`
	atts, cleaned := ExtractFileAttachments(text, dir)

	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	if !strings.Contains(atts[0].Content, "quoted content") {
		t.Errorf("expected quoted content, got %q", atts[0].Content)
	}
	if !strings.Contains(cleaned, "my file.txt") {
		t.Errorf("expected cleaned to contain display path, got %q", cleaned)
	}
}

func TestExtractFileAttachments_NonexistentFile(t *testing.T) {
	text := "check @nonexistent.txt and @param stuff"
	atts, cleaned := ExtractFileAttachments(text, "/tmp")

	// nonexistent files should be left as-is (might be words like @param)
	if len(atts) != 0 {
		t.Errorf("expected 0 attachments for nonexistent files, got %d", len(atts))
	}
	if cleaned != text {
		t.Errorf("expected unchanged text, got %q", cleaned)
	}
}

func TestExtractFileAttachments_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b"), 0644)

	text := "review @a.go and @b.go"
	atts, cleaned := ExtractFileAttachments(text, dir)

	if len(atts) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(atts))
	}
	if cleaned != "review a.go and b.go" {
		t.Errorf("expected 'review a.go and b.go', got %q", cleaned)
	}
}

func TestBuildContentBlocks(t *testing.T) {
	atts := []FileAttachment{
		{DisplayPath: "main.go", Content: "package main", IsDir: false},
	}

	blocks := BuildContentBlocks("review this", atts, nil)

	// Should be: [file content block, user text block]
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Type != "text" {
		t.Errorf("expected text block, got %s", blocks[0].Type)
	}
	if !strings.Contains(blocks[0].Text, "main.go") {
		t.Errorf("expected file content block to mention main.go")
	}
	if !strings.Contains(blocks[0].Text, "package main") {
		t.Errorf("expected file content in block")
	}
	if blocks[1].Text != "review this" {
		t.Errorf("expected user text block, got %q", blocks[1].Text)
	}
}
