package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Abraxas-365/claudio/internal/teams"
)

func setupSendMessageTool(t *testing.T) (*SendMessageTool, *teams.Manager) {
	t.Helper()
	dir := t.TempDir()
	mgr := teams.NewManager(dir)
	if _, err := mgr.CreateTeam("test-team", "A test team", "sess-1", ""); err != nil {
		t.Fatal(err)
	}
	return &SendMessageTool{Manager: mgr}, mgr
}

func TestSendMessageTool_ContextOverridesFields(t *testing.T) {
	tool, _ := setupSendMessageTool(t)

	// Tool fields are empty — without context this would fail
	ctx := WithTeamContext(context.Background(), TeamContext{
		TeamName:  "test-team",
		AgentName: "worker-1",
	})

	input, _ := json.Marshal(sendMessageInput{To: "team-lead", Message: "hello"})
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if result.Content != "Message sent to team-lead from worker-1" {
		t.Errorf("unexpected result: %s", result.Content)
	}
}

func TestSendMessageTool_FallsBackToFields(t *testing.T) {
	tool, _ := setupSendMessageTool(t)
	tool.TeamName = "test-team"
	tool.AgentName = "lead"

	// No context set — should use fields
	input, _ := json.Marshal(sendMessageInput{To: "worker-1", Message: "hello"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if result.Content != "Message sent to worker-1 from lead" {
		t.Errorf("unexpected result: %s", result.Content)
	}
}

func TestSendMessageTool_NoTeamContext(t *testing.T) {
	tool := &SendMessageTool{Manager: teams.NewManager(t.TempDir())}

	input, _ := json.Marshal(sendMessageInput{To: "someone", Message: "hi"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing team context")
	}
	if result.Content != "Not in a team context" {
		t.Errorf("unexpected error: %s", result.Content)
	}
}

func TestSendMessageTool_NilManager(t *testing.T) {
	tool := &SendMessageTool{}

	input, _ := json.Marshal(sendMessageInput{To: "someone", Message: "hi"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for nil manager")
	}
}

func TestSendMessageTool_Broadcast(t *testing.T) {
	tool, _ := setupSendMessageTool(t)

	ctx := WithTeamContext(context.Background(), TeamContext{
		TeamName:  "test-team",
		AgentName: "broadcaster",
	})

	input, _ := json.Marshal(sendMessageInput{To: "*", Message: "hello all"})
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if result.Content != "Broadcast sent to all teammates from broadcaster" {
		t.Errorf("unexpected result: %s", result.Content)
	}
}

func TestSendMessageTool_DefaultSenderIsTeamLead(t *testing.T) {
	tool, _ := setupSendMessageTool(t)

	// Context with empty AgentName — should default to "team-lead"
	ctx := WithTeamContext(context.Background(), TeamContext{
		TeamName:  "test-team",
		AgentName: "",
	})

	input, _ := json.Marshal(sendMessageInput{To: "worker", Message: "hi"})
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if result.Content != "Message sent to worker from team-lead" {
		t.Errorf("unexpected result: %s", result.Content)
	}
}

func TestSendMessageTool_MessageDelivered(t *testing.T) {
	tool, mgr := setupSendMessageTool(t)

	ctx := WithTeamContext(context.Background(), TeamContext{
		TeamName:  "test-team",
		AgentName: "sender",
	})

	input, _ := json.Marshal(sendMessageInput{To: "receiver", Message: "payload"})
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	// Verify the message actually landed on disk
	mailbox := teams.NewMailbox(mgr.TeamsDir(), "test-team")
	msgs, err := mailbox.ReadAll("receiver")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Text != "payload" {
		t.Errorf("expected message text %q, got %q", "payload", msgs[0].Text)
	}
}

func TestSendMessageTool_InvalidInput(t *testing.T) {
	tool, _ := setupSendMessageTool(t)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestTeamContextHelpers(t *testing.T) {
	// Nil when not set
	if tc := TeamContextFromCtx(context.Background()); tc != nil {
		t.Error("expected nil TeamContext from bare context")
	}

	// Round-trip
	ctx := WithTeamContext(context.Background(), TeamContext{
		TeamName:  "team-a",
		AgentName: "agent-b",
	})
	tc := TeamContextFromCtx(ctx)
	if tc == nil {
		t.Fatal("expected non-nil TeamContext")
	}
	if tc.TeamName != "team-a" || tc.AgentName != "agent-b" {
		t.Errorf("unexpected values: %+v", tc)
	}
}

// Team-lead scenario: no context, no fields, but Runner has an active team.
func TestSendMessageTool_RunnerFallback(t *testing.T) {
	tool, mgr := setupSendMessageTool(t)
	runner := teams.NewTeammateRunner(mgr, nil)
	runner.SetActiveTeam("test-team")
	tool.Runner = runner

	// No TeamContext in ctx, no fields set — Runner should provide team name.
	input, _ := json.Marshal(sendMessageInput{To: "worker", Message: "do this"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if result.Content != "Message sent to worker from team-lead" {
		t.Errorf("unexpected result: %s", result.Content)
	}
}

// Team-lead broadcast via Runner fallback.
func TestSendMessageTool_RunnerFallbackBroadcast(t *testing.T) {
	tool, mgr := setupSendMessageTool(t)
	runner := teams.NewTeammateRunner(mgr, nil)
	runner.SetActiveTeam("test-team")
	tool.Runner = runner

	input, _ := json.Marshal(sendMessageInput{To: "*", Message: "hello all"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if result.Content != "Broadcast sent to all teammates from team-lead" {
		t.Errorf("unexpected result: %s", result.Content)
	}
}

// Runner set but no active team — should still fail.
func TestSendMessageTool_RunnerNoActiveTeam(t *testing.T) {
	tool, mgr := setupSendMessageTool(t)
	runner := teams.NewTeammateRunner(mgr, nil)
	// Don't set active team
	tool.Runner = runner

	input, _ := json.Marshal(sendMessageInput{To: "someone", Message: "hi"})
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for no active team")
	}
	if result.Content != "Not in a team context" {
		t.Errorf("unexpected error: %s", result.Content)
	}
}

// Context takes priority over Runner's active team.
func TestSendMessageTool_ContextOverridesRunner(t *testing.T) {
	dir := t.TempDir()
	mgr := teams.NewManager(dir)
	// Create two teams
	if _, err := mgr.CreateTeam("team-alpha", "Alpha", "s1", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CreateTeam("team-beta", "Beta", "s1", ""); err != nil {
		t.Fatal(err)
	}

	runner := teams.NewTeammateRunner(mgr, nil)
	runner.SetActiveTeam("team-beta")

	tool := &SendMessageTool{Manager: mgr, Runner: runner}

	// Context says team-alpha; Runner says team-beta. Context wins.
	ctx := WithTeamContext(context.Background(), TeamContext{
		TeamName:  "team-alpha",
		AgentName: "alpha-worker",
	})

	input, _ := json.Marshal(sendMessageInput{To: "peer", Message: "hello"})
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	// Verify message landed in team-alpha's mailbox, not team-beta's
	mailbox := teams.NewMailbox(mgr.TeamsDir(), "team-alpha")
	msgs, err := mailbox.ReadAll("peer")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message in team-alpha, got %d", len(msgs))
	}
	if msgs[0].Text != "hello" {
		t.Errorf("unexpected message text: %s", msgs[0].Text)
	}

	// Verify nothing landed in team-beta
	mailboxBeta := teams.NewMailbox(mgr.TeamsDir(), "team-beta")
	betaMsgs, _ := mailboxBeta.ReadAll("peer")
	if len(betaMsgs) != 0 {
		t.Errorf("expected 0 messages in team-beta, got %d", len(betaMsgs))
	}
}

// Verify that the mailbox directory is created under the team's data dir.
func TestSendMessageTool_MailboxDir(t *testing.T) {
	tool, mgr := setupSendMessageTool(t)

	ctx := WithTeamContext(context.Background(), TeamContext{
		TeamName:  "test-team",
		AgentName: "a",
	})

	input, _ := json.Marshal(sendMessageInput{To: "b", Message: "x"})
	tool.Execute(ctx, input)

	// The inbox directory should exist
	inboxDir := filepath.Join(mgr.TeamsDir(), "test-team", "inboxes")
	if _, err := os.Stat(inboxDir); os.IsNotExist(err) {
		t.Error("expected inbox directory to be created")
	}
}
