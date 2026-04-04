package utils

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecNoThrow_Success(t *testing.T) {
	ctx := context.Background()
	result := ExecNoThrow(ctx, "echo", "hello")
	if result.Err != nil {
		t.Fatalf("unexpected Err: %v", result.Err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Errorf("Stdout = %q, expected to contain 'hello'", result.Stdout)
	}
	if result.Duration <= 0 {
		t.Error("Duration should be positive")
	}
}

func TestExecNoThrow_NonZeroExit(t *testing.T) {
	ctx := context.Background()
	// "false" exits with code 1 on Unix
	result := ExecNoThrow(ctx, "false")
	if result.Err != nil {
		t.Fatalf("unexpected Err: %v", result.Err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code from 'false'")
	}
}

func TestExecNoThrow_BadCommand(t *testing.T) {
	ctx := context.Background()
	result := ExecNoThrow(ctx, "__nonexistent_cmd_xyz__")
	if result.Err == nil {
		t.Error("expected Err for nonexistent command, got nil")
	}
	if result.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", result.ExitCode)
	}
}

func TestExecNoThrow_Stderr(t *testing.T) {
	ctx := context.Background()
	// sh -c 'echo err >&2; exit 1' writes to stderr
	result := ExecNoThrow(ctx, "sh", "-c", "echo err >&2; exit 1")
	if !strings.Contains(result.Stderr, "err") {
		t.Errorf("Stderr = %q, expected 'err'", result.Stderr)
	}
}

func TestExecLines_Success(t *testing.T) {
	ctx := context.Background()
	lines, err := ExecLines(ctx, "printf", "a\nb\nc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 3 {
		t.Errorf("ExecLines: got %d lines, want 3", len(lines))
	}
	if lines[0] != "a" || lines[1] != "b" || lines[2] != "c" {
		t.Errorf("ExecLines: unexpected lines %v", lines)
	}
}

func TestExecLines_EmptyOutput(t *testing.T) {
	ctx := context.Background()
	lines, err := ExecLines(ctx, "true")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("ExecLines empty: got %v, want nil", lines)
	}
}

func TestExecLines_NonZeroExit(t *testing.T) {
	ctx := context.Background()
	_, err := ExecLines(ctx, "false")
	if err == nil {
		t.Error("ExecLines(false): expected error, got nil")
	}
}

func TestExecLines_BadCommand(t *testing.T) {
	ctx := context.Background()
	_, err := ExecLines(ctx, "__nonexistent_cmd_xyz__")
	if err == nil {
		t.Error("ExecLines bad cmd: expected error, got nil")
	}
}

func TestExecSilent_Success(t *testing.T) {
	ctx := context.Background()
	if !ExecSilent(ctx, "true") {
		t.Error("ExecSilent(true) = false, want true")
	}
}

func TestExecSilent_Failure(t *testing.T) {
	ctx := context.Background()
	if ExecSilent(ctx, "false") {
		t.Error("ExecSilent(false) = true, want false")
	}
}

func TestExecSilent_BadCommand(t *testing.T) {
	ctx := context.Background()
	if ExecSilent(ctx, "__nonexistent_cmd_xyz__") {
		t.Error("ExecSilent(nonexistent) = true, want false")
	}
}

func TestExecWithDir_Success(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	result := ExecWithDir(ctx, dir, "pwd")
	if result.Err != nil {
		t.Fatalf("unexpected Err: %v", result.Err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	// pwd output should be in or under the temp dir
	if !strings.Contains(strings.TrimSpace(result.Stdout), dir) &&
		!strings.Contains(dir, strings.TrimSpace(result.Stdout)) {
		// On some platforms (macOS) /var/folders may resolve via symlinks — just check non-empty
		if strings.TrimSpace(result.Stdout) == "" {
			t.Error("ExecWithDir pwd: stdout is empty")
		}
	}
}

func TestExecWithDir_BadCommand(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	result := ExecWithDir(ctx, dir, "__nonexistent_cmd_xyz__")
	if result.Err == nil {
		t.Error("expected Err for nonexistent command")
	}
}

func TestExecWithDir_NonZeroExit(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	// "false" exits with code 1 — exercises the ExitError branch in ExecWithDir
	result := ExecWithDir(ctx, dir, "false")
	if result.Err != nil {
		t.Fatalf("unexpected Err: %v", result.Err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code from 'false' in ExecWithDir")
	}
}

func TestExecWithTimeout_Completes(t *testing.T) {
	result := ExecWithTimeout(5*time.Second, "echo", "ok")
	if result.Err != nil {
		t.Fatalf("unexpected Err: %v", result.Err)
	}
	if !strings.Contains(result.Stdout, "ok") {
		t.Errorf("Stdout = %q, expected 'ok'", result.Stdout)
	}
}

func TestExecWithTimeout_Expires(t *testing.T) {
	// sleep 10 should be killed by the 50ms timeout
	result := ExecWithTimeout(50*time.Millisecond, "sleep", "10")
	// Should have a non-zero exit code or a non-nil Err
	if result.ExitCode == 0 && result.Err == nil {
		t.Error("expected timeout to cause non-zero exit or error")
	}
}
