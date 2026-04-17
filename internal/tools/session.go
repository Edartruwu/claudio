package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/Abraxas-365/claudio/internal/prompts"
)

// AttachClient is an interface for sending events to ComandCenter.
// Defined here to avoid import cycles with cli/attachclient.
type AttachClient interface {
	SendEvent(eventType string, payload any) error
}

// --- SendToSessionTool ---

type SendToSessionTool struct {
	deferrable
	AttachClient AttachClient
	AttachURL    string
}

type sendToSessionInput struct {
	Session string `json:"session"`
	Message string `json:"message"`
}

func (t *SendToSessionTool) Name() string { return "SendToSession" }

func (t *SendToSessionTool) Description() string {
	return prompts.SendToSessionDescription()
}

func (t *SendToSessionTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"session": {
				"type": "string",
				"description": "Session ID or name to send message to (or 'master' for the master session)"
			},
			"message": {
				"type": "string",
				"description": "Message to send to the session"
			}
		},
		"required": ["session", "message"]
	}`)
}

func (t *SendToSessionTool) IsReadOnly() bool                        { return false }
func (t *SendToSessionTool) RequiresApproval(input json.RawMessage) bool { return true }

func (t *SendToSessionTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in sendToSessionInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.Session == "" || in.Message == "" {
		return &Result{Content: "Both session and message are required", IsError: true}, nil
	}

	// Check if attach client is available
	if t.AttachClient == nil {
		return &Result{
			Content: "SendMessage requires --attach flag. This session is not connected to ComandCenter. Use --attach http://localhost:8080 (or your server URL) to enable message passing between sessions.",
			IsError: true,
		}, nil
	}

	// Send message payload to the attach client
	// The attach client will forward it to ComandCenter, which routes it to the target session
	payload := map[string]interface{}{
		"content":       in.Message,
		"from_session":  in.Session,
	}

	if err := t.AttachClient.SendEvent("message.user", payload); err != nil {
		return &Result{
			Content: fmt.Sprintf("Failed to send message: %v", err),
			IsError: true,
		}, nil
	}

	return &Result{Content: fmt.Sprintf("Message sent to session '%s'", in.Session)}, nil
}

// --- SpawnSessionTool ---

type SpawnSessionTool struct {
	deferrable
	AttachURL string
}

type spawnSessionInput struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Agent     string `json:"agent,omitempty"`
	Team      string `json:"team,omitempty"`
	Headless  bool   `json:"headless,omitempty"`
}

func (t *SpawnSessionTool) Name() string { return "SpawnSession" }

func (t *SpawnSessionTool) Description() string {
	return prompts.SpawnSessionDescription()
}

func (t *SpawnSessionTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Working directory path for the new session"
			},
			"name": {
				"type": "string",
				"description": "Display name for the new session in ComandCenter"
			},
			"agent": {
				"type": "string",
				"description": "Optional: Agent persona to run as (e.g., 'backend-senior', 'prab')"
			},
			"team": {
				"type": "string",
				"description": "Optional: Team template to pre-load (e.g., 'backend-team')"
			},
			"headless": {
				"type": "boolean",
				"description": "Optional: Run in headless mode (default: false, interactive TUI)"
			}
		},
		"required": ["path", "name"]
	}`)
}

func (t *SpawnSessionTool) IsReadOnly() bool                        { return false }
func (t *SpawnSessionTool) RequiresApproval(input json.RawMessage) bool { return true }

func (t *SpawnSessionTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in spawnSessionInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.Path == "" || in.Name == "" {
		return &Result{Content: "Both path and name are required", IsError: true}, nil
	}

	// Check if attach URL is configured
	if t.AttachURL == "" {
		return &Result{
			Content: "SpawnSession requires --attach flag. The master session must be started with --master and --attach flags to coordinate child sessions.",
			IsError: true,
		}, nil
	}

	// Get password from environment (same as main attach flow)
	password := os.Getenv("COMANDCENTER_PASSWORD")

	// Build command: claudio --attach {URL} --name {name} [--agent X] [--team Y] {path}
	args := []string{
		"claudio",
		"--attach", t.AttachURL,
		"--name", in.Name,
	}

	if in.Agent != "" {
		args = append(args, "--agent", in.Agent)
	}
	if in.Team != "" {
		args = append(args, "--team", in.Team)
	}
	if in.Headless {
		args = append(args, "--headless")
	}

	// Append the working directory path
	args = append(args, in.Path)

	// Prepare command
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = in.Path

	// Inherit stdout/stderr for visibility
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Pass environment including password
	env := os.Environ()
	if password != "" {
		env = append(env, "COMANDCENTER_PASSWORD="+password)
	}
	cmd.Env = env

	// Start in background (detached) — do not wait for it
	// Use Start() not Run() so we can return immediately
	err := cmd.Start()
	if err != nil {
		return &Result{
			Content: fmt.Sprintf("Failed to spawn session: %v", err),
			IsError: true,
		}, nil
	}

	return &Result{Content: fmt.Sprintf("Session '%s' spawned. PID: %d. Attach URL: %s", in.Name, cmd.Process.Pid, t.AttachURL)}, nil
}
