package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/services/memory"
)

// RecallTool semantically searches stored memories for entries relevant to a given context.
// It uses a small model to understand intent and meaning rather than keyword matching.
type RecallTool struct {
	deferrable
	Store  *memory.ScopedStore
	Client *api.Client
	Model  string
}

func (t *RecallTool) Name() string { return "Recall" }

func (t *RecallTool) Description() string {
	return `Semantically searches all memories (global, project, and agent) for entries relevant to your current context.
Unlike Memory(search) which does keyword matching, Recall understands intent and meaning.

Use this:
- Before starting any significant task: Recall(context="about to implement JWT auth middleware")
- When something feels familiar: Recall(context="this connection pool error")
- Before architectural decisions: Recall(context="choosing between options A and B for caching")
- When you're unsure what you might have learned before about a topic

Recall searches across all scopes automatically — you do not need to specify a scope.
Returns the full facts of the most relevant entries found anywhere in memory.
For keyword search, use Memory(action="search") instead.
For browsing all memories, use Memory(action="list").`
}

func (t *RecallTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "context": {
      "type": "string",
      "description": "Describe what you are about to do or what you are looking for. Be specific — include the task domain, key terms, and goal. Example: 'about to implement JWT refresh middleware for the auth API, need to know project conventions'"
    }
  },
  "required": ["context"]
}`)
}

func (t *RecallTool) IsReadOnly() bool                        { return true }
func (t *RecallTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *RecallTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	if t.Store == nil {
		return &Result{Content: "memory store not available", IsError: true}, nil
	}

	var in struct {
		Context string `json:"context"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: "invalid input: " + err.Error(), IsError: true}, nil
	}
	if in.Context == "" {
		return &Result{Content: "context is required", IsError: true}, nil
	}

	entries := t.Store.LoadAll()
	if len(entries) == 0 {
		return &Result{Content: "No memories stored yet."}, nil
	}

	// For small sets, skip LLM and return everything.
	if len(entries) <= 5 {
		return &Result{Content: formatEntries(entries)}, nil
	}

	// Build compact index for the LLM.
	index := buildCompactIndex(entries)

	// Ask the small model to select relevant entries.
	selectedNames, err := t.selectRelevant(ctx, in.Context, index)
	if err != nil || len(selectedNames) == 0 {
		// Graceful degradation: return all entries if LLM fails.
		return &Result{Content: formatEntries(entries)}, nil
	}

	// Load full entries for selected names.
	nameSet := make(map[string]bool)
	for _, n := range selectedNames {
		nameSet[strings.TrimSpace(n)] = true
	}

	var matched []*memory.Entry
	for _, e := range entries {
		if nameSet[e.Name] {
			matched = append(matched, e)
		}
	}

	if len(matched) == 0 {
		return &Result{Content: "No relevant memories found for the given context."}, nil
	}

	return &Result{Content: formatEntries(matched)}, nil
}

// selectRelevant calls the small model and returns matched entry names.
func (t *RecallTool) selectRelevant(ctx context.Context, taskContext, index string) ([]string, error) {
	if t.Client == nil {
		return nil, fmt.Errorf("api client not configured")
	}

	systemPrompt := "You are a memory relevance selector. Given a task context and memory index, return ONLY the names of the most relevant entries, one name per line. Return at most 5 names. Return nothing if nothing is clearly relevant. Do not explain, just list names."
	userPrompt := "Task context: " + taskContext + "\n\nMemory index:\n" + index

	userContent, _ := json.Marshal(userPrompt)
	model := t.Model
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}

	req := &api.MessagesRequest{
		Model: model,
		Messages: []api.Message{
			{Role: "user", Content: userContent},
		},
		System:    systemPrompt,
		MaxTokens: 256,
	}

	resp, err := t.Client.SendMessage(ctx, req)
	if err != nil {
		return nil, err
	}

	var text string
	for _, block := range resp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}

	var names []string
	for _, line := range strings.Split(text, "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// buildCompactIndex creates a one-line-per-entry summary for the LLM prompt.
func buildCompactIndex(entries []*memory.Entry) string {
	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString("- ")
		sb.WriteString(e.Name)
		if len(e.Tags) > 0 {
			sb.WriteString(" [")
			sb.WriteString(strings.Join(e.Tags, ","))
			sb.WriteString("]")
		}
		sb.WriteString(": ")
		sb.WriteString(e.Description)
		if len(e.Facts) > 0 {
			fact := e.Facts[0]
			if len(fact) > 60 {
				fact = fact[:57] + "..."
			}
			sb.WriteString(` — "`)
			sb.WriteString(fact)
			sb.WriteString(`"`)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// formatEntries renders a list of memory entries as readable text.
func formatEntries(entries []*memory.Entry) string {
	if len(entries) == 0 {
		return "No relevant memories found."
	}

	var parts []string
	for _, e := range entries {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Memory: %s [scope: %s]\n", e.Name, e.Scope))
		if len(e.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(e.Tags, ", ")))
		}
		if e.Description != "" {
			sb.WriteString(fmt.Sprintf("Description: %s\n", e.Description))
		}
		if len(e.Facts) > 0 {
			sb.WriteString("Facts:\n")
			for i, fact := range e.Facts {
				sb.WriteString(fmt.Sprintf("  [%d] %s\n", i, fact))
			}
		}
		parts = append(parts, sb.String())
	}
	return strings.Join(parts, "\n---\n\n")
}
