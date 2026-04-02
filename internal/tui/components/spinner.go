package components

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Abraxas-365/claudio/internal/tui/styles"
)

// SpinnerModel is an animated spinner with a status message and elapsed timer.
type SpinnerModel struct {
	spinner   spinner.Model
	text      string
	active    bool
	startTime time.Time
}

// NewSpinner creates a new spinner.
func NewSpinner() SpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.SpinnerStyle
	return SpinnerModel{spinner: s}
}

// Start activates the spinner with a message and starts the timer.
func (m *SpinnerModel) Start(text string) {
	m.active = true
	m.text = text
	m.startTime = time.Now()
}

// Stop deactivates the spinner.
func (m *SpinnerModel) Stop() {
	m.active = false
	m.text = ""
}

// IsActive returns whether the spinner is running.
func (m *SpinnerModel) IsActive() bool {
	return m.active
}

// SetText updates the spinner message.
func (m *SpinnerModel) SetText(text string) {
	m.text = text
}

// Update handles spinner animation ticks.
func (m SpinnerModel) Update(msg tea.Msg) (SpinnerModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

// Tick returns the spinner tick command.
func (m SpinnerModel) Tick() tea.Cmd {
	if !m.active {
		return nil
	}
	return m.spinner.Tick
}

// View renders the spinner with elapsed time.
func (m SpinnerModel) View() string {
	if !m.active {
		return ""
	}
	elapsed := time.Since(m.startTime)
	var timer string
	if elapsed >= time.Minute {
		timer = fmt.Sprintf("%dm%02ds", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
	} else {
		timer = fmt.Sprintf("%ds", int(elapsed.Seconds()))
	}
	timerStr := styles.SpinnerTimer.Render(timer)
	text := styles.SpinnerText.Render(m.text)
	return lipgloss.JoinHorizontal(lipgloss.Center, m.spinner.View(), " ", text, " ", timerStr)
}
