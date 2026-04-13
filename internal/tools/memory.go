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
	Action      string `json:"action"` // "list", "search", "read", "save", "update", "delete"
	Query       string `json:"query,omitempty"`
	Name        string `json:"name,omitempty"`
	Content     string `json:"content,omitempty"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Scope       string `json:"scope,omitempty"`
}

func (t *MemoryTool) Name() string { return "Memory" }

func (t *MemoryTool) Description() string {
	return `Access and manage persistent memories from previous sessions.

Memories contain user preferences, project decisions, feedback, and references that were saved across sessions.

## Actions

- **list**: List all available memories with their names, types, and descriptions.
- **search**: Search memories by keyword (matches name, description, and content). Use the "query" parameter.
- **read**: Read the full content of a specific memory by name. Use the "name" parameter.
- **save**: Save a new memory. Required: name, content. Optional: description, type, scope. Returns error if name already exists.
- **update**: Update an existing memory. Required: name, content. Optional: description, type, scope. Overwrites existing.
- **delete**: Delete a memory by name. Required: name.

## When to Use

- When you need context about the user, project, or past decisions
- When the user references something from a previous conversation
- When you want to check if relevant guidance exists before making a decision
- Before starting work on a project you haven't seen recently
- To save important insights or decisions for future sessions`
}

func (t *MemoryTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["list", "search", "read", "save", "update", "delete"],
				"description": "The action to perform: list, search, read, save, update, or delete a memory"
			},
			"query": {
				"type": "string",
				"description": "Search query (for action=search). Matches against name, description, and content."
			},
			"name": {
				"type": "string",
				"description": "Memory name (for actions: read, save, update, delete)"
			},
			"content": {
				"type": "string",
				"description": "Memory content (required for save/update)"
			},
			"description": {
				"type": "string",
				"description": "One-line description (optional, for save/update)"
			},
			"type": {
				"type": "string",
				"enum": ["user", "feedback", "project", "reference"],
				"description": "Memory type (optional, default: project). One of: user, feedback, project, reference"
			},
			"scope": {
				"type": "string",
				"enum": ["project", "global", "agent"],
				"description": "Memory scope (optional, default: project). One of: project, global, agent"
			}
		},
		"required": ["action"]
	}`)
}

func (t *MemoryTool) IsReadOnly() bool                        { return false }
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
	case "save":
		if in.Name == "" {
			return &Result{Content: "Name parameter required for save action", IsError: true}, nil
		}
		if in.Content == "" {
			return &Result{Content: "Content parameter required for save action", IsError: true}, nil
		}
		return t.saveMemory(in)
	case "update":
		if in.Name == "" {
			return &Result{Content: "Name parameter required for update action", IsError: true}, nil
		}
		if in.Content == "" {
			return &Result{Content: "Content parameter required for update action", IsError: true}, nil
		}
		return t.updateMemory(in)
	case "delete":
		if in.Name == "" {
			return &Result{Content: "Name parameter required for delete action", IsError: true}, nil
		}
		return t.deleteMemory(in.Name)
	default:
		return &Result{Content: fmt.Sprintf("Unknown action: %s. Use: list, search, read, save, update, delete", in.Action), IsError: true}, nil
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
		if len(e.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("  tags: %s\n", strings.Join(e.Tags, ", ")))
		}
	}

	return &Result{Content: sb.String()}, nil
}

func (t *MemoryTool) searchMemories(query string) (*Result, error) {
	matches := t.Store.FindRelevant(query)
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
		sb.WriteString(fmt.Sprintf("## %s (%s)\n%s\n\n", e.Name, e.Type, e.Description))
		if len(e.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("**Tags:** %s\n\n", strings.Join(e.Tags, ", ")))
		}
		sb.WriteString(fmt.Sprintf("%s\n\n", content))
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

func (t *MemoryTool) saveMemory(in memoryInput) (*Result, error) {
	// Check if memory with this name already exists
	entries := t.Store.LoadAll()
	for _, e := range entries {
		if e.Name == in.Name {
			return &Result{
				Content: fmt.Sprintf("Memory '%s' already exists. Use action='update' to overwrite it.", in.Name),
				IsError: true,
			}, nil
		}
	}

	// Validate and set defaults for type and scope
	memType := in.Type
	if memType == "" {
		memType = memory.TypeProject
	} else if memType != memory.TypeUser && memType != memory.TypeFeedback && memType != memory.TypeProject && memType != memory.TypeReference {
		return &Result{
			Content: fmt.Sprintf("Invalid type '%s'. Must be one of: user, feedback, project, reference", in.Type),
			IsError: true,
		}, nil
	}

	scope := in.Scope
	if scope == "" {
		scope = memory.ScopeProject
	} else if scope != memory.ScopeProject && scope != memory.ScopeGlobal && scope != memory.ScopeAgent {
		return &Result{
			Content: fmt.Sprintf("Invalid scope '%s'. Must be one of: project, global, agent", in.Scope),
			IsError: true,
		}, nil
	}

	// Create and save entry
	entry := &memory.Entry{
		Name:        in.Name,
		Content:     in.Content,
		Description: in.Description,
		Type:        memType,
		Scope:       scope,
	}

	if err := t.Store.Save(entry); err != nil {
		return &Result{
			Content: fmt.Sprintf("Failed to save memory: %v", err),
			IsError: true,
		}, nil
	}

	return &Result{Content: fmt.Sprintf("Memory '%s' saved successfully.", in.Name)}, nil
}

func (t *MemoryTool) updateMemory(in memoryInput) (*Result, error) {
	// Validate and set defaults for type and scope
	memType := in.Type
	if memType == "" {
		memType = memory.TypeProject
	} else if memType != memory.TypeUser && memType != memory.TypeFeedback && memType != memory.TypeProject && memType != memory.TypeReference {
		return &Result{
			Content: fmt.Sprintf("Invalid type '%s'. Must be one of: user, feedback, project, reference", in.Type),
			IsError: true,
		}, nil
	}

	scope := in.Scope
	if scope == "" {
		scope = memory.ScopeProject
	} else if scope != memory.ScopeProject && scope != memory.ScopeGlobal && scope != memory.ScopeAgent {
		return &Result{
			Content: fmt.Sprintf("Invalid scope '%s'. Must be one of: project, global, agent", in.Scope),
			IsError: true,
		}, nil
	}

	// Create and save entry (overwrites if exists)
	entry := &memory.Entry{
		Name:        in.Name,
		Content:     in.Content,
		Description: in.Description,
		Type:        memType,
		Scope:       scope,
	}

	if err := t.Store.Save(entry); err != nil {
		return &Result{
			Content: fmt.Sprintf("Failed to update memory: %v", err),
			IsError: true,
		}, nil
	}

	return &Result{Content: fmt.Sprintf("Memory '%s' updated successfully.", in.Name)}, nil
}

func (t *MemoryTool) deleteMemory(name string) (*Result, error) {
	if err := t.Store.Remove(name); err != nil {
		return &Result{
			Content: fmt.Sprintf("Failed to delete memory: %v", err),
			IsError: true,
		}, nil
	}

	return &Result{Content: fmt.Sprintf("Memory '%s' deleted.", name)}, nil
}

func formatMemoryFull(e *memory.Entry) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", e.Name))
	sb.WriteString(fmt.Sprintf("- **Type:** %s\n", e.Type))
	sb.WriteString(fmt.Sprintf("- **Description:** %s\n", e.Description))
	if e.Scope != "" {
		sb.WriteString(fmt.Sprintf("- **Scope:** %s\n", e.Scope))
	}
	if len(e.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("- **Tags:** %s\n", strings.Join(e.Tags, ", ")))
	}
	sb.WriteString(fmt.Sprintf("\n## Content\n\n%s\n", e.Content))
	return sb.String()
}
