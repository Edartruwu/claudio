package query

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/hooks"
	"github.com/Abraxas-365/claudio/internal/permissions"
	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/security"
	"github.com/Abraxas-365/claudio/internal/services/analytics"
	"github.com/Abraxas-365/claudio/internal/services/cachetracker"
	"github.com/Abraxas-365/claudio/internal/services/compact"
	"github.com/Abraxas-365/claudio/internal/services/toolcache"
	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// EventHandler receives query engine events for UI rendering.
type EventHandler interface {
	OnTextDelta(text string)
	OnThinkingDelta(text string)
	OnToolUseStart(toolUse tools.ToolUse)
	OnToolUseEnd(toolUse tools.ToolUse, result *tools.Result)
	OnTurnComplete(usage api.Usage)
	OnError(err error)

	// OnRetry is called when the engine silently retries a request at escalated
	// max_tokens after hitting the limit mid-stream. The handler should tombstone
	// any tool_use_start events it already rendered for the given tool uses,
	// since the retry will re-emit them with complete inputs.
	OnRetry(toolUses []tools.ToolUse)

	// OnToolApprovalNeeded is called when a tool requires approval.
	// Returns true if the tool is approved, false if denied.
	OnToolApprovalNeeded(toolUse tools.ToolUse) bool

	// OnCostConfirmNeeded is called when session cost exceeds the confirmation threshold.
	// Returns true to continue, false to stop the session.
	OnCostConfirmNeeded(currentCost, threshold float64) bool
}

// EngineConfig holds all dependencies for the query engine.
type EngineConfig struct {
	Hooks          *hooks.Manager
	Analytics      *analytics.Tracker
	CompactState   *compact.State
	TaskRuntime    *tasks.Runtime
	SessionID      string
	Model          string
	PermissionMode string // "default", "auto", "headless", "plan"
	OnTurnEnd      func(messages []api.Message)                // background memory extraction callback
	OnAutoCompact  func(messages []api.Message, summary string) // called after successful auto-compaction

	// Permission pattern rules for content-based allow/deny
	PermissionRules []config.PermissionRule

	// Cost threshold for confirmation dialog (USD, 0 = disabled)
	CostConfirmThreshold float64

	// CavemanMsg is injected as the first user message at session start
	// and re-prepended after compaction for better model compliance.
	CavemanMsg string
}

const (
	// defaultMaxTokens is used for normal requests to avoid over-reserving output
	// capacity (input capacity = context_window - max_tokens). Escalated on retry.
	defaultMaxTokens  = 8_192
	escalatedMaxTokens = 64_000
	maxContinuations       = 5
	maxTokensRecoveryLimit = 3
)

// Engine orchestrates the AI conversation loop.
type Engine struct {
	client          *api.Client
	registry        *tools.Registry
	handler         EventHandler
	messages        []api.Message
	system          string
	hooks           *hooks.Manager
	analytics       *analytics.Tracker
	compactState    *compact.State
	taskRuntime     *tasks.Runtime
	sessionID       string
	model           string
	permissionMode       string
	prePlanPermMode      string // permission mode before AI-initiated plan mode
	permissionRules      []config.PermissionRule
	costConfirmThresh float64
	discoveredTools        map[string]bool // tools discovered via ToolSearch
	frozenDeferredReminder string          // deferred-tools system-reminder, frozen on first build
	onTurnEnd              func(messages []api.Message)                // called when a turn ends (end_turn stop reason)
	onAutoCompact          func(messages []api.Message, summary string) // called after successful auto-compaction

	// user context injection (CLAUDE.md as first user message)
	userContextMsg      string
	userContextInjected bool
	systemContext       string // git status appended to system (dynamic)

	// memory index injection (session-start memory index as second user message)
	memoryIndexMsg      string
	memoryIndexInjected bool
	onMemoryRefresh     func() string // refresh func to rebuild memory index after compaction

	// caveman mode injection (as user message at session start, re-injected after compaction)
	cavemanMsg      string
	cavemanInjected bool

	// lifecycle hook tracking
	sessionStartFired bool
	lastCwd           string

	// maxTurns limits the number of agentic API calls (0 = unlimited).
	maxTurns  int
	turnCount int

	// continuation tracking for diminishing returns detection
	continuationCount      int
	lastOutputTokens       int
	maxTokensOverride      int // persisted escalated max_tokens across loop iterations
	maxTokensRecoveryCount int // tracks max_tokens recovery attempts with tool_use

	// cache observability
	cacheTracker  *cachetracker.Tracker
	cacheExpiry   *cachetracker.ExpiryWatcher

	// tool result disk offload for oversized outputs
	toolCache *toolcache.Store

	// tool result budget — tracks replacements across turns for prompt cache stability
	replacementState *compact.ReplacementState

	// mailboxPoller is called each turn to check for incoming team messages.
	// Returns messages to inject as user messages. If a message signals "stop",
	// it should return the message and the engine will stop after injecting it.
	mailboxPoller func() []string
}

// defaultContextWindow is the context window size for all current Claude models.
const defaultContextWindow = 200_000

// NewEngine creates a new query engine (basic constructor for backwards compatibility).
func NewEngine(client *api.Client, registry *tools.Registry, handler EventHandler) *Engine {
	tc, _ := toolcache.New(os.TempDir()+"/claudio-tool-results", 0)
	return &Engine{
		client:           client,
		registry:         registry,
		handler:          handler,
		permissionMode:   "default",
		discoveredTools:  make(map[string]bool),
		cacheTracker:     &cachetracker.Tracker{},
		cacheExpiry:      cachetracker.NewExpiryWatcher(60 * time.Minute),
		toolCache:        tc,
		compactState:     &compact.State{MaxTokens: defaultContextWindow},
		replacementState: compact.NewReplacementState(),
	}
}

// SetRegistry replaces the tool registry used by the engine (e.g. after an agent persona switch).
func (e *Engine) SetRegistry(r *tools.Registry) { e.registry = r }

// Close releases resources held by the engine (cleans up persisted tool result files).
func (e *Engine) Close() {
	if e.sessionStartFired {
		e.fireHook(context.Background(), hooks.SessionEnd, "", "")
	}
	if e.toolCache != nil {
		e.toolCache.Cleanup()
	}
	// Clean up any team instantiated by InstantiateTeam during this session.
	if e.registry != nil {
		if it, err := e.registry.Get("InstantiateTeam"); err == nil {
			if tool, ok := it.(*tools.InstantiateTeamTool); ok && tool.InstantiatedTeam != "" {
				if tool.Runner != nil {
					tool.Runner.KillTeam(tool.InstantiatedTeam)
					tool.Runner.WaitForTeam(tool.InstantiatedTeam, 5*time.Second)
				}
				if tool.Manager != nil {
					_ = tool.Manager.DeleteTeam(tool.InstantiatedTeam)
				}
				if tool.Runner != nil {
					tool.Runner.CleanupTeam(tool.InstantiatedTeam)
				}
			}
		}
	}
}

// NewEngineWithConfig creates a fully-configured query engine.
func NewEngineWithConfig(client *api.Client, registry *tools.Registry, handler EventHandler, cfg EngineConfig) *Engine {
	e := NewEngine(client, registry, handler)
	e.hooks = cfg.Hooks
	e.analytics = cfg.Analytics
	if cfg.CompactState != nil {
		e.compactState = cfg.CompactState
	}
	e.taskRuntime = cfg.TaskRuntime
	e.sessionID = cfg.SessionID
	tools.GlobalTaskStore.LoadForSession(cfg.SessionID)
	// Wire session ID into InstantiateTeam tool so it can auto-name teams.
	if it, err := registry.Get("InstantiateTeam"); err == nil {
		if tool, ok := it.(*tools.InstantiateTeamTool); ok {
			sessionID := cfg.SessionID
			tool.GetSessionID = func() string { return sessionID }
		}
	}
	// Wire session ID into CronCreate tool so cron entries track their session.
	if ct, err := registry.Get("CronCreate"); err == nil {
		if tool, ok := ct.(*tools.CronCreateTool); ok {
			tool.SessionID = cfg.SessionID
		}
	}
	e.model = cfg.Model
	e.permissionMode = cfg.PermissionMode
	if e.permissionMode == "" {
		e.permissionMode = "default"
	}
	e.permissionRules = cfg.PermissionRules
	if f, err := os.OpenFile("/tmp/claudio-perm-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		fmt.Fprintf(f, "[NewEngineWithConfig] permissionMode=%q permissionRules=%d\n", e.permissionMode, len(e.permissionRules))
		for i, r := range e.permissionRules {
			fmt.Fprintf(f, "  rule[%d] tool=%q pattern=%q behavior=%q\n", i, r.Tool, r.Pattern, r.Behavior)
		}
		f.Close()
	}
	e.costConfirmThresh = cfg.CostConfirmThreshold
	e.onTurnEnd = cfg.OnTurnEnd
	e.onAutoCompact = cfg.OnAutoCompact
	e.cavemanMsg = cfg.CavemanMsg
	return e
}

// SetSystem sets the system prompt.
func (e *Engine) SetSystem(prompt string) {
	e.system = prompt
}

// SetHandler replaces the event handler (useful for web sessions that create
// a fresh handler per user message while reusing the same Engine).
func (e *Engine) SetHandler(h EventHandler) {
	e.handler = h
}

// Messages returns the current conversation messages.
func (e *Engine) Messages() []api.Message {
	return e.messages
}

// SessionID returns the session ID associated with this engine.
func (e *Engine) SessionID() string { return e.sessionID }

// SetMessages replaces the conversation messages (used after compaction).
func (e *Engine) SetMessages(msgs []api.Message) {
	e.messages = msgs
}

// SetOnTurnEnd registers a callback that fires when a turn ends (end_turn stop reason).
// The callback receives a copy of the conversation messages.
func (e *Engine) SetOnTurnEnd(fn func(messages []api.Message)) {
	e.onTurnEnd = fn
}

// SetUserContext sets the CLAUDE.md content to inject as the first user message.
// It will be prepended automatically on the first call to RunWithBlocks.
func (e *Engine) SetUserContext(msg string) {
	e.userContextMsg = msg
	e.userContextInjected = false
}

// SetMemoryIndex sets the memory index to inject as the second user message at session start.
// The index is injected once per session, after the user context (CLAUDE.md) message.
func (e *Engine) SetMemoryIndex(index string) {
	e.memoryIndexMsg = index
	e.memoryIndexInjected = false
}

// SetCavemanMsg sets the caveman mode instruction to inject as the first user message.
// It will be prepended automatically on the first call to RunWithBlocks and after compaction.
func (e *Engine) SetCavemanMsg(msg string) {
	e.cavemanMsg = msg
	e.cavemanInjected = false
}

// ReInjectCaveman prepends the caveman message to the current message history.
// Call this after manual compaction (TUI /compact command, web compact handler)
// so the caveman instruction survives the context reset.
func (e *Engine) ReInjectCaveman() {
	if e.cavemanMsg == "" {
		return
	}
	cContent, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(e.cavemanMsg)})
	e.messages = append([]api.Message{{Role: "user", Content: cContent}}, e.messages...)
}

// SetMemoryRefreshFunc sets a callback to rebuild the memory index after compaction.
// The callback should return the fresh index string. Called during the post-compaction
// phase to get a lean index for the new conversation era.
func (e *Engine) SetMemoryRefreshFunc(f func() string) {
	e.onMemoryRefresh = f
}

// SetSystemContext sets dynamic context (e.g. git status) to append to the system prompt.
func (e *Engine) SetSystemContext(ctx string) {
	e.systemContext = ctx
}

// SetMaxTurns sets the maximum number of agentic turns (API calls) before the engine stops.
// 0 means unlimited.
func (e *Engine) SetMaxTurns(n int) {
	e.maxTurns = n
}

// SetCompactThreshold sets the context-usage percentage at which full auto-compaction
// is triggered (overrides the default 95%). Only applied when t is between 1 and 100.
func (e *Engine) SetCompactThreshold(t int) {
	if e.compactState != nil && t > 0 && t <= 100 {
		e.compactState.ForceThreshold = t
	}
}

// SetMailboxPoller sets a function called each turn to check for incoming team messages.
// It should return messages to inject into the conversation.
func (e *Engine) SetMailboxPoller(fn func() []string) {
	e.mailboxPoller = fn
}

// SetPermissionRules replaces the engine's permission rules at runtime.
func (e *Engine) SetPermissionRules(rules []config.PermissionRule) {
	e.permissionRules = rules
	if f, err := os.OpenFile("/tmp/claudio-perm-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		fmt.Fprintf(f, "[SetPermissionRules] updated to %d rules\n", len(rules))
		f.Close()
	}
}

// Run executes a single user turn: sends the message, processes the AI response,
// executes any tool calls, and loops until the AI produces a final response.
func (e *Engine) Run(ctx context.Context, userMessage string) error {
	return e.RunWithImages(ctx, userMessage, nil)
}

// RunWithImages executes a user turn with optional image attachments.
func (e *Engine) RunWithImages(ctx context.Context, userMessage string, images []api.UserContentBlock) error {
	// Structured content: images + text
	blocks := make([]api.UserContentBlock, 0, len(images)+1)
	blocks = append(blocks, images...)
	blocks = append(blocks, api.NewTextBlock(userMessage))
	return e.RunWithBlocks(ctx, blocks)
}

// RunWithBlocks executes a user turn with pre-built content blocks.
func (e *Engine) RunWithBlocks(ctx context.Context, blocks []api.UserContentBlock) error {
	// Fire SessionStart on the first turn of the session.
	if !e.sessionStartFired {
		e.sessionStartFired = true
		e.lastCwd, _ = os.Getwd()
		e.fireHook(ctx, hooks.SessionStart, "", "")
	} else {
		// Detect cwd changes between turns and fire CwdChanged.
		if cwd, err := os.Getwd(); err == nil && cwd != e.lastCwd {
			e.lastCwd = cwd
			e.fireHook(ctx, hooks.CwdChanged, "", "")
		}
	}

	// Inject user context (CLAUDE.md) as the very first user message, once per session.
	if !e.userContextInjected && e.userContextMsg != "" && len(e.messages) == 0 {
		ctxContent, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(e.userContextMsg)})
		e.messages = append(e.messages, api.Message{
			Role:    "user",
			Content: ctxContent,
		})
		e.userContextInjected = true
	}

	// Inject memory index as a user turn, once per session.
	// Only inject for fresh sessions: either no messages at all, or the only message
	// is the userContext that was just injected this same call. Resumed sessions
	// have len > 1 and userContextInjected == false, so neither condition is true.
	if !e.memoryIndexInjected && e.memoryIndexMsg != "" && (len(e.messages) == 0 || e.userContextInjected) {
		idxContent, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(e.memoryIndexMsg)})
		e.messages = append(e.messages, api.Message{
			Role:    "user",
			Content: idxContent,
		})
		e.memoryIndexInjected = true
	}

	// Inject caveman mode instruction as a user message at session start, once per session.
	// Condition mirrors memory index: inject on first turn of a fresh session only.
	if !e.cavemanInjected && e.cavemanMsg != "" && (len(e.messages) == 0 || e.userContextInjected || e.memoryIndexInjected) {
		cContent, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(e.cavemanMsg)})
		e.messages = append(e.messages, api.Message{
			Role:    "user",
			Content: cContent,
		})
		e.cavemanInjected = true
	}

	content, _ := json.Marshal(blocks)
	e.messages = append(e.messages, api.Message{
		Role:    "user",
		Content: content,
	})

	// Fire UserPromptSubmit hook
	e.fireHook(ctx, hooks.UserPromptSubmit, "", string(content))

	// Loop until end_turn (no more tool calls)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// When the prompt cache has likely expired (>5 min gap), aggressively
		// clear old tool results — they can't benefit from caching anyway,
		// and clearing them frees token space. Keeps the last 5 results.
		if e.cacheExpiry.IsExpired() {
			e.messages = compact.TimeBasedMicroCompact(e.messages, 5)
			e.handler.OnTextDelta("\n[Note: prompt cache likely expired — cleared old tool results]\n")
			e.cacheExpiry.RecordCall()
		}

		// Auto-compact if approaching context limit (tiered)
		if e.compactState != nil {
			if e.compactState.ShouldForce() {
				// Full compaction at 95%
				e.fireHook(ctx, hooks.PreCompact, "", "")
				compacted, summary, err := compact.Compact(ctx, e.client, e.messages, 10, "")
				if err == nil && summary != "" {
					e.messages = compact.EnsureToolResultPairing(compacted)
					// Re-inject caveman instruction as first message in the new era
					if e.cavemanMsg != "" {
						cContent, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(e.cavemanMsg)})
						e.messages = append([]api.Message{{Role: "user", Content: cContent}}, e.messages...)
					}
					e.compactState.TotalTokens = 0
					// Reset replacement state — compacted messages are new, no cached replacements apply
					e.replacementState = compact.NewReplacementState()
					e.handler.OnTextDelta("\n[Auto-compacted conversation: " + summary[:min(len(summary), 100)] + "...]\n")
					if e.onAutoCompact != nil {
						msgsCopy := make([]api.Message, len(e.messages))
						copy(msgsCopy, e.messages)
						go e.onAutoCompact(msgsCopy, summary)
					}
					e.fireHook(ctx, hooks.PostCompact, "", "")
					prompts.ClearAllSections()
					
					// Refresh memory index for the new conversation era
					if e.onMemoryRefresh != nil {
						if freshIdx := e.onMemoryRefresh(); freshIdx != "" {
							e.memoryIndexInjected = false
							e.SetMemoryIndex("## Your Memory Index\n\n" + freshIdx)
						}
					}
				}
			} else if e.compactState.ShouldPartialCompact() {
				// At 70% context usage, run an aggressive MicroCompact pass
				// (lower target, fewer protected results) to free space.
				e.messages = compact.MicroCompact(e.messages, 5, 512, e.registry.ReadCache())
				e.handler.OnTextDelta("\n[Cleared old tool results to save context]\n")
			}
		}

		// Poll mailbox for incoming team messages before the API call,
		// so the model sees them in the current turn.
		e.pollMailbox()

		// Enforce per-message tool result budget: replace large results with
		// 2KB previews (full content saved to disk). Cached replacements are
		// re-applied byte-identically to preserve prompt cache stability.
		e.messages = compact.EnforceToolResultBudget(e.messages, e.replacementState, e.toolCache)

		// Ensure tool_use/tool_result pairs are valid after any compaction.
		// Inserts synthetic error results for orphaned tool_use blocks.
		e.messages = compact.EnsureToolResultPairing(e.messages)

		// Merge consecutive user messages before sending (reduces message count overhead)
		mergedMessages := mergeConsecutiveUserMessages(e.messages)

		// Build request with deferred tool loading
		maxTok := defaultMaxTokens
		if e.maxTokensOverride > 0 {
			maxTok = e.maxTokensOverride
		}
		req := &api.MessagesRequest{
			Messages:  mergedMessages,
			System:    e.buildSystemWithDeferredTools(),
			MaxTokens: maxTok,
			Tools:     e.registry.APIDefinitionsWithDeferral(e.discoveredTools),
		}

		// Stream response
		response, forwarded, err := e.streamResponse(ctx, req)
		if err != nil {
			// Save whatever partial content arrived before the error so the
			// model has it in history on the next attempt (e.g. after a timeout).
			if response != nil && len(response.Content) > 0 {
				e.saveAssistantMessage(response.Content)
			}
			e.handler.OnError(err)
			return err
		}

		// If we hit the output token limit and haven't escalated yet, retry the
		// same request at 64k max_tokens. If tool-use-start events were already
		// forwarded to the UI we emit OnRetry first so handlers can tombstone the
		// pending tool cards before we re-stream them with complete inputs.
		if response.StopReason == "max_tokens" && req.MaxTokens <= defaultMaxTokens {
			if forwarded && len(response.EmittedToolStarts) > 0 {
				e.handler.OnRetry(response.EmittedToolStarts)
			}
			req.MaxTokens = escalatedMaxTokens
			e.maxTokensOverride = escalatedMaxTokens
			response, _, err = e.streamResponse(ctx, req)
			if err != nil {
				if response != nil && len(response.Content) > 0 {
					e.saveAssistantMessage(response.Content)
				}
				e.handler.OnError(err)
				return err
			}
		}

		// Append assistant message — strip thinking blocks since they
		// require a signature field when sent back and are ephemeral
		e.saveAssistantMessage(response.Content)

		e.turnCount++
		if e.maxTurns > 0 && e.turnCount >= e.maxTurns {
			e.handler.OnTextDelta(fmt.Sprintf("\n[Agent stopped: reached %d turn limit]\n", e.maxTurns))
			return nil
		}

		e.handler.OnTurnComplete(response.Usage)

		// Record cache observability
		e.cacheTracker.Record(response.Usage.CacheCreate, e.system, len(e.messages))
		e.cacheExpiry.RecordCall()

		// Track analytics
		if e.analytics != nil {
			e.analytics.RecordUsage(response.Usage.InputTokens, response.Usage.OutputTokens, response.Usage.CacheRead, response.Usage.CacheCreate)
		}
		// Track compact state — use InputTokens directly as the current context size
		// (not a running sum, which would trigger compaction prematurely).
		if e.compactState != nil {
			e.compactState.TotalTokens = response.Usage.InputTokens
		}

		// Special handling: if input tokens exceed 200K, trigger aggressive
		// microcompact to avoid prompt bloat on the next turn.
		if response.Usage.InputTokens > 200_000 && e.compactState != nil {
			e.messages = compact.MicroCompact(e.messages, 5, 256, e.registry.ReadCache())
		}

		// Check cost threshold
		if e.costConfirmThresh > 0 && e.analytics != nil {
			currentCost := e.analytics.Cost()
			if currentCost >= e.costConfirmThresh {
				if !e.handler.OnCostConfirmNeeded(currentCost, e.costConfirmThresh) {
					e.handler.OnTextDelta("\n[Session stopped: cost threshold exceeded]\n")
					return nil
				}
				// Double the threshold so we don't ask again immediately
				e.costConfirmThresh *= 2
			}
		}

		// Check if we're done
		if response.StopReason == "end_turn" || (response.StopReason != "max_tokens" && len(response.ToolUses) == 0) {
			// Reset continuation tracking on clean completion
			e.continuationCount = 0
			e.lastOutputTokens = 0
			e.maxTokensOverride = 0
			e.maxTokensRecoveryCount = 0

			// Fire Stop hook
			e.fireHook(ctx, hooks.Stop, "", "")

			// Fire turn-end callback for background memory extraction
			if e.onTurnEnd != nil {
				msgsCopy := make([]api.Message, len(e.messages))
				copy(msgsCopy, e.messages)
				go e.onTurnEnd(msgsCopy)
			}

			return nil
		}

		// max_tokens mid-text (no tool uses): inject Claude Code's recovery message
		// so the model picks up mid-thought without apology. Detect diminishing
		// returns by output-token drop >50% or exceeding maxContinuations.
		if response.StopReason == "max_tokens" && len(response.ToolUses) == 0 {
			e.continuationCount++
			outputTokens := response.Usage.OutputTokens

			if e.continuationCount >= maxContinuations ||
				(e.lastOutputTokens > 0 && outputTokens < e.lastOutputTokens/2) {
				e.handler.OnTextDelta("\n[Stopped: diminishing returns on continuation]\n")
				e.continuationCount = 0
				e.lastOutputTokens = 0
				return nil
			}
			e.lastOutputTokens = outputTokens

			// Use Claude Code's exact recovery prompt.
			recoveryMsg := "Output token limit hit. Resume directly — no apology, no recap of what you were doing. " +
				"Pick up mid-thought if that is where the cut happened. Break remaining work into smaller pieces."
			contContent, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(recoveryMsg)})
			e.messages = append(e.messages, api.Message{
				Role:    "user",
				Content: contContent,
			})
			continue
		}

		// max_tokens mid-tool_use: the silent 64k escalation in streamResponse
		// already fired. If we still have tool_use (escalation also hit the cap),
		// track recovery attempts and stop after maxTokensRecoveryLimit tries.
		if response.StopReason == "max_tokens" && len(response.ToolUses) > 0 {
			e.maxTokensRecoveryCount++
			if e.maxTokensRecoveryCount > maxTokensRecoveryLimit {
				e.handler.OnTextDelta("\n[Stopped: max_tokens recovery exhausted after repeated truncated tool calls]\n")
				e.maxTokensRecoveryCount = 0
				e.maxTokensOverride = 0
				return nil
			}
			e.maxTokensOverride = escalatedMaxTokens
		}

		// Execute tools and build result message
		toolResults, injectedMsgs := e.executeTools(ctx, response.ToolUses)

		// Append tool results as a user message — must be immediately after the
		// assistant message so tool_result blocks have a matching tool_use in the
		// preceding message. pollBackgroundTasks runs after so it never wedges
		// between the assistant(tool_use) and user(tool_results) pair.
		resultContent, _ := json.Marshal(toolResults)
		e.messages = append(e.messages, api.Message{
			Role:    "user",
			Content: resultContent,
		})

		// Inject any messages requested by tools (e.g. skill content) as
		// additional user turns. These persist in conversation history exactly
		// like the newMessages mechanism in claude-code — the model reads the
		// injected instructions and follows them for the rest of the session.
		for _, msg := range injectedMsgs {
			msgContent, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(msg)})
			e.messages = append(e.messages, api.Message{
				Role:    "user",
				Content: msgContent,
			})
		}

		// If max_tokens was hit, inject a recovery message so the model
		// knows to break up large outputs on the next turn.
		if response.StopReason == "max_tokens" {
			recoveryContent, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(
				"Output token limit hit. Resume directly — no apology, no recap. " +
					"Pick up mid-thought if that is where the cut happened. " +
					"Break remaining work into smaller pieces.",
			)})
			e.messages = append(e.messages, api.Message{
				Role:    "user",
				Content: recoveryContent,
			})
		}

		// Microcompact: proactively clear large old tool results on every tool turn.
		// Keeps the last 10 results intact, clears anything larger than 2KB beyond that.
		// ReadCache is intentionally NOT invalidated on clear — keeping it valid means
		// a re-read attempt returns the dedup stub rather than the full file again.
		e.messages = compact.MicroCompact(e.messages, 10, 2048, e.registry.ReadCache())

		// Poll background tasks and inject completed results. Runs after tool
		// results are appended so completed-task notifications appear as a
		// follow-up user message, not between the assistant and its tool results.
		e.pollBackgroundTasks()
	}
}

// streamedResponse holds the parsed results of a streamed API response.
type streamedResponse struct {
	Content          []api.ContentBlock
	ToolUses         []tools.ToolUse
	EmittedToolStarts []tools.ToolUse // tool_use_start events already sent to handler
	StopReason       string
	Usage            api.Usage
}

const (
	maxStreamRetries  = 5
	baseRetryDelayMs  = 500
	maxRetryDelayMs   = 32_000
)

// retryDelay returns the backoff duration for a given attempt (1-based).
// Matches claude-code: base * 2^(attempt-1) + 25% jitter, capped at maxRetryDelayMs.
func retryDelay(attempt int) time.Duration {
	base := float64(baseRetryDelayMs) * math.Pow(2, float64(attempt-1))
	if base > maxRetryDelayMs {
		base = maxRetryDelayMs
	}
	jitter := rand.Float64() * 0.25 * base
	return time.Duration(base+jitter) * time.Millisecond
}

// streamResponse performs a streaming request with transient-error retries.
// It also returns whether any content was forwarded to the handler, so the
// caller can decide whether a follow-up escalated-tokens retry is safe.
func (e *Engine) streamResponse(ctx context.Context, req *api.MessagesRequest) (*streamedResponse, bool, error) {
	var lastErr error
	var anyForwarded bool
	for attempt := 1; attempt <= maxStreamRetries+1; attempt++ {
		resp, forwarded, err := e.streamOnce(ctx, req)
		if forwarded {
			anyForwarded = true
		}
		if err == nil {
			return resp, anyForwarded, nil
		}
		// Only retry transient errors, and only when no content was forwarded
		// to the handler yet. Retrying after partial output would duplicate
		// text already shown in the TUI.
		if forwarded || !api.IsTransientError(err) || ctx.Err() != nil {
			return resp, anyForwarded, err
		}
		if attempt > maxStreamRetries {
			return resp, anyForwarded, err
		}
		// On TCP connection reset, renew the transport to discard stale sockets.
		if api.IsConnectionResetError(err) {
			e.client.RenewTransport()
		}
		lastErr = err
		delay := retryDelay(attempt)
		e.handler.OnTextDelta(fmt.Sprintf("\n[Connection error, retrying in %s (attempt %d/%d)...]\n",
			delay.Round(time.Millisecond), attempt, maxStreamRetries))
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, anyForwarded, ctx.Err()
		}
		_ = lastErr
	}
	return nil, anyForwarded, lastErr
}

// streamOnce performs a single streaming attempt. Returns (response, anyForwarded, error).
// anyForwarded is true if any text or tool-use events were sent to the handler.
func (e *Engine) streamOnce(ctx context.Context, req *api.MessagesRequest) (*streamedResponse, bool, error) {
	eventCh, errCh := e.client.StreamMessages(ctx, req)

	response := &streamedResponse{}
	var forwarded bool // true once OnTextDelta or OnToolUseStart has been called

	// Track current content block being built
	var currentBlocks []api.ContentBlock
	var currentBlockIdx int = -1

	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				// Channel closed — stream ended via [DONE] without a message_stop event
				// (common for OpenAI-compatible providers like MiniMax via OpenRouter).
				// Finalize content so saveAssistantMessage persists the tool_use blocks.
				response.Content = currentBlocks
				return response, forwarded, nil
			}

			switch event.Type {
			case "message_start":
				// Parse initial message data including usage (input_tokens, cache tokens)
				if event.MessageField != nil {
					var msg api.MessageResp
					json.Unmarshal(event.MessageField, &msg)
					if msg.Usage.InputTokens > 0 {
						response.Usage.InputTokens = msg.Usage.InputTokens
					}
					if msg.Usage.CacheRead > 0 {
						response.Usage.CacheRead = msg.Usage.CacheRead
					}
					if msg.Usage.CacheCreate > 0 {
						response.Usage.CacheCreate = msg.Usage.CacheCreate
					}
				}

			case "content_block_start":
				// New content block - index is in event.Index, block data in event.ContentBlock
				var block api.ContentBlock
				if event.ContentBlock != nil {
					json.Unmarshal(event.ContentBlock, &block)
				}
				currentBlockIdx = event.Index
				for len(currentBlocks) <= currentBlockIdx {
					currentBlocks = append(currentBlocks, api.ContentBlock{})
				}

				if block.Type == "tool_use" {
					// Clear the initial empty {} so input_json_delta can accumulate cleanly
					block.Input = nil
					tu := tools.ToolUse{
						ID:   block.ID,
						Name: block.Name,
					}
					forwarded = true
					response.EmittedToolStarts = append(response.EmittedToolStarts, tu)
					e.handler.OnToolUseStart(tu)
				}
				currentBlocks[currentBlockIdx] = block

			case "content_block_delta":
				if event.Delta == nil {
					continue
				}
				var delta struct {
					Type         string `json:"type"`
					Text         string `json:"text"`
					PartialJSON  string `json:"partial_json"`
					Thinking     string `json:"thinking"`
					Signature    string `json:"signature"`
				}
				json.Unmarshal(event.Delta, &delta)

				switch delta.Type {
				case "text_delta":
					forwarded = true
					e.handler.OnTextDelta(delta.Text)
					if currentBlockIdx >= 0 && currentBlockIdx < len(currentBlocks) {
						currentBlocks[currentBlockIdx].Text += delta.Text
					}

				case "thinking_delta":
					e.handler.OnThinkingDelta(delta.Thinking)
					if currentBlockIdx >= 0 && currentBlockIdx < len(currentBlocks) {
						currentBlocks[currentBlockIdx].Thinking += delta.Thinking
					}

				case "signature_delta":
					// Capture the thinking block's signature for API roundtrip compliance
					if currentBlockIdx >= 0 && currentBlockIdx < len(currentBlocks) {
						currentBlocks[currentBlockIdx].Signature += delta.Signature
					}

				case "input_json_delta":
					if currentBlockIdx >= 0 && currentBlockIdx < len(currentBlocks) {
						// Accumulate partial JSON for tool input
						existing := string(currentBlocks[currentBlockIdx].Input)
						currentBlocks[currentBlockIdx].Input = json.RawMessage(existing + delta.PartialJSON)
					}
				}

			case "content_block_stop":
				// Use the event's own index — not currentBlockIdx — so parallel
				// tool calls (where a second block starts before the first stops)
				// are attributed to the correct block.
				stopIdx := event.Index
				if stopIdx >= 0 && stopIdx < len(currentBlocks) {
					block := currentBlocks[stopIdx]
					if block.Type == "tool_use" {
						// Ensure input is always a valid JSON object — the API
						// rejects tool_use blocks where input is null or missing.
						input := block.Input
						if len(input) == 0 {
							input = json.RawMessage("{}")
							currentBlocks[stopIdx].Input = input
						}
						response.ToolUses = append(response.ToolUses, tools.ToolUse{
							ID:    block.ID,
							Name:  block.Name,
							Input: input,
						})
					}
				}

			case "message_delta":
				if event.Delta != nil {
					var delta struct {
						StopReason string `json:"stop_reason"`
					}
					json.Unmarshal(event.Delta, &delta)
					if delta.StopReason != "" {
						response.StopReason = delta.StopReason
					}
				}
				if event.Usage != nil && event.Usage.OutputTokens > 0 {
					response.Usage.OutputTokens = event.Usage.OutputTokens
				}

			case "message_stop":
				response.Content = currentBlocks
				return response, forwarded, nil
			}

		case err, ok := <-errCh:
			if !ok {
				continue
			}
			// Return whatever partial content arrived before the error so the
			// engine can save it to history. The caller checks err != nil.
			response.Content = currentBlocks
			return response, forwarded, err

		case <-ctx.Done():
			// User cancelled — don't save partial content, just stop cleanly.
			return nil, forwarded, ctx.Err()
		}
	}
}

// approvedTool holds a tool use that has passed all pre-flight checks.
type approvedTool struct {
	idx  int
	tu   tools.ToolUse
	tool tools.Tool
}

// executeTools runs all tool calls, executing read-only tools concurrently and
// mutating tools sequentially (waiting for any in-flight reads to finish first).
// Order of results matches the order of toolUses.
func (e *Engine) executeTools(ctx context.Context, toolUses []tools.ToolUse) ([]toolResultBlock, []string) {
	results := make([]toolResultBlock, len(toolUses))

	// --- Pre-flight pass (always sequential: hooks + approval have UI side effects) ---
	approved := make([]approvedTool, 0, len(toolUses))
	for i, tu := range toolUses {
		tool, err := e.registry.Get(tu.Name)
		if err != nil {
			res := &tools.Result{Content: fmt.Sprintf("Unknown tool: %s", tu.Name), IsError: true}
			e.handler.OnToolUseEnd(tu, res)
			results[i] = toolResultBlock{Type: "tool_result", ToolUseID: tu.ID, Content: res.Content, IsError: true}
			continue
		}

		if blocked := e.fireHook(ctx, hooks.PreToolUse, tu.Name, string(tu.Input)); blocked {
			res := &tools.Result{Content: fmt.Sprintf("Tool %s was blocked by a PreToolUse hook", tu.Name), IsError: true}
			e.handler.OnToolUseEnd(tu, res)
			results[i] = toolResultBlock{Type: "tool_result", ToolUseID: tu.ID, Content: res.Content, IsError: true}
			continue
		}

		if behavior, matched := permissions.Match(tu.Name, tu.Input, e.permissionRules); matched && behavior == "deny" {
			res := &tools.Result{Content: fmt.Sprintf("Tool %s was denied by permission rule", tu.Name), IsError: true}
			e.handler.OnToolUseEnd(tu, res)
			results[i] = toolResultBlock{Type: "tool_result", ToolUseID: tu.ID, Content: res.Content, IsError: true}
			continue
		}

		// Run pre-approval validation so errors surface before the user is prompted.
		if v, ok := tool.(tools.Validatable); ok {
			if vr := v.Validate(ctx, tu.Input); vr != nil && vr.IsError {
				e.handler.OnToolUseEnd(tu, vr)
				results[i] = toolResultBlock{Type: "tool_result", ToolUseID: tu.ID, Content: vr.Content, IsError: true}
				continue
			}
		}

		if e.shouldRequireApproval(tool, tu.Input) {
			if !e.handler.OnToolApprovalNeeded(tu) {
				res := &tools.Result{Content: fmt.Sprintf("Tool %s was denied by user", tu.Name), IsError: true}
				e.handler.OnToolUseEnd(tu, res)
				results[i] = toolResultBlock{Type: "tool_result", ToolUseID: tu.ID, Content: res.Content, IsError: true}
				continue
			}
		}

		if e.analytics != nil {
			e.analytics.RecordToolCall()
		}
		if tu.Name == "ToolSearch" {
			e.trackDiscoveredTools(tu.Input)
		}

		approved = append(approved, approvedTool{idx: i, tu: tu, tool: tool})
	}

	// --- Execution pass: batch read-only tools concurrently, flush before mutating tools ---
	type execResult struct {
		idx int
		blk toolResultBlock
		injected []string
	}
	var batch []approvedTool
	var injectedMessages []string

	flushBatch := func() {
		if len(batch) == 0 {
			return
		}
		if len(batch) == 1 {
			at := batch[0]
			blk, msgs := e.runSingleTool(ctx, at.tu, at.tool)
			results[at.idx] = blk
			injectedMessages = append(injectedMessages, msgs...)
		} else {
			ch := make(chan execResult, len(batch))
			var wg sync.WaitGroup
			for _, at := range batch {
				wg.Add(1)
				go func(at approvedTool) {
					defer wg.Done()
					blk, msgs := e.runSingleTool(ctx, at.tu, at.tool)
					ch <- execResult{idx: at.idx, blk: blk, injected: msgs}
				}(at)
			}
			wg.Wait()
			close(ch)
			for er := range ch {
				results[er.idx] = er.blk
				injectedMessages = append(injectedMessages, er.injected...)
			}
		}
		batch = batch[:0]
	}

	for _, at := range approved {
		if at.tool.IsReadOnly() {
			batch = append(batch, at)
		} else {
			flushBatch() // drain concurrent reads before any mutating tool
			blk, msgs := e.runSingleTool(ctx, at.tu, at.tool)
			results[at.idx] = blk
			injectedMessages = append(injectedMessages, msgs...)
		}
	}
	flushBatch()

	return results, injectedMessages
}

// runSingleTool executes one tool and applies post-processing (hooks, secret scan, disk offload).
// Returns the tool result block and any messages to inject into the conversation.
func (e *Engine) runSingleTool(ctx context.Context, tu tools.ToolUse, tool tools.Tool) (toolResultBlock, []string) {
	result, err := tool.Execute(ctx, tu.Input)
	if err != nil {
		result = &tools.Result{Content: fmt.Sprintf("Tool execution error: %v", err), IsError: true}
	}

	if result.IsError {
		e.fireHook(ctx, hooks.PostToolUseFailure, tu.Name, result.Content)
	} else {
		e.fireHook(ctx, hooks.PostToolUse, tu.Name, result.Content)
	}

	// Detect cwd changes caused by the tool (e.g. EnterWorktree/ExitWorktree).
	if cwd, err := os.Getwd(); err == nil && cwd != e.lastCwd {
		e.lastCwd = cwd
		e.fireHook(ctx, hooks.CwdChanged, "", "")
	}

	if secrets := security.ScanForSecrets(result.Content); len(secrets) > 0 {
		result.Content = security.RedactSecrets(result.Content)
		result.Content += "\n\n[WARNING: Potential secrets detected and redacted in output]"
	}

	// Switch permission mode for AI-initiated plan mode transitions.
	if !result.IsError {
		switch tu.Name {
		case "EnterPlanMode":
			e.prePlanPermMode = e.permissionMode
			e.permissionMode = "plan"
		case "ExitPlanMode":
			if e.prePlanPermMode != "" {
				e.permissionMode = e.prePlanPermMode
				e.prePlanPermMode = ""
			} else {
				e.permissionMode = "default"
			}
		}
	}

	e.handler.OnToolUseEnd(tu, result)

	content := result.Content
	if e.toolCache != nil {
		content = e.toolCache.MaybePersist(tu.ID, content)
	}
	const maxContent = 100_000
	if len(content) > maxContent {
		content = content[:maxContent] + "\n... (truncated)"
	}

	// If the tool result includes images, build array content for the API.
	if len(result.Images) > 0 {
		blocks := make([]toolContentBlock, 0, 1+len(result.Images))
		blocks = append(blocks, toolContentBlock{Type: "text", Text: content})
		for _, img := range result.Images {
			blocks = append(blocks, toolContentBlock{
				Type: "image",
				Source: &toolImageSource{
					Type:      "base64",
					MediaType: img.MediaType,
					Data:      img.Data,
				},
			})
		}
		return toolResultBlock{
			Type:      "tool_result",
			ToolUseID: tu.ID,
			Content:   blocks,
			IsError:   result.IsError,
		}, result.InjectedMessages
	}

	return toolResultBlock{
		Type:      "tool_result",
		ToolUseID: tu.ID,
		Content:   content,
		IsError:   result.IsError,
	}, result.InjectedMessages
}

// pollBackgroundTasks checks for completed background tasks and injects their results
// as additional context for the next turn.
func (e *Engine) pollBackgroundTasks() {
	if e.taskRuntime == nil {
		return
	}

	completed := e.taskRuntime.PollResults()
	if len(completed) == 0 {
		return
	}

	// Build a notification message with all completed task results
	var notifications []string
	for _, t := range completed {
		status := fmt.Sprintf("[Background task %s (%s): %s]", t.ID, t.Type, t.Status)
		if t.Error != "" {
			status += fmt.Sprintf(" Error: %s", t.Error)
		}

		// Read last 4KB of output as summary
		if t.OutputFile != "" {
			content, _, _ := tasks.ReadDelta(t.OutputFile, 0, 4096)
			if content != "" {
				if len(content) > 2000 {
					content = content[len(content)-2000:]
				}
				status += "\nOutput (tail):\n" + content
			}
		}
		notifications = append(notifications, status)
	}

	if len(notifications) > 0 {
		// Inject as a system reminder
		reminderText := fmt.Sprintf("<system-reminder>\n%s\n</system-reminder>",
			strings.Join(notifications, "\n\n"))

		content, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(reminderText)})
		e.messages = append(e.messages, api.Message{
			Role:    "user",
			Content: content,
		})
	}

	// Evict old terminal tasks (older than 5 minutes)
	e.taskRuntime.Evict(5 * time.Minute)
}

// pollMailbox checks for incoming team messages and injects them into the conversation.
func (e *Engine) pollMailbox() {
	if e.mailboxPoller == nil {
		return
	}

	msgs := e.mailboxPoller()
	if len(msgs) == 0 {
		return
	}

	// Inject as a user message so the model sees the team messages
	reminderText := fmt.Sprintf("<system-reminder>\nIncoming team messages:\n%s\n</system-reminder>",
		strings.Join(msgs, "\n\n"))

	content, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(reminderText)})
	e.messages = append(e.messages, api.Message{
		Role:    "user",
		Content: content,
	})
}

// shouldRequireApproval checks if a tool needs user approval based on permission mode.
func (e *Engine) shouldRequireApproval(tool tools.Tool, input json.RawMessage) bool {
	switch e.permissionMode {
	case "auto", "headless", "dangerously-skip-permissions":
		return false // auto-approve everything
	case "plan":
		// In plan mode: allow read-only tools and plan-file writes (RequiresApproval=false).
		// Block all other write tools (they need explicit user approval).
		if tool.IsReadOnly() {
			return false
		}
		return tool.RequiresApproval(input)
	default: // "default"
		// Check content-pattern permission rules first (first match wins)
		if f, err := os.OpenFile("/tmp/claudio-perm-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			fmt.Fprintf(f, "[shouldRequireApproval] tool=%q permissionRules=%d\n", tool.Name(), len(e.permissionRules))
			f.Close()
		}
		if behavior, matched := permissions.Match(tool.Name(), input, e.permissionRules); matched {
			switch behavior {
			case "allow":
				return false
			case "deny":
				return true // will show dialog but auto-deny
			case "ask":
				return true
			}
		}
		return tool.RequiresApproval(input)
	}
}

// fireHook executes a hook and returns true if the action was blocked.
func (e *Engine) fireHook(ctx context.Context, event hooks.Event, toolName, toolInput string) bool {
	if e.hooks == nil {
		return false
	}

	cwd, _ := os.Getwd()
	hctx := hooks.HookContext{
		Event:     event,
		ToolName:  toolName,
		ToolInput: toolInput,
		SessionID: e.sessionID,
		Model:     e.model,
		CWD:       cwd,
	}

	_, blocked := e.hooks.Run(ctx, event, hctx)
	return blocked
}

// buildSystemWithDeferredTools returns the system prompt with a list of deferred
// tool names appended (and optionally the git status context), so the model knows
// they exist and can fetch them via ToolSearch.
//
// The deferred-tools section is computed once and then frozen for the lifetime of
// the session. Rebuilding it every turn (e.g. removing tools as they get discovered)
// changes the system prompt bytes on each turn, which busts the Anthropic prompt
// cache and wastes tokens on every subsequent request.
func (e *Engine) buildSystemWithDeferredTools() string {
	base := e.system
	if e.systemContext != "" {
		base = base + "\n\n" + e.systemContext
	}

	// Build the frozen reminder exactly once.
	if e.frozenDeferredReminder == "" {
		deferred := e.registry.DeferredToolNames()
		if len(deferred) > 0 {
			e.frozenDeferredReminder = "\n\n<system-reminder>\nThe following deferred tools are available via ToolSearch:\n" +
				strings.Join(deferred, "\n") + "\n</system-reminder>"
		}
	}

	return base + e.frozenDeferredReminder
}

// trackDiscoveredTools parses a ToolSearch input to mark which tools were requested,
// so their full schemas are included in subsequent API requests.
func (e *Engine) trackDiscoveredTools(input json.RawMessage) {
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return
	}

	if strings.HasPrefix(params.Query, "select:") {
		names := strings.Split(strings.TrimPrefix(params.Query, "select:"), ",")
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name != "" {
				e.discoveredTools[name] = true
			}
		}
	} else {
		// For keyword searches, we mark all matching tools as discovered after execution.
		// The actual matching happens in ToolSearchTool.Execute, but we can pre-compute
		// which tools would match using the same logic.
		query := strings.ToLower(params.Query)
		keywords := strings.Fields(query)
		hints := e.registry.ToolSearchHints()
		for name, hint := range hints {
			nameL := strings.ToLower(name)
			hintL := strings.ToLower(hint)
			for _, kw := range keywords {
				if strings.Contains(nameL, kw) || strings.Contains(hintL, kw) {
					e.discoveredTools[name] = true
					break
				}
			}
		}
	}
}

// saveAssistantMessage appends the assistant turn to the conversation history.
// Thinking blocks are preserved so they can be round-tripped on the next turn:
// Anthropic requires the original signature; interleaved-thinking OpenAI-compat
// providers (MiniMax M2, etc.) require the prior reasoning_content to keep
// tool_call/tool_result IDs in sync. Empty thinking blocks (no text and no
// signature) are dropped as they carry no useful state. Ensures tool_use blocks
// always carry a valid JSON input object (the API rejects null/missing input).
func (e *Engine) saveAssistantMessage(content []api.ContentBlock) {
	var filtered []api.ContentBlock
	for _, block := range content {
		if block.Type == "thinking" && block.Thinking == "" && block.Signature == "" {
			continue
		}
		// Drop empty text blocks — they serialize without the required "text" field
		// due to omitempty, causing API errors ("text: Field required").
		if block.Type == "text" && block.Text == "" {
			continue
		}
		if block.Type == "tool_use" && len(block.Input) == 0 {
			block.Input = json.RawMessage("{}")
		}
		filtered = append(filtered, block)
	}
	if len(filtered) == 0 {
		return
	}
	assistantContent, _ := json.Marshal(filtered)
	e.messages = append(e.messages, api.Message{
		Role:    "assistant",
		Content: assistantContent,
	})
}

// mergeConsecutiveUserMessages merges adjacent user messages into one.
// This reduces message count overhead and avoids API issues with consecutive same-role messages.
// Tool result blocks (type "tool_result") are never merged — only plain text and text-type blocks.
func mergeConsecutiveUserMessages(messages []api.Message) []api.Message {
	if len(messages) < 2 {
		return messages
	}
	result := make([]api.Message, 0, len(messages))
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role != "user" || i+1 >= len(messages) || messages[i+1].Role != "user" {
			result = append(result, msg)
			continue
		}
		// Check that neither message contains tool_result blocks
		if hasToolResultBlocks(msg.Content) || hasToolResultBlocks(messages[i+1].Content) {
			result = append(result, msg)
			continue
		}
		// Merge: concatenate as text blocks
		text1 := extractTextContent(msg.Content)
		text2 := extractTextContent(messages[i+1].Content)
		merged := text1 + "\n" + text2
		mergedContent, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(merged)})
		result = append(result, api.Message{Role: "user", Content: mergedContent})
		i++ // skip the next message
	}
	return result
}

func hasToolResultBlocks(content json.RawMessage) bool {
	var blocks []json.RawMessage
	if json.Unmarshal(content, &blocks) != nil {
		return false
	}
	for _, b := range blocks {
		var block struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(b, &block) == nil && block.Type == "tool_result" {
			return true
		}
	}
	return false
}

func extractTextContent(content json.RawMessage) string {
	// Try plain string first
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	// Try array of blocks
	var blocks []json.RawMessage
	if json.Unmarshal(content, &blocks) != nil {
		return string(content)
	}
	var parts []string
	for _, b := range blocks {
		var block struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(b, &block) == nil && block.Type == "text" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// toolResultBlock is the format the API expects for tool results.
type toolResultBlock struct {
	Type      string      `json:"type"`
	ToolUseID string      `json:"tool_use_id"`
	Content   interface{} `json:"content"`
	IsError   bool        `json:"is_error,omitempty"`
}

// toolContentBlock is a single block within a tool result's content array.
type toolContentBlock struct {
	Type   string           `json:"type"`
	Text   string           `json:"text,omitempty"`
	Source *toolImageSource `json:"source,omitempty"`
}

// toolImageSource describes a base64-encoded image for the API.
type toolImageSource struct {
	Type      string `json:"type"`       // always "base64"
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// StdoutHandler is a simple event handler that prints to stdout.
// CollectHandler captures all text deltas into a strings.Builder.
// Use this when you need the agent's output as a string rather than streaming it.
type CollectHandler struct {
	Builder *strings.Builder
}

func (h *CollectHandler) OnTextDelta(text string)                              { h.Builder.WriteString(text) }
func (h *CollectHandler) OnThinkingDelta(string)                               {}
func (h *CollectHandler) OnToolUseStart(tools.ToolUse)                         {}
func (h *CollectHandler) OnToolUseEnd(tools.ToolUse, *tools.Result)            {}
func (h *CollectHandler) OnTurnComplete(api.Usage)                             {}
func (h *CollectHandler) OnError(error)                                        {}
func (h *CollectHandler) OnRetry([]tools.ToolUse)                              {}
func (h *CollectHandler) OnToolApprovalNeeded(tools.ToolUse) bool              { return true }
func (h *CollectHandler) OnCostConfirmNeeded(_, _ float64) bool                { return true }

type StdoutHandler struct {
	Verbose bool
}

func (h *StdoutHandler) OnTextDelta(text string) {
	fmt.Print(text)
}

func (h *StdoutHandler) OnThinkingDelta(text string) {
	if h.Verbose {
		fmt.Print(text)
	}
}

func (h *StdoutHandler) OnToolUseStart(tu tools.ToolUse) {
	// Format a nice summary of what tool is being called
	var inputSummary string
	switch tu.Name {
	case "Bash":
		var in struct{ Command string }
		json.Unmarshal(tu.Input, &in)
		inputSummary = in.Command
	case "Read":
		var in struct{ FilePath string }
		json.Unmarshal(tu.Input, &in)
		inputSummary = in.FilePath
	case "Write":
		var in struct{ FilePath string }
		json.Unmarshal(tu.Input, &in)
		inputSummary = in.FilePath
	case "Edit":
		var in struct{ FilePath string }
		json.Unmarshal(tu.Input, &in)
		inputSummary = in.FilePath
	case "Glob":
		var in struct{ Pattern string }
		json.Unmarshal(tu.Input, &in)
		inputSummary = in.Pattern
	case "Grep":
		var in struct{ Pattern string }
		json.Unmarshal(tu.Input, &in)
		inputSummary = in.Pattern
	default:
		inputSummary = string(tu.Input)
		if len(inputSummary) > 80 {
			inputSummary = inputSummary[:80] + "..."
		}
	}
	fmt.Printf("\n--- %s: %s ---\n", tu.Name, inputSummary)
}

func (h *StdoutHandler) OnToolUseEnd(tu tools.ToolUse, result *tools.Result) {
	// Show abbreviated result
	content := result.Content
	lines := strings.Split(content, "\n")
	if len(lines) > 20 {
		content = strings.Join(lines[:20], "\n") + fmt.Sprintf("\n... (%d more lines)", len(lines)-20)
	}
	if result.IsError {
		fmt.Printf("[ERROR] %s\n", content)
	} else if h.Verbose {
		fmt.Printf("%s\n", content)
	}
}

func (h *StdoutHandler) OnTurnComplete(usage api.Usage) {
	if h.Verbose {
		fmt.Printf("\n[tokens: in=%d out=%d]\n", usage.InputTokens, usage.OutputTokens)
	}
}

func (h *StdoutHandler) OnToolApprovalNeeded(tu tools.ToolUse) bool {
	return true // Auto-approve in non-interactive mode
}

func (h *StdoutHandler) OnCostConfirmNeeded(currentCost, threshold float64) bool {
	fmt.Printf("\n[Cost: $%.4f exceeds threshold $%.4f — continuing in non-interactive mode]\n", currentCost, threshold)
	return true
}

func (h *StdoutHandler) OnError(err error) {
	fmt.Printf("\n[error: %v]\n", err)
}

func (h *StdoutHandler) OnRetry(_ []tools.ToolUse) {
	// stdout is non-interactive; nothing to tombstone
}
