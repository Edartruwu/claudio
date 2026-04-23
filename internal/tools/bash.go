package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/Abraxas-365/claudio/internal/tools/outputfilter"
)

// BashTool executes shell commands.
type BashTool struct {
	Security            SecurityChecker
	TaskRuntime         *tasks.Runtime
	SessionID           string // owning session — passed to background tasks for access control
	OutputFilterEnabled bool
	// FilterRecorder is an optional callback invoked after output filtering
	// with the normalized command key and byte counts (in, out).
	// If nil, filtering still applies but no analytics are recorded.
	FilterRecorder func(cmd string, bytesIn, bytesOut int)
}

type bashInput struct {
	Command              string `json:"command"`
	Description          string `json:"description,omitempty"`
	Timeout              int    `json:"timeout,omitempty"`               // milliseconds, default 120000
	RunInBackground      bool   `json:"run_in_background,omitempty"`
	ForegroundTimeoutMs  int    `json:"foreground_timeout_ms,omitempty"` // milliseconds, default 30000; exceeded → auto-promote to bg
}

func (t *BashTool) Name() string { return "Bash" }

func (t *BashTool) Description() string {
	return prompts.BashDescription()
}

func (t *BashTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "The command to execute"
			},
			"description": {
				"type": "string",
				"description": "Clear, concise description of what this command does in active voice"
			},
			"timeout": {
				"type": "number",
				"description": "Optional timeout in milliseconds (max 600000)"
			},
			"run_in_background": {
				"type": "boolean",
				"description": "Set to true to run this command in the background"
			},
			"foreground_timeout_ms": {
				"type": "number",
				"description": "Optional foreground budget in milliseconds (default 30000). If the command has not finished within this budget it is automatically promoted to a background task and the tool returns immediately with the task ID."
			}
		},
		"required": ["command"]
	}`)
}

func (t *BashTool) IsReadOnly() bool { return false }

func (t *BashTool) RequiresApproval(input json.RawMessage) bool {
	return true // Always requires approval
}

func (t *BashTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in bashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.Command == "" {
		return &Result{Content: "No command provided", IsError: true}, nil
	}

	// Detect cat/head/tail used as file readers — redirect to Read tool.
	// cat bypasses the Read tool's dedup cache and size limits, causing the
	// same file content to be re-sent to the model on every invocation.
	if isCatFileCommand(in.Command) {
		return &Result{
			Content: "Use the Read tool instead of cat/head/tail/sed to read files. " +
				"The Read tool caches results so unchanged files don't consume extra tokens, " +
				"and enforces size limits. Call Read with the file path directly. " +
				"Use the offset and limit parameters for line ranges instead of sed -n.",
			IsError: true,
		}, nil
	}

	// Safety checks — use injected security if available, fallback to built-in
	if t.Security != nil {
		if err := t.Security.CheckCommand(in.Command); err != nil {
			return &Result{Content: fmt.Sprintf("Command blocked: %v", err), IsError: true}, nil
		}
	} else if err := checkCommandSafety(in.Command); err != nil {
		return &Result{Content: fmt.Sprintf("Command blocked: %v", err), IsError: true}, nil
	}

	// Background execution
	if in.RunInBackground {
		if t.TaskRuntime == nil {
			return &Result{Content: "Cannot run in background: no task runtime available", IsError: true}, nil
		}
		taskTimeout := time.Duration(0)
		if in.Timeout > 0 {
			taskTimeout = time.Duration(in.Timeout) * time.Millisecond
		}
		state, err := tasks.SpawnShellTask(t.TaskRuntime, tasks.ShellTaskInput{
			Command:     in.Command,
			Description: in.Description,
			Timeout:     taskTimeout,
			SessionID:   t.SessionID,
		})
		if err != nil {
			return &Result{Content: fmt.Sprintf("Failed to start background task: %v", err), IsError: true}, nil
		}
		return &Result{Content: fmt.Sprintf("Background task started: %s\nTask ID: %s\nUse TaskOutput to check results.", state.Description, state.ID)}, nil
	}

	// Overall command timeout (caps the total run time).
	overallTimeout := 120 * time.Second
	if in.Timeout > 0 {
		overallTimeout = time.Duration(in.Timeout) * time.Millisecond
	}

	// Foreground budget: how long we block the current turn before auto-promoting
	// the command to a background task.  Defaults to 30 s.
	const defaultFgBudgetMs = 30_000
	fgBudgetMs := in.ForegroundTimeoutMs
	if fgBudgetMs <= 0 {
		fgBudgetMs = defaultFgBudgetMs
	}
	fgBudget := time.Duration(fgBudgetMs) * time.Millisecond

	// Auto-promote only makes sense when:
	//   (a) a task runtime is available, and
	//   (b) the fg budget is shorter than the overall timeout (otherwise the
	//       command would time-out before we could promote it anyway).
	autoPromote := t.TaskRuntime != nil && fgBudget < overallTimeout

	// Pick the timeout that governs the foreground run.
	runTimeout := overallTimeout
	if autoPromote {
		runTimeout = fgBudget
	}

	runCtx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	// Capture CWD before the context is used in exec.CommandContext so the
	// helper reads from the *parent* ctx (still valid after runCtx is done).
	cwd := CwdFromContext(ctx)

	cmd := exec.CommandContext(runCtx, "bash", "-c", in.Command)

	// Kill the entire process group on timeout, not just bash.
	// Without this, child processes keep stdout/stderr pipes open
	// and cmd.Run() blocks indefinitely even after bash is killed.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	// WaitDelay: after Cancel fires, wait up to 5s for pipes to drain,
	// then forcibly close them so cmd.Run() returns.
	cmd.WaitDelay = 5 * time.Second

	// Use context CWD override for worktree-isolated agents
	if cwd != "" {
		cmd.Dir = cwd
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var output strings.Builder
	if stdout.Len() > 0 {
		output.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("STDERR:\n")
		output.WriteString(stderr.String())
	}

	if err != nil {
		// Foreground budget expired → auto-promote to background.
		if runCtx.Err() == context.DeadlineExceeded && autoPromote {
			bgTimeout := time.Duration(0)
			if in.Timeout > 0 {
				bgTimeout = time.Duration(in.Timeout) * time.Millisecond
			}
			state, spawnErr := tasks.SpawnShellTask(t.TaskRuntime, tasks.ShellTaskInput{
				Command:     in.Command,
				Description: in.Description,
				Timeout:     bgTimeout,
				SessionID:   t.SessionID,
			})
			if spawnErr != nil {
				return &Result{
					Content: fmt.Sprintf("Command exceeded foreground budget and failed to promote to background: %v", spawnErr),
					IsError: true,
				}, nil
			}
			return &Result{
				Content: fmt.Sprintf("Command is running in background. Task ID: %s. Result will be injected when complete.", state.ID),
			}, nil
		}

		if runCtx.Err() == context.DeadlineExceeded {
			return &Result{
				Content: fmt.Sprintf("Command timed out after %v\n%s", overallTimeout, output.String()),
				IsError: true,
			}, nil
		}
		return &Result{
			Content: fmt.Sprintf("Exit code: %v\n%s", err, output.String()),
			IsError: true,
		}, nil
	}

	result := output.String()
	if result == "" {
		result = "(no output)"
	}

	// Apply output filter to reduce token usage (RTK-style)
	if t.OutputFilterEnabled && result != "(no output)" {
		result = outputfilter.FilterAndRecord(in.Command, result, t.FilterRecorder)
	}

	// Truncate large outputs — matches Claude Code's 30KB Bash cap.
	// Explicit message so the model knows it got a partial view and can
	// switch to the Read/Grep tools which have deduplication and proper limits.
	const maxOutput = 30_000
	if len(result) > maxOutput {
		result = result[:maxOutput] + fmt.Sprintf(
			"\n\n[Bash output truncated at %d chars. If you were reading a file with cat, use the Read tool instead — it has caching and proper size limits. If searching, use Grep instead of grep/awk.]",
			maxOutput,
		)
	}

	return &Result{Content: result}, nil
}

// isCatFileCommand returns true when the command is essentially just reading
// a file with cat/head/tail — something the Read tool handles better.
// It intentionally avoids blocking cat in pipelines (e.g. cat file | grep X)
// since those have legitimate uses.
func isCatFileCommand(command string) bool {
	cmd := strings.TrimSpace(command)
	// Simple "cat file" or "cat -n file" with no pipes/redirects
	if strings.Contains(cmd, "|") || strings.Contains(cmd, ">") || strings.Contains(cmd, ";") || strings.Contains(cmd, "&&") {
		return false
	}
	prefixes := []string{"cat ", "cat -n ", "head ", "head -n ", "tail ", "tail -n ", "sed -n "}
	for _, p := range prefixes {
		if strings.HasPrefix(cmd, p) {
			return true
		}
	}
	return false
}

func checkCommandSafety(command string) error {
	// Block obviously dangerous patterns
	dangerous := []string{
		":(){ :|:& };:", // fork bomb
		"rm -rf /",
		"mkfs.",
		"dd if=/dev/zero",
		"> /dev/sda",
	}
	lower := strings.ToLower(command)
	for _, d := range dangerous {
		if strings.Contains(lower, d) {
			return fmt.Errorf("potentially destructive command: contains %q", d)
		}
	}
	return nil
}
