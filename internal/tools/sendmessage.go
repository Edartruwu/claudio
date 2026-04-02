package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Abraxas-365/claudio/internal/teams"
)

// SendMessageTool sends messages between teammates.
type SendMessageTool struct {
	deferrable
	Manager   *teams.Manager
	TeamName  string // current team context
	AgentName string // sender's name
}

type sendMessageInput struct {
	To      string `json:"to"`      // agent name, or "*" for broadcast
	Message string `json:"message"` // text or JSON for structured messages
}

func (t *SendMessageTool) Name() string { return "SendMessage" }

func (t *SendMessageTool) Description() string {
	return `Send a message to a teammate or broadcast to all teammates.

## Recipient Syntax
- "agent_name" — Direct message to one teammate
- "*" — Broadcast to all teammates

## Message Types
- Plain text: Regular coordination message
- Structured JSON: Control messages like shutdown requests

## Protocol Responses
When a teammate requests shutdown or plan approval, respond with:
- {"type": "shutdown_response", "approve": true}
- {"type": "plan_approval_response", "approve": true}

Keep messages concise and actionable.`
}

func (t *SendMessageTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"to": {
				"type": "string",
				"description": "Recipient agent name, or \"*\" for broadcast"
			},
			"message": {
				"type": "string",
				"description": "Message text or JSON for structured messages"
			}
		},
		"required": ["to", "message"]
	}`)
}

func (t *SendMessageTool) IsReadOnly() bool                        { return false }
func (t *SendMessageTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *SendMessageTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in sendMessageInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if t.Manager == nil || t.TeamName == "" {
		return &Result{Content: "Not in a team context", IsError: true}, nil
	}

	mailbox := teams.NewMailbox(t.Manager.TeamsDir(), t.TeamName)

	from := t.AgentName
	if from == "" {
		from = "team-lead"
	}

	msg := teams.Message{
		Text:  in.Message,
		Color: "", // could be set from identity
	}

	if in.To == "*" {
		if err := mailbox.Broadcast(from, msg); err != nil {
			return &Result{Content: fmt.Sprintf("Broadcast failed: %v", err), IsError: true}, nil
		}
		return &Result{Content: fmt.Sprintf("Broadcast sent to all teammates from %s", from)}, nil
	}

	if err := mailbox.Send(from, in.To, msg); err != nil {
		return &Result{Content: fmt.Sprintf("Send failed: %v", err), IsError: true}, nil
	}

	return &Result{Content: fmt.Sprintf("Message sent to %s from %s", in.To, from)}, nil
}
