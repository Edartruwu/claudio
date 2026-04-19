package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ListDesignsTool scans design sessions and returns those matching the current project.
type ListDesignsTool struct {
	designsDir string
}

// NewListDesignsTool creates a ListDesignsTool that scans designsDir.
func NewListDesignsTool(designsDir string) *ListDesignsTool {
	return &ListDesignsTool{designsDir: designsDir}
}

// ListDesignsInput is the JSON input schema for this tool.
type ListDesignsInput struct {
	ProjectPath string `json:"project_path"` // if empty, use os.Getwd()
}

// DesignSession represents a single design session with its metadata.
type DesignSession struct {
	Session        string           `json:"session"`         // dir basename e.g. "20260418-login"
	SessionDir     string           `json:"session_dir"`     // abs path
	Screens        []ScreenManifest `json:"screens"`         // artboard screen metadata from manifest
	CreatedAt      string           `json:"created_at"`      // RFC3339 timestamp
	HandoffDir     string           `json:"handoff_dir"`     // {sessionDir}/handoff
	HasHandoff     bool             `json:"has_handoff"`     // spec.md exists in handoff dir
	BundlePath     string           `json:"bundle_path"`     // {sessionDir}/bundle/mockup.html if exists
	ScreenshotsDir string           `json:"screenshots_dir"` // {sessionDir}/screenshots
}

// ListDesignsOutput is the JSON result returned by this tool.
type ListDesignsOutput struct {
	Designs []DesignSession `json:"designs"`
	Total   int             `json:"total"`
}

func (t *ListDesignsTool) Name() string { return "ListDesigns" }

func (t *ListDesignsTool) Description() string {
	return `List all design sessions for the current project. Returns sessions with their artboard screens, handoff status, and file paths. Use this to discover what designs are available before creating implementation tasks.`
}

func (t *ListDesignsTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"project_path": {
				"type": "string",
				"description": "Absolute path to the project. If empty, uses os.Getwd()."
			}
		}
	}`)
}

func (t *ListDesignsTool) IsReadOnly() bool { return true }

func (t *ListDesignsTool) RequiresApproval(_ json.RawMessage) bool { return false }

// Execute implements the tool.
func (t *ListDesignsTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in ListDesignsInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	projectPath := in.ProjectPath
	if projectPath == "" {
		var err error
		projectPath, err = os.Getwd()
		if err != nil {
			projectPath = ""
		}
	}

	// Scan {designsDir}/*/manifest.json
	entries, err := os.ReadDir(t.designsDir)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to read designs dir: %v", err), IsError: true}, nil
	}

	var designs []DesignSession

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sessionName := entry.Name()
		sessionDir := filepath.Join(t.designsDir, sessionName)
		manifestPath := filepath.Join(sessionDir, "manifest.json")

		// Read manifest
		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			// Manifest doesn't exist for this session, skip
			continue
		}

		var manifest ManifestJSON
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			// Unmarshal failed, skip
			continue
		}

		// Check handoff existence
		handoffDir := filepath.Join(sessionDir, "handoff")
		specPath := filepath.Join(handoffDir, "spec.md")
		hasHandoff := false
		if _, err := os.Stat(specPath); err == nil {
			hasHandoff = true
		}

		// Check bundle existence
		bundlePath := filepath.Join(sessionDir, "bundle", "mockup.html")
		bundlePathResult := ""
		if _, err := os.Stat(bundlePath); err == nil {
			bundlePathResult = bundlePath
		}

		// Screenshots dir
		screenshotsDir := filepath.Join(sessionDir, "screenshots")

		designs = append(designs, DesignSession{
			Session:        sessionName,
			SessionDir:     sessionDir,
			Screens:        manifest.Screens,
			CreatedAt:      manifest.CreatedAt,
			HandoffDir:     handoffDir,
			HasHandoff:     hasHandoff,
			BundlePath:     bundlePathResult,
			ScreenshotsDir: screenshotsDir,
		})
	}

	// Sort by CreatedAt descending (newest first)
	sort.Slice(designs, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, designs[i].CreatedAt)
		tj, _ := time.Parse(time.RFC3339, designs[j].CreatedAt)
		return ti.After(tj)
	})

	out := ListDesignsOutput{
		Designs: designs,
		Total:   len(designs),
	}

	outJSON, _ := json.MarshalIndent(out, "", "  ")
	return &Result{Content: string(outJSON)}, nil
}
