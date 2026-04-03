package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/storage"
	"github.com/Abraxas-365/claudio/internal/tasks"
)

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

type ctxKeyAgentType struct{}

// WithAgentType stores the agent type in context so the sub-agent runner can label its sub-session.
func WithAgentType(ctx context.Context, agentType string) context.Context {
	return context.WithValue(ctx, ctxKeyAgentType{}, agentType)
}

// AgentTypeFromContext retrieves the agent type (empty string if not set).
func AgentTypeFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyAgentType{}).(string)
	return v
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

// AgentTool spawns sub-agents for complex, multi-step tasks.
type AgentTool struct {
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
}

type agentInput struct {
	Prompt          string `json:"prompt"`
	Description     string `json:"description,omitempty"`
	SubagentType    string `json:"subagent_type,omitempty"`
	Model           string `json:"model,omitempty"`
	MaxTurns        int    `json:"max_turns,omitempty"`
	RunInBackground bool   `json:"run_in_background,omitempty"`
	Isolation       string `json:"isolation,omitempty"` // "worktree"
}

func (t *AgentTool) Name() string { return "Agent" }

func (t *AgentTool) Description() string {
	return prompts.AgentDescription(agents.AgentTypesList())
}

func (t *AgentTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
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
				"enum": ["sonnet", "opus", "haiku"]
			},
			"max_turns": {
				"type": "number",
				"description": "Maximum number of agentic turns (API calls) before the agent stops. If omitted, uses the agent type's default limit."
			},
			"run_in_background": {
				"type": "boolean",
				"description": "Set to true to run this agent in the background"
			},
			"isolation": {
				"type": "string",
				"description": "Isolation mode. \"worktree\" creates a temporary git worktree.",
				"enum": ["worktree"]
			}
		},
		"required": ["description", "prompt"]
	}`)
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

	// Background execution via task runtime
	if in.RunInBackground && t.TaskRuntime != nil {
		runFn := t.RunAgent
		state, err := tasks.SpawnAgentTask(t.TaskRuntime, tasks.AgentTaskInput{
			Prompt:      in.Prompt,
			Description: desc,
			AgentType:   agentDef.Type,
			Model:       in.Model,
			System:      agentDef.SystemPrompt,
			RunAgent: func(ctx context.Context, system, prompt string) (string, error) {
				if runFn != nil {
					return runFn(ctx, system, prompt)
				}
				return "", fmt.Errorf("agent execution not configured")
			},
		})
		if err != nil {
			return &Result{Content: fmt.Sprintf("Failed to start background agent: %v", err), IsError: true}, nil
		}
		return &Result{Content: fmt.Sprintf("Background agent started: %s\nTask ID: %s\nAgent type: %s\nUse TaskOutput to check results.", desc, state.ID, agentDef.Type)}, nil
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

// NewAgentPool creates an agent pool with a max concurrency limit.
func NewAgentPool(max int) *AgentPool {
	return &AgentPool{
		agents:   make(map[string]*runningAgent),
		maxAgent: max,
	}
}
