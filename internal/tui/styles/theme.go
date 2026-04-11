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

	ToolBadge = lipgloss.NewStyle().
			Foreground(Orange).
			Bold(true)

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

	// Thinking message styles (cached for performance in hot render path)
	ThinkingHeaderStyle = lipgloss.NewStyle().
			Foreground(Orange).
			Italic(true).
			Bold(true)

	ThinkingHintStyle = lipgloss.NewStyle().
			Foreground(Subtle).
			Italic(true)

	ThinkingBoxCollapsed = lipgloss.NewStyle().
			BorderStyle(lipgloss.Border{Left: "▌"}).
			BorderLeft(true).
			BorderForeground(Orange).
			PaddingLeft(1)

	ThinkingBoxExpanded = lipgloss.NewStyle().
			BorderStyle(lipgloss.Border{Left: "▌"}).
			BorderLeft(true).
			BorderForeground(Orange).
			PaddingLeft(1)

	ThinkingBodyStyle = lipgloss.NewStyle().
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
			Foreground(Muted).
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

	// Status bar vim mode styles (cached for performance in hot render path)
	StatusVimModeNormal = lipgloss.NewStyle().
			Background(Secondary).
			Foreground(Surface).
			Bold(true).
			Padding(0, 1)

	StatusVimModeVisual = lipgloss.NewStyle().
			Background(Primary).
			Foreground(Surface).
			Bold(true).
			Padding(0, 1)

	StatusVimModeViewport = lipgloss.NewStyle().
			Background(Warning).
			Foreground(Surface).
			Bold(true).
			Padding(0, 1)

	StatusVimModeInsert = lipgloss.NewStyle().
			Background(Success).
			Foreground(Surface).
			Bold(true).
			Padding(0, 1)

	// Status bar inline styles (cached for performance in hot render path)
	StatusSessionStyle = lipgloss.NewStyle().
			Foreground(Aqua)

	StatusPanelStyle = lipgloss.NewStyle().
			Foreground(Primary)

	StatusBackgroundSessions = lipgloss.NewStyle().
			Foreground(Warning).
			Bold(true)

	StatusTeamStyle = lipgloss.NewStyle().
			Foreground(Warning)

	StatusMailStyle = lipgloss.NewStyle().
			Foreground(Warning).
			Bold(true)

	StatusOverageStyle = lipgloss.NewStyle().
			Foreground(Warning).
			Bold(true)

	StatusRateLimitError = lipgloss.NewStyle().
			Foreground(Error).
			Bold(true)

	StatusRateLimitWarning = lipgloss.NewStyle().
			Foreground(Warning)

	// Context bar styles (cached for performance in hot render path)
	ContextBarEmpty = lipgloss.NewStyle().
			Foreground(Subtle)

	// Status line styles (cached for performance in hot render path)
	StatusLineCenterStyle = lipgloss.NewStyle().
			Foreground(Dim)

	StatusLineRightStyle = lipgloss.NewStyle().
			Foreground(Muted)

	StatusLineSeparator = lipgloss.NewStyle().
			Foreground(Subtle)

	// Status line mode pill styles
	StatusLinePillViewport = lipgloss.NewStyle().
			Foreground(Secondary).
			Bold(true)

	StatusLinePillPanel = lipgloss.NewStyle().
			Foreground(Warning).
			Bold(true)

	StatusLinePillPrompt = lipgloss.NewStyle().
			Foreground(Success).
			Bold(true)
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
			Foreground(Muted).
			Padding(0, 1).
			Margin(0, 1)

	FooterPillActive = lipgloss.NewStyle().
				Foreground(Primary).
				Bold(true).
				Padding(0, 1).
				Margin(0, 1)
)

// ── Ask User Dialog ─────────────────────────────────────

var (
	AskUserTitle = lipgloss.NewStyle().
			Foreground(Aqua).
			Bold(true)

	AskUserLabel = lipgloss.NewStyle().
			Foreground(Text).
			Bold(true)

	AskUserDim = lipgloss.NewStyle().
			Foreground(Dim)

	AskUserSelected = lipgloss.NewStyle().
			Foreground(Success).
			Bold(true)

	AskUserProgress = lipgloss.NewStyle().
			Foreground(Muted)
)

// ── Plan Mode ────────────────────────────────────────────

var (
	PlanDialogTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Primary)

	PlanOptionCursor = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	PlanOptionStyle = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	PlanPreviewStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#aaaaaa"))

	PlanHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))
)

// ── Agent Detail Overlay ─────────────────────────────────

var (
	AgentDetailNameStyle = lipgloss.NewStyle().
			Bold(true)

	AgentDetailDimLine = lipgloss.NewStyle().
			Foreground(Subtle)

	AgentDetailInfoStyle = lipgloss.NewStyle().
			Foreground(Dim).
			PaddingLeft(1)

	AgentDetailTaskStyle = lipgloss.NewStyle().
			Foreground(Dim).
			PaddingLeft(1)

	AgentDetailWorkingStyle = lipgloss.NewStyle().
			Foreground(Warning).
			PaddingLeft(1)

	AgentDetailEscHint = lipgloss.NewStyle().
			Foreground(Subtle)

	AgentDetailAgentStyle = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true).
			PaddingLeft(1)

	AgentDetailTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ebdbb2")).
			PaddingLeft(3)

	AgentDetailToolStyle = lipgloss.NewStyle().
			Foreground(Warning).
			PaddingLeft(1)

	AgentDetailToolDimStyle = lipgloss.NewStyle().
			Foreground(Dim).
			PaddingLeft(1)

	AgentDetailDoneStyle = lipgloss.NewStyle().
			Foreground(Success).
			PaddingLeft(1)

	AgentDetailContentStyle = lipgloss.NewStyle().
			Foreground(Dim).
			PaddingLeft(3)

	AgentDetailErrorStyle = lipgloss.NewStyle().
			Foreground(Error).
			PaddingLeft(1)

	AgentDetailMessageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#83a598")).
			PaddingLeft(1)
)

// ── Session Picker ──────────────────────────────────────

var (
	SessionNumStyle = lipgloss.NewStyle().
			Foreground(Warning).
			Bold(true)

	SessionTitleStyle = lipgloss.NewStyle().
			Foreground(Text)

	SessionDateStyle = lipgloss.NewStyle().
			Foreground(Subtle)

	SessionHintStyle = lipgloss.NewStyle().
			Foreground(Dim)

	SessionLabelStyle = lipgloss.NewStyle().
			Foreground(Dim)
)

// ── Mode Line / Search ──────────────────────────────────

var (
	ModeLineStyle = lipgloss.NewStyle().
			Foreground(Muted)

	ModeLineArrowStyle = lipgloss.NewStyle().
			Foreground(Primary)

	ModeLineHintStyle = lipgloss.NewStyle().
			Foreground(Dim)

	SearchPlanStyle = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	SearchQueueStyle = lipgloss.NewStyle().
			Foreground(Warning)

	SearchHeaderStyle = lipgloss.NewStyle().
			Foreground(Warning).
			Bold(true)

	SearchQueryStyle = lipgloss.NewStyle().
			Foreground(Text)
)

// ── Which Key Popup ──────────────────────────────────

var (
	WhichKeyKey = lipgloss.NewStyle().
			Foreground(Warning).
			Bold(true)

	WhichKeyDesc = lipgloss.NewStyle().
			Foreground(Dim)

	WhichKeySep = lipgloss.NewStyle().
			Foreground(Subtle)

	WhichKeyTitle = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	WhichKeyBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(SurfaceAlt)
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
			Foreground(Muted).
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
