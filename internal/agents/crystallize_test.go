package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Abraxas-365/claudio/internal/services/memory"
)

// ---------------------------------------------------------------------------
// sanitizeAgentName (tested more broadly in agents_test.go; a few more here)
// ---------------------------------------------------------------------------

func TestSanitizeAgentName_HyphensAndUnderscores(t *testing.T) {
	got := sanitizeAgentName("hello-world_agent")
	if got != "hello-world_agent" {
		t.Errorf("got %q", got)
	}
}

func TestSanitizeAgentName_Digits(t *testing.T) {
	got := sanitizeAgentName("Agent42")
	if got != "agent42" {
		t.Errorf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// buildAgentPrompt
// ---------------------------------------------------------------------------

func TestBuildAgentPrompt_ContainsName(t *testing.T) {
	got := buildAgentPrompt("MyBot", "", "", nil)
	if !strings.Contains(got, "MyBot") {
		t.Errorf("prompt should contain agent name, got:\n%s", got)
	}
}

func TestBuildAgentPrompt_ContainsDescription(t *testing.T) {
	got := buildAgentPrompt("bot", "does cool stuff", "", nil)
	if !strings.Contains(got, "does cool stuff") {
		t.Errorf("prompt should contain description, got:\n%s", got)
	}
}

func TestBuildAgentPrompt_ContainsSummary(t *testing.T) {
	got := buildAgentPrompt("bot", "desc", "this is the background", nil)
	if !strings.Contains(got, "this is the background") {
		t.Errorf("prompt should contain summary, got:\n%s", got)
	}
}

func TestBuildAgentPrompt_NoDescriptionSkipsSection(t *testing.T) {
	got := buildAgentPrompt("bot", "", "", nil)
	if strings.Contains(got, "## Purpose") {
		t.Error("prompt should not contain Purpose section when description is empty")
	}
}

func TestBuildAgentPrompt_NoSummarySkipsSection(t *testing.T) {
	got := buildAgentPrompt("bot", "desc", "", nil)
	if strings.Contains(got, "## Background") {
		t.Error("prompt should not contain Background section when summary is empty")
	}
}

func TestBuildAgentPrompt_WithMemories(t *testing.T) {
	memories := []*memory.Entry{
		{Name: "mem1", Type: memory.TypeUser, Content: "content one"},
		{Name: "mem2", Type: memory.TypeProject, Content: "content two"},
	}
	got := buildAgentPrompt("bot", "desc", "summary", memories)
	if !strings.Contains(got, "mem1") {
		t.Error("prompt should contain memory name 'mem1'")
	}
	if !strings.Contains(got, "content one") {
		t.Error("prompt should contain memory content")
	}
	if !strings.Contains(got, "mem2") {
		t.Error("prompt should contain memory name 'mem2'")
	}
	if !strings.Contains(got, "## Key Knowledge") {
		t.Error("prompt should contain Key Knowledge section")
	}
}

func TestBuildAgentPrompt_NoMemories_NoKeyKnowledgeSection(t *testing.T) {
	got := buildAgentPrompt("bot", "desc", "summary", nil)
	if strings.Contains(got, "## Key Knowledge") {
		t.Error("prompt should not contain Key Knowledge section when no memories given")
	}
}

func TestBuildAgentPrompt_LargeMemories_Truncated(t *testing.T) {
	// Build enough memories to exceed the 8000-char threshold.
	var memories []*memory.Entry
	for i := 0; i < 20; i++ {
		memories = append(memories, &memory.Entry{
			Name:    "bigmem",
			Type:    memory.TypeUser,
			Content: strings.Repeat("x", 500),
		})
	}
	got := buildAgentPrompt("bot", "desc", "summary", memories)
	if !strings.Contains(got, "additional knowledge available") {
		t.Error("prompt should mention truncation when memories exceed limit")
	}
}

func TestBuildAgentPrompt_ContainsGuidelines(t *testing.T) {
	got := buildAgentPrompt("bot", "", "", nil)
	if !strings.Contains(got, "## Guidelines") {
		t.Error("prompt should always contain Guidelines section")
	}
}

// ---------------------------------------------------------------------------
// CrystallizeSession
// ---------------------------------------------------------------------------

func TestCrystallizeSession_ReturnsAgentDefinition(t *testing.T) {
	dir := t.TempDir()
	def, err := CrystallizeSession(dir, "Test Agent", "a test", "sess1", "proj1", "summary", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def == nil {
		t.Fatal("expected non-nil AgentDefinition")
	}
}

func TestCrystallizeSession_ErrorOnEmptyName(t *testing.T) {
	dir := t.TempDir()
	_, err := CrystallizeSession(dir, "", "desc", "sess", "proj", "sum", nil)
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestCrystallizeSession_TypeIsSanitizedName(t *testing.T) {
	dir := t.TempDir()
	def, err := CrystallizeSession(dir, "My Cool Agent", "desc", "", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Type != "my-cool-agent" {
		t.Errorf("expected type %q, got %q", "my-cool-agent", def.Type)
	}
}

func TestCrystallizeSession_WritesAgentFile(t *testing.T) {
	dir := t.TempDir()
	_, err := CrystallizeSession(dir, "file-test", "desc", "", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	agentFile := filepath.Join(dir, "file-test.md")
	if _, err := os.Stat(agentFile); os.IsNotExist(err) {
		t.Error("expected agent .md file to be created")
	}
}

func TestCrystallizeSession_AgentFileContainsFrontmatter(t *testing.T) {
	dir := t.TempDir()
	_, err := CrystallizeSession(dir, "fm-test", "my description", "sess99", "projX", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "fm-test.md"))
	if err != nil {
		t.Fatalf("reading agent file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "my description") {
		t.Error("agent file should contain description in frontmatter")
	}
	if !strings.Contains(content, "sess99") {
		t.Error("agent file should contain sourceSession")
	}
	if !strings.Contains(content, "projX") {
		t.Error("agent file should contain sourceProject")
	}
}

func TestCrystallizeSession_CreatesMemoryDir(t *testing.T) {
	dir := t.TempDir()
	def, err := CrystallizeSession(dir, "mem-test", "desc", "", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.MemoryDir == "" {
		t.Error("MemoryDir should not be empty")
	}
	if info, err := os.Stat(def.MemoryDir); err != nil || !info.IsDir() {
		t.Errorf("memory dir %q should exist as a directory", def.MemoryDir)
	}
}

func TestCrystallizeSession_ToolsAreWildcard(t *testing.T) {
	dir := t.TempDir()
	def, err := CrystallizeSession(dir, "tools-test", "desc", "", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(def.Tools) != 1 || def.Tools[0] != "*" {
		t.Errorf("expected Tools=[*], got %v", def.Tools)
	}
}

func TestCrystallizeSession_WhenToUseIsDescription(t *testing.T) {
	dir := t.TempDir()
	def, err := CrystallizeSession(dir, "wtu-test", "my when-to-use", "", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.WhenToUse != "my when-to-use" {
		t.Errorf("WhenToUse should equal description, got %q", def.WhenToUse)
	}
}

func TestCrystallizeSession_SourceSessionAndProject(t *testing.T) {
	dir := t.TempDir()
	def, err := CrystallizeSession(dir, "src-test", "desc", "session-42", "project-7", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.SourceSession != "session-42" {
		t.Errorf("expected SourceSession=session-42, got %q", def.SourceSession)
	}
	if def.SourceProject != "project-7" {
		t.Errorf("expected SourceProject=project-7, got %q", def.SourceProject)
	}
}

func TestCrystallizeSession_WithMemories_SavesMemoryFiles(t *testing.T) {
	dir := t.TempDir()
	memories := []*memory.Entry{
		{Name: "important-fact", Type: memory.TypeUser, Content: "remember this"},
	}
	def, err := CrystallizeSession(dir, "memfile-test", "desc", "", "", "", memories)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The memory store saves files as sanitized-name.md
	memFile := filepath.Join(def.MemoryDir, "important-fact.md")
	if _, err := os.Stat(memFile); os.IsNotExist(err) {
		t.Errorf("expected memory file %q to be created", memFile)
	}
}

func TestCrystallizeSession_SystemPromptContainsName(t *testing.T) {
	dir := t.TempDir()
	def, err := CrystallizeSession(dir, "Name Check", "desc", "", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(def.SystemPrompt, "Name Check") {
		t.Error("SystemPrompt should contain the agent name")
	}
}

func TestCrystallizeSession_AgentFileNoSessionIfEmpty(t *testing.T) {
	dir := t.TempDir()
	_, err := CrystallizeSession(dir, "no-session", "desc", "", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "no-session.md"))
	if err != nil {
		t.Fatalf("reading agent file: %v", err)
	}
	if strings.Contains(string(data), "sourceSession") {
		t.Error("agent file should not contain sourceSession when sessionID is empty")
	}
}
