package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/tasks"
)

// BashTool executes shell commands.
type BashTool struct {
	Security     SecurityChecker
	TaskRuntime  *tasks.Runtime
}

type bashInput struct {
	Command          string `json:"command"`
	Description      string `json:"description,omitempty"`
	Timeout          int    `json:"timeout,omitempty"` // milliseconds, default 120000
	RunInBackground  bool   `json:"run_in_background,omitempty"`
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

	// Safety checks — use injected security if available, fallback to built-in
	if t.Security != nil {
		if err := t.Security.CheckCommand(in.Command); err != nil {
			return &Result{Content: fmt.Sprintf("Command blocked: %v", err), IsError: true}, nil
		}
	} else if err := checkCommandSafety(in.Command); err != nil {
		return &Result{Content: fmt.Sprintf("Command blocked: %v", err), IsError: true}, nil
	}

	// Background execution
	if in.RunInBackground && t.TaskRuntime != nil {
		taskTimeout := time.Duration(0)
		if in.Timeout > 0 {
			taskTimeout = time.Duration(in.Timeout) * time.Millisecond
		}
		state, err := tasks.SpawnShellTask(t.TaskRuntime, tasks.ShellTaskInput{
			Command:     in.Command,
			Description: in.Description,
			Timeout:     taskTimeout,
		})
		if err != nil {
			return &Result{Content: fmt.Sprintf("Failed to start background task: %v", err), IsError: true}, nil
		}
		return &Result{Content: fmt.Sprintf("Background task started: %s\nTask ID: %s\nUse TaskOutput to check results.", state.Description, state.ID)}, nil
	}

	timeout := 120 * time.Second
	if in.Timeout > 0 {
		timeout = time.Duration(in.Timeout) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", in.Command)

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
		if ctx.Err() == context.DeadlineExceeded {
			return &Result{
				Content: fmt.Sprintf("Command timed out after %v\n%s", timeout, output.String()),
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

	// Truncate very large outputs
	const maxOutput = 100000
	if len(result) > maxOutput {
		result = result[:maxOutput] + "\n... (output truncated)"
	}

	return &Result{Content: result}, nil
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
