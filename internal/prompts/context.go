package prompts

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GetGitStatus collects current git context: branch, main branch, user, status, recent commits.
// Returns empty string if not a git repo or on error.
// Status output is capped at 2000 characters to avoid bloating the prompt.
func GetGitStatus() string {
	// Check if we're in a git repo
	if err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return ""
	}

	branch := runGit("rev-parse", "--abbrev-ref", "HEAD")
	if branch == "" {
		return ""
	}

	// Determine main branch
	mainBranch := ""
	for _, candidate := range []string{"main", "master", "develop", "trunk"} {
		out := runGit("rev-parse", "--verify", "--quiet", candidate)
		if out != "" {
			mainBranch = candidate
			break
		}
	}

	user := runGit("config", "user.name")

	status := runGit("status", "--short")
	if len(status) > 2000 {
		status = status[:2000] + "\n... (truncated, use Bash tool for full status)"
	}
	if status == "" {
		status = "(clean)"
	}

	log := runGit("log", "--oneline", "-5")

	parts := []string{
		"This is the git status at the start of the conversation. Note that this status is a snapshot in time, and will not update during the conversation.",
		fmt.Sprintf("Current branch: %s", branch),
	}
	if mainBranch != "" {
		parts = append(parts, fmt.Sprintf("Main branch (you will usually use this for PRs): %s", mainBranch))
	}
	if user != "" {
		parts = append(parts, fmt.Sprintf("Git user: %s", user))
	}
	parts = append(parts, fmt.Sprintf("Status:\n%s", status))
	if log != "" {
		parts = append(parts, fmt.Sprintf("Recent commits:\n%s", log))
	}
	return strings.Join(parts, "\n\n")
}

// FormatSystemContext formats the git status for appending to the system prompt.
func FormatSystemContext(gitStatus string) string {
	if gitStatus == "" {
		return ""
	}
	return "gitStatus: " + gitStatus
}

// FormatUserContextMessage returns a <system-reminder> user message containing
// CLAUDE.md content and the current date. This should be prepended as the first
// user message in the conversation so user-specific context stays out of the
// cached system prompt prefix.
func FormatUserContextMessage(claudeMD, currentDate string) string {
	var parts []string

	if claudeMD != "" {
		parts = append(parts, fmt.Sprintf(`# claudeMd
Codebase and user instructions are shown below. Be sure to adhere to these instructions. IMPORTANT: These instructions OVERRIDE any default behavior and you MUST follow them exactly as written.

%s`, claudeMD))
	}

	if currentDate == "" {
		currentDate = time.Now().Format("2006-01-02")
	}
	parts = append(parts, fmt.Sprintf("# currentDate\nToday's date is %s.", currentDate))

	if len(parts) == 0 {
		return ""
	}

	return fmt.Sprintf(`<system-reminder>
As you answer the user's questions, you can use the following context:
%s

      IMPORTANT: this context may or may not be relevant to your tasks. You should not respond to this context unless it is highly relevant to your task.
</system-reminder>`, strings.Join(parts, "\n\n"))
}

func runGit(args ...string) string {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
