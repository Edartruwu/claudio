package teams

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
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
	Background     bool // true when the agent runs in the background (not blocking the lead)
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
	MemoryDir      string // agent-scoped memory directory (empty for ephemeral teammates)
	SystemPrompt   string // resolved system prompt used for the run (for revival)

	// EngineMessages holds the full API-level conversation after the agent
	// completes. Used to resume the conversation when a new message arrives.
	EngineMessages []api.Message

	// Foreground is true when the lead called WaitForOne on this agent.
	// In that case the completion event is suppressed — the lead already has
	// the result directly and a task-notification would be redundant noise.
	Foreground bool

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

// messagesSinkKey carries a callback that receives the engine's final
// message history after a sub-agent run completes. Used by the runner so
// revival can continue the same conversation.
type messagesSinkKey struct{}

// resumeHistoryKey carries pre-existing engine messages to be restored
// before the next engine.Run call (used for agent revival).
type resumeHistoryKey struct{}

// MessagesSink receives the engine's final messages after a run.
type MessagesSink func(messages []api.Message)

// WithMessagesSink installs a MessagesSink into the context.
func WithMessagesSink(ctx context.Context, sink MessagesSink) context.Context {
	return context.WithValue(ctx, messagesSinkKey{}, sink)
}

// GetMessagesSink retrieves the MessagesSink from context, or nil.
func GetMessagesSink(ctx context.Context) MessagesSink {
	if s, ok := ctx.Value(messagesSinkKey{}).(MessagesSink); ok {
		return s
	}
	return nil
}

// WithResumeHistory installs pre-existing engine messages into the context.
func WithResumeHistory(ctx context.Context, history []api.Message) context.Context {
	return context.WithValue(ctx, resumeHistoryKey{}, history)
}

// GetResumeHistory retrieves the pre-existing engine messages from context.
func GetResumeHistory(ctx context.Context) []api.Message {
	if h, ok := ctx.Value(resumeHistoryKey{}).([]api.Message); ok {
		return h
	}
	return nil
}

// RunAgentFunc is the callback to execute an agent.
// Receives (ctx, systemPrompt, userPrompt) and returns text output.
type RunAgentFunc func(ctx context.Context, system, prompt string) (string, error)

// RunAgentWithMemoryFunc is the callback to execute an agent with agent-scoped
// memory injection. Used when a teammate is backed by a crystallized agent that
// has its own memory directory.
type RunAgentWithMemoryFunc func(ctx context.Context, system, prompt, memoryDir string) (string, error)

// RunAgentResumeFunc resumes an existing agent conversation. It receives the
// system prompt, optional memory directory, the full prior message history,
// and the new user message to append. Returns the agent's new response text.
type RunAgentResumeFunc func(ctx context.Context, system, memoryDir string, history []api.Message, newMessage string) (string, error)

// ContextDecorator allows the app layer to inject per-teammate context values
// (e.g., SubAgentObserver, SubAgentDB) without the teams package importing tools.
type ContextDecorator func(ctx context.Context, state *TeammateState) context.Context

// CwdInjector injects a CWD override into context. Set by the app layer so
// the teams package doesn't need to import the tools package.
// mainRoot is the main repo root from which the worktree was forked; it is
// used by file tools to remap absolute main-repo paths into worktree paths.
type CwdInjector func(ctx context.Context, worktreePath, mainRoot string) context.Context

// TaskCompleter is called when an agent finishes to update assigned tasks.
// Set by the app layer to bridge teams → tasks without circular imports.
// taskIDs are the explicit task IDs to mark; status is "completed" or "failed".
type TaskCompleter func(taskIDs []string, status string)

// TeammateRunner manages in-process teammate goroutines.
type TeammateRunner struct {
	mu                 sync.RWMutex
	teammates          map[string]*TeammateState // keyed by agent ID
	manager            *Manager
	mailboxes          map[string]*Mailbox
	runAgent           RunAgentFunc
	runAgentResume     RunAgentResumeFunc
	runAgentWithMemory RunAgentWithMemoryFunc
	eventHandler       TeammateEventHandler
	parentCtx          context.Context // parent context with observer/DB from TUI
	contextDecorator   ContextDecorator
	cwdInjector        CwdInjector
	taskCompleter      TaskCompleter
	activeTeam         string // explicitly set active team name
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

// SetRunAgentWithMemory sets the callback used when spawning teammates that
// are backed by a crystallized agent (i.e., one with a MemoryDir). When set,
// teammates with a non-empty SpawnConfig.MemoryDir will use this runner so
// the agent's accumulated memory is loaded.
func (r *TeammateRunner) SetRunAgentWithMemory(fn RunAgentWithMemoryFunc) {
	r.runAgentWithMemory = fn
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
	TeamName     string
	AgentName    string
	Prompt       string
	System       string   // system prompt override
	Model        string   // model override
	SubagentType string   // agent definition used (e.g. "backend-senior", "prab")
	MaxTurns     int      // optional max agentic turns (0 = unlimited)
	Isolation    string   // "worktree" for git worktree isolation
	MemoryDir    string   // optional agent-scoped memory directory (for crystallized agents)
	Foreground   bool     // true when the lead is blocking on WaitForOne — suppresses task-notification
	TaskIDs      []string // task IDs to auto-complete when agent finishes
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
	member, err := r.manager.AddMember(cfg.TeamName, cfg.AgentName, cfg.Model, cfg.Prompt, cfg.SubagentType)
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
		MemoryDir:    cfg.MemoryDir,
		Foreground:   cfg.Foreground,
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
	var worktreeHeadCommit string // SHA captured before agent runs, used to detect new commits
	if cfg.Isolation == "worktree" {
		cwd, _ := os.Getwd()
		repo := git.NewRepo(cwd)
		if repo.IsRepo() {
			// Use a timestamp suffix so concurrent/repeated runs don't collide.
			runID := fmt.Sprintf("%d", time.Now().UnixMilli()%1000000)
			branch := fmt.Sprintf("claudio/%s/%s-%s", cfg.TeamName, cfg.AgentName, runID)
			root, _ := repo.Root()
			wtPath := filepath.Join(root, ".claudio-worktrees", branch)
			// Capture HEAD SHA before creating worktree — used to detect new commits later
			if sha, err := repo.HeadCommit(); err == nil {
				worktreeHeadCommit = sha
			}
			wtErr := repo.WorktreeAddOrReuse(wtPath, branch)
			if wtErr == nil {
				worktreeRepo = git.NewRepo(wtPath)

				state.mu.Lock()
				state.WorktreePath = wtPath
				state.WorktreeBranch = branch
				state.mu.Unlock()

				// Inject CWD override + main root into context so file tools
				// can remap absolute main-repo paths into worktree paths.
				if r.cwdInjector != nil {
					ctx = r.cwdInjector(ctx, wtPath, root)
				}
			}
		}
	}

	// Emit started event
	r.EmitEvent(TeammateEvent{
		TeamName:   cfg.TeamName,
		AgentID:    state.Identity.AgentID,
		AgentName:  cfg.AgentName,
		Type:       "started",
		Text:       cfg.Prompt,
		Color:      state.Identity.Color,
		Background: !state.Foreground,
	})

	// Build system prompt for teammate.
	// Teammate context is always appended; if the agent has its own persona prompt
	// (from a crystallized agent or custom agent definition), that comes first.
	teammateCtx := fmt.Sprintf(`You are %s, a teammate in the "%s" team.

Your role: Complete your assigned task and report results clearly.

Guidelines:
- Focus on your specific task
- Report findings concisely when done
- If you need help from another teammate, explain what you need
- When finished, provide a clear summary of what you accomplished

Your task will be provided in the user message.`, cfg.AgentName, cfg.TeamName)

	var system string
	if cfg.System != "" {
		system = cfg.System + "\n\n" + teammateCtx
	} else {
		system = teammateCtx
	}

	// Add worktree notice to system prompt
	if state.WorktreePath != "" {
		cwd, _ := os.Getwd()
		system += fmt.Sprintf("\n\nYou are operating in an isolated git worktree at %s — same repository, same relative file structure, separate working copy. "+
			"Paths in conversation context from the parent agent refer to %s; translate them to your worktree root. "+
			"Re-read files before editing if the parent may have modified them. "+
			"Your changes stay in this worktree and will not affect the parent's files.\n\n"+
			"IMPORTANT: When you have finished making changes, you MUST commit them with git before returning your final response. "+
			"Stage all modified files and create a descriptive commit. This is required so your work can be reviewed and merged — "+
			"uncommitted changes in a worktree cannot be diffed or merged by the team lead.", state.WorktreePath, cwd)
	}

	// Persist the resolved system prompt so revival can reuse it verbatim.
	state.mu.Lock()
	state.SystemPrompt = system
	state.mu.Unlock()

	// Install a messages sink so we capture the engine's final history for
	// potential revival. The sink is honored by runSubAgentWithMemory.
	ctx = WithMessagesSink(ctx, func(msgs []api.Message) {
		state.mu.Lock()
		state.EngineMessages = msgs
		state.mu.Unlock()
	})

	var result string
	var err error
	if state.MemoryDir != "" && r.runAgentWithMemory != nil {
		result, err = r.runAgentWithMemory(ctx, system, cfg.Prompt, state.MemoryDir)
	} else {
		result, err = r.runAgent(ctx, system, cfg.Prompt)
	}

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

	// Worktree cleanup: remove if no changes, keep if there are changes (committed or not).
	// Uses the pre-fork HEAD SHA to detect both uncommitted files and new commits.
	if worktreeRepo != nil && state.WorktreePath != "" {
		hasChanges, chkErr := worktreeRepo.HasAnyWork(worktreeHeadCommit)
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

	// Auto-complete assigned tasks
	if r.taskCompleter != nil && len(cfg.TaskIDs) > 0 {
		taskStatus := "completed"
		if state.Status == StatusFailed {
			taskStatus = "failed"
		}
		r.taskCompleter(cfg.TaskIDs, taskStatus)
	}

	// Update team status
	r.manager.UpdateMemberStatus(cfg.TeamName, state.Identity.AgentID, state.Status)

	// Send completion notification to leader's inbox BEFORE emitting the event,
	// so the TUI's ReadUnread call (triggered by the event) sees the message and
	// marks it read — avoiding a race where the inbox write arrives after the read.
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

	// Emit completion event (after worktree cleanup so worktree fields reflect final state).
	// Suppressed for foreground agents — the lead is blocking on WaitForOne and already
	// has the result; firing an event would produce a redundant task-notification.
	if err != nil {
		state.AddConversation(ConversationEntry{
			Time:    time.Now(),
			Type:    "error",
			Content: state.Error,
		})
		if !state.Foreground {
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
		}
	} else {
		state.AddConversation(ConversationEntry{
			Time:    time.Now(),
			Type:    "complete",
			Content: truncateForSummary(result, 500),
		})
		if !state.Foreground {
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

// KillTeam cancels all running teammates belonging to the given team.
// Members of other teams are unaffected.
func (r *TeammateRunner) KillTeam(teamName string) {
	r.mu.RLock()
	states := make([]*TeammateState, 0)
	for _, s := range r.teammates {
		if s.TeamName == teamName {
			states = append(states, s)
		}
	}
	r.mu.RUnlock()

	for _, s := range states {
		if s.cancel != nil {
			s.cancel()
		}
	}
}

// WaitForTeam blocks until every teammate of the given team is idle, or timeout.
// Returns true if all became idle within the timeout.
func (r *TeammateRunner) WaitForTeam(teamName string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		allIdle := true
		r.mu.RLock()
		for _, s := range r.teammates {
			if s.TeamName != teamName {
				continue
			}
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
		case <-time.After(100 * time.Millisecond):
			continue
		}
	}
}

// CleanupTeam removes all in-memory state for a team: teammate states and the
// mailbox entry. Callers must ensure members are idle (use KillTeam +
// WaitForTeam) before calling, otherwise running goroutines will continue but
// be invisible to the runner.
func (r *TeammateRunner) CleanupTeam(teamName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, s := range r.teammates {
		if s.TeamName == teamName {
			delete(r.teammates, id)
		}
	}
	delete(r.mailboxes, teamName)

	if r.activeTeam == teamName {
		r.activeTeam = ""
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

// SetRunAgentResume wires the resume callback used by Revive.
func (r *TeammateRunner) SetRunAgentResume(fn RunAgentResumeFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runAgentResume = fn
}

// Revive resumes a completed or failed agent with a new message, continuing
// the existing conversation (full API-level history is preserved). If the
// agent is still working, the message will be picked up via pollMailbox
// naturally and Revive is a no-op.
func (r *TeammateRunner) Revive(agentName, newMessage string) error {
	state, ok := r.GetStateByName(agentName)
	if !ok {
		return fmt.Errorf("agent %q not found", agentName)
	}

	state.mu.Lock()
	if !state.IsIdle || state.Status == StatusShutdown {
		state.mu.Unlock()
		return nil // still working or explicitly killed — nothing to do
	}
	if r.runAgentResume == nil {
		state.mu.Unlock()
		return fmt.Errorf("resume callback not wired")
	}

	// Reset state for another run
	history := state.EngineMessages
	state.IsIdle = false
	state.Status = StatusWorking
	state.Error = ""
	state.idleCh = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	state.cancel = cancel
	state.mu.Unlock()

	cfg := SpawnConfig{
		TeamName:  state.TeamName,
		AgentName: state.Identity.AgentName,
		MaxTurns:  state.MaxTurns,
	}

	r.manager.UpdateMemberStatus(cfg.TeamName, state.Identity.AgentID, StatusWorking)

	go func() {
		defer func() {
			state.mu.Lock()
			state.IsIdle = true
			close(state.idleCh)
			state.mu.Unlock()
		}()

		resumeCtx := ctx
		if r.contextDecorator != nil {
			resumeCtx = r.contextDecorator(ctx, state)
		}

		result, err := r.runAgentResume(resumeCtx, state.SystemPrompt, state.MemoryDir, history, newMessage)

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

		r.manager.UpdateMemberStatus(cfg.TeamName, state.Identity.AgentID, state.Status)

		if mb := r.getMailbox(cfg.TeamName); mb != nil {
			summary := truncateForSummary(result, 200)
			if err != nil {
				summary = fmt.Sprintf("FAILED: %s", state.Error)
			}
			completionText := fmt.Sprintf("[%s] Task complete: %s\n\nResult:\n%s", state.Status, state.Prompt, summary)
			mb.Send(state.Identity.AgentName, "team-lead", Message{
				Text:    completionText,
				Summary: fmt.Sprintf("%s: %s", state.Identity.AgentName, state.Status),
				Color:   state.Identity.Color,
			})
		}

		r.EmitEvent(TeammateEvent{
			TeamName:  cfg.TeamName,
			AgentID:   state.Identity.AgentID,
			AgentName: cfg.AgentName,
			Type:      func() string {
				if err != nil {
					return "error"
				}
				return "complete"
			}(),
			Text:  truncateForSummary(result, 200),
			Color: state.Identity.Color,
		})
	}()

	return nil
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
