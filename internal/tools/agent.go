package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// AgentTool spawns sub-agents for complex, multi-step tasks.
type AgentTool struct {
	// ParentRegistry is set by the registry after construction.
	ParentRegistry *Registry
	// APIClient is set externally for sub-agent API access.
	APIClientFactory func() interface{}
}

type agentInput struct {
	Prompt      string `json:"prompt"`
	Description string `json:"description,omitempty"`
	Model       string `json:"model,omitempty"`
}

func (t *AgentTool) Name() string { return "Agent" }

func (t *AgentTool) Description() string {
	return `Launch a sub-agent to handle complex, multi-step tasks autonomously. The sub-agent has access to the same tools (Bash, Read, Write, Edit, Glob, Grep) and will return a summary of its work. Use this for tasks that benefit from isolated context or parallel execution.`
}

func (t *AgentTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": {
				"type": "string",
				"description": "The task for the sub-agent to perform"
			},
			"description": {
				"type": "string",
				"description": "A short (3-5 word) description of the task"
			}
		},
		"required": ["prompt"]
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

	// For now, sub-agents execute tools directly in a simplified loop.
	// In production, this would spawn a full query.Engine with its own API client.
	desc := in.Description
	if desc == "" {
		desc = truncateStr(in.Prompt, 50)
	}

	// Collect sub-agent output
	var output strings.Builder
	output.WriteString(fmt.Sprintf("[Sub-agent: %s]\n", desc))
	output.WriteString(fmt.Sprintf("Task: %s\n\n", in.Prompt))
	output.WriteString("(Sub-agent execution requires API client integration — returning task description for now)\n")

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
