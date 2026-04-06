package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/teams"
)

// blockingRunFn returns an agent function that blocks until its context is canceled.
func blockingRunFn() teams.RunAgentFunc {
	return func(ctx context.Context, system, prompt string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
}

func TestTeamDeleteTool_DrainsAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	mgr := teams.NewManager(dir)
	if _, err := mgr.CreateTeam("doomed", "", "sess-1", ""); err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	runner := teams.NewTeammateRunner(mgr, blockingRunFn())

	// Spawn two long-running members.
	if _, err := runner.Spawn(teams.SpawnConfig{
		TeamName: "doomed", AgentName: "w1", Prompt: "wait",
	}); err != nil {
		t.Fatalf("Spawn w1: %v", err)
	}
	if _, err := runner.Spawn(teams.SpawnConfig{
		TeamName: "doomed", AgentName: "w2", Prompt: "wait",
	}); err != nil {
		t.Fatalf("Spawn w2: %v", err)
	}

	// Confirm preconditions: members are working, mailbox exists.
	time.Sleep(50 * time.Millisecond)
	if c := runner.WorkingCount(); c != 2 {
		t.Fatalf("expected 2 working members before delete, got %d", c)
	}

	tool := &TeamDeleteTool{Manager: mgr, Runner: runner}
	input, _ := json.Marshal(teamDeleteInput{TeamName: "doomed"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "doomed") {
		t.Errorf("expected team name in result, got %q", result.Content)
	}
	if strings.Contains(result.Content, "warning") {
		t.Errorf("did not expect drain warning, got %q", result.Content)
	}

	// Manager should no longer know the team.
	if _, ok := mgr.GetTeam("doomed"); ok {
		t.Error("manager still has the team after delete")
	}
	// Runner should have no remaining state for it.
	if len(runner.AllStates()) != 0 {
		t.Errorf("expected 0 teammate states after delete, got %d", len(runner.AllStates()))
	}
	if runner.ActiveTeamName() != "" {
		t.Errorf("expected empty active team, got %q", runner.ActiveTeamName())
	}
}

func TestTeamDeleteTool_NoRunner(t *testing.T) {
	// TeamDeleteTool should still work when only the manager is wired up
	// (e.g., a team created with no running members at all).
	dir := t.TempDir()
	mgr := teams.NewManager(dir)
	if _, err := mgr.CreateTeam("empty", "", "sess-1", ""); err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}

	tool := &TeamDeleteTool{Manager: mgr}
	input, _ := json.Marshal(teamDeleteInput{TeamName: "empty"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if _, ok := mgr.GetTeam("empty"); ok {
		t.Error("team should be removed from manager")
	}
}

func TestTeamDeleteTool_MissingTeamName(t *testing.T) {
	tool := &TeamDeleteTool{Manager: teams.NewManager(t.TempDir())}
	input, _ := json.Marshal(teamDeleteInput{})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing team_name")
	}
}

func TestTeamDeleteTool_NilManager(t *testing.T) {
	tool := &TeamDeleteTool{}
	input, _ := json.Marshal(teamDeleteInput{TeamName: "x"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Error("expected error when manager is nil")
	}
}
