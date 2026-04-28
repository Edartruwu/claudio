package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/storage"
	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/Abraxas-365/claudio/internal/teams"
)

// CWD context key — allows goroutine-scoped CWD override for worktree isolation.
type ctxKeyCwd struct{}
type ctxKeyMainRoot struct{}

// WithCwd stores a working directory override in the context together with the
// main repo root from which the worktree was forked.  Tools that respect CWD
// (Bash, Glob, Grep) use the worktree path; file tools (Read, Edit, Write) use
// both to remap absolute paths from the main repo into the worktree.
func WithCwd(ctx context.Context, cwd string) context.Context {
	return context.WithValue(ctx, ctxKeyCwd{}, cwd)
}

// WithMainRoot stores the main repo root alongside the worktree CWD so that
// RemapPathForWorktree can translate absolute main-repo paths into worktree paths.
func WithMainRoot(ctx context.Context, mainRoot string) context.Context {
	return context.WithValue(ctx, ctxKeyMainRoot{}, mainRoot)
}

// CwdFromContext returns the CWD override, or "" if none is set.
func CwdFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyCwd{}).(string)
	return v
}

// mainRootFromContext returns the stored main repo root, or "".
func mainRootFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyMainRoot{}).(string)
	return v
}

// RemapPathForWorktree rewrites a path so file tools operate inside the
// agent's worktree instead of the main repo. It handles two cases:
//
//  1. Relative paths (e.g. "internal/tui/theme.go") — resolved against the
//     worktree root rather than the process CWD. Without this, Read/Edit/Write
//     would silently operate on the main tree because os.Open() uses os.Getwd().
//  2. Absolute paths under the main repo root — rewritten to the equivalent
//     path under the worktree root.
//
// Paths outside the main repo (e.g. /etc/hosts, $HOME) are returned unchanged.
// When no CWD override is set the original path is returned unchanged.
func RemapPathForWorktree(ctx context.Context, path string) string {
	worktreeRoot := CwdFromContext(ctx)
	if worktreeRoot == "" {
		return path
	}
	if !filepath.IsAbs(path) {
		// Relative path — resolve against the worktree root, not the
		// process CWD (which is still the main repo root).
		return filepath.Join(worktreeRoot, path)
	}
	// Already inside the worktree — nothing to remap.
	if strings.HasPrefix(path, worktreeRoot+string(filepath.Separator)) || path == worktreeRoot {
		return path
	}
	mainRoot := mainRootFromContext(ctx)
	if mainRoot == "" {
		return path
	}
	rel, err := filepath.Rel(mainRoot, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		// path is outside the main repo — do not remap
		return path
	}
	// Paths inside .claudio-worktrees are sibling worktrees, not project source.
	// Remapping them would produce a double-nested path. Return as-is.
	if strings.HasPrefix(rel, ".claudio-worktrees"+string(filepath.Separator)) || rel == ".claudio-worktrees" {
		return path
	}
	return filepath.Join(worktreeRoot, rel)
}

// SubAgentDBContext carries DB access for sub-agent session persistence.
type SubAgentDBContext struct {
	DB       *storage.DB
	ParentID string
	Model    string
}

type ctxKeySubAgentDB struct{}

// WithSubAgentDB stores DB context for sub-agent persistence.
func WithSubAgentDB(ctx context.Context, db *storage.DB, parentID, model string) context.Context {
	return context.WithValue(ctx, ctxKeySubAgentDB{}, &SubAgentDBContext{DB: db, ParentID: parentID, Model: model})
}

// SubAgentDBFromContext retrieves the DB context (nil if not set).
func SubAgentDBFromContext(ctx context.Context) *SubAgentDBContext {
	v, _ := ctx.Value(ctxKeySubAgentDB{}).(*SubAgentDBContext)
	return v
}

// TeamContext carries team identity for sub-agents so SendMessage works
// without mutating shared tool pointers.
type TeamContext struct {
	TeamName  string
	AgentName string
	Foreground bool // true when spawned synchronously (run_in_background=false)
}

type ctxKeyTeamContext struct{}

// WithTeamContext stores team identity in context for teammate sub-agents.
func WithTeamContext(ctx context.Context, tc TeamContext) context.Context {
	return context.WithValue(ctx, ctxKeyTeamContext{}, &tc)
}

// TeamContextFromCtx retrieves the team context (nil if not in a team).
func TeamContextFromCtx(ctx context.Context) *TeamContext {
	v, _ := ctx.Value(ctxKeyTeamContext{}).(*TeamContext)
	return v
}

// WithAgentType stores the agent type in context so the sub-agent runner can label its sub-session.
func WithAgentType(ctx context.Context, agentType string) context.Context {
	return agents.WithAgentType(ctx, agentType)
}

// AgentTypeFromContext retrieves the agent type (empty string if not set).
func AgentTypeFromContext(ctx context.Context) string {
	return agents.AgentTypeFromContext(ctx)
}

type ctxKeyMaxTurns struct{}

// WithMaxTurns stores a maxTurns value in the context for sub-agent engines.
func WithMaxTurns(ctx context.Context, n int) context.Context {
	return context.WithValue(ctx, ctxKeyMaxTurns{}, n)
}

// MaxTurnsFromContext retrieves the maxTurns value from context (0 if not set).
func MaxTurnsFromContext(ctx context.Context) int {
	if v, ok := ctx.Value(ctxKeyMaxTurns{}).(int); ok {
		return v
	}
	return 0
}

type ctxKeyCompactThreshold struct{}

// WithCompactThreshold stores an auto-compact threshold (percentage) in the context for sub-agent engines.
func WithCompactThreshold(ctx context.Context, t int) context.Context {
	return context.WithValue(ctx, ctxKeyCompactThreshold{}, t)
}

// CompactThresholdFromContext retrieves the auto-compact threshold (0 if not set).
func CompactThresholdFromContext(ctx context.Context) int {
	if v, ok := ctx.Value(ctxKeyCompactThreshold{}).(int); ok {
		return v
	}
	return 0
}

type ctxKeyAgentDepth struct{}

// WithAgentDepth increments the nesting depth in context.
func WithAgentDepth(ctx context.Context, d int) context.Context {
	return context.WithValue(ctx, ctxKeyAgentDepth{}, d)
}

// AgentDepthFromContext returns the current agent nesting depth (0 = main session).
func AgentDepthFromContext(ctx context.Context) int {
	if v, ok := ctx.Value(ctxKeyAgentDepth{}).(int); ok {
		return v
	}
	return 0
}

type ctxKeySubAgentModel struct{}

// WithSubAgentModel stores a model alias/ID override in context for sub-agent engines.
func WithSubAgentModel(ctx context.Context, model string) context.Context {
	return context.WithValue(ctx, ctxKeySubAgentModel{}, model)
}

// SubAgentModelFromContext retrieves the model override from context ("" if not set).
func SubAgentModelFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeySubAgentModel{}).(string)
	return v
}

type ctxKeyExtraTools struct{}

// WithExtraTool appends a Tool to the list of extra tools carried in context.
// Extra tools are injected into the per-agent registry in runSubAgentWithMemory
// before the agent runs. Multiple calls accumulate tools.
func WithExtraTool(ctx context.Context, tool Tool) context.Context {
	existing, _ := ctx.Value(ctxKeyExtraTools{}).([]Tool)
	updated := make([]Tool, len(existing)+1)
	copy(updated, existing)
	updated[len(existing)] = tool
	return context.WithValue(ctx, ctxKeyExtraTools{}, updated)
}

// ExtraToolsFromContext returns all extra tools stored in context via WithExtraTool.
func ExtraToolsFromContext(ctx context.Context) []Tool {
	tools, _ := ctx.Value(ctxKeyExtraTools{}).([]Tool)
	return tools
}

// AgentTool spawns sub-agents for complex, multi-step tasks.
type AgentTool struct {
	// descMu guards the state-aware description cache.
	// cachedDesc is invalidated when team-active state changes so custom agents
	// appear only when a team template is active.
	descMu          sync.Mutex
	cachedDesc      string
	cachedTeamActive bool

	// cachedSchema is regenerated when team-active state changes.
	cachedSchema           json.RawMessage
	cachedSchemaTeamActive bool

	// ParentRegistry is set by the registry after construction.
	ParentRegistry *Registry
	// RunAgent executes a sub-agent synchronously. Set by app initialization.
	// Receives (ctx, systemPrompt, userPrompt) and returns the text output.
	RunAgent func(ctx context.Context, system, prompt string) (string, error)
	// RunAgentWithMemory executes a sub-agent with agent-scoped memory injection.
	// The memoryDir parameter points to the agent's own memory directory.
	RunAgentWithMemory func(ctx context.Context, system, prompt, memoryDir string) (string, error)
	// TaskRuntime for background agent execution.
	TaskRuntime *tasks.Runtime
	// SessionID is the owning session — passed to background tasks for access control.
	SessionID string
	// TeamRunner for spawning agents visible in the team panel.
	TeamRunner *teams.TeammateRunner
	// AvailableModels lists extra model names from configured providers (e.g., Groq, OpenAI-compatible).
	AvailableModels []string
	// EventBus for publishing agent status events.
	EventBus *bus.Bus
}

type agentInput struct {
	Prompt          string   `json:"prompt"`
	Description     string   `json:"description,omitempty"`
	SubagentType    string   `json:"subagent_type,omitempty"`
	Model           string   `json:"model,omitempty"`
	MaxTurns        int      `json:"max_turns,omitempty"`
	RunInBackground   bool     `json:"run_in_background,omitempty"`
	Isolation         string   `json:"isolation,omitempty"` // "worktree"
	TaskIDs           []string `json:"task_ids,omitempty"`  // task IDs to auto-complete when agent finishes
	MergeWhenFinished bool     `json:"merge_when_finished,omitempty"`
}

func (t *AgentTool) Name() string { return "Agent" }

func (t *AgentTool) Description() string {
	teamActive := t.TeamRunner != nil && t.TeamRunner.ActiveTeamName() != ""
	t.descMu.Lock()
	defer t.descMu.Unlock()
	if t.cachedDesc != "" && t.cachedTeamActive == teamActive {
		return t.cachedDesc
	}
	var dirs []string
	if teamActive {
		dirs = agents.GetCustomDirs()
	}
	t.cachedDesc = prompts.AgentDescription(agents.FormatAgentTypes(agents.AllAgents(dirs...)))
	t.cachedTeamActive = teamActive
	return t.cachedDesc
}

func (t *AgentTool) InputSchema() json.RawMessage {
	teamActive := t.TeamRunner != nil && t.TeamRunner.ActiveTeamName() != ""

	t.descMu.Lock()
	if t.cachedSchema != nil && t.cachedSchemaTeamActive == teamActive {
		schema := t.cachedSchema
		t.descMu.Unlock()
		return schema
	}
	t.descMu.Unlock()

	modelEnum := buildModelEnum(t.AvailableModels)
	bgField := ""
	if teamActive {
		bgField = `"run_in_background": {
				"type": "boolean",
				"description": "Set to true to run this agent in the background"
			},`
	}
	schema := json.RawMessage(fmt.Sprintf(`{
		"type": "object",
		"properties": {
			"prompt": {
				"type": "string",
				"description": "The task for the agent to perform"
			},
			"description": {
				"type": "string",
				"description": "A short (3-5 word) description of the task"
			},
			"subagent_type": {
				"type": "string",
				"description": "The type of specialized agent to use for this task"
			},
			"model": {
				"type": "string",
				"description": "Optional model override for this agent",
				"enum": %s
			},
			"max_turns": {
				"type": "number",
				"description": "Maximum number of agentic turns (API calls) before the agent stops. If omitted, uses the agent type's default limit."
			},
			%s
			"task_ids": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Task IDs (e.g. [\"1\",\"3\"]) to automatically mark completed when this agent finishes. Use the IDs returned by TaskCreate."
			},
			"isolation": {
				"type": "string",
				"description": "Isolation mode. \"worktree\" creates a temporary git worktree.",
				"enum": ["worktree"]
			},
			"merge_when_finished": {
				"type": "boolean",
				"description": "If true, automatically merge the agent's worktree branch into main when it finishes. Default false — worktree is preserved for manual inspection/merge."
			}
		},
		"required": ["description", "prompt"]
	}`, modelEnum, bgField))

	t.descMu.Lock()
	t.cachedSchema = schema
	t.cachedSchemaTeamActive = teamActive
	t.descMu.Unlock()

	return schema
}

func (t *AgentTool) IsReadOnly() bool                        { return false }
func (t *AgentTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *AgentTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in agentInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.Prompt == "" {
		return &Result{Content: "No prompt provided", IsError: true}, nil
	}

	// Enforce max nesting depth (main=0, teammate=1, sub-agent=2).
	// This prevents infinite recursion while still allowing teammates to spawn
	// read-only exploration sub-agents (e.g. Explore).
	currentDepth := AgentDepthFromContext(ctx)
	const maxAgentDepth = 2
	if currentDepth >= maxAgentDepth {
		return &Result{Content: fmt.Sprintf("Agent nesting limit reached (depth %d/%d)", currentDepth, maxAgentDepth), IsError: true}, nil
	}
	// Pass incremented depth into sub-agent context
	ctx = WithAgentDepth(ctx, currentDepth+1)

	desc := in.Description
	if desc == "" {
		desc = truncateStr(in.Prompt, 50)
	}

	// Resolve agent type
	agentType := in.SubagentType
	if agentType == "" {
		agentType = "general-purpose"
	}
	agentDef := agents.GetAgent(agentType)

	// Inject agent type into context for sub-session labeling.
	ctx = WithAgentType(ctx, agentDef.Type)

	// Inject maxTurns into context: caller-specified takes priority, then agent type default.
	maxTurns := in.MaxTurns
	if maxTurns <= 0 {
		maxTurns = agentDef.MaxTurns
	}
	if maxTurns > 0 {
		ctx = WithMaxTurns(ctx, maxTurns)
	}

	// Inject model override: caller-specified takes priority, then agent type default.
	modelOverride := in.Model
	if modelOverride == "" {
		modelOverride = agentDef.Model
	}
	if modelOverride != "" {
		ctx = WithSubAgentModel(ctx, modelOverride)
	}

	// Team-aware execution: if a team is active, route through TeammateRunner
	// so agents appear in the team panel.
	if t.TeamRunner != nil && t.TeamRunner.ActiveTeamName() != "" {
		teamName := t.TeamRunner.ActiveTeamName()
		shortName := slugifyName(desc)
		state, err := t.TeamRunner.Spawn(teams.SpawnConfig{
			TeamName:          teamName,
			AgentName:         shortName,
			SubagentType:      agentDef.Type,
			Prompt:            in.Prompt,
			System:            agentDef.SystemPrompt,
			Model:             modelOverride,
			MaxTurns:          maxTurns,
			MemoryDir:         agentDef.MemoryDir,
			Foreground:        !in.RunInBackground,
			Ephemeral:         true,
			TaskIDs:           in.TaskIDs,
			ParentAgentID:     teams.TeammateAgentIDFromContext(ctx),
			MergeWhenFinished: in.MergeWhenFinished,
		})
		if err != nil {
			return &Result{Content: fmt.Sprintf("Failed to spawn teammate: %v", err), IsError: true}, nil
		}

		if in.RunInBackground {
			msg := fmt.Sprintf("Teammate spawned in team %q: %s\nAgent ID: %s", teamName, desc, state.Identity.AgentID)
			if state.WorktreePath != "" {
				msg += fmt.Sprintf("\nRunning in isolated worktree: %s", state.WorktreePath)
			}
			msg += "\n\nDo not duplicate this agent's work — avoid working with the same files or topics it is using."
			return &Result{Content: msg}, nil
		}

		// Foreground: wait for the teammate to finish
		done := t.TeamRunner.WaitForOne(state.Identity.AgentID, 30*time.Minute)
		if !done {
			return &Result{Content: fmt.Sprintf("Teammate %s timed out after 30 minutes", desc), IsError: true}, nil
		}
		if state.Error != "" {
			return &Result{Content: fmt.Sprintf("Teammate error: %s", state.Error), IsError: true}, nil
		}
		result := state.Result
		const maxAgentBytes = 50_000
		if len(result) > maxAgentBytes {
			result = result[:maxAgentBytes] + fmt.Sprintf("\n[Agent output truncated at %d bytes]", maxAgentBytes)
		}
		if state.WorktreePath != "" {
			result += fmt.Sprintf("\n\nWorktree: %s\nBranch: %s", state.WorktreePath, state.WorktreeBranch)
			if state.MergeStatus != "" {
				result += fmt.Sprintf("\nMerge status: %s", state.MergeStatus)
			} else {
				result += "\nMerge status: not-requested (worktree preserved)"
			}
		}
		return &Result{Content: result}, nil
	}

	// Background sub-agent execution requires an active team — without one, the
	// built-in agents are read-only investigators that must return results to the
	// caller. Reject so the model retries in foreground mode.
	if in.RunInBackground {
		return &Result{Content: "Background agent execution requires an active team. Activate a team first, or call the Agent tool without run_in_background to get results inline.", IsError: true}, nil
	}

	// Foreground execution
	if agentDef.MemoryDir != "" && t.RunAgentWithMemory != nil {
		result, err := t.RunAgentWithMemory(ctx, agentDef.SystemPrompt, in.Prompt, agentDef.MemoryDir)
		if err != nil {
			return &Result{Content: fmt.Sprintf("Agent error: %v", err), IsError: true}, nil
		}
		const maxAgentBytes = 50_000
		if len(result) > maxAgentBytes {
			result = result[:maxAgentBytes] + fmt.Sprintf("\n[Agent output truncated at %d bytes]", maxAgentBytes)
		}
		return &Result{Content: result}, nil
	}
	if t.RunAgent != nil {
		result, err := t.RunAgent(ctx, agentDef.SystemPrompt, in.Prompt)
		if err != nil {
			return &Result{Content: fmt.Sprintf("Agent error: %v", err), IsError: true}, nil
		}
		const maxAgentBytes = 50_000
		if len(result) > maxAgentBytes {
			result = result[:maxAgentBytes] + fmt.Sprintf("\n[Agent output truncated at %d bytes]", maxAgentBytes)
		}
		return &Result{Content: result}, nil
	}

	// Fallback: no RunAgent callback configured
	var output strings.Builder
	output.WriteString(fmt.Sprintf("[Agent: %s (%s)]\n", desc, agentDef.Type))
	output.WriteString(fmt.Sprintf("Task: %s\n\n", in.Prompt))
	output.WriteString("(Sub-agent execution requires API client configuration)\n")
	return &Result{Content: output.String()}, nil
}

// AgentPool manages concurrent sub-agents.
type AgentPool struct {
	mu       sync.Mutex
	agents   map[string]*runningAgent
	maxAgent int
}

type runningAgent struct {
	id        string
	desc      string
	startTime time.Time
	cancel    context.CancelFunc
}

// slugifyName turns a description like "Explore core packages tests" into "explore-core-pkg".
// Takes first 3 words, lowercases, joins with dashes, max 20 chars.
func slugifyName(s string) string {
	// Strip everything except letters, digits, spaces, and dashes
	var cleaned strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' || r == '-' {
			cleaned.WriteRune(r)
		}
	}
	words := strings.Fields(cleaned.String())
	if len(words) > 3 {
		words = words[:3]
	}
	slug := strings.Join(words, "-")
	if len(slug) > 20 {
		slug = slug[:20]
		// Trim trailing dash
		slug = strings.TrimRight(slug, "-")
	}
	if slug == "" {
		slug = "agent"
	}
	return slug
}

// NewAgentPool creates an agent pool with a max concurrency limit.
func NewAgentPool(max int) *AgentPool {
	return &AgentPool{
		agents:   make(map[string]*runningAgent),
		maxAgent: max,
	}
}
