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
	Runner    *teams.TeammateRunner
	TeamName  string // current team context
	AgentName string // sender's name
}

type sendMessageInput struct {
	To      string `json:"to"`      // agent name, or "*" for broadcast
	Message string `json:"message"` // text or JSON for structured messages
}

func (t *SendMessageTool) Name() string { return "SendMessage" }

func (t *SendMessageTool) Description() string {
	return `Send a message to another agent.

## Recipient Syntax
- "agent_name" — Direct message to one teammate
- "*" — Broadcast to all teammates — expensive (linear in team size), use only when everyone genuinely needs it

Your plain text output is NOT visible to other agents — to communicate, you MUST call this tool. Messages from teammates are delivered automatically; you don't check an inbox. Refer to teammates by name, never by UUID. When relaying, don't quote the original — it's already rendered to the user.

## Protocol Responses

If you receive a JSON message with ` + "`type: \"shutdown_request\"`" + ` or ` + "`type: \"plan_approval_request\"`" + `, respond with the matching ` + "`_response`" + ` type — echo the ` + "`request_id`" + `, set ` + "`approve`" + ` true/false:

` + "```json" + `
{"to": "team-lead", "message": {"type": "shutdown_response", "request_id": "...", "approve": true}}
{"to": "researcher", "message": {"type": "plan_approval_response", "request_id": "...", "approve": false, "feedback": "add error handling"}}
` + "```" + `

Approving shutdown terminates your process. Rejecting plan sends the teammate back to revise. Don't send structured JSON status messages — use TaskUpdate.`
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

	// Resolve team context: prefer context (set per-teammate) over tool fields.
	teamName := t.TeamName
	agentName := t.AgentName
	if tc := TeamContextFromCtx(ctx); tc != nil {
		teamName = tc.TeamName
		agentName = tc.AgentName
	}

	// Fall back to the active team from the runner (covers the team-lead case).
	if teamName == "" && t.Runner != nil {
		teamName = t.Runner.ActiveTeamName()
	}

	if t.Manager == nil || teamName == "" {
		return &Result{Content: "Not in a team context", IsError: true}, nil
	}

	mailbox := teams.NewMailbox(t.Manager.TeamsDir(), teamName)

	from := agentName
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

	// If the recipient agent is idle (completed/failed), revive it so it can
	// read the new message and continue the conversation. Revive itself is
	// a no-op if the agent is still working or was explicitly shutdown.
	revived := ""
	if t.Runner != nil {
		if err := t.Runner.Revive(in.To, in.Message); err == nil {
			if state, ok := t.Runner.GetStateByName(in.To); ok && state.Status == teams.StatusWorking {
				revived = " (revived)"
			}
		}
	}

	return &Result{Content: fmt.Sprintf("Message sent to %s from %s%s", in.To, from, revived)}, nil
}
