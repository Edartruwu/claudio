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

// DeferrableTool is an optional interface tools can implement to support deferred loading.
// Deferred tools are sent to the API with only their name (no description/schema),
// saving tokens. The model can fetch full schemas on demand via ToolSearch.
type DeferrableTool interface {
	// ShouldDefer returns true if this tool should be deferred (not sent with full schema).
	ShouldDefer() bool
	// SearchHint returns a 3-10 word hint for ToolSearch keyword matching.
	SearchHint() string
}

// APIToolDef is the format the Anthropic API expects for tool definitions.
type APIToolDef struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"input_schema"`
	DeferLoading bool            `json:"defer_loading,omitempty"`
}

// SecurityChecker validates file paths and commands against security policies.
type SecurityChecker interface {
	CheckPath(path string) error
	CheckCommand(cmd string) error
}
