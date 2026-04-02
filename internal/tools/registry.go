package tools

import (
	"encoding/json"
	"fmt"
)

// Registry holds all registered tools.
type Registry struct {
	tools map[string]Tool
	order []string // preserve insertion order
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
	r.order = append(r.order, t.Name())
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return t, nil
}

// All returns all registered tools in order.
func (r *Registry) All() []Tool {
	result := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.tools[name])
	}
	return result
}

// APIDefinitions returns tool definitions in the format expected by the Anthropic API.
func (r *Registry) APIDefinitions() json.RawMessage {
	defs := make([]APIToolDef, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		defs = append(defs, APIToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		})
	}
	data, _ := json.Marshal(defs)
	return data
}

// Remove removes a tool from the registry by name.
func (r *Registry) Remove(name string) {
	delete(r.tools, name)
	for i, n := range r.order {
		if n == name {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

// Clone creates a copy of the registry (for sub-agent filtered registries).
func (r *Registry) Clone() *Registry {
	clone := NewRegistry()
	for _, name := range r.order {
		clone.Register(r.tools[name])
	}
	return clone
}

// Names returns the names of all registered tools.
func (r *Registry) Names() []string {
	result := make([]string, len(r.order))
	copy(result, r.order)
	return result
}

// DefaultRegistry creates a registry with all core tools.
func DefaultRegistry() *Registry {
	r := NewRegistry()

	// Core file & shell tools
	r.Register(&BashTool{})
	r.Register(&FileReadTool{})
	r.Register(&FileWriteTool{})
	r.Register(&FileEditTool{})
	r.Register(&GlobTool{})
	r.Register(&GrepTool{})

	// Web tools
	r.Register(&WebSearchTool{})
	r.Register(&WebFetchTool{})

	// Code intelligence
	r.Register(&LSPTool{})

	// Notebook
	r.Register(&NotebookEditTool{})

	// Task management
	r.Register(&TaskCreateTool{})
	r.Register(&TaskListTool{})
	r.Register(&TaskGetTool{})
	r.Register(&TaskUpdateTool{})

	// Workspace
	r.Register(&EnterWorktreeTool{})
	r.Register(&ExitWorktreeTool{})
	r.Register(&EnterPlanModeTool{})
	r.Register(&ExitPlanModeTool{})

	// Agent
	r.Register(&AgentTool{})

	// Background task management (TaskStop and TaskOutput need Runtime injected later)
	r.Register(&TaskStopTool{})
	r.Register(&TaskOutputTool{})

	return r
}
