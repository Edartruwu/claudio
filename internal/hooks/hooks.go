// Package hooks provides lifecycle event hooks for Claudio.
// Hooks allow shell commands to execute at specific lifecycle stages,
// enabling automation, security checks, and custom workflows.
package hooks

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

// Event represents a lifecycle event type.
type Event string

const (
	// PreToolUse fires before a tool is executed.
	PreToolUse Event = "PreToolUse"
	// PostToolUse fires after a tool completes.
	PostToolUse Event = "PostToolUse"
	// PostToolUseFailure fires when a tool fails.
	PostToolUseFailure Event = "PostToolUseFailure"
	// PreCompact fires before conversation compaction.
	PreCompact Event = "PreCompact"
	// SessionStart fires when a new session begins.
	SessionStart Event = "SessionStart"
	// SessionEnd fires when a session ends.
	SessionEnd Event = "SessionEnd"
	// Stop fires when the assistant finishes responding.
	Stop Event = "Stop"
)

// HookDef defines a single hook configuration.
type HookDef struct {
	ID          string `json:"id,omitempty"`
	Type        string `json:"type"`    // "command"
	Command     string `json:"command"` // shell command to execute
	Timeout     int    `json:"timeout,omitempty"` // milliseconds, default 10000
	Async       bool   `json:"async,omitempty"`   // non-blocking execution
	Description string `json:"description,omitempty"`
}

// HookMatcher associates a tool/event matcher with hooks.
type HookMatcher struct {
	Matcher string    `json:"matcher"` // tool name glob or "*"
	Hooks   []HookDef `json:"hooks"`
}

// HooksConfig is the full hooks configuration.
type HooksConfig struct {
	PreToolUse         []HookMatcher `json:"PreToolUse,omitempty"`
	PostToolUse        []HookMatcher `json:"PostToolUse,omitempty"`
	PostToolUseFailure []HookMatcher `json:"PostToolUseFailure,omitempty"`
	PreCompact         []HookMatcher `json:"PreCompact,omitempty"`
	SessionStart       []HookMatcher `json:"SessionStart,omitempty"`
	SessionEnd         []HookMatcher `json:"SessionEnd,omitempty"`
	Stop               []HookMatcher `json:"Stop,omitempty"`
}

// HookContext provides data to hook scripts via environment variables.
type HookContext struct {
	Event     Event  `json:"event"`
	ToolName  string `json:"tool_name,omitempty"`
	ToolInput string `json:"tool_input,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Model     string `json:"model,omitempty"`
	CWD       string `json:"cwd,omitempty"`
}

// HookResult holds the result of running a hook.
type HookResult struct {
	HookID   string
	Command  string
	ExitCode int
	Stdout   string
	Stderr   string
	Blocked  bool // true if hook returned exit code 1 (blocking)
	Error    error
}

// Manager loads and executes hooks.
type Manager struct {
	config  HooksConfig
	enabled bool
}

// NewManager creates a hooks manager from a configuration.
func NewManager(config HooksConfig) *Manager {
	return &Manager{
		config:  config,
		enabled: true,
	}
}

// LoadFromFile loads hooks configuration from a JSON file.
func LoadFromFile(path string) (*Manager, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manager{enabled: false}, nil
		}
		return nil, fmt.Errorf("reading hooks config: %w", err)
	}

	// The file might have hooks nested under a "hooks" key
	var wrapper struct {
		Hooks HooksConfig `json:"hooks"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		// Try direct parse
		var config HooksConfig
		if err2 := json.Unmarshal(data, &config); err2 != nil {
			return nil, fmt.Errorf("parsing hooks config: %w", err)
		}
		return NewManager(config), nil
	}
	return NewManager(wrapper.Hooks), nil
}

// LoadFromSettings loads hooks from settings.json files.
func LoadFromSettings(settingsPaths ...string) *Manager {
	merged := HooksConfig{}
	for _, path := range settingsPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var settings struct {
			Hooks HooksConfig `json:"hooks"`
		}
		if err := json.Unmarshal(data, &settings); err != nil {
			continue
		}
		merged = mergeConfigs(merged, settings.Hooks)
	}
	return NewManager(merged)
}

// Run executes all hooks matching the given event and tool name.
// Returns results and whether any hook blocked the action (exit code 1).
func (m *Manager) Run(ctx context.Context, event Event, hctx HookContext) ([]HookResult, bool) {
	if !m.enabled {
		return nil, false
	}

	matchers := m.matchersForEvent(event)
	if len(matchers) == 0 {
		return nil, false
	}

	var results []HookResult
	blocked := false

	for _, matcher := range matchers {
		if !matchTool(matcher.Matcher, hctx.ToolName) {
			continue
		}
		for _, hook := range matcher.Hooks {
			if hook.Type != "command" || hook.Command == "" {
				continue
			}

			if hook.Async {
				go m.executeHook(context.Background(), hook, hctx)
				continue
			}

			result := m.executeHook(ctx, hook, hctx)
			results = append(results, result)

			if result.ExitCode == 1 {
				blocked = true
			}
		}
	}

	return results, blocked
}

func (m *Manager) executeHook(ctx context.Context, hook HookDef, hctx HookContext) HookResult {
	timeout := 10 * time.Second
	if hook.Timeout > 0 {
		timeout = time.Duration(hook.Timeout) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", hook.Command)

	// Set environment variables for the hook
	cmd.Env = append(os.Environ(),
		"CLAUDIO_EVENT="+string(hctx.Event),
		"CLAUDIO_TOOL_NAME="+hctx.ToolName,
		"CLAUDIO_SESSION_ID="+hctx.SessionID,
		"CLAUDIO_MODEL="+hctx.Model,
	)

	if hctx.CWD != "" {
		cmd.Dir = hctx.CWD
	}

	// Pass tool input via stdin
	if hctx.ToolInput != "" {
		cmd.Stdin = strings.NewReader(hctx.ToolInput)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := HookResult{
		HookID:  hook.ID,
		Command: hook.Command,
		Stdout:  stdout.String(),
		Stderr:  stderr.String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			if result.ExitCode == 1 {
				result.Blocked = true
			}
		} else {
			result.Error = err
			result.ExitCode = -1
		}
	}

	return result
}

func (m *Manager) matchersForEvent(event Event) []HookMatcher {
	switch event {
	case PreToolUse:
		return m.config.PreToolUse
	case PostToolUse:
		return m.config.PostToolUse
	case PostToolUseFailure:
		return m.config.PostToolUseFailure
	case PreCompact:
		return m.config.PreCompact
	case SessionStart:
		return m.config.SessionStart
	case SessionEnd:
		return m.config.SessionEnd
	case Stop:
		return m.config.Stop
	default:
		return nil
	}
}

// matchTool checks if a tool name matches a matcher pattern.
func matchTool(pattern, toolName string) bool {
	if pattern == "*" || pattern == "" {
		return true
	}
	matched, _ := filepath.Match(pattern, toolName)
	return matched || pattern == toolName
}

func mergeConfigs(base, overlay HooksConfig) HooksConfig {
	base.PreToolUse = append(base.PreToolUse, overlay.PreToolUse...)
	base.PostToolUse = append(base.PostToolUse, overlay.PostToolUse...)
	base.PostToolUseFailure = append(base.PostToolUseFailure, overlay.PostToolUseFailure...)
	base.PreCompact = append(base.PreCompact, overlay.PreCompact...)
	base.SessionStart = append(base.SessionStart, overlay.SessionStart...)
	base.SessionEnd = append(base.SessionEnd, overlay.SessionEnd...)
	base.Stop = append(base.Stop, overlay.Stop...)
	return base
}
