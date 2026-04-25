package modelselector

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// Section identifies which section of the selector is focused.
var (
	msTitleStyle          = lipgloss.NewStyle().Foreground(styles.Text).Bold(true)
	msSectionTitleStyle   = lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	msSectionTitleDimStyle = lipgloss.NewStyle().Foreground(styles.Dim).Bold(true)
	msHintStyle           = lipgloss.NewStyle().Foreground(styles.Warning).Italic(true)
	msEffortStyle         = lipgloss.NewStyle().Foreground(styles.Warning).Bold(true)
	msEffortDotStyle      = lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	msEffortDotDimStyle   = lipgloss.NewStyle().Foreground(styles.Dim)
	msArrowStyle          = lipgloss.NewStyle().Foreground(styles.Dim)
	msSelectedNumStyle    = lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
	msSelectedNameStyle   = lipgloss.NewStyle().Foreground(styles.Text).Bold(true)
	msSelectedDescStyle   = lipgloss.NewStyle().Foreground(styles.Dim)
	msDimNumStyle         = lipgloss.NewStyle().Foreground(styles.Dim)
	msDimNameStyle        = lipgloss.NewStyle().Foreground(styles.Dim)
	msDimDescStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563"))
	msSelectedPrefixStyle = lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
)

type Section int

const (
	SectionModel Section = iota
	SectionThinking
	SectionBudget
)

// ModelOption represents a selectable model.
type ModelOption struct {
	Label       string
	ID          string
	Description string
}

// ThinkingOption represents a thinking mode option.
type ThinkingOption struct {
	Label       string
	Mode        string // "adaptive", "enabled", "disabled", "" (auto)
	Description string
}

// BudgetOption represents a thinking budget option.
type BudgetOption struct {
	Label  string
	Tokens int
}

var effortLevels = []string{"low", "medium", "high"}

func effortLabel(level string) string {
	switch level {
	case "low":
		return "Low"
	case "high":
		return "High"
	default:
		return "Medium"
	}
}

func effortDescription(level string) string {
	switch level {
	case "low":
		return "Quick, minimal overhead"
	case "high":
		return "Comprehensive, extensive reasoning"
	default:
		return "Balanced speed and intelligence"
	}
}

func effortIndex(level string) int {
	for i, l := range effortLevels {
		if l == level {
			return i
		}
	}
	return 1 // default to medium
}

// DefaultModels returns the standard model options.
func DefaultModels() []ModelOption {
	return []ModelOption{
		{
			Label:       "Default (recommended)",
			ID:          "claude-opus-4-6",
			Description: "Opus 4.6 with 1M context - Most capable for complex work",
		},
		{
			Label:       "Sonnet",
			ID:          "claude-sonnet-4-6",
			Description: "Sonnet 4.6 - Best for everyday tasks",
		},
		{
			Label:       "Haiku",
			ID:          "claude-haiku-4-5-20251001",
			Description: "Haiku 4.5 - Fastest for quick answers",
		},
	}
}

// DefaultThinkingOptions returns the thinking mode options.
func DefaultThinkingOptions() []ThinkingOption {
	return []ThinkingOption{
		{Label: "Auto", Mode: "", Description: "Adaptive thinking for supported models"},
		{Label: "Adaptive", Mode: "adaptive", Description: "Model decides when and how much to think"},
		{Label: "Enabled (fixed budget)", Mode: "enabled", Description: "Always think with a token budget"},
		{Label: "Disabled", Mode: "disabled", Description: "No extended thinking"},
	}
}

// DefaultBudgetOptions returns the thinking budget presets.
func DefaultBudgetOptions() []BudgetOption {
	return []BudgetOption{
		{Label: "8k tokens (quick)", Tokens: 8000},
		{Label: "16k tokens (moderate)", Tokens: 16000},
		{Label: "32k tokens (deep)", Tokens: 32000},
		{Label: "64k tokens (very deep)", Tokens: 64000},
		{Label: "128k tokens (ultrathink)", Tokens: 128000},
	}
}

// ModelSelectedMsg is sent when the user confirms all selections.
type ModelSelectedMsg struct {
	ModelID      string
	Label        string
	ThinkingMode string
	BudgetTokens int
	EffortLevel  string
}

// DismissMsg is sent when the user cancels.
type DismissMsg struct{}

// Model is the model + thinking + effort selector component.
type Model struct {
	models   []ModelOption
	thinking []ThinkingOption
	budgets  []BudgetOption

	modelIdx    int
	thinkingIdx int
	budgetIdx   int
	effortIdx   int // index into effortLevels

	currentModel    string
	currentThinking string
	currentBudget   int
	currentEffort   string

	section Section
	active  bool
	width   int
}

// WithExtraModels creates a new model selector that includes additional provider models.
func NewWithModels(currentModel, currentThinking string, currentBudget int, currentEffort string, extraModels []ModelOption) Model {
	models := DefaultModels()
	if len(extraModels) > 0 {
		models = append(models, extraModels...)
	}
	return newSelector(models, currentModel, currentThinking, currentBudget, currentEffort)
}

// New creates a new model selector with only the default Anthropic models.
func New(currentModel, currentThinking string, currentBudget int, currentEffort string) Model {
	return newSelector(DefaultModels(), currentModel, currentThinking, currentBudget, currentEffort)
}

func newSelector(models []ModelOption, currentModel, currentThinking string, currentBudget int, currentEffort string) Model {
	thinking := DefaultThinkingOptions()
	budgets := DefaultBudgetOptions()

	modelIdx := 0
	for i, o := range models {
		if o.ID == currentModel {
			modelIdx = i
			break
		}
	}

	thinkingIdx := 0
	for i, o := range thinking {
		if o.Mode == currentThinking {
			thinkingIdx = i
			break
		}
	}

	budgetIdx := 2 // default to 32k
	for i, o := range budgets {
		if o.Tokens == currentBudget {
			budgetIdx = i
			break
		}
	}

	return Model{
		models:          models,
		thinking:        thinking,
		budgets:         budgets,
		modelIdx:        modelIdx,
		thinkingIdx:     thinkingIdx,
		budgetIdx:       budgetIdx,
		effortIdx:       effortIndex(currentEffort),
		currentModel:    currentModel,
		currentThinking: currentThinking,
		currentBudget:   currentBudget,
		currentEffort:   currentEffort,
		section:         SectionModel,
		active:          true,
	}
}

func (m Model) IsActive() bool  { return m.active }
func (m *Model) SetWidth(w int) { m.width = w }

func (m Model) selectedThinkingMode() string {
	return m.thinking[m.thinkingIdx].Mode
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "up", "k":
		switch m.section {
		case SectionModel:
			if m.modelIdx > 0 {
				m.modelIdx--
			}
		case SectionThinking:
			if m.thinkingIdx > 0 {
				m.thinkingIdx--
			}
		case SectionBudget:
			if m.budgetIdx > 0 {
				m.budgetIdx--
			}
		}

	case "down", "j":
		switch m.section {
		case SectionModel:
			if m.modelIdx < len(m.models)-1 {
				m.modelIdx++
			}
		case SectionThinking:
			if m.thinkingIdx < len(m.thinking)-1 {
				m.thinkingIdx++
			}
		case SectionBudget:
			if m.budgetIdx < len(m.budgets)-1 {
				m.budgetIdx++
			}
		}

	case "left", "h":
		// Cycle effort level left
		if m.effortIdx > 0 {
			m.effortIdx--
		}

	case "right", "l":
		// Cycle effort level right
		if m.effortIdx < len(effortLevels)-1 {
			m.effortIdx++
		}

	case "tab":
		switch m.section {
		case SectionModel:
			m.section = SectionThinking
		case SectionThinking:
			if m.selectedThinkingMode() == "enabled" {
				m.section = SectionBudget
			} else {
				m.section = SectionModel
			}
		case SectionBudget:
			m.section = SectionModel
		}

	case "shift+tab":
		switch m.section {
		case SectionModel:
			if m.selectedThinkingMode() == "enabled" {
				m.section = SectionBudget
			} else {
				m.section = SectionThinking
			}
		case SectionThinking:
			m.section = SectionModel
		case SectionBudget:
			m.section = SectionThinking
		}

	case "enter":
		m.active = false
		opt := m.models[m.modelIdx]
		thinkOpt := m.thinking[m.thinkingIdx]
		budgetTokens := 0
		if thinkOpt.Mode == "enabled" {
			budgetTokens = m.budgets[m.budgetIdx].Tokens
		}
		return m, func() tea.Msg {
			return ModelSelectedMsg{
				ModelID:      opt.ID,
				Label:        opt.Label,
				ThinkingMode: thinkOpt.Mode,
				BudgetTokens: budgetTokens,
				EffortLevel:  effortLevels[m.effortIdx],
			}
		}

	case "esc", "q":
		m.active = false
		return m, func() tea.Msg {
			return DismissMsg{}
		}
	}

	return m, nil
}

func (m Model) View() string {
	if !m.active {
		return ""
	}

	titleStyle := msTitleStyle
	sectionTitle := msSectionTitleStyle
	sectionTitleDim := msSectionTitleDimStyle
	hintStyle := msHintStyle

	var lines []string
	lines = append(lines, titleStyle.Render("Model & Thinking Configuration"))
	lines = append(lines, "")

	// --- Model Section ---
	if m.section == SectionModel {
		lines = append(lines, sectionTitle.Render("Model"))
	} else {
		lines = append(lines, sectionTitleDim.Render("Model"))
	}

	for i, opt := range m.models {
		line := m.renderOption(i, opt.Label, opt.Description, opt.ID == m.currentModel, m.section == SectionModel, m.modelIdx)
		lines = append(lines, line)
	}

	lines = append(lines, "")

	// --- Thinking Section ---
	if m.section == SectionThinking {
		lines = append(lines, sectionTitle.Render("Extended Thinking"))
	} else {
		lines = append(lines, sectionTitleDim.Render("Extended Thinking"))
	}

	for i, opt := range m.thinking {
		isCurrent := opt.Mode == m.currentThinking
		line := m.renderOption(i, opt.Label, opt.Description, isCurrent, m.section == SectionThinking, m.thinkingIdx)
		lines = append(lines, line)
	}

	// --- Budget Section (only if thinking is "enabled") ---
	if m.selectedThinkingMode() == "enabled" {
		lines = append(lines, "")

		if m.section == SectionBudget {
			lines = append(lines, sectionTitle.Render("Thinking Budget"))
		} else {
			lines = append(lines, sectionTitleDim.Render("Thinking Budget"))
		}

		for i, opt := range m.budgets {
			isCurrent := opt.Tokens == m.currentBudget
			line := m.renderOption(i, opt.Label, "", isCurrent, m.section == SectionBudget, m.budgetIdx)
			lines = append(lines, line)
		}
	}

	lines = append(lines, "")

	// --- Effort Slider (always visible, controlled by ←/→) ---
	effortStyle := msEffortStyle
	effortDot := msEffortDotStyle.Render("\u25CF")
	effortDotDim := msEffortDotDimStyle.Render("\u25CB")
	arrowStyle := msArrowStyle

	var dots []string
	for i := range effortLevels {
		if i == m.effortIdx {
			dots = append(dots, effortDot)
		} else {
			dots = append(dots, effortDotDim)
		}
	}

	effortLine := effortStyle.Render(effortLabel(effortLevels[m.effortIdx])+" effort") +
		"  " + arrowStyle.Render("\u2190") + " " + strings.Join(dots, " ") + " " + arrowStyle.Render("\u2192") +
		"  " + msEffortDotDimStyle.Render(effortDescription(effortLevels[m.effortIdx]))

	lines = append(lines, effortLine)

	lines = append(lines, "")
	lines = append(lines, hintStyle.Render("Tab switch sections \u00B7 j/k navigate \u00B7 \u2190/\u2192 effort \u00B7 Enter confirm \u00B7 Esc cancel"))

	content := strings.Join(lines, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Padding(1, 2).
		Width(min(m.width-4, 85)).
		Render(content)

	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(box)
}

func (m Model) renderOption(idx int, label, description string, isCurrent, isFocusedSection bool, sectionCursor int) string {
	selected := isFocusedSection && idx == sectionCursor

	check := ""
	if isCurrent {
		check = " \u2714"
	}

	var numStyle, nameStyle, descStyle lipgloss.Style

	if selected {
		numStyle = msSelectedNumStyle
		nameStyle = msSelectedNameStyle
		descStyle = msSelectedDescStyle
	} else {
		numStyle = msDimNumStyle
		nameStyle = msDimNameStyle
		descStyle = msDimDescStyle
	}

	prefix := "  "
	if selected {
		prefix = msSelectedPrefixStyle.Render("\u203A ")
	}

	num := fmt.Sprintf("%d. ", idx+1)
	nameText := nameStyle.Render(label + check)

	line := prefix + numStyle.Render(num) + nameText
	if description != "" {
		padding := strings.Repeat(" ", max(1, 30-lipgloss.Width(label+check)))
		line += padding + descStyle.Render(description)
	}

	return line
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
