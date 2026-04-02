package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// AskUserTool allows the AI to ask the user structured questions with options.
type AskUserTool struct {
	deferrable

	// RequestCh receives question requests from Execute and forwards to TUI.
	// ResponseCh sends user answers back to Execute.
	// These are set by the TUI layer during wiring.
	RequestCh  chan AskUserRequest
	ResponseCh chan AskUserResponse
}

// AskUserRequest is sent from the tool to the TUI.
type AskUserRequest struct {
	Questions []AskQuestion
}

// AskUserResponse is sent back from the TUI to the tool.
type AskUserResponse struct {
	Answers map[string]string // question label → selected answer(s)
}

// AskQuestion defines a single question with options.
type AskQuestion struct {
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Options     []string `json:"options"`
	MultiSelect bool     `json:"multi_select,omitempty"`
}

func (t *AskUserTool) Name() string        { return "AskUser" }
func (t *AskUserTool) Description() string  { return "Ask the user structured questions with predefined options" }
func (t *AskUserTool) IsReadOnly() bool     { return true }
func (t *AskUserTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *AskUserTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"questions": {
				"type": "array",
				"description": "1-4 questions to ask the user",
				"items": {
					"type": "object",
					"properties": {
						"label": {"type": "string", "description": "Short question text"},
						"description": {"type": "string", "description": "Additional context for the question"},
						"options": {"type": "array", "items": {"type": "string"}, "description": "2-6 answer options"},
						"multi_select": {"type": "boolean", "description": "Allow multiple selections"}
					},
					"required": ["label", "options"]
				},
				"minItems": 1,
				"maxItems": 4
			}
		},
		"required": ["questions"]
	}`)
}

func (t *AskUserTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var params struct {
		Questions []AskQuestion `json:"questions"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return errResult("invalid input: " + err.Error()), nil
	}

	if len(params.Questions) == 0 {
		return errResult("at least one question is required"), nil
	}
	if len(params.Questions) > 4 {
		return errResult("maximum 4 questions allowed"), nil
	}

	// If no TUI channels are set, fall back to inline text
	if t.RequestCh == nil || t.ResponseCh == nil {
		// Non-interactive fallback: format questions as text and return
		var sb strings.Builder
		sb.WriteString("Questions for the user (please respond in your next message):\n\n")
		for i, q := range params.Questions {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, q.Label))
			if q.Description != "" {
				sb.WriteString(fmt.Sprintf("   %s\n", q.Description))
			}
			sb.WriteString("   Options: " + strings.Join(q.Options, ", ") + "\n")
		}
		return &Result{Content: sb.String()}, nil
	}

	// Send request to TUI
	select {
	case t.RequestCh <- AskUserRequest{Questions: params.Questions}:
	case <-ctx.Done():
		return errResult("cancelled"), nil
	}

	// Wait for response
	select {
	case resp := <-t.ResponseCh:
		// Format answers as JSON for the AI
		data, _ := json.MarshalIndent(resp.Answers, "", "  ")
		return &Result{Content: string(data)}, nil
	case <-ctx.Done():
		return errResult("cancelled"), nil
	}
}
