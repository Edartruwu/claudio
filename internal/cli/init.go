package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Claudio in the current project",
	Long:  `Creates a .claudio/ directory with default configuration files for the current project.`,
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

	// Check if already initialized
	if _, err := os.Stat(claudioDir); err == nil {
		fmt.Println("Project already has .claudio/ directory.")
		fmt.Print("Re-initialize? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	fmt.Println("Initializing Claudio project...")

	// Phase 1: Detect project characteristics
	fmt.Println("\nDetecting project...")
	info := DetectProject(projectDir)
	if len(info.Languages) > 0 {
		fmt.Printf("  Languages: %s\n", strings.Join(info.Languages, ", "))
	}
	if len(info.Frameworks) > 0 {
		fmt.Printf("  Frameworks: %s\n", strings.Join(info.Frameworks, ", "))
	}
	if info.BuildSystem != "" {
		fmt.Printf("  Build system: %s\n", info.BuildSystem)
	}
	if info.HasCI {
		fmt.Println("  CI/CD: detected")
	}
	if info.HasDocker {
		fmt.Println("  Docker: detected")
	}

	// Create directories
	dirs := []string{
		claudioDir,
		filepath.Join(claudioDir, "rules"),
		filepath.Join(claudioDir, "skills"),
		filepath.Join(claudioDir, "agents"),
		filepath.Join(claudioDir, "memory"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}

	// Interactive model selection
	model := "claude-sonnet-4-6"
	fmt.Printf("\nDefault model [%s]: ", model)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)
	if answer != "" {
		model = answer
	}

	// Permission mode
	permMode := "default"
	fmt.Printf("Permission mode (default/auto/plan) [%s]: ", permMode)
	answer, _ = reader.ReadString('\n')
	answer = strings.TrimSpace(answer)
	if answer != "" {
		permMode = answer
	}

	// Create project settings.json
	settingsPath := filepath.Join(claudioDir, "settings.json")
	if !fileExistsAt(settingsPath) {
		settings := map[string]interface{}{
			"model":              model,
			"permissionMode":     permMode,
			"autoMemoryExtract":  true,
			"memorySelection":    "ai",
		}
		data, _ := json.MarshalIndent(settings, "", "  ")
		os.WriteFile(settingsPath, data, 0644)
		fmt.Printf("  Created %s\n", relPath(projectDir, settingsPath))
	} else {
		fmt.Printf("  Skipped %s (exists)\n", relPath(projectDir, settingsPath))
	}

	// Create CLAUDIO.md using detected project info
	claudioMD := filepath.Join(projectDir, "CLAUDIO.md")
	if !fileExistsAt(claudioMD) {
		content := generateCLAUDIOMD(filepath.Base(projectDir), info)
		os.WriteFile(claudioMD, []byte(content), 0644)
		fmt.Printf("  Created %s\n", "CLAUDIO.md")
	} else {
		fmt.Printf("  Skipped %s (exists)\n", "CLAUDIO.md")
	}

	// Create example rule
	exampleRule := filepath.Join(claudioDir, "rules", "project.md")
	if !fileExistsAt(exampleRule) {
		content := `---
name: project-conventions
description: Project-specific conventions and rules
---

<!-- Add your project-specific rules here. These are injected into the system prompt. -->
<!-- Examples: -->
<!-- - Always use structured logging -->
<!-- - Prefer composition over inheritance -->
<!-- - All public functions must have doc comments -->
`
		os.WriteFile(exampleRule, []byte(content), 0644)
		fmt.Printf("  Created %s\n", relPath(projectDir, exampleRule))
	}

	// Create .gitignore for local-only files
	gitignorePath := filepath.Join(claudioDir, ".gitignore")
	if !fileExistsAt(gitignorePath) {
		content := `# Local settings (not shared with team)
settings.local.json
worktrees/
`
		os.WriteFile(gitignorePath, []byte(content), 0644)
		fmt.Printf("  Created %s\n", relPath(projectDir, gitignorePath))
	}

	// Offer to install built-in skills
	fmt.Print("\nInstall review skills (review, security-review)? (Y/n): ")
	answer, _ = reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "" || answer == "y" || answer == "yes" {
		installBuiltinSkill(filepath.Join(claudioDir, "skills", "review.md"), reviewSkillContent)
		installBuiltinSkill(filepath.Join(claudioDir, "skills", "security-review.md"), securityReviewSkillContent)
		fmt.Println("  Installed review and security-review skills")
	}

	fmt.Println("\nDone! Your project is ready for Claudio.")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review and customize CLAUDIO.md")
	fmt.Println("  2. Add rules in .claudio/rules/")
	fmt.Println("  3. Add custom skills in .claudio/skills/")
	fmt.Println("  4. Run: claudio")

	return nil
}

func generateCLAUDIOMD(projectName string, info *ProjectInfo) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s — Project Instructions\n\n", projectName))

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
	sb.WriteString("<!-- Add build, test, and run instructions -->\n\n")

	sb.WriteString("## Architecture\n")
	sb.WriteString("<!-- Key architectural decisions and patterns -->\n\n")

	sb.WriteString("## Conventions\n")
	sb.WriteString("<!-- Coding style, naming conventions, etc. -->\n\n")

	sb.WriteString("## Important Notes\n")
	sb.WriteString("<!-- Anything the AI should know when working on this project -->\n")

	return sb.String()
}

func installBuiltinSkill(path, content string) {
	if fileExistsAt(path) {
		return
	}
	os.WriteFile(path, []byte(content), 0644)
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
