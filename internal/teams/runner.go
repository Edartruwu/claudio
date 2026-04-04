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

// ConversationEntry represents a single entry in an agent's conversation history.
type ConversationEntry struct {
	Time     time.Time
	Type     string // "text", "tool_start", "tool_end", "complete", "error", "message_in", "message_out"
	Content  string
	ToolName string
}

// TeammateEvent is emitted when something happens in a teammate's execution.
type TeammateEvent struct {
	TeamName  string
	AgentID   string
	AgentName string
	Type      string // "started", "text", "tool_start", "tool_end", "complete", "error"
	Text      string
	ToolName  string
	Color     string
}

// TeammateEventHandler receives events from teammate execution.
type TeammateEventHandler interface {
	OnTeammateEvent(event TeammateEvent)
}

const maxConversationEntries = 200

// TeammateState holds the runtime state of an in-process teammate.
type TeammateState struct {
	Identity     TeammateIdentity
	TeamName     string
	Prompt       string
	Status       MemberStatus
	Progress     TeammateProgress
	Result       string // final output
	Error        string
	IsIdle       bool
	StartedAt    time.Time
	Conversation []ConversationEntry

	cancel context.CancelFunc
	mu     sync.Mutex
	idleCh chan struct{} // closed when teammate becomes idle
}

// AddConversation appends an entry to the agent's conversation, thread-safe.
func (s *TeammateState) AddConversation(entry ConversationEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Conversation) >= maxConversationEntries {
		s.Conversation = s.Conversation[1:]
	}
	s.Conversation = append(s.Conversation, entry)
}

// AddActivity adds a recent activity string, thread-safe.
func (s *TeammateState) AddActivity(activity string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Progress.Activities = append(s.Progress.Activities, activity)
	if len(s.Progress.Activities) > 5 {
		s.Progress.Activities = s.Progress.Activities[len(s.Progress.Activities)-5:]
	}
	s.Progress.LastUpdate = time.Now()
}

// IncrToolCalls increments the tool call counter, thread-safe.
func (s *TeammateState) IncrToolCalls() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Progress.ToolCalls++
}

// GetConversation returns a snapshot of the conversation, thread-safe.
func (s *TeammateState) GetConversation() []ConversationEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ConversationEntry, len(s.Conversation))
	copy(out, s.Conversation)
	return out
}

// GetProgress returns a snapshot of the progress, thread-safe.
func (s *TeammateState) GetProgress() TeammateProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.Progress
	acts := make([]string, len(p.Activities))
	copy(acts, p.Activities)
	p.Activities = acts
	return p
}

// RunAgentFunc is the callback to execute an agent.
// Receives (ctx, systemPrompt, userPrompt) and returns text output.
type RunAgentFunc func(ctx context.Context, system, prompt string) (string, error)

// ContextDecorator allows the app layer to inject per-teammate context values
// (e.g., SubAgentObserver, SubAgentDB) without the teams package importing tools.
type ContextDecorator func(ctx context.Context, state *TeammateState) context.Context

// TeammateRunner manages in-process teammate goroutines.
type TeammateRunner struct {
	mu               sync.RWMutex
	teammates        map[string]*TeammateState // keyed by agent ID
	manager          *Manager
	mailbox          *Mailbox
	runAgent         RunAgentFunc
	eventHandler     TeammateEventHandler
	parentCtx        context.Context // parent context with observer/DB from TUI
	contextDecorator ContextDecorator
}

// NewTeammateRunner creates a runner for spawning in-process teammates.
func NewTeammateRunner(manager *Manager, runAgent RunAgentFunc) *TeammateRunner {
	return &TeammateRunner{
		teammates: make(map[string]*TeammateState),
		manager:   manager,
		runAgent:  runAgent,
		parentCtx: context.Background(),
	}
}

// SetEventHandler sets the handler that receives teammate lifecycle events.
func (r *TeammateRunner) SetEventHandler(h TeammateEventHandler) {
	r.eventHandler = h
}

// SetParentContext sets the parent context used for spawning teammates.
// This should carry SubAgentObserver and SubAgentDB from the TUI.
func (r *TeammateRunner) SetParentContext(ctx context.Context) {
	r.parentCtx = ctx
}

// SetContextDecorator sets a function that decorates the context for each teammate.
func (r *TeammateRunner) SetContextDecorator(d ContextDecorator) {
	r.contextDecorator = d
}

// EmitEvent sends an event to the registered handler.
func (r *TeammateRunner) EmitEvent(event TeammateEvent) {
	if r.eventHandler != nil {
		r.eventHandler.OnTeammateEvent(event)
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

	ctx, cancel := context.WithCancel(r.parentCtx)

	state := &TeammateState{
		Identity:     member.Identity,
		TeamName:     cfg.TeamName,
		Prompt:       cfg.Prompt,
		Status:       StatusWorking,
		StartedAt:    time.Now(),
		cancel:       cancel,
		idleCh:       make(chan struct{}),
		Conversation: make([]ConversationEntry, 0, 32),
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

	// Decorate context with per-teammate observer/DB if configured
	if r.contextDecorator != nil {
		ctx = r.contextDecorator(ctx, state)
	}

	// Emit started event
	r.EmitEvent(TeammateEvent{
		TeamName:  cfg.TeamName,
		AgentID:   state.Identity.AgentID,
		AgentName: cfg.AgentName,
		Type:      "started",
		Text:      cfg.Prompt,
		Color:     state.Identity.Color,
	})

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

	// Add completion to conversation
	if err != nil {
		state.AddConversation(ConversationEntry{
			Time:    time.Now(),
			Type:    "error",
			Content: state.Error,
		})
		r.EmitEvent(TeammateEvent{
			TeamName:  cfg.TeamName,
			AgentID:   state.Identity.AgentID,
			AgentName: cfg.AgentName,
			Type:      "error",
			Text:      state.Error,
			Color:     state.Identity.Color,
		})
	} else {
		state.AddConversation(ConversationEntry{
			Time:    time.Now(),
			Type:    "complete",
			Content: truncateForSummary(result, 500),
		})
		r.EmitEvent(TeammateEvent{
			TeamName:  cfg.TeamName,
			AgentID:   state.Identity.AgentID,
			AgentName: cfg.AgentName,
			Type:      "complete",
			Text:      truncateForSummary(result, 200),
			Color:     state.Identity.Color,
		})
	}

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

// GetStateByName returns a teammate's state by agent name.
func (r *TeammateRunner) GetStateByName(name string) (*TeammateState, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.teammates {
		if s.Identity.AgentName == name {
			return s, true
		}
	}
	return nil, false
}

// ActiveTeamName returns the name of the team that has active teammates, or empty string.
func (r *TeammateRunner) ActiveTeamName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.teammates {
		return s.TeamName
	}
	return ""
}

// WorkingCount returns the number of teammates currently working.
func (r *TeammateRunner) WorkingCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, s := range r.teammates {
		s.mu.Lock()
		if s.Status == StatusWorking {
			count++
		}
		s.mu.Unlock()
	}
	return count
}

// GetMailbox returns the mailbox, if any.
func (r *TeammateRunner) GetMailbox() *Mailbox {
	return r.mailbox
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
