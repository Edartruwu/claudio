// Package git provides high-level git operations for Claudio.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Repo represents a git repository.
type Repo struct {
	Dir string
}

// NewRepo creates a Repo for the given directory.
func NewRepo(dir string) *Repo {
	return &Repo{Dir: dir}
}

// IsRepo checks if the directory is inside a git repository.
func (r *Repo) IsRepo() bool {
	_, err := r.run("rev-parse", "--is-inside-work-tree")
	return err == nil
}

// Root returns the git root directory.
func (r *Repo) Root() (string, error) {
	return r.run("rev-parse", "--show-toplevel")
}

// CurrentBranch returns the current branch name.
func (r *Repo) CurrentBranch() (string, error) {
	return r.run("rev-parse", "--abbrev-ref", "HEAD")
}

// Status returns the short status output.
func (r *Repo) Status() (string, error) {
	return r.run("status", "--short")
}

// StatusPorcelain returns machine-readable status.
func (r *Repo) StatusPorcelain() (string, error) {
	return r.run("status", "--porcelain")
}

// Diff returns the diff of uncommitted changes.
func (r *Repo) Diff() (string, error) {
	return r.run("diff")
}

// DiffStaged returns the diff of staged changes.
func (r *Repo) DiffStaged() (string, error) {
	return r.run("diff", "--cached")
}

// DiffBranch returns the diff between the current branch and a base branch.
func (r *Repo) DiffBranch(base string) (string, error) {
	return r.run("diff", base+"...HEAD")
}

// Log returns recent commit log.
func (r *Repo) Log(n int) (string, error) {
	return r.run("log", fmt.Sprintf("-%d", n), "--oneline")
}

// LogDetailed returns detailed commit log.
func (r *Repo) LogDetailed(n int) (string, error) {
	return r.run("log", fmt.Sprintf("-%d", n), "--format=%H %s (%an, %ar)")
}

// HasRemote checks if the current branch tracks a remote.
func (r *Repo) HasRemote() bool {
	_, err := r.run("rev-parse", "--abbrev-ref", "@{u}")
	return err == nil
}

// RemoteURL returns the origin remote URL.
func (r *Repo) RemoteURL() (string, error) {
	return r.run("remote", "get-url", "origin")
}

// Add stages files.
func (r *Repo) Add(files ...string) error {
	args := append([]string{"add"}, files...)
	_, err := r.run(args...)
	return err
}

// Commit creates a commit with the given message.
func (r *Repo) Commit(message string) error {
	_, err := r.run("commit", "-m", message)
	return err
}

// Push pushes the current branch to remote.
func (r *Repo) Push() error {
	_, err := r.run("push")
	return err
}

// PushUpstream pushes with -u flag to set upstream.
func (r *Repo) PushUpstream(remote, branch string) error {
	_, err := r.run("push", "-u", remote, branch)
	return err
}

// Stash stashes current changes.
func (r *Repo) Stash(message string) error {
	args := []string{"stash", "push"}
	if message != "" {
		args = append(args, "-m", message)
	}
	_, err := r.run(args...)
	return err
}

// StashPop pops the latest stash.
func (r *Repo) StashPop() error {
	_, err := r.run("stash", "pop")
	return err
}

// CreateBranch creates and checks out a new branch.
func (r *Repo) CreateBranch(name string) error {
	_, err := r.run("checkout", "-b", name)
	return err
}

// WorktreeAdd creates a new git worktree.
func (r *Repo) WorktreeAdd(path, branch string) error {
	_, err := r.run("worktree", "add", "-b", branch, path)
	return err
}

// WorktreeRemove removes a git worktree.
func (r *Repo) WorktreeRemove(path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	_, err := r.run(args...)
	return err
}

// HeadCommit returns the current HEAD commit hash.
func (r *Repo) HeadCommit() (string, error) {
	return r.run("rev-parse", "HEAD")
}

// HasChanges checks if there are uncommitted changes in the working tree.
func (r *Repo) HasChanges() (bool, error) {
	out, err := r.run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// DeleteBranch deletes a local branch.
func (r *Repo) DeleteBranch(name string) error {
	_, err := r.run("branch", "-D", name)
	return err
}

// Blame returns git blame for a file.
func (r *Repo) Blame(file string) (string, error) {
	return r.run("blame", "--line-porcelain", file)
}

// RepoInfo returns a summary of the repository state.
func (r *Repo) RepoInfo() string {
	var info strings.Builder

	branch, err := r.CurrentBranch()
	if err != nil {
		return "Not a git repository"
	}
	info.WriteString(fmt.Sprintf("Branch: %s\n", branch))

	if status, err := r.Status(); err == nil && status != "" {
		info.WriteString(fmt.Sprintf("Changes:\n%s\n", status))
	} else {
		info.WriteString("Working tree clean\n")
	}

	if r.HasRemote() {
		if url, err := r.RemoteURL(); err == nil {
			info.WriteString(fmt.Sprintf("Remote: %s\n", url))
		}
	}

	return info.String()
}

func (r *Repo) run(args ...string) (string, error) {
	ctx := context.Background()
	return r.RunContext(ctx, args...)
}

// RunContext executes a git command with context.
func (r *Repo) RunContext(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.Dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}
