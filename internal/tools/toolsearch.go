package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ToolSearchTool is a meta-tool that lets the model fetch full schemas for deferred tools.
type ToolSearchTool struct {
	registry *Registry
}

func (t *ToolSearchTool) Name() string { return "ToolSearch" }

func (t *ToolSearchTool) Description() string {
	return `Fetches full schema definitions for deferred tools so they can be called.

Deferred tools appear by name in <system-reminder> messages. Until fetched, only the name is known — there is no parameter schema, so the tool cannot be invoked. This tool takes a query, matches it against the deferred tool list, and returns the matched tools' complete JSONSchema definitions inside a <functions> block. Once a tool's schema appears in that result, it is callable exactly like any tool defined at the top of the prompt.

Result format: each matched tool appears as one <function>{"description": "...", "name": "...", "parameters": {...}}</function> line inside the <functions> block — the same encoding as the tool list at the top of this prompt.

Query forms:
- "select:Read,Edit,Grep" — fetch these exact tools by name
- "notebook jupyter" — keyword search, up to max_results best matches
- "+slack send" — require "slack" in the name, rank by remaining terms`
}

func (t *ToolSearchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Query to find deferred tools. Use \"select:<tool_name>\" for direct selection, or keywords to search."
			},
			"max_results": {
				"type": "number",
				"description": "Maximum number of results to return (default: 5)",
				"default": 5
			}
		},
		"required": ["query"]
	}`)
}

func (t *ToolSearchTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var params struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}
	if params.MaxResults <= 0 {
		params.MaxResults = 5
	}

	if t.registry == nil {
		return &Result{Content: "ToolSearch not initialized: no registry", IsError: true}, nil
	}

	var matched []Tool

	if strings.HasPrefix(params.Query, "select:") {
		// Direct selection: "select:TaskCreate,TaskUpdate"
		names := strings.Split(strings.TrimPrefix(params.Query, "select:"), ",")
		for _, name := range names {
			name = strings.TrimSpace(name)
			if tool, err := t.registry.Get(name); err == nil {
				matched = append(matched, tool)
			}
		}
	} else {
		// Keyword search against tool names and search hints
		query := strings.ToLower(params.Query)
		keywords := strings.Fields(query)
		hints := t.registry.ToolSearchHints()

		type scored struct {
			tool  Tool
			score int
		}
		var candidates []scored

		for _, tool := range t.registry.All() {
			name := strings.ToLower(tool.Name())
			hint := strings.ToLower(hints[tool.Name()])
			score := 0
			for _, kw := range keywords {
				if strings.Contains(name, kw) {
					score += 2
				}
				if strings.Contains(hint, kw) {
					score++
				}
			}
			if score > 0 {
				candidates = append(candidates, scored{tool, score})
			}
		}

		// Sort by score descending (simple selection)
		for i := 0; i < len(candidates); i++ {
			for j := i + 1; j < len(candidates); j++ {
				if candidates[j].score > candidates[i].score {
					candidates[i], candidates[j] = candidates[j], candidates[i]
				}
			}
		}

		for i, c := range candidates {
			if i >= params.MaxResults {
				break
			}
			matched = append(matched, c.tool)
		}
	}

	if len(matched) == 0 {
		return &Result{Content: "No tools matched the query."}, nil
	}

	// Build response with full tool definitions
	var defs []APIToolDef
	for _, tool := range matched {
		defs = append(defs, APIToolDef{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.InputSchema(),
		})
	}

	// Format as tool_reference blocks so the engine can track discovered tools
	var sb strings.Builder
	sb.WriteString("<functions>\n")
	for _, def := range defs {
		defJSON, _ := json.Marshal(def)
		sb.WriteString(fmt.Sprintf("<function>%s</function>\n", string(defJSON)))
	}
	sb.WriteString("</functions>")

	return &Result{Content: sb.String()}, nil
}

func (t *ToolSearchTool) IsReadOnly() bool                            { return true }
func (t *ToolSearchTool) RequiresApproval(_ json.RawMessage) bool     { return false }

// SetRegistry injects the registry after construction (avoids circular dependency).
func (t *ToolSearchTool) SetRegistry(r *Registry) {
	t.registry = r
}
