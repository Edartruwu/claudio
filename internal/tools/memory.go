package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Abraxas-365/claudio/internal/services/memory"
)

// MemoryTool lets the agent search, list, and read memories.
type MemoryTool struct {
	deferrable
	Store *memory.ScopedStore
}

type memoryInput struct {
	Action string `json:"action"` // "list", "search", "read"
	Query  string `json:"query,omitempty"`
	Name   string `json:"name,omitempty"`
}

func (t *MemoryTool) Name() string { return "Memory" }

func (t *MemoryTool) Description() string {
	return `Access persistent memories from previous sessions.

Memories contain user preferences, project decisions, feedback, and references that were saved across sessions.

## Actions

- **list**: List all available memories with their names, types, and descriptions.
- **search**: Search memories by keyword (matches name, description, and content). Use the "query" parameter.
- **read**: Read the full content of a specific memory by name. Use the "name" parameter.

## When to Use

- When you need context about the user, project, or past decisions
- When the user references something from a previous conversation
- When you want to check if relevant guidance exists before making a decision
- Before starting work on a project you haven't seen recently`
}

func (t *MemoryTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["list", "search", "read"],
				"description": "The action to perform: list all memories, search by keyword, or read a specific memory"
			},
			"query": {
				"type": "string",
				"description": "Search query (for action=search). Matches against name, description, and content."
			},
			"name": {
				"type": "string",
				"description": "Memory name to read (for action=read)"
			}
		},
		"required": ["action"]
	}`)
}

func (t *MemoryTool) IsReadOnly() bool                        { return true }
func (t *MemoryTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *MemoryTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	if t.Store == nil {
		return &Result{Content: "Memory store not available", IsError: true}, nil
	}

	var in memoryInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	switch in.Action {
	case "list":
		return t.listMemories()
	case "search":
		if in.Query == "" {
			return &Result{Content: "Query parameter required for search action", IsError: true}, nil
		}
		return t.searchMemories(in.Query)
	case "read":
		if in.Name == "" {
			return &Result{Content: "Name parameter required for read action", IsError: true}, nil
		}
		return t.readMemory(in.Name)
	default:
		return &Result{Content: fmt.Sprintf("Unknown action: %s. Use: list, search, read", in.Action), IsError: true}, nil
	}
}

func (t *MemoryTool) listMemories() (*Result, error) {
	entries := t.Store.LoadAll()
	if len(entries) == 0 {
		return &Result{Content: "No memories found."}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d memories:\n\n", len(entries)))

	for _, e := range entries {
		scope := ""
		if e.Scope != "" {
			scope = fmt.Sprintf(" [%s]", e.Scope)
		}
		sb.WriteString(fmt.Sprintf("- **%s** (%s)%s — %s\n", e.Name, e.Type, scope, e.Description))
	}

	return &Result{Content: sb.String()}, nil
}

func (t *MemoryTool) searchMemories(query string) (*Result, error) {
	entries := t.Store.LoadAll()
	if len(entries) == 0 {
		return &Result{Content: "No memories found."}, nil
	}

	lower := strings.ToLower(query)
	var matches []*memory.Entry

	for _, e := range entries {
		searchable := strings.ToLower(e.Name + " " + e.Description + " " + e.Content)
		if strings.Contains(searchable, lower) {
			matches = append(matches, e)
		}
	}

	if len(matches) == 0 {
		return &Result{Content: fmt.Sprintf("No memories matching %q found.", query)}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d memories matching %q:\n\n", len(matches), query))

	for _, e := range matches {
		content := e.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("## %s (%s)\n%s\n\n%s\n\n", e.Name, e.Type, e.Description, content))
	}

	return &Result{Content: sb.String()}, nil
}

func (t *MemoryTool) readMemory(name string) (*Result, error) {
	entries := t.Store.LoadAll()

	// Try exact match first, then case-insensitive
	for _, e := range entries {
		if e.Name == name {
			return &Result{Content: formatMemoryFull(e)}, nil
		}
	}
	lowerName := strings.ToLower(name)
	for _, e := range entries {
		if strings.ToLower(e.Name) == lowerName {
			return &Result{Content: formatMemoryFull(e)}, nil
		}
	}
	// Partial match
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Name), lowerName) {
			return &Result{Content: formatMemoryFull(e)}, nil
		}
	}

	return &Result{Content: fmt.Sprintf("Memory %q not found. Use action=list to see available memories.", name), IsError: true}, nil
}

func formatMemoryFull(e *memory.Entry) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", e.Name))
	sb.WriteString(fmt.Sprintf("- **Type:** %s\n", e.Type))
	sb.WriteString(fmt.Sprintf("- **Description:** %s\n", e.Description))
	if e.Scope != "" {
		sb.WriteString(fmt.Sprintf("- **Scope:** %s\n", e.Scope))
	}
	sb.WriteString(fmt.Sprintf("\n## Content\n\n%s\n", e.Content))
	return sb.String()
}
