package teampanel

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Abraxas-365/claudio/internal/teams"
	"github.com/Abraxas-365/claudio/internal/tui/panels"
)

func setupPanel(t *testing.T) (*Panel, *teams.TeammateRunner) {
	t.Helper()
	dir := t.TempDir()
	mgr := teams.NewManager(dir)
	mgr.CreateTeam("test-team", "test", "sess-1")

	runner := teams.NewTeammateRunner(mgr, func(ctx context.Context, system, prompt string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	t.Cleanup(func() {
		runner.KillAll()
		runner.WaitForAll(5 * time.Second)
	})

	p := New(mgr, runner)
	p.SetSize(40, 30)
	return p, runner
}

func TestPanel_IsActive(t *testing.T) {
	p, _ := setupPanel(t)

	if p.IsActive() {
		t.Error("expected inactive on creation")
	}
	p.Activate()
	if !p.IsActive() {
		t.Error("expected active after Activate()")
	}
	p.Deactivate()
	if p.IsActive() {
		t.Error("expected inactive after Deactivate()")
	}
}

func TestPanel_ViewInactive(t *testing.T) {
	p, _ := setupPanel(t)
	if p.View() != "" {
		t.Error("expected empty view when inactive")
	}
}

func TestPanel_ViewNoAgents(t *testing.T) {
	p, _ := setupPanel(t)
	p.Activate()

	view := p.View()
	if !strings.Contains(view, "Agents") {
		t.Error("expected 'Agents' in view")
	}
	if !strings.Contains(view, "No active agents") {
		t.Error("expected 'No active agents' in view")
	}
}

func TestPanel_ViewWithAgents(t *testing.T) {
	p, runner := setupPanel(t)

	s1, _ := runner.Spawn(teams.SpawnConfig{TeamName: "test-team", AgentName: "worker1", Prompt: "task1"})
	s2, _ := runner.Spawn(teams.SpawnConfig{TeamName: "test-team", AgentName: "worker2", Prompt: "task2"})
	_ = s1
	_ = s2

	time.Sleep(50 * time.Millisecond)

	p.Activate()
	view := p.View()

	if !strings.Contains(view, "worker1") {
		t.Error("expected worker1 in view")
	}
	if !strings.Contains(view, "worker2") {
		t.Error("expected worker2 in view")
	}
	if !strings.Contains(view, "2/2 working") {
		t.Errorf("expected '2/2 working' in view, got:\n%s", view)
	}
}

func TestPanel_Navigation(t *testing.T) {
	p, runner := setupPanel(t)

	runner.Spawn(teams.SpawnConfig{TeamName: "test-team", AgentName: "a1", Prompt: "t"})
	runner.Spawn(teams.SpawnConfig{TeamName: "test-team", AgentName: "a2", Prompt: "t"})
	runner.Spawn(teams.SpawnConfig{TeamName: "test-team", AgentName: "a3", Prompt: "t"})
	time.Sleep(50 * time.Millisecond)
	p.Activate()

	// Initial cursor is 0
	name, _ := p.SelectedAgent()
	if name != "a1" {
		t.Errorf("expected a1 selected initially, got %q", name)
	}

	// Move down
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	name, _ = p.SelectedAgent()
	if name != "a2" {
		t.Errorf("expected a2 after j, got %q", name)
	}

	// Move down again
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	name, _ = p.SelectedAgent()
	if name != "a3" {
		t.Errorf("expected a3 after j, got %q", name)
	}

	// Move down at bottom — should clamp
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	name, _ = p.SelectedAgent()
	if name != "a3" {
		t.Errorf("expected a3 at bottom, got %q", name)
	}

	// Move up
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	name, _ = p.SelectedAgent()
	if name != "a2" {
		t.Errorf("expected a2 after k, got %q", name)
	}
}

func TestPanel_NavigationUpAtTop(t *testing.T) {
	p, runner := setupPanel(t)
	runner.Spawn(teams.SpawnConfig{TeamName: "test-team", AgentName: "a1", Prompt: "t"})

	time.Sleep(50 * time.Millisecond)
	p.Activate()

	// k at top should stay at 0
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	name, _ := p.SelectedAgent()
	if name != "a1" {
		t.Errorf("expected a1 at top, got %q", name)
	}
}

func TestPanel_EnterEmitsDetailAction(t *testing.T) {
	p, runner := setupPanel(t)
	state, _ := runner.Spawn(teams.SpawnConfig{TeamName: "test-team", AgentName: "target", Prompt: "t"})

	time.Sleep(50 * time.Millisecond)
	p.Activate()

	cmd, consumed := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !consumed {
		t.Error("expected enter to be consumed")
	}
	if cmd == nil {
		t.Fatal("expected cmd from enter")
	}

	msg := cmd()
	action, ok := msg.(panels.ActionMsg)
	if !ok {
		t.Fatalf("expected ActionMsg, got %T", msg)
	}
	if action.Type != "agent_detail" {
		t.Errorf("expected type 'agent_detail', got %q", action.Type)
	}
	if action.Payload != state.Identity.AgentID {
		t.Errorf("expected payload %q, got %v", state.Identity.AgentID, action.Payload)
	}
}

func TestPanel_MEmitsMessageAction(t *testing.T) {
	p, runner := setupPanel(t)
	runner.Spawn(teams.SpawnConfig{TeamName: "test-team", AgentName: "msgtest", Prompt: "t"})

	time.Sleep(50 * time.Millisecond)
	p.Activate()

	cmd, consumed := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if !consumed {
		t.Error("expected m to be consumed")
	}
	if cmd == nil {
		t.Fatal("expected cmd from m")
	}

	msg := cmd()
	action, ok := msg.(panels.ActionMsg)
	if !ok {
		t.Fatalf("expected ActionMsg, got %T", msg)
	}
	if action.Type != "agent_message" {
		t.Errorf("expected type 'agent_message', got %q", action.Type)
	}
	if action.Payload != "msgtest" {
		t.Errorf("expected payload 'msgtest', got %v", action.Payload)
	}
}

func TestPanel_UnhandledKeyNotConsumed(t *testing.T) {
	p, runner := setupPanel(t)
	runner.Spawn(teams.SpawnConfig{TeamName: "test-team", AgentName: "a1", Prompt: "t"})

	time.Sleep(50 * time.Millisecond)
	p.Activate()

	_, consumed := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	if consumed {
		t.Error("expected unhandled key to not be consumed")
	}
}

func TestPanel_HandleRefresh(t *testing.T) {
	p, runner := setupPanel(t)
	runner.Spawn(teams.SpawnConfig{TeamName: "test-team", AgentName: "a1", Prompt: "t"})

	time.Sleep(50 * time.Millisecond)
	p.Activate()

	cmd := p.HandleRefresh()
	if cmd == nil {
		t.Error("expected tick cmd when agents are working")
	}

	// tick should increment
	if p.tick != 1 {
		t.Errorf("expected tick 1, got %d", p.tick)
	}
}

func TestPanel_HandleRefreshNoWorking(t *testing.T) {
	_, _ = setupPanel(t)

	// Spawn agent that completes immediately
	immediateRunner := teams.NewTeammateRunner(
		func() *teams.Manager {
			dir := t.TempDir()
			mgr := teams.NewManager(dir)
			mgr.CreateTeam("done-team", "", "s")
			return mgr
		}(),
		func(ctx context.Context, system, prompt string) (string, error) {
			return "ok", nil
		},
	)
	state, _ := immediateRunner.Spawn(teams.SpawnConfig{TeamName: "done-team", AgentName: "fast", Prompt: "t"})
	immediateRunner.WaitForOne(state.Identity.AgentID, 5*time.Second)

	p2 := New(nil, immediateRunner)
	p2.SetSize(40, 30)
	p2.Activate()

	cmd := p2.HandleRefresh()
	if cmd != nil {
		t.Error("expected nil cmd when no agents are working")
	}
}

func TestPanel_SelectedAgentEmpty(t *testing.T) {
	p, _ := setupPanel(t)
	p.Activate()

	name, id := p.SelectedAgent()
	if name != "" || id != "" {
		t.Errorf("expected empty selection with no agents, got name=%q id=%q", name, id)
	}
}

func TestPanel_TeamSummary(t *testing.T) {
	p, runner := setupPanel(t)
	runner.Spawn(teams.SpawnConfig{TeamName: "test-team", AgentName: "a1", Prompt: "t"})
	runner.Spawn(teams.SpawnConfig{TeamName: "test-team", AgentName: "a2", Prompt: "t"})

	time.Sleep(50 * time.Millisecond)
	p.Activate()

	summary := p.TeamSummary()
	if !strings.Contains(summary, "test-team") {
		t.Errorf("expected team name in summary, got %q", summary)
	}
	if !strings.Contains(summary, "2/2") {
		t.Errorf("expected 2/2 in summary, got %q", summary)
	}
}

func TestPanel_TeamSummaryNoRunner(t *testing.T) {
	p := New(nil, nil)
	if p.TeamSummary() != "" {
		t.Error("expected empty summary with nil runner")
	}
}

func TestPanel_UnreadCount(t *testing.T) {
	p := New(nil, nil)
	if p.UnreadCount() != 0 {
		t.Error("expected 0 unread with nil runner")
	}
}

func TestSmartDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{5 * time.Second, "5s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m"},
		{90 * time.Second, "1m30s"},
		{300 * time.Second, "5m"},
	}
	for _, tt := range tests {
		got := smartDuration(tt.d)
		if got != tt.want {
			t.Errorf("smartDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
