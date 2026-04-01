package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// --- EnterWorktreeTool ---

type EnterWorktreeTool struct{}

type worktreeEnterInput struct {
	Name string `json:"name,omitempty"`
}

func (t *EnterWorktreeTool) Name() string { return "EnterWorktree" }
func (t *EnterWorktreeTool) Description() string {
	return `Creates and enters a git worktree for isolated development. The worktree gets its own branch and working directory. Use this for parallel work that shouldn't interfere with the main branch.`
}
func (t *EnterWorktreeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "Name for the worktree branch (auto-generated if omitted)"}
		}
	}`)
}
func (t *EnterWorktreeTool) IsReadOnly() bool                        { return false }
func (t *EnterWorktreeTool) RequiresApproval(_ json.RawMessage) bool { return true }
func (t *EnterWorktreeTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in worktreeEnterInput
	json.Unmarshal(input, &in)

	name := in.Name
	if name == "" {
		name = fmt.Sprintf("claudio-%d", time.Now().Unix())
	}

	// Find git root
	gitRoot, err := gitCommand(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return &Result{Content: "Not in a git repository", IsError: true}, nil
	}

	worktreeDir := filepath.Join(gitRoot, ".claudio", "worktrees", name)

	// Create worktree with new branch
	branchName := "claudio/" + name
	_, err = gitCommand(ctx, "worktree", "add", "-b", branchName, worktreeDir)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create worktree: %v", err), IsError: true}, nil
	}

	// Change to worktree directory
	if err := os.Chdir(worktreeDir); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to enter worktree: %v", err), IsError: true}, nil
	}

	return &Result{Content: fmt.Sprintf("Created worktree at %s (branch: %s)", worktreeDir, branchName)}, nil
}

// --- ExitWorktreeTool ---

type ExitWorktreeTool struct{}

type worktreeExitInput struct {
	Action         string `json:"action"` // "keep" or "remove"
	DiscardChanges bool   `json:"discard_changes,omitempty"`
}

func (t *ExitWorktreeTool) Name() string { return "ExitWorktree" }
func (t *ExitWorktreeTool) Description() string {
	return `Exits the current git worktree. Use action "keep" to preserve changes or "remove" to clean up.`
}
func (t *ExitWorktreeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["keep", "remove"], "description": "Whether to keep or remove the worktree"},
			"discard_changes": {"type": "boolean", "description": "Discard uncommitted changes when removing"}
		},
		"required": ["action"]
	}`)
}
func (t *ExitWorktreeTool) IsReadOnly() bool                        { return false }
func (t *ExitWorktreeTool) RequiresApproval(_ json.RawMessage) bool { return true }
func (t *ExitWorktreeTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in worktreeExitInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	cwd, _ := os.Getwd()

	// Find the main repo root
	gitRoot, err := gitCommand(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return &Result{Content: "Not in a git repository", IsError: true}, nil
	}

	// Go back to original repo
	mainRoot := findMainRoot(gitRoot)
	if err := os.Chdir(mainRoot); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to return to main repo: %v", err), IsError: true}, nil
	}

	if in.Action == "remove" {
		args := []string{"worktree", "remove"}
		if in.DiscardChanges {
			args = append(args, "--force")
		}
		args = append(args, cwd)
		if _, err := gitCommand(ctx, args...); err != nil {
			return &Result{Content: fmt.Sprintf("Failed to remove worktree: %v", err), IsError: true}, nil
		}
		return &Result{Content: fmt.Sprintf("Removed worktree at %s", cwd)}, nil
	}

	return &Result{Content: fmt.Sprintf("Exited worktree. Changes kept at %s", cwd)}, nil
}

func gitCommand(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %s", err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func findMainRoot(path string) string {
	// Walk up to find a non-worktree git root
	for {
		parent := filepath.Dir(path)
		if parent == path {
			return path
		}
		if _, err := os.Stat(filepath.Join(parent, ".git")); err == nil {
			return parent
		}
		path = parent
	}
}
