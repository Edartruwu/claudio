package teams

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/git"
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
	TeamName       string
	AgentID        string
	AgentName      string
	Type           string // "started", "text", "tool_start", "tool_end", "complete", "error"
	Text           string
	ToolName       string
	Color          string
	WorktreePath   string // set on complete/error if worktree has changes
	WorktreeBranch string
}

// TeammateEventHandler receives events from teammate execution.
type TeammateEventHandler interface {
	OnTeammateEvent(event TeammateEvent)
}

const maxConversationEntries = 200

// TeammateState holds the runtime state of an in-process teammate.
type TeammateState struct {
	Identity       TeammateIdentity
	TeamName       string
	Prompt         string
	Model          string // model override for this teammate
	MaxTurns       int    // optional max agentic turns (0 = unlimited)
	Status         MemberStatus
	Progress       TeammateProgress
	Result         string // final output
	Error          string
	IsIdle         bool
	StartedAt      time.Time
	FinishedAt     time.Time
	Conversation   []ConversationEntry
	WorktreePath   string // path to git worktree (empty if no isolation)
	WorktreeBranch string // branch name used for the worktree

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

// GetStatus returns the current status, thread-safe.
func (s *TeammateState) GetStatus() MemberStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Status
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

// CwdInjector injects a CWD override into context. Set by the app layer so
// the teams package doesn't need to import the tools package.
type CwdInjector func(ctx context.Context, cwd string) context.Context

// TaskCompleter is called when an agent finishes to update assigned tasks.
// Set by the app layer to bridge teams → tasks without circular imports.
type TaskCompleter func(agentName, status string)

// TeammateRunner manages in-process teammate goroutines.
type TeammateRunner struct {
	mu               sync.RWMutex
	teammates        map[string]*TeammateState // keyed by agent ID
	manager          *Manager
	mailboxes        map[string]*Mailbox
	runAgent         RunAgentFunc
	eventHandler     TeammateEventHandler
	parentCtx        context.Context // parent context with observer/DB from TUI
	contextDecorator ContextDecorator
	cwdInjector      CwdInjector
	taskCompleter    TaskCompleter
	activeTeam       string // explicitly set active team name
}

// NewTeammateRunner creates a runner for spawning in-process teammates.
func NewTeammateRunner(manager *Manager, runAgent RunAgentFunc) *TeammateRunner {
	return &TeammateRunner{
		teammates: make(map[string]*TeammateState),
		mailboxes: make(map[string]*Mailbox),
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

// SetCwdInjector sets the function that injects CWD override into context.
func (r *TeammateRunner) SetCwdInjector(fn CwdInjector) {
	r.cwdInjector = fn
}

// SetTaskCompleter sets the callback for updating tasks when agents finish.
func (r *TeammateRunner) SetTaskCompleter(fn TaskCompleter) {
	r.taskCompleter = fn
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
	MaxTurns    int    // optional max agentic turns (0 = unlimited)
	Isolation   string // "worktree" for git worktree isolation
}

// Spawn starts a new teammate goroutine.
func (r *TeammateRunner) Spawn(cfg SpawnConfig) (*TeammateState, error) {
	// Fall back to team-level default model if no per-agent model specified
	if cfg.Model == "" {
		if team, ok := r.manager.GetTeam(cfg.TeamName); ok && team.Model != "" {
			cfg.Model = team.Model
		}
	}

	// Default to worktree isolation when inside a git repo
	if cfg.Isolation == "" {
		cwd, _ := os.Getwd()
		repo := git.NewRepo(cwd)
		if repo.IsRepo() {
			cfg.Isolation = "worktree"
		}
	}

	// Add member to team
	member, err := r.manager.AddMember(cfg.TeamName, cfg.AgentName, cfg.Model, cfg.Prompt)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(r.parentCtx)

	state := &TeammateState{
		Identity:     member.Identity,
		TeamName:     cfg.TeamName,
		Prompt:       cfg.Prompt,
		Model:        cfg.Model,
		MaxTurns:     cfg.MaxTurns,
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

	// Set up mailbox for this team (one per team)
	if _, ok := r.mailboxes[cfg.TeamName]; !ok {
		team, _ := r.manager.GetTeam(cfg.TeamName)
		if team != nil {
			r.mailboxes[cfg.TeamName] = NewMailbox(r.manager.teamsDir, cfg.TeamName)
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

	// Set up worktree isolation if requested
	var worktreeRepo *git.Repo
	if cfg.Isolation == "worktree" {
		cwd, _ := os.Getwd()
		repo := git.NewRepo(cwd)
		if repo.IsRepo() {
			branch := fmt.Sprintf("claudio/%s/%s", cfg.TeamName, cfg.AgentName)
			root, _ := repo.Root()
			wtPath := filepath.Join(root, ".claudio-worktrees", branch)
			wtErr := repo.WorktreeAdd(wtPath, branch)
			if wtErr == nil {
				worktreeRepo = git.NewRepo(wtPath)

				state.mu.Lock()
				state.WorktreePath = wtPath
				state.WorktreeBranch = branch
				state.mu.Unlock()

				// Inject CWD override into context
				if r.cwdInjector != nil {
					ctx = r.cwdInjector(ctx, wtPath)
				}
			}
		}
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

	// Add worktree notice to system prompt
	if state.WorktreePath != "" {
		cwd, _ := os.Getwd()
		system += fmt.Sprintf("\n\nYou are operating in an isolated git worktree at %s — same repository, same relative file structure, separate working copy. "+
			"Paths in conversation context from the parent agent refer to %s; translate them to your worktree root. "+
			"Re-read files before editing if the parent may have modified them. "+
			"Your changes stay in this worktree and will not affect the parent's files.", state.WorktreePath, cwd)
	}

	result, err := r.runAgent(ctx, system, cfg.Prompt)

	state.mu.Lock()
	state.FinishedAt = time.Now()
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

	// Worktree cleanup: remove if no changes, keep if there are changes
	if worktreeRepo != nil && state.WorktreePath != "" {
		hasChanges, chkErr := worktreeRepo.HasChanges()
		if chkErr != nil || !hasChanges {
			// No changes — clean up worktree and branch
			cwd, _ := os.Getwd()
			mainRepo := git.NewRepo(cwd)
			if mainRepo.IsRepo() {
				_ = mainRepo.WorktreeRemove(state.WorktreePath, true)
				if state.WorktreeBranch != "" {
					_ = mainRepo.DeleteBranch(state.WorktreeBranch)
				}
			}
			state.mu.Lock()
			state.WorktreePath = ""
			state.WorktreeBranch = ""
			state.mu.Unlock()
		} else {
			// Has changes — append worktree info to result
			worktreeNote := fmt.Sprintf("\n\n[Worktree with changes kept at: %s (branch: %s)]", state.WorktreePath, state.WorktreeBranch)
			state.mu.Lock()
			state.Result += worktreeNote
			state.mu.Unlock()
		}
	}

	// Emit completion event (after worktree cleanup so worktree fields reflect final state)
	if err != nil {
		state.AddConversation(ConversationEntry{
			Time:    time.Now(),
			Type:    "error",
			Content: state.Error,
		})
		r.EmitEvent(TeammateEvent{
			TeamName:       cfg.TeamName,
			AgentID:        state.Identity.AgentID,
			AgentName:      cfg.AgentName,
			Type:           "error",
			Text:           state.Error,
			Color:          state.Identity.Color,
			WorktreePath:   state.WorktreePath,
			WorktreeBranch: state.WorktreeBranch,
		})
	} else {
		state.AddConversation(ConversationEntry{
			Time:    time.Now(),
			Type:    "complete",
			Content: truncateForSummary(result, 500),
		})
		r.EmitEvent(TeammateEvent{
			TeamName:       cfg.TeamName,
			AgentID:        state.Identity.AgentID,
			AgentName:      cfg.AgentName,
			Type:           "complete",
			Text:           truncateForSummary(result, 200),
			Color:          state.Identity.Color,
			WorktreePath:   state.WorktreePath,
			WorktreeBranch: state.WorktreeBranch,
		})
	}

	// Auto-complete assigned tasks
	if r.taskCompleter != nil {
		taskStatus := "completed"
		if state.Status == StatusFailed {
			taskStatus = "failed"
		}
		r.taskCompleter(cfg.AgentName, taskStatus)
	}

	// Update team status
	r.manager.UpdateMemberStatus(cfg.TeamName, state.Identity.AgentID, state.Status)

	// Send completion notification to leader's inbox
	if mb := r.getMailbox(cfg.TeamName); mb != nil {
		team, _ := r.manager.GetTeam(cfg.TeamName)
		if team != nil {
			summary := truncateForSummary(result, 200)
			if err != nil {
				summary = fmt.Sprintf("FAILED: %s", err.Error())
			}
			// Include worktree info in completion message if changes were kept
			completionText := fmt.Sprintf("[%s] Task complete: %s\n\nResult:\n%s", state.Status, state.Prompt, summary)
			if state.WorktreePath != "" {
				completionText += fmt.Sprintf("\n\n[Changes in worktree: %s (branch: %s)]", state.WorktreePath, state.WorktreeBranch)
			}
			mb.Send(state.Identity.AgentName, "team-lead", Message{
				Text:    completionText,
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

// ListTeamNames returns all known team names from the manager.
func (r *TeammateRunner) ListTeamNames() []string {
	all := r.manager.ListTeams()
	names := make([]string, len(all))
	for i, t := range all {
		names[i] = t.Name
	}
	return names
}

// SetActiveTeam explicitly sets the active team name.
func (r *TeammateRunner) SetActiveTeam(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activeTeam = name
}

// ActiveTeamName returns the explicitly set active team, or infers from running teammates.
func (r *TeammateRunner) ActiveTeamName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.activeTeam != "" {
		return r.activeTeam
	}
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

// GetMailbox returns the mailbox for the active team, if any.
func (r *TeammateRunner) GetMailbox() *Mailbox {
	return r.getMailbox(r.ActiveTeamName())
}

// getMailbox returns the mailbox for a specific team.
func (r *TeammateRunner) getMailbox(teamName string) *Mailbox {
	if teamName == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mailboxes[teamName]
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
