package utils

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ExecResult holds the result of a command execution.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
	Duration time.Duration
}

// ExecNoThrow runs a command and returns its result without throwing.
// Unlike exec.Command().Run(), this never returns an error for non-zero exit codes.
func ExecNoThrow(ctx context.Context, name string, args ...string) ExecResult {
	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()

	err := cmd.Run()
	result := ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(start),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Err = err
			result.ExitCode = -1
		}
	}

	return result
}

// ExecLines runs a command and returns stdout split into lines.
func ExecLines(ctx context.Context, name string, args ...string) ([]string, error) {
	result := ExecNoThrow(ctx, name, args...)
	if result.Err != nil {
		return nil, result.Err
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("exit code %d: %s", result.ExitCode, result.Stderr)
	}

	output := strings.TrimSpace(result.Stdout)
	if output == "" {
		return nil, nil
	}
	return strings.Split(output, "\n"), nil
}

// ExecSilent runs a command silently, returning only whether it succeeded.
func ExecSilent(ctx context.Context, name string, args ...string) bool {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// ExecWithDir runs a command in a specific directory.
func ExecWithDir(ctx context.Context, dir, name string, args ...string) ExecResult {
	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()

	err := cmd.Run()
	result := ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(start),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Err = err
			result.ExitCode = -1
		}
	}

	return result
}

// ExecWithTimeout runs a command with a specific timeout.
func ExecWithTimeout(timeout time.Duration, name string, args ...string) ExecResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return ExecNoThrow(ctx, name, args...)
}
