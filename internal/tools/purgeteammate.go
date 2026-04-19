package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Abraxas-365/claudio/internal/teams"
)

// PurgeTeammatesTool removes all completed and failed agents and cleans up
// their git worktrees in a single call.
type PurgeTeammatesTool struct {
	deferrable
	Runner *teams.TeammateRunner
}

func (t *PurgeTeammatesTool) Name() string { return "PurgeTeammates" }

func (t *PurgeTeammatesTool) Description() string {
	return "Remove all completed and failed agents and clean up their git worktrees"
}

func (t *PurgeTeammatesTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (t *PurgeTeammatesTool) IsReadOnly() bool                        { return false }
func (t *PurgeTeammatesTool) RequiresApproval(_ json.RawMessage) bool { return true }

func (t *PurgeTeammatesTool) Execute(ctx context.Context, _ json.RawMessage) (*Result, error) {
	if t.Runner == nil {
		return &Result{Content: "Team system not available", IsError: true}, nil
	}

	count := t.Runner.PurgeDone()
	return &Result{Content: fmt.Sprintf("Purged %d agent(s)", count)}, nil
}
