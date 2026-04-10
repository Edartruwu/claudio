package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/services/compact"
)

// advisorMaxIter caps the tool-calling loop to prevent runaway advisor sessions.
const advisorMaxIter = 5

// advisorWhitelist is the authoritative set of tools the advisor is allowed to call.
// This is enforced in code — no agent config or definition can override it.
var advisorWhitelist = map[string]bool{
	"WebSearch": true,
	"WebFetch":  true,
}

// advisorToolResultBlock is the JSON shape the API expects for tool results.
type advisorToolResultBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// AdvisorToolConfig holds the configuration for creating an AdvisorTool.
type AdvisorToolConfig struct {
	Definition  agents.AgentDefinition // advisor's system prompt + tools
	Model       string                 // model override (e.g. "claude-opus-4-6")
	MaxUses     int                    // 0 = unlimited
	UsedCount   *int                   // pointer so spawn wiring shares the counter
	GetMessages func() []api.Message   // callback to get executor's current messages
	Client      *api.Client            // api client to use for advisor calls
}

// AdvisorTool consults a strategic advisor (e.g. Opus) at key decision points.
// It supports two modes: "plan" (after orientation) and "review" (before declaring done).
type AdvisorTool struct {
	definition  agents.AgentDefinition
	model       string
	maxUses     int
	usedCount   *int
	getMessages func() []api.Message
	client      *api.Client
}

// NewAdvisorTool creates a new AdvisorTool from the given configuration.
func NewAdvisorTool(cfg AdvisorToolConfig) *AdvisorTool {
	return &AdvisorTool{
		definition:  cfg.Definition,
		model:       cfg.Model,
		maxUses:     cfg.MaxUses,
		usedCount:   cfg.UsedCount,
		getMessages: cfg.GetMessages,
		client:      cfg.Client,
	}
}

// advisorInput is the union of both plan and review mode inputs.
type advisorInput struct {
	Mode string `json:"mode"` // "plan" or "review"

	// Plan mode fields
	OrientationSummary string `json:"orientation_summary,omitempty"`
	ProposedApproach   string `json:"proposed_approach,omitempty"`
	DecisionNeeded     string `json:"decision_needed,omitempty"`
	ContextNotes       string `json:"context_notes,omitempty"`

	// Review mode fields
	OriginalPlan     string `json:"original_plan,omitempty"`
	ExecutionSummary string `json:"execution_summary,omitempty"`
	OutcomeArtifacts string `json:"outcome_artifacts,omitempty"`
	Confidence       string `json:"confidence,omitempty"`
}

func (t *AdvisorTool) Name() string { return "advisor" }

func (t *AdvisorTool) Description() string {
	return "Consult the strategic advisor for a plan (before substantive work) or verdict (before declaring done). Use mode=plan after orientation, mode=review after execution."
}

func (t *AdvisorTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
	"type": "object",
	"properties": {
		"mode": {
			"type": "string",
			"enum": ["plan", "review"],
			"description": "Consultation mode: 'plan' for strategic planning after orientation, 'review' for verdict before declaring done"
		},
		"orientation_summary": {
			"type": "string",
			"description": "(plan mode) Summary of orientation findings"
		},
		"proposed_approach": {
			"type": "string",
			"description": "(plan mode) Your proposed approach to the task"
		},
		"decision_needed": {
			"type": "string",
			"description": "(plan mode) Specific decision or guidance needed from the advisor"
		},
		"context_notes": {
			"type": "string",
			"description": "(plan mode, optional) Additional context or constraints"
		},
		"original_plan": {
			"type": "string",
			"description": "(review mode) The plan that was executed"
		},
		"execution_summary": {
			"type": "string",
			"description": "(review mode) Summary of what was actually done"
		},
		"outcome_artifacts": {
			"type": "string",
			"description": "(review mode) Key outputs, files changed, or results produced"
		},
		"confidence": {
			"type": "string",
			"enum": ["high", "medium", "low"],
			"description": "(review mode) Your confidence level in the outcome"
		}
	},
	"required": ["mode"]
}`)
}

func (t *AdvisorTool) IsReadOnly() bool { return true }

func (t *AdvisorTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *AdvisorTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in advisorInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.Mode != "plan" && in.Mode != "review" {
		return &Result{Content: fmt.Sprintf("Invalid mode %q: must be \"plan\" or \"review\"", in.Mode), IsError: true}, nil
	}

	// Check budget.
	if t.maxUses > 0 && *t.usedCount >= t.maxUses {
		return &Result{
			Content: fmt.Sprintf("Advisor budget exhausted (%d/%d uses). Proceed with your best judgment based on prior advisor guidance.", *t.usedCount, t.maxUses),
		}, nil
	}

	// Get executor messages and compress them.
	messages := t.getMessages()
	compressedMessages, _, err := compact.Compact(
		ctx, t.client, messages, 10,
		"Summarize the work done so far, preserving key decisions, findings, and errors.",
	)
	if err != nil {
		// Graceful fallback: use original messages if compression fails.
		compressedMessages = messages
	}

	// Build advisor system prompt.
	systemPrompt := t.definition.SystemPrompt
	systemPrompt += "\n\nIMPORTANT: Respond in ≤100 words. Use numbered steps. No explanations unless critical to the decision."

	// Format the brief into a user prompt.
	userPrompt := formatBrief(in)

	// Append the brief as a new user message to the compressed history.
	briefContent, _ := json.Marshal([]map[string]string{{"type": "text", "text": userPrompt}})
	advisorMessages := make([]api.Message, len(compressedMessages), len(compressedMessages)+1)
	copy(advisorMessages, compressedMessages)
	advisorMessages = append(advisorMessages, api.Message{
		Role:    "user",
		Content: json.RawMessage(briefContent),
	})

	// Ensure alternating user/assistant roles — if the last compressed message
	// was also a user message, merge them or insert a minimal assistant turn.
	advisorMessages = ensureAlternatingRoles(advisorMessages)

	// Build tool definitions — only WebSearch and WebFetch are permitted.
	webSearch := &WebSearchTool{}
	webFetch := &WebFetchTool{}
	toolDefs := []APIToolDef{
		{Name: webSearch.Name(), Description: webSearch.Description(), InputSchema: webSearch.InputSchema()},
		{Name: webFetch.Name(), Description: webFetch.Description(), InputSchema: webFetch.InputSchema()},
	}
	toolDefsJSON, _ := json.Marshal(toolDefs)

	// Tool-calling loop — capped at advisorMaxIter to prevent runaway sessions.
	var responseText strings.Builder
	for range advisorMaxIter {
		resp, err := t.client.SendMessage(ctx, &api.MessagesRequest{
			Model:    t.model,
			System:   systemPrompt,
			Messages: advisorMessages,
			Tools:    toolDefsJSON,
		})
		if err != nil {
			return &Result{Content: fmt.Sprintf("Advisor call failed: %v", err), IsError: true}, nil
		}

		// Collect text blocks from this response.
		responseText.Reset()
		for _, block := range resp.Content {
			if block.Type == "text" && block.Text != "" {
				if responseText.Len() > 0 {
					responseText.WriteString("\n")
				}
				responseText.WriteString(block.Text)
			}
		}

		// Collect tool_use blocks.
		var toolUseBlocks []api.ContentBlock
		for _, block := range resp.Content {
			if block.Type == "tool_use" {
				toolUseBlocks = append(toolUseBlocks, block)
			}
		}

		// No tool calls — advisor is done.
		if len(toolUseBlocks) == 0 {
			break
		}

		// Append the full assistant turn (may include text + tool_use blocks).
		assistantContent, _ := json.Marshal(resp.Content)
		advisorMessages = append(advisorMessages, api.Message{
			Role:    "assistant",
			Content: assistantContent,
		})

		// Execute whitelisted tools; block everything else.
		toolResults := make([]advisorToolResultBlock, 0, len(toolUseBlocks))
		for _, block := range toolUseBlocks {
			if !advisorWhitelist[block.Name] {
				// Blocked tool — return a clear error to the model.
				toolResults = append(toolResults, advisorToolResultBlock{
					Type:      "tool_result",
					ToolUseID: block.ID,
					Content:   fmt.Sprintf("Tool %q is not available to the advisor.", block.Name),
					IsError:   true,
				})
				continue
			}

			// Select and execute the whitelisted tool.
			var tool Tool
			switch block.Name {
			case "WebSearch":
				tool = webSearch
			case "WebFetch":
				tool = webFetch
			}
			result, execErr := tool.Execute(ctx, block.Input)
			if execErr != nil {
				toolResults = append(toolResults, advisorToolResultBlock{
					Type:      "tool_result",
					ToolUseID: block.ID,
					Content:   fmt.Sprintf("Tool execution error: %v", execErr),
					IsError:   true,
				})
			} else {
				toolResults = append(toolResults, advisorToolResultBlock{
					Type:      "tool_result",
					ToolUseID: block.ID,
					Content:   result.Content,
					IsError:   result.IsError,
				})
			}
		}

		// Append tool results as a user message.
		resultContent, _ := json.Marshal(toolResults)
		advisorMessages = append(advisorMessages, api.Message{
			Role:    "user",
			Content: resultContent,
		})
	}

	// Increment usage counter.
	*t.usedCount++

	return &Result{Content: responseText.String()}, nil
}

// formatBrief turns the advisor input into a human-readable prompt.
func formatBrief(in advisorInput) string {
	var b strings.Builder

	switch in.Mode {
	case "plan":
		b.WriteString("## Advisor Consultation: Strategic Plan Request\n\n")
		b.WriteString("**Orientation Summary:**\n")
		b.WriteString(in.OrientationSummary)
		b.WriteString("\n\n**Proposed Approach:**\n")
		b.WriteString(in.ProposedApproach)
		b.WriteString("\n\n**Decision Needed:**\n")
		b.WriteString(in.DecisionNeeded)
		if in.ContextNotes != "" {
			b.WriteString("\n\n**Additional Context:**\n")
			b.WriteString(in.ContextNotes)
		}
		b.WriteString("\n\nPlease provide a numbered plan with clear steps.")

	case "review":
		b.WriteString("## Advisor Consultation: Execution Review\n\n")
		b.WriteString("**Original Plan:**\n")
		b.WriteString(in.OriginalPlan)
		b.WriteString("\n\n**Execution Summary:**\n")
		b.WriteString(in.ExecutionSummary)
		b.WriteString("\n\n**Outcome Artifacts:**\n")
		b.WriteString(in.OutcomeArtifacts)
		b.WriteString("\n\n**Confidence Level:** ")
		b.WriteString(in.Confidence)
		b.WriteString("\n\nPlease provide a verdict: approve, request changes, or flag concerns.")
	}

	return b.String()
}

// ensureAlternatingRoles fixes message sequences where two consecutive messages
// have the same role, which the API rejects. If the last two messages are both
// "user", it inserts a minimal assistant acknowledgment between them.
func ensureAlternatingRoles(messages []api.Message) []api.Message {
	if len(messages) < 2 {
		return messages
	}

	result := make([]api.Message, 0, len(messages)+len(messages)/2)
	for i, msg := range messages {
		if i > 0 && msg.Role == messages[i-1].Role {
			// Insert a filler message with the opposite role.
			fillerRole := "assistant"
			if msg.Role == "assistant" {
				fillerRole = "user"
			}
			fillerContent, _ := json.Marshal([]map[string]string{{"type": "text", "text": "[continued]"}})
			result = append(result, api.Message{
				Role:    fillerRole,
				Content: json.RawMessage(fillerContent),
			})
		}
		result = append(result, msg)
	}
	return result
}
