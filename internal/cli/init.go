package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/config"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Claudio in the current project",
	Long: `AI-powered project initialization. Claudio explores your codebase,
detects languages/frameworks/tools, and generates tailored configuration files.
You review and approve each proposal before it's written.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		return runInit(cwd)
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(projectDir string) error {
	claudioDir := filepath.Join(projectDir, ".claudio")
	reader := bufio.NewReader(os.Stdin)

	// Check if already initialized
	if _, err := os.Stat(claudioDir); err == nil {
		fmt.Println("Project already has .claudio/ directory.")
		fmt.Print("Re-initialize? (y/n): ")
		answer, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	fmt.Println()
	fmt.Println("  Initializing Claudio project...")
	fmt.Println()

	// ── Phase 1: Detect project ──────────────────────────────────
	fmt.Println("  Phase 1: Detecting project characteristics...")
	info := DetectProject(projectDir)
	printDetection(info)

	// ── Phase 2: Gather codebase context for AI ──────────────────
	fmt.Println("\n  Phase 2: Exploring codebase...")
	codebaseContext := gatherCodebaseContext(projectDir, info)
	fmt.Printf("  Gathered %d bytes of project context\n", len(codebaseContext))

	// ── Phase 3: Create directory structure ──────────────────────
	dirs := []string{
		claudioDir,
		filepath.Join(claudioDir, "rules"),
		filepath.Join(claudioDir, "skills"),
		filepath.Join(claudioDir, "agents"),
		filepath.Join(claudioDir, "memory"),
	}
	for _, dir := range dirs {
		os.MkdirAll(dir, 0755)
	}

	// ── Phase 4: AI generates proposals ──────────────────────────
	hasAI := appInstance != nil && appInstance.Auth.IsLoggedIn()

	if hasAI {
		fmt.Println("\n  Phase 3: AI is analyzing your project...")
		if err := runAIInit(projectDir, claudioDir, info, codebaseContext, reader); err != nil {
			fmt.Fprintf(os.Stderr, "\n  AI init failed: %v\n", err)
			fmt.Println("  Falling back to template-based init...")
			runTemplateInit(projectDir, claudioDir, info, reader)
		}
	} else {
		fmt.Println()
		fmt.Println("  No auth configured — using template-based init.")
		fmt.Println("  Tip: Run 'claudio auth login' first for AI-powered init.")
		runTemplateInit(projectDir, claudioDir, info, reader)
	}

	// ── Always create: .gitignore ────────────────────────────────
	gitignorePath := filepath.Join(claudioDir, ".gitignore")
	if !fileExistsAt(gitignorePath) {
		os.WriteFile(gitignorePath, []byte("# Local settings (not shared with team)\nsettings.local.json\nworktrees/\n"), 0644)
	}

	// ── Phase 5: Offer built-in skills ───────────────────────────
	fmt.Print("\n  Install review skills (review, security-review)? (Y/n): ")
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "" || answer == "y" || answer == "yes" {
		installBuiltinSkill(filepath.Join(claudioDir, "skills", "review.md"), reviewSkillContent)
		installBuiltinSkill(filepath.Join(claudioDir, "skills", "security-review.md"), securityReviewSkillContent)
		fmt.Println("  Installed review and security-review skills")
	}

	fmt.Println("\n  Done! Your project is ready for Claudio.")
	fmt.Println("\n  Next steps:")
	fmt.Println("    1. Review CLAUDIO.md and adjust as needed")
	fmt.Println("    2. Run: claudio")

	return nil
}

// runAIInit uses the API to generate project configuration.
func runAIInit(projectDir, claudioDir string, info *ProjectInfo, codebaseCtx string, reader *bufio.Reader) error {
	ctx := context.Background()

	systemPrompt := `You are helping initialize a coding assistant (Claudio) for a project.
You will receive information about the project's codebase and must generate configuration files.

IMPORTANT RULES:
- Only include information that would help an AI avoid mistakes when working on this project
- Do NOT include generic advice — only project-specific, actionable instructions
- CLAUDIO.md must be ultra-lean (max 20 lines): ONLY build/test/lint commands + hard constraints (3-5 lines) + @.claudio/rules/project.md reference. NO tech stack overview, NO architecture patterns, NO naming conventions — those go in rules/project.md
- For long reference material, use @path/to/file.md imports instead of inlining
- Do NOT include information that can be derived by reading the code (like "this is a Go project")
- DO include non-obvious things: "tests must run against a real database", "never import from internal/legacy"

You will be asked to generate files one at a time. For each, output ONLY the file content — no explanation, no markdown code fences, no preamble.`

	// ── Generate CLAUDIO.md ──────────────────────────────────────
	claudioMDPath := filepath.Join(projectDir, "CLAUDIO.md")
	if !fileExistsAt(claudioMDPath) {
		fmt.Println("\n  Generating CLAUDIO.md...")

		prompt := fmt.Sprintf(`Based on the following project analysis, generate a CLAUDIO.md file.
This file is loaded into the AI's system prompt on EVERY turn — keep it ultra-lean (max 20 lines total).

STRICT FORMAT — output exactly this structure, nothing more:

# <ProjectName>

## Build & Test
- `+"`"+`<build command>`+"`"+`
- `+"`"+`<test command>`+"`"+`
- `+"`"+`<lint command if applicable>`+"`"+`

## Hard Constraints
- <non-obvious constraint 1 — things that MUST never be violated>
- <non-obvious constraint 2>
- <add up to 5 total, only if they exist>

## Project Rules
@.claudio/rules/project.md

RULES:
- NO tech stack overview
- NO architecture section
- NO naming conventions
- NO patterns or examples
- ONLY exact commands and hard constraints that are non-obvious
- If you have nothing non-obvious to say in Hard Constraints, output only "- None" there
- Max 20 lines total

PROJECT INFO:
- Directory: %s
- Languages: %s
- Frameworks: %s
- Build system: %s
- Test command: %s
- Has CI: %v
- Has Docker: %v

CODEBASE CONTEXT:
%s

Generate the CLAUDIO.md content now. Start with "# %s" as the heading.`,
			filepath.Base(projectDir),
			strings.Join(info.Languages, ", "),
			strings.Join(info.Frameworks, ", "),
			info.BuildSystem,
			info.TestCommand,
			info.HasCI,
			info.HasDocker,
			codebaseCtx,
			filepath.Base(projectDir),
		)

		content, err := callAI(ctx, systemPrompt, prompt)
		if err != nil {
			return fmt.Errorf("generating CLAUDIO.md: %w", err)
		}

		if final, accepted := proposeFile(reader, "CLAUDIO.md", content); accepted {
			os.WriteFile(claudioMDPath, []byte(final), 0644)
			fmt.Println("  Wrote CLAUDIO.md")
		}
	} else {
		fmt.Println("  Skipped CLAUDIO.md (exists)")
	}

	// ── Generate settings.json ───────────────────────────────────
	settingsPath := filepath.Join(claudioDir, "settings.json")
	if !fileExistsAt(settingsPath) {
		fmt.Println("\n  Generating .claudio/settings.json...")

		prompt := fmt.Sprintf(`Generate a .claudio/settings.json for this project.

PROJECT:
- Languages: %s
- Build system: %s

Choose appropriate settings:
- model: "claude-sonnet-4-6" is good for most projects, "claude-opus-4-6" for complex codebases
- permissionMode: "default" for most, "auto" if the user likely wants fast iteration
- effortLevel: "medium" is default, "high" for complex projects
- outputStyle: "normal" unless the project suggests otherwise

Output valid JSON only. Available settings:
{
  "model": "claude-sonnet-4-6",
  "permissionMode": "default",
  "effortLevel": "medium",
  "outputStyle": "normal",
  "autoMemoryExtract": false,
  "memorySelection": "none"
}`,
			strings.Join(info.Languages, ", "),
			info.BuildSystem,
		)

		content, err := callAI(ctx, systemPrompt, prompt)
		if err != nil {
			return fmt.Errorf("generating settings.json: %w", err)
		}

		// Validate JSON
		content = strings.TrimSpace(content)
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
		var jsonCheck map[string]interface{}
		if json.Unmarshal([]byte(content), &jsonCheck) != nil {
			// Fallback to safe defaults
			content = `{
  "model": "claude-sonnet-4-6",
  "permissionMode": "default",
  "effortLevel": "medium",
  "autoMemoryExtract": false,
  "memorySelection": "none"
}`
		}

		if final, accepted := proposeFile(reader, ".claudio/settings.json", content); accepted {
			os.WriteFile(settingsPath, []byte(final), 0644)
			fmt.Println("  Wrote .claudio/settings.json")
		}
	} else {
		fmt.Println("  Skipped .claudio/settings.json (exists)")
	}

	// ── Generate project rules ───────────────────────────────────
	rulesPath := filepath.Join(claudioDir, "rules", "project.md")
	if !fileExistsAt(rulesPath) {
		fmt.Println("\n  Generating .claudio/rules/project.md...")

		prompt := fmt.Sprintf(`Generate a project rules file (.claudio/rules/project.md) for this project.
This file carries the full architecture knowledge and coding conventions for the project.

PROJECT:
- Languages: %s
- Frameworks: %s
- Build: %s
- Test: %s

CODEBASE CONTEXT:
%s

The file MUST follow this exact structure:

1. YAML frontmatter with name and description
2. ## Rules section — project-specific coding conventions (NOT generic advice):
   - Naming conventions
   - Error handling patterns
   - Import restrictions
   - Testing requirements
   - Commit message format
3. ## Architecture section — NEW, must include all four subsections:
   ### Package map
   List each major package/directory and what it owns (one line each, e.g. "internal/cli — CLI commands and user interaction")
   ### Key files
   The most important entry points per domain (e.g. "main.go — binary entry point", "internal/api/client.go — HTTP client")
   ### Wiring
   How major pieces connect: what calls what, how layers interact, dependency direction
   ### What lives where
   Quick lookup table for common tasks (e.g. "Adding a new CLI command → internal/cli/", "Changing AI provider → internal/api/")

Only include rules and architecture facts that are specific to THIS project.
Do NOT include generic advice.

Example format:
---
name: project-conventions
description: Coding standards and architecture guide for this project
---

## Rules

- Always use structured logging with slog
- Error types must implement the Error interface from internal/errors

## Architecture

### Package map
- internal/cli — CLI commands and cobra wiring
- internal/api — AI provider HTTP client

### Key files
- main.go — binary entry point
- internal/cli/root.go — root cobra command

### Wiring
CLI commands call internal/api.Client; settings loaded at startup in root.go

### What lives where
- New CLI command → internal/cli/
- New AI provider feature → internal/api/
`,
			strings.Join(info.Languages, ", "),
			strings.Join(info.Frameworks, ", "),
			info.BuildSystem,
			info.TestCommand,
			codebaseCtx,
		)

		content, err := callAI(ctx, systemPrompt, prompt)
		if err != nil {
			return fmt.Errorf("generating rules: %w", err)
		}

		if final, accepted := proposeFile(reader, ".claudio/rules/project.md", content); accepted {
			os.WriteFile(rulesPath, []byte(final), 0644)
			fmt.Println("  Wrote .claudio/rules/project.md")

			fmt.Println("\n  Seeding architecture memory...")
			seedArchitectureMemory(ctx, projectDir, final)
			fmt.Println("  Memory seeded — first agent skips re-investigation.")
		}

		// Always regenerate the index after rules generation (even if skipped, project.md may already exist)
		indexPath := filepath.Join(claudioDir, "rules", "index.md")
		if indexContent, err := buildRulesIndex(filepath.Join(claudioDir, "rules")); err == nil {
			os.WriteFile(indexPath, []byte(indexContent), 0644)
			fmt.Println("  Wrote .claudio/rules/index.md")
		}
	}

	return nil
}

// buildRulesIndex reads all .md files in rulesDir, extracts their frontmatter description,
// and returns a formatted index markdown string.
func buildRulesIndex(rulesDir string) (string, error) {
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("# Rules Index\n\n")

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if entry.Name() == "index.md" {
			continue
		}
		description := extractFrontmatterDescription(filepath.Join(rulesDir, entry.Name()))
		if description == "" {
			description = "project rules"
		}
		sb.WriteString(fmt.Sprintf("- `%s` — %s\n", entry.Name(), description))
	}

	return sb.String(), nil
}

// extractFrontmatterDescription parses YAML frontmatter from a markdown file
// and returns the value of the "description" field, or empty string if not found.
func extractFrontmatterDescription(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := string(data)

	// Frontmatter must start at the very beginning of the file
	if !strings.HasPrefix(content, "---") {
		return ""
	}

	// Find the closing ---
	rest := content[3:]
	end := strings.Index(rest, "\n---")
	if end == -1 {
		return ""
	}
	frontmatter := rest[:end]

	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "description:") {
			val := strings.TrimPrefix(line, "description:")
			return strings.TrimSpace(val)
		}
	}
	return ""
}

// seedArchitectureMemory extracts discrete architecture facts from the rules content
// and writes them to the project memory directory so the first agent skips re-investigation.
// Errors are non-fatal — a warning is printed and init continues.
func seedArchitectureMemory(ctx context.Context, projectDir, rulesContent string) {
	memDir := config.ProjectMemoryDir(projectDir)
	if err := os.MkdirAll(memDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not create memory dir: %v\n", err)
		return
	}

	archivePath := filepath.Join(memDir, "architecture.md")
	if _, err := os.Stat(archivePath); err == nil {
		// Already exists — skip
		return
	}

	extractPrompt := fmt.Sprintf(`Extract 10-15 discrete, one-sentence facts about this project's architecture from the rules below.
Each fact must be specific and actionable (e.g. "New CLI commands go in internal/cli/", "Migrations are appended-only in internal/storage/db.go").
Output one fact per line. No bullets, no numbers, no blank lines.

RULES:
%s`, rulesContent)

	response, err := callAI(ctx, "You are a technical documentation assistant. Extract precise, actionable facts.", extractPrompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not extract architecture facts: %v\n", err)
		return
	}

	// Parse facts: split by newline, trim, filter empty
	var facts []string
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			facts = append(facts, line)
		}
	}
	if len(facts) == 0 {
		fmt.Fprintf(os.Stderr, "  Warning: no architecture facts extracted\n")
		return
	}

	// Write in the format ParseEntry() expects (YAML frontmatter with facts list)
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("name: architecture\n")
	sb.WriteString("description: Project architecture map — package roles, key files, wiring\n")
	sb.WriteString("type: project\n")
	sb.WriteString("tags: [architecture, packages, wiring]\n")
	sb.WriteString(fmt.Sprintf("updated_at: %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString("facts:\n")
	for _, fact := range facts {
		sb.WriteString(fmt.Sprintf("  - %q\n", fact))
	}
	sb.WriteString("---\n")

	if err := os.WriteFile(archivePath, []byte(sb.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not write architecture memory: %v\n", err)
		return
	}
}

// callAI sends a single prompt to the API and returns the text response.
func callAI(ctx context.Context, system, prompt string) (string, error) {
	contentJSON, _ := json.Marshal([]api.UserContentBlock{
		{Type: "text", Text: prompt},
	})

	req := &api.MessagesRequest{
		Messages: []api.Message{
			{Role: "user", Content: contentJSON},
		},
		System:    system,
		MaxTokens: 4096,
	}

	resp, err := appInstance.API.SendMessage(ctx, req)
	if err != nil {
		return "", err
	}

	var result strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			result.WriteString(block.Text)
		}
	}
	return result.String(), nil
}

// proposeFile shows a generated file to the user and asks for confirmation.
// Returns the final content (potentially edited) and whether it was accepted.
func proposeFile(reader *bufio.Reader, name, content string) (string, bool) {
	fmt.Println()
	fmt.Printf("  ┌─ Proposed: %s ─────────────────────────────\n", name)

	// Show preview (first 30 lines)
	lines := strings.Split(content, "\n")
	maxPreview := 30
	if len(lines) < maxPreview {
		maxPreview = len(lines)
	}
	for _, line := range lines[:maxPreview] {
		fmt.Printf("  │ %s\n", line)
	}
	if len(lines) > 30 {
		fmt.Printf("  │ ... (%d more lines)\n", len(lines)-30)
	}
	fmt.Println("  └────────────────────────────────────────────")

	fmt.Printf("\n  Accept? (Y)es / (e)dit / (s)kip: ")
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	switch answer {
	case "", "y", "yes":
		return content, true
	case "e", "edit":
		edited := openInEditor(content)
		fmt.Println("  (edited version will be used)")
		return edited, true
	case "s", "skip", "n", "no":
		fmt.Println("  Skipped.")
		return "", false
	default:
		return content, true
	}
}

// openInEditor writes content to a temp file, opens $EDITOR, returns modified content.
func openInEditor(content string) string {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	tmpFile, err := os.CreateTemp("", "claudio-init-*.md")
	if err != nil {
		return content
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString(content)
	tmpFile.Close()

	cmd := exec.Command(editor, tmpFile.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return content
	}

	edited, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return content
	}
	return string(edited)
}

// runTemplateInit is the fallback when AI is not available.
func runTemplateInit(projectDir, claudioDir string, info *ProjectInfo, reader *bufio.Reader) {
	// Model selection
	model := "claude-sonnet-4-6"
	fmt.Printf("  Default model [%s]: ", model)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)
	if answer != "" {
		model = answer
	}

	// Permission mode
	permMode := "default"
	fmt.Printf("  Permission mode (default/auto/plan) [%s]: ", permMode)
	answer, _ = reader.ReadString('\n')
	answer = strings.TrimSpace(answer)
	if answer != "" {
		permMode = answer
	}

	// Settings
	settingsPath := filepath.Join(claudioDir, "settings.json")
	if !fileExistsAt(settingsPath) {
		settings := map[string]interface{}{
			"model":             model,
			"permissionMode":    permMode,
			"autoMemoryExtract": false,
			"memorySelection":   "none",
		}
		data, _ := json.MarshalIndent(settings, "", "  ")
		os.WriteFile(settingsPath, data, 0644)
		fmt.Printf("  Created %s\n", relPath(projectDir, settingsPath))
	}

	// CLAUDIO.md
	claudioMD := filepath.Join(projectDir, "CLAUDIO.md")
	if !fileExistsAt(claudioMD) {
		content := generateCLAUDIOMD(filepath.Base(projectDir), info)
		os.WriteFile(claudioMD, []byte(content), 0644)
		fmt.Println("  Created CLAUDIO.md")
	}

	// Rules
	exampleRule := filepath.Join(claudioDir, "rules", "project.md")
	if !fileExistsAt(exampleRule) {
		content := "---\nname: project-conventions\ndescription: Project-specific conventions and rules\n---\n\n<!-- Add your project-specific rules here -->\n"
		os.WriteFile(exampleRule, []byte(content), 0644)
	}
}

// gatherCodebaseContext collects key files and structure for the AI to analyze.
func gatherCodebaseContext(projectDir string, info *ProjectInfo) string {
	var sb strings.Builder

	// Directory structure (2 levels deep)
	sb.WriteString("=== Directory Structure ===\n")
	if tree, err := getDirTree(projectDir, 2); err == nil {
		sb.WriteString(tree)
	}
	sb.WriteString("\n")

	// Key manifest files
	manifests := []string{
		"go.mod", "package.json", "Cargo.toml", "pyproject.toml",
		"pom.xml", "build.gradle", "Makefile", "Dockerfile",
	}
	for _, name := range manifests {
		path := filepath.Join(projectDir, name)
		if content := readFileTruncated(path, 3000); content != "" {
			sb.WriteString(fmt.Sprintf("=== %s ===\n%s\n\n", name, content))
		}
	}

	// README
	for _, name := range []string{"README.md", "readme.md"} {
		path := filepath.Join(projectDir, name)
		if content := readFileTruncated(path, 4000); content != "" {
			sb.WriteString(fmt.Sprintf("=== %s ===\n%s\n\n", name, content))
		}
	}

	// CI config
	ciFiles := []string{
		".github/workflows/ci.yml", ".github/workflows/ci.yaml",
		".github/workflows/test.yml", ".github/workflows/build.yml",
		".gitlab-ci.yml",
	}
	for _, name := range ciFiles {
		path := filepath.Join(projectDir, name)
		if content := readFileTruncated(path, 2000); content != "" {
			sb.WriteString(fmt.Sprintf("=== %s ===\n%s\n\n", name, content))
		}
	}

	// Existing CLAUDE.md or .cursor/rules (for migration)
	for _, name := range []string{"CLAUDE.md", ".cursor/rules", ".cursorrules"} {
		path := filepath.Join(projectDir, name)
		if content := readFileTruncated(path, 3000); content != "" {
			sb.WriteString(fmt.Sprintf("=== %s (existing, for reference) ===\n%s\n\n", name, content))
		}
	}

	// Linting/formatting config
	for _, name := range []string{".eslintrc.json", ".prettierrc", ".golangci.yml", "rustfmt.toml", ".editorconfig"} {
		path := filepath.Join(projectDir, name)
		if content := readFileTruncated(path, 1000); content != "" {
			sb.WriteString(fmt.Sprintf("=== %s ===\n%s\n\n", name, content))
		}
	}

	// Cap total context
	result := sb.String()
	const maxContext = 30000
	if len(result) > maxContext {
		result = result[:maxContext] + "\n... (truncated)"
	}
	return result
}

// getDirTree returns a text representation of the directory tree.
func getDirTree(dir string, maxDepth int) (string, error) {
	var sb strings.Builder
	walkDir(dir, dir, &sb, 0, maxDepth)
	return sb.String(), nil
}

func walkDir(root, dir string, sb *strings.Builder, depth, maxDepth int) {
	if depth > maxDepth {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	indent := strings.Repeat("  ", depth)
	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden dirs, node_modules, vendor, etc.
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" ||
			name == "__pycache__" || name == "target" || name == "dist" || name == "build" {
			continue
		}

		if entry.IsDir() {
			sb.WriteString(fmt.Sprintf("%s%s/\n", indent, name))
			walkDir(root, filepath.Join(dir, name), sb, depth+1, maxDepth)
		} else {
			sb.WriteString(fmt.Sprintf("%s%s\n", indent, name))
		}
	}
}

// readFileTruncated reads a file up to maxBytes.
func readFileTruncated(path string, maxBytes int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := string(data)
	if len(s) > maxBytes {
		s = s[:maxBytes] + "\n... (truncated)"
	}
	return s
}

func printDetection(info *ProjectInfo) {
	if len(info.Languages) > 0 {
		fmt.Printf("    Languages: %s\n", strings.Join(info.Languages, ", "))
	}
	if len(info.Frameworks) > 0 {
		fmt.Printf("    Frameworks: %s\n", strings.Join(info.Frameworks, ", "))
	}
	if info.BuildSystem != "" {
		fmt.Printf("    Build system: %s\n", info.BuildSystem)
	}
	if info.TestCommand != "" {
		fmt.Printf("    Test command: %s\n", info.TestCommand)
	}
	if info.HasCI {
		fmt.Println("    CI/CD: detected")
	}
	if info.HasDocker {
		fmt.Println("    Docker: detected")
	}
}

func generateCLAUDIOMD(projectName string, info *ProjectInfo) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", projectName))

	sb.WriteString("## Overview\n")
	if info.Description != "" {
		sb.WriteString(info.Description + "\n")
	} else {
		sb.WriteString("<!-- Describe what this project does -->\n")
	}
	sb.WriteString("\n")

	if len(info.Languages) > 0 || len(info.Frameworks) > 0 {
		sb.WriteString("## Tech Stack\n")
		if len(info.Languages) > 0 {
			sb.WriteString("- Languages: " + strings.Join(info.Languages, ", ") + "\n")
		}
		if len(info.Frameworks) > 0 {
			sb.WriteString("- Frameworks: " + strings.Join(info.Frameworks, ", ") + "\n")
		}
		if info.BuildSystem != "" {
			sb.WriteString("- Build: " + info.BuildSystem + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Development\n")
	if info.TestCommand != "" {
		sb.WriteString(fmt.Sprintf("- Test: `%s`\n", info.TestCommand))
	}
	if info.BuildSystem != "" {
		sb.WriteString(fmt.Sprintf("- Build: `%s`\n", info.BuildSystem))
	}
	sb.WriteString("\n## Conventions\n<!-- Add project conventions -->\n")

	return sb.String()
}

func installBuiltinSkill(path, content string) {
	if fileExistsAt(path) {
		return
	}
	os.WriteFile(path, []byte(content), 0644)
}

func fileExistsAt(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func relPath(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}

const reviewSkillContent = `---
name: review
description: Structured code review
---
Review the code changes (staged or specified files). Analyze for:
1. **Correctness** — Logic errors, edge cases, off-by-one
2. **Style** — Naming, formatting, consistency with codebase
3. **Performance** — Unnecessary allocations, N+1 queries
4. **Maintainability** — Complexity, coupling, test coverage
5. **Security** — Input validation, injection, auth

Output a structured report with severity (critical/warning/info) per finding.
`

const securityReviewSkillContent = `---
name: security-review
description: OWASP-focused security audit
---
Perform a security audit of the code. Check for:
1. **Injection** — SQL, command, XSS, template
2. **Auth** — Broken authentication, session management
3. **Sensitive Data** — Hardcoded secrets, logging PII
4. **Access Control** — IDOR, privilege escalation
5. **Dependencies** — Known CVEs, outdated packages

Output findings with OWASP category, severity, file:line, and remediation.
`
