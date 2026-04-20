package teams

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/git"
	"github.com/Abraxas-365/claudio/internal/prompts"
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
	Input          string // truncated tool input (for "tool_start" events)
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
	Identity             TeammateIdentity
	TeamName             string
	Prompt               string
	Model                string // model override for this teammate
	MaxTurns             int    // optional max agentic turns (0 = unlimited)
	AutoCompactThreshold int    // % of context window to trigger full compact (0 = engine default 95%)
	Status               MemberStatus
	Progress       TeammateProgress
	Result         string // final output
	Error          string
	IsIdle         bool
	StartedAt      time.Time
	FinishedAt     time.Time
	Conversation   []ConversationEntry
	WorktreePath     string // path to git worktree (empty if no isolation)
	WorktreeBranch   string // branch name used for the worktree
	WorktreeMainRoot string // main repo root from which the worktree was forked
	MemoryDir      string // agent-scoped memory directory (empty for ephemeral teammates)
	SystemPrompt   string // resolved system prompt used for the run (for revival)

	// CurrentTool is the name of the tool currently executing (empty when idle).
	CurrentTool string

	// ParentAgentID is non-empty when this agent was spawned by another teammate.
	ParentAgentID string

	// EngineMessages holds the full API-level conversation after the agent
	// completes. Used to resume the conversation when a new message arrives.
	EngineMessages []api.Message

	// AdvisorConfig holds the advisor specification for this agent, if any.
	// Set by Spawn() and used by the context decorator to inject the advisor tool.
	AdvisorConfig *AdvisorConfig

	// MergeStatus records the outcome of worktree cleanup after the agent finishes.
	// One of: "auto-merged: ...", "merge-failed: ...", "no-changes: ...", or "" if no worktree.
	MergeStatus string

	// InactivityCount tracks how many human messages have been sent without
	// this agent receiving any message. Incremented by IncrementInactivity;
	// reset to 0 by SendMessage when a message is routed to this agent.
	InactivityCount int

	// Foreground is true when the lead called WaitForOne on this agent.
	// In that case the completion event is suppressed — the lead already has
	// the result directly and a task-notification would be redundant noise.
	Foreground bool

	cancel context.CancelFunc
	mu     sync.Mutex
	idleCh chan struct{} // closed when teammate becomes idle
}

// GetEngineMessages returns the current engine message history, thread-safe.
// Used by the advisor tool to get the executor's live conversation transcript.
func (s *TeammateState) GetEngineMessages() []api.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.EngineMessages
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

// SetCurrentTool sets the currently executing tool name, thread-safe.
func (s *TeammateState) SetCurrentTool(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentTool = name
}

// GetCurrentTool returns the currently executing tool name, thread-safe.
func (s *TeammateState) GetCurrentTool() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.CurrentTool
}

// GetCallCount returns the current tool call count, thread-safe.
func (s *TeammateState) GetCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Progress.ToolCalls
}

// GetElapsedSecs returns seconds since agent started, thread-safe.
func (s *TeammateState) GetElapsedSecs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.StartedAt.IsZero() {
		return 0
	}
	return int(time.Since(s.StartedAt).Seconds())
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

// ctxKeyTeammateAgentID carries the running agent's own ID into sub-calls,
// so SpawnTeammate can record the parent-child relationship.
type ctxKeyTeammateAgentID struct{}

// WithTeammateAgentID stores the current agent's ID in ctx.
func WithTeammateAgentID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyTeammateAgentID{}, id)
}

// TeammateAgentIDFromContext retrieves the running agent's ID from ctx, or "".
func TeammateAgentIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyTeammateAgentID{}).(string)
	return v
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
	PluginsSection     string // injected into every sub-agent's system prompt
	Settings           *config.Settings // optional; used to inject caveman prefix
	eventBus           *bus.Bus // optional; used to publish agent status events
	sessionID          string   // principal session ID; stamped on published bus events for routing

	childrenMu sync.Mutex
	children   map[string][]string // parentAgentID → []childAgentID
}

// NewTeammateRunner creates a runner for spawning in-process teammates.
func NewTeammateRunner(manager *Manager, runAgent RunAgentFunc) *TeammateRunner {
	return &TeammateRunner{
		teammates: make(map[string]*TeammateState),
		mailboxes: make(map[string]*Mailbox),
		manager:   manager,
		runAgent:  runAgent,
		parentCtx: context.Background(),
		children:  make(map[string][]string),
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

// SetBus sets the event bus for publishing agent status events.
func (r *TeammateRunner) SetBus(b *bus.Bus) {
	r.eventBus = b
}

// SetSessionID records the session this runner belongs to.
// The ID is stamped on every bus.Event published by this runner.
// SetSessionID sets the principal session ID that is stamped on bus events,
// allowing subscribers to filter notifications for the correct session.
func (r *TeammateRunner) SetSessionID(id string) {
	r.sessionID = id
}

// EmitEvent sends an event to the registered handler.
func (r *TeammateRunner) EmitEvent(event TeammateEvent) {
	// Persist event into the agent's TeammateState so the AGUI panel can read
	// live conversation / tool / activity data without relying solely on TUI callbacks.
	if event.AgentID != "" {
		r.mu.RLock()
		state, ok := r.teammates[event.AgentID]
		r.mu.RUnlock()
		if ok {
			switch event.Type {
			case "text":
				if event.Text != "" {
					state.AddConversation(ConversationEntry{
						Time:    time.Now(),
						Type:    "text",
						Content: event.Text,
					})
					state.AddActivity(truncateActivity(event.Text, 80))
				}
			case "tool_start":
				state.IncrToolCalls()
				state.SetCurrentTool(event.ToolName)
				state.AddConversation(ConversationEntry{
					Time:     time.Now(),
					Type:     "tool_start",
					Content:  event.Input,
					ToolName: event.ToolName,
				})
				state.AddActivity("→ " + event.ToolName)
			case "tool_end":
				state.SetCurrentTool("")
				state.AddConversation(ConversationEntry{
					Time:     time.Now(),
					Type:     "tool_end",
					Content:  truncateActivity(event.Text, 200),
					ToolName: event.ToolName,
				})
			case "complete":
				state.AddConversation(ConversationEntry{
					Time:    time.Now(),
					Type:    "complete",
					Content: event.Text,
				})
			case "error":
				state.AddConversation(ConversationEntry{
					Time:    time.Now(),
					Type:    "error",
					Content: event.Text,
				})
			}
		}
	}

	if r.eventHandler != nil {
		r.eventHandler.OnTeammateEvent(event)
	}
}

func truncateActivity(s string, max int) string {
	// strip newlines for single-line activity display
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}

// SpawnConfig defines how to spawn a teammate.
type SpawnConfig struct {
	TeamName             string
	AgentName            string
	Prompt               string
	System               string         // system prompt override
	Model                string         // model override
	SubagentType         string         // agent definition used (e.g. "backend-senior", "prab")
	MaxTurns             int            // optional max agentic turns (0 = unlimited)
	Isolation            string         // "worktree" for git worktree isolation
	MemoryDir            string         // optional agent-scoped memory directory (for crystallized agents)
	Foreground           bool           // true when the lead is blocking on WaitForOne — suppresses task-notification
	TaskIDs              []string       // task IDs to auto-complete when agent finishes
	AutoCompactThreshold int            // % of context window to trigger full compact (0 = engine default 95%)
	ParentAgentID        string         // non-empty when spawned by another teammate
	AdvisorConfig        *AdvisorConfig // optional; if set, advisor tool is injected into executor
}

// Spawn starts a new teammate goroutine.
func (r *TeammateRunner) Spawn(cfg SpawnConfig) (*TeammateState, error) {
	// Fall back to team-level default model if no per-agent model specified
	if cfg.Model == "" {
		if team, ok := r.manager.GetTeam(cfg.TeamName); ok && team.Model != "" {
			cfg.Model = team.Model
		}
	}

	// Resolve auto-compact threshold: per-member (stored from InstantiateTeam) >
	// team-level default > 0 (engine default 95%).
	if cfg.AutoCompactThreshold <= 0 {
		if team, ok := r.manager.GetTeam(cfg.TeamName); ok {
			agentID := FormatAgentID(cfg.AgentName, cfg.TeamName)
			for _, mem := range team.Members {
				if mem.Identity.AgentID == agentID && mem.AutoCompactThreshold > 0 {
					cfg.AutoCompactThreshold = mem.AutoCompactThreshold
					break
				}
			}
			if cfg.AutoCompactThreshold <= 0 {
				cfg.AutoCompactThreshold = team.AutoCompactThreshold
			}
		}
	}

	// Resolve advisor config: explicit SpawnConfig value takes priority; fall back
	// to the per-member value stored by InstantiateTeam (same pattern as AutoCompactThreshold).
	if cfg.AdvisorConfig == nil {
		if team, ok := r.manager.GetTeam(cfg.TeamName); ok {
			agentID := FormatAgentID(cfg.AgentName, cfg.TeamName)
			for _, mem := range team.Members {
				if mem.Identity.AgentID == agentID && mem.AdvisorConfig != nil {
					cfg.AdvisorConfig = mem.AdvisorConfig
					break
				}
			}
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
		Identity:             member.Identity,
		TeamName:             cfg.TeamName,
		Prompt:               cfg.Prompt,
		Model:                cfg.Model,
		MaxTurns:             cfg.MaxTurns,
		AutoCompactThreshold: cfg.AutoCompactThreshold,
		MemoryDir:            cfg.MemoryDir,
		Foreground:           cfg.Foreground,
		AdvisorConfig:        cfg.AdvisorConfig,
		Status:       StatusWorking,
		StartedAt:    time.Now(),
		cancel:       cancel,
		idleCh:       make(chan struct{}),
		Conversation: make([]ConversationEntry, 0, 32),
	}

	if cfg.ParentAgentID != "" {
		state.ParentAgentID = cfg.ParentAgentID
		r.childrenMu.Lock()
		r.children[cfg.ParentAgentID] = append(r.children[cfg.ParentAgentID], member.Identity.AgentID)
		r.childrenMu.Unlock()
	} else {
		// Agents spawned directly by the TUI session (no parent teammate) are
		// principal agents — mark them as lead so sub-agents can be identified.
		state.Identity.IsLead = true
	}

	r.mu.Lock()
	r.teammates[member.Identity.AgentID] = state
	r.mu.Unlock()

	// Update team status
	r.manager.UpdateMemberStatus(cfg.TeamName, member.Identity.AgentID, StatusWorking)

	// Emit agent spawn event (initial status="running") so agent appears immediately in CommandCenter team list.
	// Without this, agents only appear after completion (EventAgentStatus fired in runTeammate cleanup).
	if r.eventBus != nil {
		payload, _ := json.Marshal(attach.AgentStatusPayload{
			Name:          cfg.AgentName,
			Status:        "working",
			CurrentTool:   state.GetCurrentTool(),
			CallCount:     state.GetCallCount(),
			ElapsedSecs:   state.GetElapsedSecs(),
			ParentAgentID: cfg.ParentAgentID,
		})
		r.eventBus.Publish(bus.Event{
			Type:      attach.EventAgentStatus,
			SessionID: r.sessionID,
			Payload:   payload,
		})
	}

	// Set up mailbox for this team (one per team)
	if _, ok := r.mailboxes[cfg.TeamName]; !ok {
		team, _ := r.manager.GetTeam(cfg.TeamName)
		if team != nil {
			r.mailboxes[cfg.TeamName] = NewMailbox(r.manager.teamsDir, cfg.TeamName)
		}
	}

	// Launch goroutine
	go r.runTeammate(ctx, state, cfg)

	// Launch heartbeat ticker — emits periodic EventAgentStatus so the Team tab
	// shows live metrics (call_count, elapsed_secs, current_tool) while running.
	if r.eventBus != nil {
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					payload, _ := json.Marshal(attach.AgentStatusPayload{
						Name:          cfg.AgentName,
						Status:        "working",
						CurrentTool:   state.GetCurrentTool(),
						CallCount:     state.GetCallCount(),
						ElapsedSecs:   state.GetElapsedSecs(),
						ParentAgentID: cfg.ParentAgentID,
					})
					r.eventBus.Publish(bus.Event{
						Type:      attach.EventAgentStatus,
						SessionID: r.sessionID,
						Payload:   payload,
					})
				case <-state.idleCh:
					return
				}
			}
		}()
	}

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
	if cfg.Isolation == "worktree" {
		cwd, _ := os.Getwd()
		repo := git.NewRepo(cwd)
		if repo.IsRepo() {
			// Use a timestamp suffix so concurrent/repeated runs don't collide.
			runID := fmt.Sprintf("%d", time.Now().UnixMilli()%1000000)
			branch := fmt.Sprintf("claudio/%s/%s-%s", cfg.TeamName, cfg.AgentName, runID)
			root, _ := repo.Root()
			wtPath := filepath.Join(root, ".claudio-worktrees", branch)
			// Note: HEAD SHA capture removed — worktrees no longer auto-cleaned on agent completion.
			// All worktrees are preserved; cleanup is explicit via PurgeDone().
			wtErr := repo.WorktreeAddOrReuse(wtPath, branch)
			if wtErr == nil {
				_ = git.NewRepo(wtPath) // worktree repo created but no longer tracked for auto-cleanup

				state.mu.Lock()
				state.WorktreePath = wtPath
				state.WorktreeBranch = branch
				state.WorktreeMainRoot = root
				state.mu.Unlock()

				// Inject CWD override + main root into context so file tools
				// can remap absolute main-repo paths into worktree paths.
				if r.cwdInjector != nil {
					ctx = r.cwdInjector(ctx, wtPath, root)
				}
			} else {
				// Worktree creation failed (e.g. repo has no commits yet).
				// Continue without isolation and warn the team lead in the TUI.
				r.EmitEvent(TeammateEvent{
					TeamName:  cfg.TeamName,
					AgentID:   state.Identity.AgentID,
					AgentName: cfg.AgentName,
					Type:      "warning",
					Text:      fmt.Sprintf("worktree isolation failed (%v) — running in main repo", wtErr),
					Color:     state.Identity.Color,
				})
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

## Escalating decisions to the team lead

If you hit a decision you cannot resolve confidently (ambiguous requirements, architectural fork, missing context), do NOT guess. Instead:

1. Finish any safe work you can complete
2. Commit your progress (if working in a worktree)
3. End your final result with exactly this marker on its own line:

   QUESTION: <your focused, single question>

Ask one question at a time — never a list. After sending, you will go idle. The team lead will answer (possibly after consulting the user) via SendMessage, and you will automatically resume with your full conversation history intact — continue from exactly where you left off using the answer.

A short pause for a good answer beats hours of rework on the wrong approach.` +
		"\n\n## Memory — check before exploring, update after discovering\n\n" +
		"Before spawning an Explore sub-agent for codebase knowledge, always check memory first:\n" +
		"1. `Memory(action=\"search\", query=\"<directory or topic>\")` or `Memory(action=\"read\", name=\"<path-slug>\")`\n" +
		"2. If found and relevant, use it — skip the Explore agent entirely\n" +
		"3. If not found or clearly stale, spawn Explore as normal\n\n" +
		"After the Explore agent returns structural findings about a directory or module, save them:\n" +
		"`Memory(action=\"save\", name=\"<path-slug>\", description=\"...\", facts=[...], tags=[\"codebase-map\"])`\n\n" +
		"Key naming rule: lowercase the directory path, replace `/` with `-`, strip leading `/`\n" +
		"(e.g. `internal/tools` → `internal-tools`, `src/auth` → `src-auth`, `lib/payments` → `lib-payments`)" +
		`

## Skills — check at task start

Before doing any work, check whether a skill matches your task. Skills provide domain-specific instructions, conventions, and step-by-step procedures that override default behavior.

- Call Skill(skill="<name>") at the start of your task if a skill matches (e.g. "commit", "review", "caveman")
- The Skill tool lists all available skills in its description — check it
- If you are a specialized agent (e.g. go-htmx-frontend), proactively invoke any skill that matches your domain

## Context management — REQUIRED

**Do NOT read files directly for exploration or investigation.** Instead, always spawn an Explore sub-agent via the Agent tool (subagent_type="Explore") and describe what you need to know.

The Explore agent uses a smaller model, returns a focused summary, and keeps your context clean. You MUST use it for:
- Understanding directory structure or file layout
- Reading multiple files to understand a codebase
- Searching for symbols, patterns, or code structure
- Any task where you are gathering information rather than writing code

Only use Read/Grep/Glob directly when:
- You are about to edit a specific file you already know the path to
- The lookup is a single targeted read (one file, < 50 lines)

## Tool discipline — REQUIRED

**Never use bash to read or edit files.** Always use the dedicated tools:
- Read a file: use Read (offset + limit for line ranges) — NEVER cat, head, tail, sed -n, grep -n "."
- Edit a file: use Edit (exact string replacement) — NEVER sed, awk, or bash redirection
- Create a file: use Write — NEVER echo > or cat <<EOF
- Search content: use Grep — NEVER grep or rg directly
- Find files: use Glob — NEVER find or ls

**If Read returns "file not found":** the file does not yet exist — create it with Write. Do NOT conclude that Read/Edit/Write are broken and switch to bash.

**Read supports line ranges:** use offset + limit parameters instead of sed -n 'X,Yp'. Example: to read lines 100-150, pass offset=100, limit=50.

## Retry discipline

When running tests, builds, or validations:
1. Run → read the actual output
2. If it fails, diagnose the error, fix it, run again
3. After **3 failed attempts** on the same failure, stop — do not guess a fourth fix. Escalate with QUESTION: instead.

This applies to any iterative loop: tests, lint, build, deployment checks. Always report how many attempts you made.

## Completion report

When your task is done, always end your final response with this section:

### Done
- **Outcome**: what was done, found, produced, or changed — and why
- **Evidence**: test/build/validation output for implementers; key findings, files, or symbols for researchers (paste actual output, not summaries)
- **Attempts**: note how many attempts if anything failed or needed retrying
- **Risks / follow-ups**: deferred decisions, open questions, or anything the team lead should know (or "none")
- **Plan file**: if you created a plan (via EnterPlanMode/ExitPlanMode), include the full path to the plan file here so the team lead can review it directly

Your task will be provided in the user message.`, cfg.AgentName, cfg.TeamName)

	if r.PluginsSection != "" {
		teammateCtx += "\n\n" + r.PluginsSection
	}

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
			"WORKTREE TOOL USAGE: The Read, Edit, Write, Glob, and Grep tools are fully path-remapped into your worktree automatically — always use them. "+
			"Do NOT use bash/sed/awk for file reading or editing even in a worktree. "+
			"If Read returns 'file not found', the file does not exist in your worktree yet — create it with Write.\n\n"+
			"IMPORTANT: When you have finished making changes, you MUST commit them with git before returning your final response. "+
			"Stage all modified files and create a descriptive commit. This is required so your work can be reviewed and merged — "+
			"uncommitted changes in a worktree cannot be diffed or merged by the team lead.", state.WorktreePath, cwd)
	}

	// Append advisor protocol section when an advisor is configured.
	if cfg.AdvisorConfig != nil {
		system += "\n\n" + prompts.AdvisorProtocolSection()
	}

	// Persist the resolved system prompt so revival can reuse it verbatim.
	state.mu.Lock()
	state.SystemPrompt = system
	state.mu.Unlock()

	// Also persist to disk so we can reconstruct state after eviction.
	r.manager.UpdateMemberSystemPrompt(cfg.TeamName, state.Identity.AgentID, system)

	// Install a messages sink so we capture the engine's final history for
	// potential revival. The sink is honored by runSubAgentWithMemory.
	ctx = WithMessagesSink(ctx, func(msgs []api.Message) {
		state.mu.Lock()
		state.EngineMessages = msgs
		state.mu.Unlock()
	})

	// Inject this agent's ID so any SpawnTeammate calls it makes can record
	// themselves as children of this agent.
	ctx = WithTeammateAgentID(ctx, state.Identity.AgentID)

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
	} else if containsQuestion(result) {
		state.Status = StatusWaitingForInput
		state.Result = result
	} else {
		state.Status = StatusComplete
		state.Result = result
	}
	status := state.Status
	state.mu.Unlock()

	// Publish agent status event.
	// Normalize internal MemberStatus to the protocol status vocabulary:
	// StatusComplete → "done" (matches attach.AgentStatusPayload comment: idle|working|done|waiting).
	if r.eventBus != nil {
		protocolStatus := string(status)
		if status == StatusComplete {
			protocolStatus = "done"
		}
		payload, _ := json.Marshal(attach.AgentStatusPayload{
			Name:          state.Identity.AgentName,
			Status:        protocolStatus,
			CurrentTool:   state.GetCurrentTool(),
			CallCount:     state.GetCallCount(),
			ElapsedSecs:   state.GetElapsedSecs(),
			Result:        state.Result,
			ParentAgentID: state.ParentAgentID,
		})
		r.eventBus.Publish(bus.Event{
			Type:      attach.EventAgentStatus,
			SessionID: r.sessionID,
			Payload:   payload,
		})
	}

	// Worktree preservation: all worktrees are kept after agent finishes.
	// Cleanup is explicitly requested via PurgeDone() or explicit deletion — not automatic.
	// This ensures agent work is not lost if cleanup is triggered unintentionally.
	if state.WorktreePath != "" {
		worktreeNote := fmt.Sprintf("\n\n[Agent worktree preserved at: %s (branch: %s) — use PurgeDone() to remove when ready]", state.WorktreePath, state.WorktreeBranch)
		state.mu.Lock()
		state.Result += worktreeNote
		state.mu.Unlock()
	}

	// Auto-complete assigned tasks
	if r.taskCompleter != nil && len(cfg.TaskIDs) > 0 {
		taskStatus := "completed"
		if state.Status == StatusFailed {
			taskStatus = "failed"
		}
		r.taskCompleter(cfg.TaskIDs, taskStatus)
	}

	// Kill and remove any child agents spawned by this teammate.
	r.childrenMu.Lock()
	childIDs := r.children[state.Identity.AgentID]
	delete(r.children, state.Identity.AgentID)
	r.childrenMu.Unlock()
	for _, childID := range childIDs {
		_ = r.Kill(childID) // best-effort
	}
	if len(childIDs) > 0 {
		// Give children a moment to stop.
		deadline := time.Now().Add(5 * time.Second)
		for _, childID := range childIDs {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				break
			}
			r.WaitForOne(childID, remaining)
		}
		// Remove child states from the runner.
		r.mu.Lock()
		for _, childID := range childIDs {
			delete(r.teammates, childID)
		}
		r.mu.Unlock()
	}

	// Update team status
	r.manager.UpdateMemberStatus(cfg.TeamName, state.Identity.AgentID, state.Status)

	// Send completion notification to leader's inbox BEFORE emitting the event,
	// so the TUI's ReadUnread call (triggered by the event) sees the message and
	// marks it read — avoiding a race where the inbox write arrives after the read.
	// Skip for foreground (synchronous) spawns — the caller already receives the
	// result directly; mailbox messages are for async coordination only.
	if !state.Foreground {
		if mb := r.getMailbox(cfg.TeamName); mb != nil {
			team, _ := r.manager.GetTeam(cfg.TeamName)
			if team != nil {
				// Use state.Result (not raw result) so worktree notes appended during
				// cleanup (auto-merge success/failure, kept-for-inspection) are included.
				summary := state.Result
				if err != nil {
					summary = fmt.Sprintf("FAILED: %s", err.Error())
				}
				mergeInfo := ""
				if state.MergeStatus != "" {
					mergeInfo = fmt.Sprintf("\nMerge status: %s", state.MergeStatus)
				}
				completionText := fmt.Sprintf("[%s] Task complete: %s%s\n\nResult:\n%s", state.Status, state.Prompt, mergeInfo, summary)
				mb.Send(state.Identity.AgentName, "team-lead", Message{
					Text:    completionText,
					Summary: fmt.Sprintf("%s: %s", state.Identity.AgentName, state.Status),
					Color:   state.Identity.Color,
				})
			}
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
			Content: result,
		})
		if !state.Foreground {
			r.EmitEvent(TeammateEvent{
				TeamName:       cfg.TeamName,
				AgentID:        state.Identity.AgentID,
				AgentName:      cfg.AgentName,
				Type:           "complete",
				Text:           result,
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

// GetStateByName returns a teammate's state by agent name or agent ID.
func (r *TeammateRunner) GetStateByName(name string) (*TeammateState, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.teammates {
		if s.Identity.AgentName == name || s.Identity.AgentID == name {
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

// getMailboxLocked is like getMailbox but assumes r.mu is already held.
func (r *TeammateRunner) getMailboxLocked(teamName string) *Mailbox {
	if teamName == "" {
		return nil
	}
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

// RemoveAgent removes a finished/failed/shutdown agent from the runner and team config.
// Returns an error if the agent is still running.
func (r *TeammateRunner) RemoveAgent(agentID string) error {
	r.mu.Lock()
	s, ok := r.teammates[agentID]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	if s.Status == StatusWorking {
		r.mu.Unlock()
		return fmt.Errorf("agent %q is still running", agentID)
	}
	teamName := s.TeamName
	delete(r.teammates, agentID)
	r.mu.Unlock()
	if r.manager != nil && teamName != "" {
		_ = r.manager.RemoveMember(teamName, agentID)
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
		// State was evicted by IncrementInactivity — try to reconstruct
		// from the persisted team config so the agent can be revived.
		state, ok = r.reconstructStateFromConfig(agentName)
		if !ok {
			return fmt.Errorf("agent %q not found", agentName)
		}
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
	worktreePath := state.WorktreePath
	worktreeMainRoot := state.WorktreeMainRoot
	state.IsIdle = false
	state.Status = StatusWorking
	state.Error = ""
	state.idleCh = make(chan struct{})
	ctx, cancel := context.WithCancel(r.parentCtx)
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
		// Restore worktree CWD so revived agents continue to operate in their
		// isolated worktree rather than falling back to the main repo root.
		if worktreePath != "" && r.cwdInjector != nil {
			resumeCtx = r.cwdInjector(resumeCtx, worktreePath, worktreeMainRoot)
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
			summary := result
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
			Text:  result,
			Color: state.Identity.Color,
		})
	}()

	return nil
}

// SendMessage delivers a message to a specific agent via the team mailbox and
// resets that agent's inactivity counter. It is the preferred path for routing
// any message (human or inter-agent) to a named agent.
func (r *TeammateRunner) SendMessage(agentName string, msg Message) error {
	state, ok := r.GetStateByName(agentName)
	if !ok {
		return fmt.Errorf("agent %q not found", agentName)
	}

	// Reset the inactivity counter — this agent is being spoken to.
	state.mu.Lock()
	state.InactivityCount = 0
	state.mu.Unlock()

	mb := r.GetMailbox()
	if mb == nil {
		return fmt.Errorf("no mailbox available for active team")
	}
	mb.Send("you", agentName, msg)
	return nil
}

// reconstructStateFromConfig rebuilds a minimal TeammateState from the
// persisted TeamConfig. This is used by Revive when the in-memory state was
// evicted by IncrementInactivity.
func (r *TeammateRunner) reconstructStateFromConfig(agentName string) (*TeammateState, bool) {
	for _, team := range r.manager.ListTeams() {
		for _, mem := range team.Members {
			if mem.Identity.AgentName != agentName {
				continue
			}

			systemPrompt := mem.SystemPrompt
			// Best-effort fallback: if the system prompt was never persisted
			// (agent spawned before this fix), leave it empty — the agent
			// will still run, just without the original system prompt.

			state := &TeammateState{
				Identity:             mem.Identity,
				TeamName:             team.Name,
				Prompt:               mem.Prompt,
				Model:                mem.Model,
				AutoCompactThreshold: mem.AutoCompactThreshold,
				Status:               StatusComplete,
				IsIdle:               true,
				SystemPrompt:         systemPrompt,
				AdvisorConfig:        mem.AdvisorConfig,
				idleCh:               make(chan struct{}),
			}
			// The idleCh should be closed since the agent is idle.
			close(state.idleCh)

			r.mu.Lock()
			r.teammates[mem.Identity.AgentID] = state
			r.mu.Unlock()

			return state, true
		}
	}
	return nil, false
}

// IncrementInactivity increments the inactivity counter for every idle
// (done) agent. When an agent's counter reaches threshold it is removed from
// memory. A threshold of -1 disables auto-deletion.
func (r *TeammateRunner) IncrementInactivity(threshold int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var toDelete []string
	for id, state := range r.teammates {
		state.mu.Lock()
		if state.IsIdle {
			state.InactivityCount++
			if threshold != -1 && state.InactivityCount >= threshold {
				// Don't evict agents that have unread mailbox messages —
				// they need to stay so Revive can wake them up.
				if mb := r.getMailboxLocked(state.TeamName); mb != nil && mb.UnreadCount(state.Identity.AgentName) > 0 {
					state.InactivityCount = 0
				} else {
					toDelete = append(toDelete, id)
				}
			}
		}
		state.mu.Unlock()
	}

	for _, id := range toDelete {
		delete(r.teammates, id)
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

// PurgeDone removes all completed and failed agents and cleans up their git
// worktrees. Children are removed before their parents. Returns the number of
// agents removed.
func (r *TeammateRunner) PurgeDone() int {
	// Snapshot done agents under read lock.
	r.mu.RLock()
	doneStates := make(map[string]*TeammateState)
	for id, s := range r.teammates {
		if s.Status == StatusComplete || s.Status == StatusFailed {
			doneStates[id] = s
		}
	}
	r.mu.RUnlock()

	if len(doneStates) == 0 {
		return 0
	}

	// Topological sort: children before parents using post-order DFS.
	// r.children maps parentAgentID → []childAgentID.
	ordered := make([]string, 0, len(doneStates))
	visited := make(map[string]bool, len(doneStates))

	r.childrenMu.Lock()
	childrenSnapshot := make(map[string][]string, len(r.children))
	for k, v := range r.children {
		childrenSnapshot[k] = v
	}
	r.childrenMu.Unlock()

	var visit func(id string)
	visit = func(id string) {
		if visited[id] {
			return
		}
		visited[id] = true
		for _, childID := range childrenSnapshot[id] {
			if _, isDone := doneStates[childID]; isDone {
				visit(childID)
			}
		}
		ordered = append(ordered, id)
	}

	for id := range doneStates {
		visit(id)
	}

	count := 0
	for _, id := range ordered {
		state := doneStates[id]

		// Remove git worktree from disk.
		if state.WorktreePath != "" {
			root := state.WorktreeMainRoot
			if root == "" {
				root, _ = os.Getwd()
			}
			cmd := exec.Command("git", "worktree", "remove", "--force", state.WorktreePath)
			cmd.Dir = root
			_ = cmd.Run() // ignore error — worktree may already be gone

			// Delete the branch.
			if state.WorktreeBranch != "" {
				bcmd := exec.Command("git", "branch", "-D", state.WorktreeBranch)
				bcmd.Dir = root
				_ = bcmd.Run() // ignore error — branch may already be gone
			}
		}

		_ = r.RemoveAgent(id)
		count++
	}
	return count
}

// containsQuestion returns true when the agent result contains a QUESTION: marker,
// indicating the agent is waiting for input from the team lead before it can continue.
func containsQuestion(result string) bool {
	for _, line := range strings.Split(result, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "QUESTION:") {
			return true
		}
	}
	return false
}
