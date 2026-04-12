package templates

// ProjectInfo represents a project for the project browser.
type ProjectInfo struct {
	Path        string
	Name        string
	Initialized bool
}

// ChatMessage represents a message in the chat history.
type ChatMessage struct {
	Role     string // "user", "assistant", or "tool"
	Content  string
	ToolName string // tool name for Role="tool"
	ToolOut  string // tool output for Role="tool"
}

// SessionInfo represents a session summary for the sidebar.
type SessionInfo struct {
	ID       string
	Title    string
	State    string // "idle", "streaming", "approval"
	MsgCount int
	Active   bool // currently selected
}

// PanelData holds data for the side panels.
type PanelData struct {
	// Analytics
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CacheRead    int
	CacheCreate  int
	Cost         string // formatted cost string

	// Config
	Model          string
	PermissionMode string
	ProjectPath    string

	// Tasks
	Tasks []TaskInfo

	// Tools
	Tools     []ToolInfo
	SessionID string
}

// ToolInfo represents a single tool row in the tools manager panel.
type ToolInfo struct {
	Name       string
	Hint       string
	Deferred   bool // current effective state
	Deferrable bool // false = always-eager core tool (cannot be toggled)
	Overridden bool // user has set an explicit override
}

// TaskInfo represents a task for the tasks panel.
type TaskInfo struct {
	ID          string
	Title       string
	Status      string // "pending", "running", "done", "failed"
	Description string
}

// AgentInfo represents an active agent in the agents panel.
type AgentInfo struct {
	ID    string // agent type/name
	Name  string // display name
	Model string // model name
	Status string // "running", "idle", "done", "error"
}
