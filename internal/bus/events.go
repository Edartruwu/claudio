package bus

// Event type constants
const (
	// Session lifecycle
	EventSessionStart   = "session.start"
	EventSessionEnd     = "session.end"
	EventSessionCompact = "session.compact"

	// Messages
	EventMessageUser      = "message.user"
	EventMessageAssistant = "message.assistant"
	EventMessageSystem    = "message.system"

	// Streaming
	EventStreamStart = "stream.start"
	EventStreamChunk = "stream.chunk"
	EventStreamDone  = "stream.done"
	EventStreamError = "stream.error"

	// Tool execution
	EventToolStart      = "tool.start"
	EventToolEnd        = "tool.end"
	EventToolPermission = "tool.permission"

	// Auth
	EventAuthLogin  = "auth.login"
	EventAuthLogout = "auth.logout"
	EventAuthRefresh = "auth.refresh"

	// MCP
	EventMCPConnect    = "mcp.connect"
	EventMCPDisconnect = "mcp.disconnect"
	EventMCPToolCall   = "mcp.tool_call"

	// Learning
	EventInstinctLearned = "instinct.learned"
	EventInstinctEvolved = "instinct.evolved"

	// Rate limits
	EventRateLimitChanged = "ratelimit.changed"

	// Audit
	EventAuditEntry = "audit.entry"

	// Background tasks
	EventBgTaskComplete = "task.bg_complete"
)

// BgTaskCompletePayload is published when a background task reaches a terminal state.
type BgTaskCompletePayload struct {
	TaskID     string `json:"task_id"`
	Output     string `json:"output"`
	ExitCode   int    `json:"exit_code"`
	Err        string `json:"error,omitempty"`
	IsSubAgent bool   `json:"is_sub_agent,omitempty"`
}
