package templates

// ProjectInfo represents a project for the project browser.
type ProjectInfo struct {
	Path        string
	Name        string
	Initialized bool
}

// ChatMessage represents a message in the chat history.
type ChatMessage struct {
	Role    string // "user" or "assistant"
	Content string
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
}

// TaskInfo represents a task for the tasks panel.
type TaskInfo struct {
	ID          string
	Title       string
	Status      string // "pending", "running", "done", "failed"
	Description string
}
