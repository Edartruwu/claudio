package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// BashTool executes shell commands.
type BashTool struct{}

type bashInput struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"` // milliseconds, default 120000
}

func (t *BashTool) Name() string { return "Bash" }

func (t *BashTool) Description() string {
	return `Executes a bash command and returns its output. Use this for running shell commands, installing packages, running tests, git operations, etc. The working directory persists between calls.`
}

func (t *BashTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "The bash command to execute"
			},
			"timeout": {
				"type": "number",
				"description": "Timeout in milliseconds (default: 120000)"
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

	// Safety checks
	if err := checkCommandSafety(in.Command); err != nil {
		return &Result{Content: fmt.Sprintf("Command blocked: %v", err), IsError: true}, nil
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
