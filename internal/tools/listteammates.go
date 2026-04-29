package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/teams"
)

// ListTeammatesTool lists all teammate agents spawned via SpawnTeammate.
type ListTeammatesTool struct {
	deferrable
	Runner *teams.TeammateRunner
}

type listTeammatesInput struct {
	OnlyRunning bool `json:"only_running,omitempty"`
}

func (t *ListTeammatesTool) Name() string { return "ListTeammates" }

func (t *ListTeammatesTool) Description() string {
	return "List all teammate agents spawned via SpawnTeammate. Shows name, status (idle/working/complete/failed), current tool, recent activities, and timing. Use this — not BgTaskList — to check if a teammate is still running or has finished."
}

func (t *ListTeammatesTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"only_running": {
				"type": "boolean",
				"description": "If true, only show non-terminal teammates (working/idle/waiting_for_input). Default: false (show all)."
			}
		}
	}`)
}

func (t *ListTeammatesTool) IsReadOnly() bool                        { return true }
func (t *ListTeammatesTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *ListTeammatesTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in listTeammatesInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if t.Runner == nil {
		return &Result{Content: "Team system not available", IsError: true}, nil
	}

	all := t.Runner.AllStates()

	// Optionally filter to non-terminal statuses.
	var filtered []*teams.TeammateState
	for _, s := range all {
		if in.OnlyRunning {
			switch s.Status {
			case teams.StatusComplete, teams.StatusFailed, teams.StatusShutdown:
				continue
			}
		}
		filtered = append(filtered, s)
	}

	if len(filtered) == 0 {
		if in.OnlyRunning {
			return &Result{Content: "No running teammates."}, nil
		}
		return &Result{Content: "No teammates found."}, nil
	}

	now := time.Now()
	var sb strings.Builder
	for _, s := range filtered {
		started := now.Sub(s.StartedAt).Round(time.Second)
		sb.WriteString(fmt.Sprintf("name: %s  status: %s  started: %s ago\n",
			s.Identity.AgentName, s.Status, started))
		if ct := s.GetCurrentTool(); ct != "" {
			sb.WriteString(fmt.Sprintf("  doing: %s\n", ct))
		}
		prog := s.GetProgress()
		if len(prog.Activities) > 0 {
			sb.WriteString(fmt.Sprintf("  last: %s\n", prog.Activities[len(prog.Activities)-1]))
		}
		if !s.FinishedAt.IsZero() {
			finished := now.Sub(s.FinishedAt).Round(time.Second)
			sb.WriteString(fmt.Sprintf("  finished: %s ago\n", finished))
		}
	}

	return &Result{Content: strings.TrimRight(sb.String(), "\n")}, nil
}
