package tasks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// ShellTaskInput defines parameters for spawning a background shell task.
type ShellTaskInput struct {
	Command     string
	Description string
	Timeout     time.Duration // 0 = no timeout
	SessionID   string        // owning session for access control
}

// SpawnShellTask starts a shell command in the background and returns immediately.
func SpawnShellTask(rt *Runtime, input ShellTaskInput) (*TaskState, error) {
	id := rt.GenerateID(TypeShell)

	// Create output file
	output, err := NewTaskOutput(rt.outputDir, id)
	if err != nil {
		return nil, fmt.Errorf("create output: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if input.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), input.Timeout)
	}

	desc := input.Description
	if desc == "" {
		desc = truncateCmd(input.Command, 60)
	}

	state := &TaskState{
		ID:          id,
		Type:        TypeShell,
		Status:      StatusRunning,
		Description: desc,
		SessionID:   input.SessionID,
		Command:     input.Command,
		OutputFile:  output.Path(),
		StartTime:   time.Now(),
		cancel:      cancel,
	}

	rt.Register(state)

	// Spawn the command
	go runShellTask(ctx, rt, state, output, input.Command)

	return state, nil
}

func runShellTask(ctx context.Context, rt *Runtime, state *TaskState, output *TaskOutput, command string) {
	defer output.Close()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.Env = os.Environ()

	// Start stall watchdog
	stallCtx, stallCancel := context.WithCancel(ctx)
	defer stallCancel()
	go stallWatchdog(stallCtx, output, state)

	err := cmd.Run()

	if err != nil {
		if ctx.Err() == context.Canceled {
			rt.SetStatus(state.ID, StatusKilled, "killed by user")
			code := -1
			state.ExitCode = &code
			return
		}
		if ctx.Err() == context.DeadlineExceeded {
			rt.SetStatus(state.ID, StatusFailed, fmt.Sprintf("timeout after %s", time.Since(state.StartTime).Round(time.Second)))
			code := -1
			state.ExitCode = &code
			return
		}

		// Extract exit code
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			state.ExitCode = &code
			// exit code 137 = killed by signal, not a "failure"
			if code == 137 {
				rt.SetStatus(state.ID, StatusKilled, "")
			} else {
				rt.SetStatus(state.ID, StatusFailed, fmt.Sprintf("exit code %d", code))
			}
		} else {
			rt.SetStatus(state.ID, StatusFailed, err.Error())
		}
		return
	}

	code := 0
	state.ExitCode = &code
	rt.SetStatus(state.ID, StatusCompleted, "")
}

// stallWatchdog monitors a background shell task for stalled output that
// might indicate an interactive prompt the user doesn't see.
func stallWatchdog(ctx context.Context, output *TaskOutput, state *TaskState) {
	const checkInterval = 5 * time.Second
	const stallThreshold = 45 * time.Second

	var lastSize int64
	var lastChange time.Time = time.Now()

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentSize := output.Size()
			if currentSize != lastSize {
				lastSize = currentSize
				lastChange = time.Now()
				continue
			}

			// Output hasn't grown
			if time.Since(lastChange) > stallThreshold && state.Status == StatusRunning {
				// Read last 4KB to check for interactive prompts
				content, _, _ := ReadDelta(output.Path(), max(0, currentSize-4096), 4096)
				if looksInteractive(content) {
					// Append warning to output
					output.Write([]byte("\n\n[STALL WARNING] Output stopped growing. " +
						"The command may be waiting for interactive input. " +
						"Consider killing this task and using non-interactive flags.\n"))
					return // Only warn once
				}
			}
		}
	}
}

// looksInteractive checks if the tail of output looks like an interactive prompt.
func looksInteractive(content string) bool {
	lower := strings.ToLower(content)
	patterns := []string{
		"(y/n)",
		"[y/n]",
		"(yes/no)",
		"press enter",
		"press any key",
		"continue?",
		"proceed?",
		"password:",
		"passphrase:",
		"username:",
		"login:",
		"are you sure",
	}

	lastLine := lastNonEmptyLine(content)
	for _, p := range patterns {
		tailStart := len(lower) - 200
		if tailStart < 0 {
			tailStart = 0
		}
		if strings.Contains(strings.ToLower(lastLine), p) || strings.Contains(lower[tailStart:], p) {
			return true
		}
	}
	return false
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func truncateCmd(cmd string, maxLen int) string {
	cmd = strings.TrimSpace(cmd)
	if len(cmd) <= maxLen {
		return cmd
	}
	return cmd[:maxLen-3] + "..."
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
