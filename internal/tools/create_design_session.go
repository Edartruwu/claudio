package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/config"
)

// CreateDesignSessionTool creates a timestamped design session directory and
// copies starter JSX files into it, so agents never compute paths themselves.
type CreateDesignSessionTool struct{}

// NewCreateDesignSessionTool creates a CreateDesignSessionTool.
func NewCreateDesignSessionTool() *CreateDesignSessionTool {
	return &CreateDesignSessionTool{}
}

// CreateDesignSessionInput is the JSON input schema for this tool.
type CreateDesignSessionInput struct {
	Name string `json:"name"` // optional human label, e.g. "dashboard-hifi"
}

// CreateDesignSessionOutput is the JSON result returned by this tool.
type CreateDesignSessionOutput struct {
	SessionDir  string   `json:"session_dir"`  // absolute path to the new session directory
	SessionName string   `json:"session_name"` // e.g. "20260426-120000-dashboard-hifi"
	Starters    []string `json:"starters"`     // list of starter filenames copied in
}

var sanitizeRe = regexp.MustCompile(`[^a-z0-9-]+`)

func sanitizeLabel(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = sanitizeRe.ReplaceAllString(s, "")
	// collapse consecutive hyphens
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

func (t *CreateDesignSessionTool) Name() string { return "CreateDesignSession" }

func (t *CreateDesignSessionTool) Description() string {
	return `Creates a new timestamped design session directory under the project-scoped designs path and copies all starter JSX files into it. Returns the absolute session_dir path. Call this at the start of every new design — never compute the path yourself.`
}

func (t *CreateDesignSessionTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Optional human label appended to the timestamp, e.g. \"dashboard-hifi\". Only [a-z0-9-] characters (spaces converted to hyphens, others stripped)."
			}
		}
	}`)
}

func (t *CreateDesignSessionTool) IsReadOnly() bool { return false }

func (t *CreateDesignSessionTool) RequiresApproval(_ json.RawMessage) bool { return false }

// Execute implements the tool.
func (t *CreateDesignSessionTool) Execute(_ context.Context, input json.RawMessage) (*Result, error) {
	var in CreateDesignSessionInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	wd, _ := os.Getwd()
	designsDir := config.ProjectDesignsDir(wd)

	// Build session name: timestamp [+ -label]
	sessionName := time.Now().Format("20060102-150405")
	if in.Name != "" {
		if label := sanitizeLabel(in.Name); label != "" {
			sessionName = sessionName + "-" + label
		}
	}

	sessionDir := filepath.Join(designsDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create session dir %s: %v", sessionDir, err), IsError: true}, nil
	}

	// Copy all *.jsx starters into the session dir.
	startersDir := config.GetPaths().Starters
	entries, err := os.ReadDir(startersDir)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to read starters dir %s: %v", startersDir, err), IsError: true}, nil
	}

	var copied []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsx") {
			continue
		}
		src := filepath.Join(startersDir, entry.Name())
		dst := filepath.Join(sessionDir, entry.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			return &Result{Content: fmt.Sprintf("Failed to read starter %s: %v", src, err), IsError: true}, nil
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return &Result{Content: fmt.Sprintf("Failed to write starter %s: %v", dst, err), IsError: true}, nil
		}
		copied = append(copied, entry.Name())
	}

	out := CreateDesignSessionOutput{
		SessionDir:  sessionDir,
		SessionName: sessionName,
		Starters:    copied,
	}

	outJSON, _ := json.MarshalIndent(out, "", "  ")
	return &Result{Content: string(outJSON)}, nil
}
