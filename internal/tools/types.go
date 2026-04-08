package tools

import (
	"context"
	"encoding/json"
)

// Tool defines the interface all tools must implement.
type Tool interface {
	// Name returns the tool's unique identifier.
	Name() string

	// Description returns a human-readable description for the AI.
	Description() string

	// InputSchema returns the JSON Schema for the tool's input parameters.
	InputSchema() json.RawMessage

	// Execute runs the tool with the given JSON input.
	Execute(ctx context.Context, input json.RawMessage) (*Result, error)

	// IsReadOnly returns true if the tool only reads and never modifies state.
	IsReadOnly() bool

	// RequiresApproval returns true if the tool needs user permission before execution.
	RequiresApproval(input json.RawMessage) bool
}

// Result holds the output of a tool execution.
type Result struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error,omitempty"`

	// InjectedMessages holds additional text messages to inject into the
	// conversation as a user turn immediately after the tool result block.
	// This mirrors claude-code's newMessages mechanism: skill content is
	// injected here so it becomes part of conversation history and persists
	// across compaction, rather than being a transient tool result.
	InjectedMessages []string
}

// ToolUse represents a tool invocation from the AI response.
type ToolUse struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolResult is sent back to the API after tool execution.
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// Validatable is an optional interface tools can implement to run lightweight
// pre-checks before the user is prompted for approval. This avoids wasting
// tokens on an approval dialog for a tool call that will certainly fail.
// Validate should only perform fast, non-destructive checks (e.g. cache
// lookups, input parsing). Return nil if validation passes.
type Validatable interface {
	Validate(input json.RawMessage) *Result
}

// DeferrableTool is an optional interface tools can implement to support deferred loading.
// Deferred tools are sent to the API with only their name (no description/schema),
// saving tokens. The model can fetch full schemas on demand via ToolSearch.
type DeferrableTool interface {
	// ShouldDefer returns true if this tool should be deferred (not sent with full schema).
	ShouldDefer() bool
	// SearchHint returns a 3-10 word hint for ToolSearch keyword matching.
	SearchHint() string
}

// AutoActivatable is an optional interface for deferrable tools that should
// auto-activate (skip deferral) when their backing service is available.
// For example, the LSP tool auto-activates when an LSP server is configured
// for the project's language.
type AutoActivatable interface {
	// AutoActivate returns true if this tool should be sent with full schema
	// even though it is normally deferred.
	AutoActivate() bool
}

// APIToolDef is the format the Anthropic API expects for tool definitions.
type APIToolDef struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"input_schema"`
	DeferLoading bool            `json:"defer_loading,omitempty"`
}

// SubAgentObserver receives forwarded events from sub-agent execution.
// Defined in tools to avoid circular imports with query.
type SubAgentObserver interface {
	OnSubAgentToolStart(agentDesc string, tu ToolUse)
	OnSubAgentToolEnd(agentDesc string, tu ToolUse, result *Result)
	OnSubAgentText(agentDesc string, text string)
}

type subAgentObserverKey struct{}

// WithSubAgentObserver injects a SubAgentObserver into the context.
func WithSubAgentObserver(ctx context.Context, obs SubAgentObserver) context.Context {
	return context.WithValue(ctx, subAgentObserverKey{}, obs)
}

// GetSubAgentObserver retrieves the SubAgentObserver from context, or nil.
func GetSubAgentObserver(ctx context.Context) SubAgentObserver {
	if obs, ok := ctx.Value(subAgentObserverKey{}).(SubAgentObserver); ok {
		return obs
	}
	return nil
}

// SecurityChecker validates file paths and commands against security policies.
type SecurityChecker interface {
	CheckPath(path string) error
	CheckCommand(cmd string) error
}
