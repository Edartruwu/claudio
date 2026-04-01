package styles

import "github.com/charmbracelet/lipgloss"

// Colors
var (
	Primary   = lipgloss.Color("#7C3AED") // purple
	Secondary = lipgloss.Color("#06B6D4") // cyan
	Success   = lipgloss.Color("#22C55E") // green
	Warning   = lipgloss.Color("#F59E0B") // amber
	Error     = lipgloss.Color("#EF4444") // red
	Muted     = lipgloss.Color("#6B7280") // gray
	Surface   = lipgloss.Color("#1F2937") // dark gray
	Text      = lipgloss.Color("#F9FAFB") // white
	Dim       = lipgloss.Color("#9CA3AF") // light gray
)

// Component styles
var (
	// App frame
	AppStyle = lipgloss.NewStyle().Padding(0, 1)

	// Status bar
	StatusBar = lipgloss.NewStyle().
			Background(lipgloss.Color("#374151")).
			Foreground(Text).
			Padding(0, 1)

	StatusLabel = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	StatusValue = lipgloss.NewStyle().
			Foreground(Dim)

	// Messages
	UserMessage = lipgloss.NewStyle().
			Foreground(Secondary).
			Bold(true)

	UserPrefix = lipgloss.NewStyle().
			Foreground(Secondary).
			Bold(true).
			SetString("❯ ")

	AssistantPrefix = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true).
			SetString("● ")

	ThinkingStyle = lipgloss.NewStyle().
			Foreground(Muted).
			Italic(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(Error).
			Bold(true)

	// Tool use
	ToolHeader = lipgloss.NewStyle().
			Foreground(Warning).
			Bold(true)

	ToolName = lipgloss.NewStyle().
			Foreground(Warning)

	ToolInput = lipgloss.NewStyle().
			Foreground(Dim)

	ToolResult = lipgloss.NewStyle().
			Foreground(Muted)

	ToolError = lipgloss.NewStyle().
			Foreground(Error)

	// Prompt
	PromptBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Primary).
			Padding(0, 1)

	PromptFocused = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7C3AED")).
			Padding(0, 1)

	PromptBlurred = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Muted).
			Padding(0, 1)

	// Permission dialog
	PermissionBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Warning).
			Padding(1, 2).
			Margin(1, 0)

	PermissionTitle = lipgloss.NewStyle().
			Foreground(Warning).
			Bold(true)

	PermissionAllow = lipgloss.NewStyle().
			Foreground(Success).
			Bold(true)

	PermissionDeny = lipgloss.NewStyle().
			Foreground(Error).
			Bold(true)

	// Spinner
	SpinnerStyle = lipgloss.NewStyle().
			Foreground(Primary)

	SpinnerText = lipgloss.NewStyle().
			Foreground(Dim)

	// Footer
	FooterPill = lipgloss.NewStyle().
			Background(lipgloss.Color("#374151")).
			Foreground(Dim).
			Padding(0, 1).
			Margin(0, 1)

	FooterPillActive = lipgloss.NewStyle().
			Background(Primary).
			Foreground(Text).
			Padding(0, 1).
			Margin(0, 1)

	// Separator
	Separator = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#374151"))
)

// SeparatorLine returns a horizontal line of the given width.
func SeparatorLine(width int) string {
	line := ""
	for i := 0; i < width; i++ {
		line += "─"
	}
	return Separator.Render(line)
}
