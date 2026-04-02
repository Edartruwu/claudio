package teams

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// TeammateProgress tracks a teammate's work activity.
type TeammateProgress struct {
	ToolCalls  int
	Tokens     int
	Activities []string // recent activity descriptions (max 5)
	LastUpdate time.Time
}

// TeammateState holds the runtime state of an in-process teammate.
type TeammateState struct {
	Identity   TeammateIdentity
	Prompt     string
	Status     MemberStatus
	Progress   TeammateProgress
	Result     string // final output
	Error      string
	IsIdle     bool
	StartedAt  time.Time

	cancel     context.CancelFunc
	mu         sync.Mutex
	idleCh     chan struct{} // closed when teammate becomes idle
}

// RunAgentFunc is the callback to execute an agent.
// Receives (ctx, systemPrompt, userPrompt) and returns text output.
type RunAgentFunc func(ctx context.Context, system, prompt string) (string, error)

// TeammateRunner manages in-process teammate goroutines.
type TeammateRunner struct {
	mu        sync.RWMutex
	teammates map[string]*TeammateState // keyed by agent ID
	manager   *Manager
	mailbox   *Mailbox
	runAgent  RunAgentFunc
}

// NewTeammateRunner creates a runner for spawning in-process teammates.
func NewTeammateRunner(manager *Manager, runAgent RunAgentFunc) *TeammateRunner {
	return &TeammateRunner{
		teammates: make(map[string]*TeammateState),
		manager:   manager,
		runAgent:  runAgent,
	}
}

// SpawnConfig defines how to spawn a teammate.
type SpawnConfig struct {
	TeamName    string
	AgentName   string
	Prompt      string
	System      string // system prompt override
	Model       string // model override
}

// Spawn starts a new teammate goroutine.
func (r *TeammateRunner) Spawn(cfg SpawnConfig) (*TeammateState, error) {
	// Add member to team
	member, err := r.manager.AddMember(cfg.TeamName, cfg.AgentName, cfg.Model)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	state := &TeammateState{
		Identity:  member.Identity,
		Prompt:    cfg.Prompt,
		Status:    StatusWorking,
		StartedAt: time.Now(),
		cancel:    cancel,
		idleCh:    make(chan struct{}),
	}

	r.mu.Lock()
	r.teammates[member.Identity.AgentID] = state
	r.mu.Unlock()

	// Update team status
	r.manager.UpdateMemberStatus(cfg.TeamName, member.Identity.AgentID, StatusWorking)

	// Set up mailbox
	if r.mailbox == nil {
		team, _ := r.manager.GetTeam(cfg.TeamName)
		if team != nil {
			r.mailbox = NewMailbox(r.manager.teamsDir, cfg.TeamName)
		}
	}

	// Launch goroutine
	go r.runTeammate(ctx, state, cfg)

	return state, nil
}

func (r *TeammateRunner) runTeammate(ctx context.Context, state *TeammateState, cfg SpawnConfig) {
	defer func() {
		state.mu.Lock()
		state.IsIdle = true
		close(state.idleCh)
		state.mu.Unlock()
	}()

	// Build system prompt for teammate
	system := cfg.System
	if system == "" {
		system = fmt.Sprintf(`You are %s, a teammate in the "%s" team.

Your role: Complete your assigned task and report results clearly.

Guidelines:
- Focus on your specific task
- Report findings concisely when done
- If you need help from another teammate, explain what you need
- When finished, provide a clear summary of what you accomplished

Your task will be provided in the user message.`, cfg.AgentName, cfg.TeamName)
	}

	result, err := r.runAgent(ctx, system, cfg.Prompt)

	state.mu.Lock()
	if err != nil {
		if ctx.Err() == context.Canceled {
			state.Status = StatusShutdown
			state.Error = "shutdown requested"
		} else {
			state.Status = StatusFailed
			state.Error = err.Error()
		}
	} else {
		state.Status = StatusComplete
		state.Result = result
	}
	state.mu.Unlock()

	// Update team status
	r.manager.UpdateMemberStatus(cfg.TeamName, state.Identity.AgentID, state.Status)

	// Send completion notification to leader's inbox
	if r.mailbox != nil {
		team, _ := r.manager.GetTeam(cfg.TeamName)
		if team != nil {
			summary := truncateForSummary(result, 200)
			if err != nil {
				summary = fmt.Sprintf("FAILED: %s", err.Error())
			}
			r.mailbox.Send(state.Identity.AgentName, "team-lead", Message{
				Text:    fmt.Sprintf("[%s] Task complete: %s\n\nResult:\n%s", state.Status, state.Prompt, summary),
				Summary: fmt.Sprintf("%s: %s", state.Identity.AgentName, state.Status),
				Color:   state.Identity.Color,
			})
		}
	}
}

// GetState returns a teammate's current state.
func (r *TeammateRunner) GetState(agentID string) (*TeammateState, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, ok := r.teammates[agentID]
	return state, ok
}

// AllStates returns all teammate states.
func (r *TeammateRunner) AllStates() []*TeammateState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*TeammateState, 0, len(r.teammates))
	for _, s := range r.teammates {
		result = append(result, s)
	}
	return result
}

// Kill terminates a teammate.
func (r *TeammateRunner) Kill(agentID string) error {
	r.mu.RLock()
	state, ok := r.teammates[agentID]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("teammate %s not found", agentID)
	}

	if state.cancel != nil {
		state.cancel()
	}
	return nil
}

// KillAll terminates all teammates.
func (r *TeammateRunner) KillAll() {
	r.mu.RLock()
	states := make([]*TeammateState, 0, len(r.teammates))
	for _, s := range r.teammates {
		states = append(states, s)
	}
	r.mu.RUnlock()

	for _, s := range states {
		if s.cancel != nil {
			s.cancel()
		}
	}
}

// WaitForAll blocks until all teammates are idle.
func (r *TeammateRunner) WaitForAll(timeout time.Duration) bool {
	deadline := time.After(timeout)

	for {
		allIdle := true
		r.mu.RLock()
		for _, s := range r.teammates {
			s.mu.Lock()
			if !s.IsIdle {
				allIdle = false
			}
			s.mu.Unlock()
			if !allIdle {
				break
			}
		}
		r.mu.RUnlock()

		if allIdle {
			return true
		}

		select {
		case <-deadline:
			return false
		case <-time.After(500 * time.Millisecond):
			continue
		}
	}
}

// WaitForOne blocks until a specific teammate is idle.
func (r *TeammateRunner) WaitForOne(agentID string, timeout time.Duration) bool {
	r.mu.RLock()
	state, ok := r.teammates[agentID]
	r.mu.RUnlock()

	if !ok {
		return false
	}

	select {
	case <-state.idleCh:
		return true
	case <-time.After(timeout):
		return false
	}
}

// FormatStatus returns a summary of all teammates.
func (r *TeammateRunner) FormatStatus() string {
	states := r.AllStates()
	if len(states) == 0 {
		return "No active teammates"
	}

	var sb strings.Builder
	sb.WriteString("Teammates:\n")

	for _, s := range states {
		icon := "○"
		switch s.Status {
		case StatusWorking:
			icon = "◐"
		case StatusComplete:
			icon = "●"
		case StatusFailed:
			icon = "✗"
		case StatusShutdown:
			icon = "⊘"
		}

		duration := time.Since(s.StartedAt).Round(time.Second)
		sb.WriteString(fmt.Sprintf("  %s %s [%s] (%s) — %s\n",
			icon, s.Identity.AgentName, s.Status, duration,
			truncateForSummary(s.Prompt, 60)))

		if s.Error != "" {
			sb.WriteString(fmt.Sprintf("    Error: %s\n", s.Error))
		}
	}

	return sb.String()
}
