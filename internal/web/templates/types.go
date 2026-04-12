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

// ToolCatalogInfo represents a tool for the tools catalog page.
type ToolCatalogInfo struct {
	Name        string
	Description string
	Category    string // "core", "deferred", "optional", etc.
}

// MemoryEntryInfo represents a memory entry for the memory page.
type MemoryEntryInfo struct {
	Key   string // memory name
	Value string // content preview (truncated)
	Scope string // "session", "agent", "global"
}

// ConfigDisplaySection represents a section of config for display.
type ConfigDisplaySection struct {
	Name  string // "Model", "Permissions", "Storage"
	Items []ConfigItem
}

// ConfigItem represents a single config key-value pair.
type ConfigItem struct {
	Key   string
	Value string
}

// AgentOption represents an agent type option in the picker.
type AgentOption struct {
	ID          string
	Name        string
	Description string
}

// TeamOption represents a team template option in the picker.
type TeamOption struct {
	ID          string
	Name        string
	Description string
}
