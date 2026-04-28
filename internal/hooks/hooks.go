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
	"sync"
	"time"
)

// Event represents a lifecycle event type.
type Event string

var (
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
	// PostCompact fires after conversation compaction completes.
	PostCompact Event = "PostCompact"
	// SubagentStart fires before a sub-agent is launched.
	SubagentStart Event = "SubagentStart"
	// SubagentStop fires after a sub-agent completes.
	SubagentStop Event = "SubagentStop"
	// UserPromptSubmit fires before processing user input.
	UserPromptSubmit Event = "UserPromptSubmit"
	// TaskCreated fires when a new task is created.
	TaskCreated Event = "TaskCreated"
	// TaskCompleted fires when a task is marked complete.
	TaskCompleted Event = "TaskCompleted"
	// WorktreeCreate fires when a git worktree is created.
	WorktreeCreate Event = "WorktreeCreate"
	// WorktreeRemove fires when a git worktree is removed.
	WorktreeRemove Event = "WorktreeRemove"
	// ConfigChange fires when a settings value is changed.
	ConfigChange Event = "ConfigChange"
	// CwdChanged fires when the working directory changes.
	CwdChanged Event = "CwdChanged"
	// FileChanged fires when a watched file is modified.
	FileChanged Event = "FileChanged"
	// Notification fires when a system notification is triggered.
	Notification Event = "Notification"
)

// EventTypeInfo describes a registered hook event type.
type EventTypeInfo struct {
	Name        string
	Description string
	BuiltIn     bool // true for Claudio core events
}

// eventRegistry holds all known hook event types.
type eventRegistry struct {
	mu    sync.RWMutex
	types map[string]*EventTypeInfo
}

func newEventRegistry() *eventRegistry {
	return &eventRegistry{types: make(map[string]*EventTypeInfo)}
}

func (r *eventRegistry) register(name, description string, builtin bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.types[name] = &EventTypeInfo{Name: name, Description: description, BuiltIn: builtin}
}

func (r *eventRegistry) isKnown(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.types[name]
	return ok
}

func (r *eventRegistry) all() []*EventTypeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*EventTypeInfo, 0, len(r.types))
	for _, v := range r.types {
		out = append(out, v)
	}
	return out
}

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
	PostCompact        []HookMatcher `json:"PostCompact,omitempty"`
	SessionStart       []HookMatcher `json:"SessionStart,omitempty"`
	SessionEnd         []HookMatcher `json:"SessionEnd,omitempty"`
	Stop               []HookMatcher `json:"Stop,omitempty"`
	SubagentStart      []HookMatcher `json:"SubagentStart,omitempty"`
	SubagentStop       []HookMatcher `json:"SubagentStop,omitempty"`
	UserPromptSubmit   []HookMatcher `json:"UserPromptSubmit,omitempty"`
	TaskCreated        []HookMatcher `json:"TaskCreated,omitempty"`
	TaskCompleted      []HookMatcher `json:"TaskCompleted,omitempty"`
	WorktreeCreate     []HookMatcher `json:"WorktreeCreate,omitempty"`
	WorktreeRemove     []HookMatcher `json:"WorktreeRemove,omitempty"`
	ConfigChange       []HookMatcher `json:"ConfigChange,omitempty"`
	CwdChanged         []HookMatcher `json:"CwdChanged,omitempty"`
	FileChanged        []HookMatcher `json:"FileChanged,omitempty"`
	Notification       []HookMatcher `json:"Notification,omitempty"`
}

// HookContext provides data to hook scripts via environment variables.
type HookContext struct {
	Event        Event  `json:"event"`
	ToolName     string `json:"tool_name,omitempty"`
	ToolInput    string `json:"tool_input,omitempty"`
	ToolOutput   string `json:"tool_output,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	Model        string `json:"model,omitempty"`
	CWD          string `json:"cwd,omitempty"`
	TaskID       string `json:"task_id,omitempty"`
	WorktreePath string `json:"worktree_path,omitempty"`
	ConfigKey    string `json:"config_key,omitempty"`
	FilePath     string `json:"file_path,omitempty"`
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

// InlineHookFn is a Go function registered as a hook handler (e.g. from Lua plugins).
// Unlike shell-command hooks, inline hooks run in-process.
type InlineHookFn func(ctx context.Context, hctx HookContext) error

// inlineHook pairs a matcher pattern with a Go function.
type inlineHook struct {
	event   Event
	matcher string
	fn      InlineHookFn
}

// Manager loads and executes hooks.
type Manager struct {
	config      HooksConfig
	enabled     bool
	inlineHooks []inlineHook
	eventTypes  *eventRegistry
}

// NewManager creates a hooks manager from a configuration.
func NewManager(config HooksConfig) *Manager {
	m := &Manager{
		config:     config,
		enabled:    true,
		eventTypes: newEventRegistry(),
	}
	// Pre-register all built-in event types.
	m.eventTypes.register(string(PreToolUse), "Fires before any tool executes", true)
	m.eventTypes.register(string(PostToolUse), "Fires after a tool completes", true)
	m.eventTypes.register(string(PostToolUseFailure), "Fires when a tool fails", true)
	m.eventTypes.register(string(PreCompact), "Fires before conversation compaction", true)
	m.eventTypes.register(string(PostCompact), "Fires after conversation compaction completes", true)
	m.eventTypes.register(string(SessionStart), "Fires when a new session begins", true)
	m.eventTypes.register(string(SessionEnd), "Fires when a session ends", true)
	m.eventTypes.register(string(Stop), "Fires when the assistant finishes responding", true)
	m.eventTypes.register(string(SubagentStart), "Fires before a sub-agent is launched", true)
	m.eventTypes.register(string(SubagentStop), "Fires after a sub-agent completes", true)
	m.eventTypes.register(string(UserPromptSubmit), "Fires before processing user input", true)
	m.eventTypes.register(string(TaskCreated), "Fires when a new task is created", true)
	m.eventTypes.register(string(TaskCompleted), "Fires when a task is marked complete", true)
	m.eventTypes.register(string(WorktreeCreate), "Fires when a git worktree is created", true)
	m.eventTypes.register(string(WorktreeRemove), "Fires when a git worktree is removed", true)
	m.eventTypes.register(string(ConfigChange), "Fires when a settings value is changed", true)
	m.eventTypes.register(string(CwdChanged), "Fires when the working directory changes", true)
	m.eventTypes.register(string(FileChanged), "Fires when a watched file is modified", true)
	m.eventTypes.register(string(Notification), "Fires when a system notification is triggered", true)
	return m
}

// RegisterEventType registers a new hook event type (e.g. for plugins).
// Built-in types are pre-registered at Manager construction.
func (m *Manager) RegisterEventType(name, description string) {
	m.eventTypes.register(name, description, false)
}

// GetRegisteredEventTypes returns all known event types (built-in + plugin).
func (m *Manager) GetRegisteredEventTypes() []*EventTypeInfo {
	return m.eventTypes.all()
}

// RegisterInlineHook registers a Go function as a hook handler.
// Used by Lua plugins to register hooks without shell commands.
func (m *Manager) RegisterInlineHook(event Event, matcher string, fn InlineHookFn) {
	m.inlineHooks = append(m.inlineHooks, inlineHook{
		event:   event,
		matcher: matcher,
		fn:      fn,
	})
}

// RegisterSkillHooks appends skill-defined hooks to the manager at runtime.
// eventType must match a HooksConfig field name: "PreToolUse", "PostToolUse",
// "PostToolUseFailure", "PreCompact", "PostCompact", "SessionStart", "SessionEnd", "Stop".
func (m *Manager) RegisterSkillHooks(eventType, matcher string, defs []HookDef) {
	hm := HookMatcher{Matcher: matcher, Hooks: defs}
	switch eventType {
	case "PreToolUse":
		m.config.PreToolUse = append(m.config.PreToolUse, hm)
	case "PostToolUse":
		m.config.PostToolUse = append(m.config.PostToolUse, hm)
	case "PostToolUseFailure":
		m.config.PostToolUseFailure = append(m.config.PostToolUseFailure, hm)
	case "PreCompact":
		m.config.PreCompact = append(m.config.PreCompact, hm)
	case "PostCompact":
		m.config.PostCompact = append(m.config.PostCompact, hm)
	case "SessionStart":
		m.config.SessionStart = append(m.config.SessionStart, hm)
	case "SessionEnd":
		m.config.SessionEnd = append(m.config.SessionEnd, hm)
	case "Stop":
		m.config.Stop = append(m.config.Stop, hm)
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

	// Run inline hooks (registered by Lua plugins, etc.)
	for _, ih := range m.inlineHooks {
		if ih.event != event {
			continue
		}
		if !matchTool(ih.matcher, hctx.ToolName) {
			continue
		}
		if err := ih.fn(ctx, hctx); err != nil {
			results = append(results, HookResult{
				HookID:  "inline",
				Stderr:  err.Error(),
				Error:   err,
			})
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
		"CLAUDIO_TASK_ID="+hctx.TaskID,
		"CLAUDIO_WORKTREE_PATH="+hctx.WorktreePath,
		"CLAUDIO_CONFIG_KEY="+hctx.ConfigKey,
		"CLAUDIO_FILE_PATH="+hctx.FilePath,
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
	case PostCompact:
		return m.config.PostCompact
	case SessionStart:
		return m.config.SessionStart
	case SessionEnd:
		return m.config.SessionEnd
	case Stop:
		return m.config.Stop
	case SubagentStart:
		return m.config.SubagentStart
	case SubagentStop:
		return m.config.SubagentStop
	case UserPromptSubmit:
		return m.config.UserPromptSubmit
	case TaskCreated:
		return m.config.TaskCreated
	case TaskCompleted:
		return m.config.TaskCompleted
	case WorktreeCreate:
		return m.config.WorktreeCreate
	case WorktreeRemove:
		return m.config.WorktreeRemove
	case ConfigChange:
		return m.config.ConfigChange
	case CwdChanged:
		return m.config.CwdChanged
	case FileChanged:
		return m.config.FileChanged
	case Notification:
		return m.config.Notification
	default:
		return nil
	}
}

// matchTool checks if a tool name matches a matcher pattern.
// Patterns may use "|" to specify alternatives (e.g. "Write|Edit|NotebookEdit"),
// and each alternative supports filepath.Match glob syntax.
func matchTool(pattern, toolName string) bool {
	if pattern == "*" || pattern == "" {
		return true
	}
	for _, alt := range strings.Split(pattern, "|") {
		alt = strings.TrimSpace(alt)
		if alt == "" {
			continue
		}
		if alt == "*" || alt == toolName {
			return true
		}
		if matched, _ := filepath.Match(alt, toolName); matched {
			return true
		}
	}
	return false
}

func mergeConfigs(base, overlay HooksConfig) HooksConfig {
	base.PreToolUse = append(base.PreToolUse, overlay.PreToolUse...)
	base.PostToolUse = append(base.PostToolUse, overlay.PostToolUse...)
	base.PostToolUseFailure = append(base.PostToolUseFailure, overlay.PostToolUseFailure...)
	base.PreCompact = append(base.PreCompact, overlay.PreCompact...)
	base.PostCompact = append(base.PostCompact, overlay.PostCompact...)
	base.SessionStart = append(base.SessionStart, overlay.SessionStart...)
	base.SessionEnd = append(base.SessionEnd, overlay.SessionEnd...)
	base.Stop = append(base.Stop, overlay.Stop...)
	base.SubagentStart = append(base.SubagentStart, overlay.SubagentStart...)
	base.SubagentStop = append(base.SubagentStop, overlay.SubagentStop...)
	base.UserPromptSubmit = append(base.UserPromptSubmit, overlay.UserPromptSubmit...)
	base.TaskCreated = append(base.TaskCreated, overlay.TaskCreated...)
	base.TaskCompleted = append(base.TaskCompleted, overlay.TaskCompleted...)
	base.WorktreeCreate = append(base.WorktreeCreate, overlay.WorktreeCreate...)
	base.WorktreeRemove = append(base.WorktreeRemove, overlay.WorktreeRemove...)
	base.ConfigChange = append(base.ConfigChange, overlay.ConfigChange...)
	base.CwdChanged = append(base.CwdChanged, overlay.CwdChanged...)
	base.FileChanged = append(base.FileChanged, overlay.FileChanged...)
	base.Notification = append(base.Notification, overlay.Notification...)
	return base
}
