package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Gruvbox Dark palette
var (
	Primary    = lipgloss.Color("#d3869b") // gruvbox purple - assistant, focus
	Secondary  = lipgloss.Color("#83a598") // gruvbox blue - user input
	Success    = lipgloss.Color("#b8bb26") // gruvbox green - tool success, allow
	Warning    = lipgloss.Color("#fabd2f") // gruvbox yellow - tool calls, permissions
	Error      = lipgloss.Color("#fb4934") // gruvbox red - errors, deny
	Muted      = lipgloss.Color("#928374") // gruvbox gray - secondary text
	Surface    = lipgloss.Color("#282828") // gruvbox bg - backgrounds
	SurfaceAlt = lipgloss.Color("#3c3836") // gruvbox bg1 - pills, borders
	Text       = lipgloss.Color("#ebdbb2") // gruvbox fg - primary text
	Dim        = lipgloss.Color("#bdae93") // gruvbox fg2 - tertiary text
	Subtle     = lipgloss.Color("#504945") // gruvbox bg2 - tree lines
	Orange     = lipgloss.Color("#fe8019") // gruvbox orange - inline code, accents
	Aqua       = lipgloss.Color("#8ec07c") // gruvbox aqua - headings, decorators
)

// ── Messages ──────────────────────────────────────────────

var (
	UserPrefix = lipgloss.NewStyle().
			Foreground(Secondary).
			Bold(true)

	UserContent = lipgloss.NewStyle().
			Foreground(Text).
			PaddingLeft(1)

	UserBlock = lipgloss.NewStyle().
			BorderStyle(lipgloss.Border{Left: "▌"}).
			BorderLeft(true).
			BorderForeground(Secondary).
			PaddingLeft(1)

	AssistantPrefix = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	// Tool calls
	ToolIcon = lipgloss.NewStyle().
			Foreground(Warning)

	ToolName = lipgloss.NewStyle().
			Foreground(Warning).
			Bold(true)

	ToolSummary = lipgloss.NewStyle().
			Foreground(Dim)

	ToolSuccess = lipgloss.NewStyle().
			Foreground(Success)

	ToolError = lipgloss.NewStyle().
			Foreground(Error).
			Bold(true)

	ToolConnector = lipgloss.NewStyle().
			Foreground(Subtle)

	ToolFilePath = lipgloss.NewStyle().
			Foreground(Aqua)

	ToolDescription = lipgloss.NewStyle().
			Foreground(Muted).
			Italic(true)

	ToolDiffOld = lipgloss.NewStyle().
			Foreground(Error)

	ToolDiffNew = lipgloss.NewStyle().
			Foreground(Success)

	ToolExpandHint = lipgloss.NewStyle().
			Foreground(Subtle).
			Italic(true)

	ToolResultPreview = lipgloss.NewStyle().
			Foreground(Muted)

	ViewportCursor = lipgloss.NewStyle().
			Foreground(Warning).
			Bold(true)

	PinIcon = lipgloss.NewStyle().
		Foreground(Orange).
		Bold(true)

	// Other message types
	ThinkingStyle = lipgloss.NewStyle().
			Foreground(Muted).
			Italic(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(Error).
			Bold(true)

	SystemStyle = lipgloss.NewStyle().
			Foreground(Muted)
)

// ── Prompt ────────────────────────────────────────────────

var (
	PromptBarFocused = lipgloss.NewStyle().
				BorderStyle(lipgloss.Border{Left: "▌"}).
				BorderLeft(true).
				BorderForeground(Primary).
				PaddingLeft(1)

	PromptBarBlurred = lipgloss.NewStyle().
				BorderStyle(lipgloss.Border{Left: "▎"}).
				BorderLeft(true).
				BorderForeground(Muted).
				PaddingLeft(1)

	PromptHint = lipgloss.NewStyle().
			Foreground(Subtle).
			Italic(true).
			PaddingLeft(3)
)

// ── Status Bar ────────────────────────────────────────────

var (
	StatusBar = lipgloss.NewStyle().
			Background(SurfaceAlt).
			Foreground(Dim).
			Padding(0, 1)

	StatusModel = lipgloss.NewStyle().
			Foreground(Text).
			Bold(true)

	StatusSeparator = lipgloss.NewStyle().
			Foreground(Subtle)

	StatusHint = lipgloss.NewStyle().
			Foreground(Dim)

	StatusActive = lipgloss.NewStyle().
			Foreground(Warning)
)

// ── Overlays / Dialogs ───────────────────────────────────

var (
	PermissionBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Warning).
			Padding(1, 2)

	PermissionTitle = lipgloss.NewStyle().
			Foreground(Warning).
			Bold(true)

	ButtonAllow = lipgloss.NewStyle().
			Background(Success).
			Foreground(Surface).
			Bold(true).
			Padding(0, 1)

	ButtonDeny = lipgloss.NewStyle().
			Background(Error).
			Foreground(Text).
			Bold(true).
			Padding(0, 1)

	ButtonAllowAlways = lipgloss.NewStyle().
				Background(Primary).
				Foreground(Text).
				Bold(true).
				Padding(0, 1)

	ButtonInactive = lipgloss.NewStyle().
			Foreground(Dim).
			Padding(0, 1)

	DialogBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Primary).
			Padding(1, 2)
)

// ── Spinner ──────────────────────────────────────────────

var (
	SpinnerStyle = lipgloss.NewStyle().
			Foreground(Primary)

	SpinnerText = lipgloss.NewStyle().
			Foreground(Dim)

	SpinnerTimer = lipgloss.NewStyle().
			Foreground(Muted)
)

// ── Palette / Picker ─────────────────────────────────────

var (
	PaletteItem = lipgloss.NewStyle().
			Foreground(Dim)

	PaletteItemSelected = lipgloss.NewStyle().
				Foreground(Text).
				Bold(true)

	PaletteDesc = lipgloss.NewStyle().
			Foreground(Subtle)

	PaletteDescSelected = lipgloss.NewStyle().
				Foreground(Dim)

	PalettePrefix = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	PickerAdd = lipgloss.NewStyle().
			Foreground(Success).
			Bold(true)
)

// ── Footer (legacy compat) ───────────────────────────────

var (
	FooterPill = lipgloss.NewStyle().
			Background(SurfaceAlt).
			Foreground(Dim).
			Padding(0, 1).
			Margin(0, 1)

	FooterPillActive = lipgloss.NewStyle().
				Background(Primary).
				Foreground(Text).
				Padding(0, 1).
				Margin(0, 1)
)

// ── Panels ──────────────────────────────────────────────

var (
	PanelBorder = lipgloss.NewStyle().
			Border(lipgloss.Border{
			Top:         "─",
			Bottom:      "─",
			Left:        "│",
			Right:       "│",
			TopLeft:     "╭",
			TopRight:    "╮",
			BottomLeft:  "╰",
			BottomRight: "╯",
		}).
		BorderForeground(SurfaceAlt)

	PanelTitle = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true).
			Padding(0, 1)

	PanelItem = lipgloss.NewStyle().
			Foreground(Dim).
			PaddingLeft(2)

	PanelItemSelected = lipgloss.NewStyle().
				Foreground(Text).
				Bold(true).
				PaddingLeft(1)

	PanelBadge = lipgloss.NewStyle().
			Foreground(Surface).
			Background(Primary).
			Padding(0, 1)

	PanelBadgeUser = lipgloss.NewStyle().
			Foreground(Surface).
			Background(Secondary)

	PanelBadgeProject = lipgloss.NewStyle().
				Foreground(Surface).
				Background(Orange)

	PanelBadgeBundled = lipgloss.NewStyle().
				Foreground(Surface).
				Background(Aqua)

	PanelHint = lipgloss.NewStyle().
			Foreground(Subtle).
			Italic(true).
			PaddingLeft(2)

	PanelSearch = lipgloss.NewStyle().
			Foreground(Warning).
			Bold(true)
)

// ── Utilities ────────────────────────────────────────────

// SeparatorLine returns a horizontal line of the given width.
func SeparatorLine(width int) string {
	return lipgloss.NewStyle().
		Foreground(SurfaceAlt).
		Render(strings.Repeat("─", width))
}

// Sep returns a styled │ separator for the status bar.
func Sep() string {
	return StatusSeparator.Render(" │ ")
}
