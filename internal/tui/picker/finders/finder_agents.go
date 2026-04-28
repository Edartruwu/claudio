package finders

import (
	"context"

	"github.com/Abraxas-365/claudio/internal/teams"
	"github.com/Abraxas-365/claudio/internal/tui/picker"
)

// agentFinder emits entries for every agent known to a TeammateRunner.
type agentFinder struct {
	runner *teams.TeammateRunner
}

// NewAgentFinder returns a Finder over all agents in runner (running or completed).
// Each entry: Display = agent name + status emoji, Ordinal = agent name,
// Meta["agentID"] = agent ID, Meta["status"] = MemberStatus string.
func NewAgentFinder(runner *teams.TeammateRunner) picker.Finder {
	return &agentFinder{runner: runner}
}

func (f *agentFinder) Find(ctx context.Context) <-chan picker.Entry {
	states := f.runner.AllStates()
	ch := make(chan picker.Entry, len(states))
	go func() {
		defer close(ch)
		for _, s := range states {
			status := string(s.GetStatus())
			emoji := statusEmoji(status)
			name := s.Identity.AgentName
			e := picker.Entry{
				Value:   s,
				Display: name + " " + emoji,
				Ordinal: name,
				Meta: map[string]any{
					"agentID": s.Identity.AgentID,
					"status":  status,
				},
			}
			select {
			case <-ctx.Done():
				return
			case ch <- e:
			}
		}
	}()
	return ch
}

func (f *agentFinder) Close() {}

// statusEmoji maps MemberStatus to a display emoji.
func statusEmoji(status string) string {
	switch status {
	case "working":
		return "⟳"
	case "complete":
		return "✓"
	case "failed":
		return "✗"
	case "waiting_for_input":
		return "⏳"
	case "shutdown":
		return "⏹"
	default:
		return "○"
	}
}
