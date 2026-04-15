package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Abraxas-365/claudio/internal/services/memory"
)

// MemoryTool lets the agent persist and manage facts across sessions.
type MemoryTool struct {
	Store *memory.ScopedStore
}

type memoryInput struct {
	Action      string   `json:"action"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	Facts       []string `json:"facts,omitempty"`
	Fact        string   `json:"fact,omitempty"`
	FactIndex   int      `json:"fact_index,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Concepts    []string `json:"concepts,omitempty"`
	Query       string   `json:"query,omitempty"`
	SourceFiles []string `json:"source_files,omitempty"`
}

func (t *MemoryTool) Name() string { return "Memory" }

func (t *MemoryTool) Description() string {
	return `Persist and manage facts that survive across sessions. Use Memory to remember decisions, project conventions, user preferences, architecture patterns, and anything worth recalling later.

Facts are discrete, one-sentence, specific statements — not prose. Each memory entry has a name, description, tags, and a list of facts.

## Actions

- **save** — Create a new memory entry with initial facts. Returns error if name already exists (use append or replace-fact instead).
  Required: name, description, facts (array of strings), tags (array of strings). Optional: type, scope.

- **append** — Add one fact to an existing memory. Fastest way to update — no full rewrite needed.
  Required: name, fact (single string).

- **replace-fact** — Replace a specific fact by its index. Use after Memory(read) to see fact indices.
  Required: name, fact_index (int, 0-based), fact (string — the replacement).

- **delete-fact** — Remove a specific fact by its index. Use after Memory(read) to see fact indices.
  Required: name, fact_index (int, 0-based).

- **delete** — Remove an entire memory entry permanently.
  Required: name.

- **invalidate** — Invalidate a cached memory entry so it will be re-investigated next time. Deletes the entry without implying permanent removal — use when source files have changed and the cached facts are stale.
  Required: name.

- **read** — Load a full entry with all facts numbered (0-based). Use before replace-fact or delete-fact to see indices.
  Required: name.

- **list** — Show all memory entries with name, tags, description, and first fact. No parameters needed.

- **search** — Keyword search across name, description, tags, and facts. Returns matching entries.
  Required: query.

## When to Use

- Call Memory(list) or Memory(search) at the start of a task to recall relevant context.
- Call Memory(save) when you learn something worth remembering: a decision, a convention, a gotcha.
- Call Memory(append) to add a new fact to an existing entry (e.g., a new convention for a known topic).
- Call Memory(read) before replace-fact or delete-fact to see current facts with their indices.
- Facts should be discrete, one-sentence, specific. Good: "JWT tokens expire in 24h". Bad: "The JWT configuration is complex and involves many settings...".
- Use the Recall tool for semantic/fuzzy search across memories.`
}

func (t *MemoryTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["save", "append", "replace-fact", "delete-fact", "delete", "invalidate", "read", "list", "search"],
				"description": "The action to perform."
			},
			"name": {
				"type": "string",
				"description": "Memory entry name. Required for: save, append, replace-fact, delete-fact, delete, read."
			},
			"description": {
				"type": "string",
				"description": "One-line description of the memory entry. Required for save."
			},
			"facts": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Array of discrete one-sentence facts. Required for save."
			},
			"fact": {
				"type": "string",
				"description": "A single fact string. Required for append and replace-fact."
			},
			"fact_index": {
				"type": "integer",
				"description": "0-based index of the fact to replace or delete. Required for replace-fact and delete-fact."
			},
			"tags": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Tags for categorization. Required for save."
			},
			"concepts": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Optional semantic concept tags (broader than tags). Examples: ['token-lifecycle', 'session-management']. Auto-extracted by Dream if omitted."
			},
			"type": {
				"type": "string",
				"enum": ["user", "feedback", "project", "reference"],
				"description": "Memory type (optional, default: project)."
			},
			"scope": {
				"type": "string",
				"enum": ["project", "global", "agent"],
				"description": "Where this memory lives. Ask: would this be true in a completely different project? If yes → global (user preferences, personal style). If no → project (architecture, conventions, decisions for this repo). If it belongs to a specific agent persona → agent. Default: project."
			},
			"query": {
				"type": "string",
				"description": "Search query string. Required for search."
			},
			"source_files": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Optional list of source file paths (absolute or relative) whose content hashes will be stored with the memory entry. On Recall, these are re-hashed to detect staleness. Use when saving facts derived from specific files."
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
	case "save":
		return t.saveMemory(in)
	case "append":
		return t.appendFact(in)
	case "replace-fact":
		return t.replaceFact(in)
	case "delete-fact":
		return t.deleteFact(in)
	case "delete":
		return t.deleteMemory(in.Name)
	case "invalidate":
		return t.invalidateMemory(in.Name)
	case "read":
		return t.readMemory(in.Name)
	case "list":
		return t.listMemories()
	case "search":
		return t.searchMemories(in.Query)
	default:
		return &Result{
			Content: fmt.Sprintf("Unknown action: %s. Use: save, append, replace-fact, delete-fact, delete, invalidate, read, list, search", in.Action),
			IsError: true,
		}, nil
	}
}

func (t *MemoryTool) saveMemory(in memoryInput) (*Result, error) {
	if in.Name == "" {
		return &Result{Content: "name is required for save", IsError: true}, nil
	}
	if len(in.Facts) == 0 {
		return &Result{Content: "facts[] is required for save (array of one-sentence fact strings)", IsError: true}, nil
	}

	// Check if already exists
	if _, err := t.Store.Load(in.Name); err == nil {
		return &Result{
			Content: fmt.Sprintf("Memory '%s' already exists. Use action='append' to add facts or action='replace-fact' to update a fact.", in.Name),
			IsError: true,
		}, nil
	}

	memType := in.Type
	if memType == "" {
		memType = memory.TypeProject
	}
	scope := in.Scope
	if scope == "" {
		scope = memory.ScopeProject
	}

	// Compute content hashes for any provided source files.
	var sourceFileHashes map[string]string
	if len(in.SourceFiles) > 0 {
		sourceFileHashes = make(map[string]string, len(in.SourceFiles))
		for _, p := range in.SourceFiles {
			data, err := os.ReadFile(p)
			if err != nil {
				return &Result{Content: fmt.Sprintf("Failed to read source file %q: %v", p, err), IsError: true}, nil
			}
			sum := sha256.Sum256(data)
			sourceFileHashes[p] = hex.EncodeToString(sum[:])
		}
	}

	entry := &memory.Entry{
		Name:        in.Name,
		Description: in.Description,
		Type:        memType,
		Scope:       scope,
		Facts:       in.Facts,
		Tags:        in.Tags,
		Concepts:    in.Concepts,
		SourceFiles: sourceFileHashes,
	}

	if err := t.Store.Save(entry); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to save: %v", err), IsError: true}, nil
	}

	msg := fmt.Sprintf("Memory '%s' saved with %d facts.", in.Name, len(in.Facts))
	if len(sourceFileHashes) > 0 {
		msg += fmt.Sprintf(" Content hashes stored for %d source file(s).", len(sourceFileHashes))
	}
	return &Result{Content: msg}, nil
}

func (t *MemoryTool) appendFact(in memoryInput) (*Result, error) {
	if in.Name == "" {
		return &Result{Content: "name is required for append", IsError: true}, nil
	}
	if in.Fact == "" {
		return &Result{Content: "fact is required for append (single fact string)", IsError: true}, nil
	}

	if err := t.Store.AppendFact(in.Name, in.Fact); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to append fact: %v", err), IsError: true}, nil
	}

	return &Result{Content: fmt.Sprintf("Fact appended to '%s'.", in.Name)}, nil
}

func (t *MemoryTool) replaceFact(in memoryInput) (*Result, error) {
	if in.Name == "" {
		return &Result{Content: "name is required for replace-fact", IsError: true}, nil
	}
	if in.Fact == "" {
		return &Result{Content: "fact is required for replace-fact (the replacement text)", IsError: true}, nil
	}

	if err := t.Store.ReplaceFact(in.Name, in.FactIndex, in.Fact); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to replace fact: %v", err), IsError: true}, nil
	}

	return &Result{Content: fmt.Sprintf("Fact %d replaced in '%s'.", in.FactIndex, in.Name)}, nil
}

func (t *MemoryTool) deleteFact(in memoryInput) (*Result, error) {
	if in.Name == "" {
		return &Result{Content: "name is required for delete-fact", IsError: true}, nil
	}

	if err := t.Store.RemoveFact(in.Name, in.FactIndex); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to delete fact: %v", err), IsError: true}, nil
	}

	return &Result{Content: fmt.Sprintf("Fact %d deleted from '%s'.", in.FactIndex, in.Name)}, nil
}

func (t *MemoryTool) deleteMemory(name string) (*Result, error) {
	if name == "" {
		return &Result{Content: "name is required for delete", IsError: true}, nil
	}

	if err := t.Store.Remove(name); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to delete memory: %v", err), IsError: true}, nil
	}

	return &Result{Content: fmt.Sprintf("Memory '%s' deleted.", name)}, nil
}

// invalidateMemory removes the named entry so it will be re-investigated next time.
// Semantically equivalent to delete but signals cache-busting intent rather than permanent removal.
func (t *MemoryTool) invalidateMemory(name string) (*Result, error) {
	if name == "" {
		return &Result{Content: "name is required for invalidate", IsError: true}, nil
	}

	if err := t.Store.Remove(name); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to invalidate memory: %v", err), IsError: true}, nil
	}

	return &Result{Content: fmt.Sprintf("Memory '%s' invalidated. Re-investigate and save again when ready.", name)}, nil
}

func (t *MemoryTool) readMemory(name string) (*Result, error) {
	if name == "" {
		return &Result{Content: "name is required for read", IsError: true}, nil
	}

	entry, err := t.Store.Load(name)
	if err != nil {
		return &Result{
			Content: fmt.Sprintf("Memory %q not found. Use action=list to see available memories.", name),
			IsError: true,
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", entry.Name))
	if entry.Description != "" {
		sb.WriteString(fmt.Sprintf("**Description:** %s\n", entry.Description))
	}
	sb.WriteString(fmt.Sprintf("**Type:** %s\n", entry.Type))
	if entry.Scope != "" {
		sb.WriteString(fmt.Sprintf("**Scope:** %s\n", entry.Scope))
	}
	if len(entry.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("**Tags:** %s\n", strings.Join(entry.Tags, ", ")))
	}
	sb.WriteString(fmt.Sprintf("**Updated:** %s\n", entry.UpdatedAt.Format("2006-01-02 15:04")))

	sb.WriteString("\n## Facts\n\n")
	if len(entry.Facts) == 0 {
		sb.WriteString("(no facts)\n")
	} else {
		for i, fact := range entry.Facts {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i, fact))
		}
	}

	return &Result{Content: sb.String()}, nil
}

func (t *MemoryTool) listMemories() (*Result, error) {
	entries := t.Store.LoadAll()
	if len(entries) == 0 {
		return &Result{Content: "No memories found."}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d memories:\n\n", len(entries)))

	for _, e := range entries {
		sb.WriteString("- **")
		sb.WriteString(e.Name)
		sb.WriteString("**")

		if len(e.Tags) > 0 {
			sb.WriteString(" [")
			sb.WriteString(strings.Join(e.Tags, ", "))
			sb.WriteString("]")
		}

		sb.WriteString(" — ")
		sb.WriteString(e.Description)

		if len(e.Facts) > 0 {
			first := e.Facts[0]
			if len(first) > 80 {
				first = first[:77] + "..."
			}
			sb.WriteString(fmt.Sprintf(" · \"%s\"", first))
		}

		sb.WriteString("\n")
	}

	return &Result{Content: sb.String()}, nil
}

func (t *MemoryTool) searchMemories(query string) (*Result, error) {
	if query == "" {
		return &Result{Content: "query is required for search", IsError: true}, nil
	}

	matches := t.Store.FindRelevant(query)
	if len(matches) == 0 {
		return &Result{Content: fmt.Sprintf("No memories matching %q found.", query)}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d memories matching %q:\n\n", len(matches), query))

	for _, e := range matches {
		sb.WriteString(fmt.Sprintf("## %s\n", e.Name))
		if e.Description != "" {
			sb.WriteString(e.Description)
			sb.WriteString("\n")
		}
		if len(e.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("**Tags:** %s\n", strings.Join(e.Tags, ", ")))
		}
		sb.WriteString("\n**Facts:**\n")
		for i, fact := range e.Facts {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i, fact))
		}
		sb.WriteString("\n")
	}

	return &Result{Content: sb.String()}, nil
}
