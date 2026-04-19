package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
)

// ReviewDesignFidelityTool compares a live running app's UI against saved
// design session screenshots using a vision-capable model.
type ReviewDesignFidelityTool struct {
	designsDir string
	client     *api.Client
	model      string
}

// NewReviewDesignFidelityTool creates a ReviewDesignFidelityTool.
func NewReviewDesignFidelityTool(designsDir string, client *api.Client, model string) *ReviewDesignFidelityTool {
	return &ReviewDesignFidelityTool{designsDir: designsDir, client: client, model: model}
}

// ReviewDesignFidelityInput is the JSON input schema.
type ReviewDesignFidelityInput struct {
	URL            string             `json:"url"`             // single URL — compare to all design screens
	Screens        []ScreenURLMapping `json:"screens"`         // OR per-screen [{name, url}]
	SessionName    string             `json:"session_name"`    // optional, default = latest
	ViewportWidth  int                `json:"viewport_width"`  // default 1440
	ViewportHeight int                `json:"viewport_height"` // default 900
}

// ScreenURLMapping maps a design screen name to a live URL.
type ScreenURLMapping struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ReviewDesignFidelityOutput is the structured result.
type ReviewDesignFidelityOutput struct {
	OverallScore int                   `json:"overall_score"`
	Pass         bool                  `json:"pass"`
	SessionName  string                `json:"session_name"`
	Screens      []ScreenFidelityResult `json:"screens"`
}

// ScreenFidelityResult holds the fidelity result for one screen.
type ScreenFidelityResult struct {
	Name             string   `json:"name"`
	DesignScreenshot string   `json:"design_screenshot"`
	LiveScreenshot   string   `json:"live_screenshot"`
	FidelityScore    int      `json:"fidelity_score"`
	Gaps             []string `json:"gaps"`
	Suggestions      []string `json:"suggestions"`
}

// fidelityVisionResponse is what the LLM returns per screen.
type fidelityVisionResponse struct {
	FidelityScore int      `json:"fidelity_score"`
	Gaps          []string `json:"gaps"`
	Suggestions   []string `json:"suggestions"`
}

// screenJob is an internal work item: one design screen + its target URL.
type screenJob struct {
	name                string
	designScreenshotPath string
	targetURL           string
}

func (t *ReviewDesignFidelityTool) Name() string { return "ReviewDesignFidelity" }

func (t *ReviewDesignFidelityTool) Description() string {
	return `Compare a live running app's UI against saved design session screenshots.

Captures a live screenshot of the provided URL using Playwright (headless Chromium),
then sends both the design mockup image and the live screenshot to a vision-capable
model for fidelity scoring.

Returns a 0-100 fidelity score per screen plus gaps and suggestions.
Pass threshold: overall_score >= 75.

Requires Node.js >= 18 and Playwright installed:
  npm install -g playwright  (or npx playwright install chromium)`
}

func (t *ReviewDesignFidelityTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
	"type": "object",
	"properties": {
		"url": {
			"type": "string",
			"description": "Single URL to screenshot and compare against all design screens."
		},
		"screens": {
			"type": "array",
			"description": "Per-screen mappings [{name, url}]. Use instead of url for different URLs per screen.",
			"items": {
				"type": "object",
				"properties": {
					"name": { "type": "string" },
					"url":  { "type": "string" }
				},
				"required": ["name", "url"]
			}
		},
		"session_name": {
			"type": "string",
			"description": "Design session name to compare against. Defaults to the most recent session."
		},
		"viewport_width": {
			"type": "integer",
			"description": "Browser viewport width in CSS pixels. Default: 1440."
		},
		"viewport_height": {
			"type": "integer",
			"description": "Browser viewport height in CSS pixels. Default: 900."
		}
	}
}`)
}

func (t *ReviewDesignFidelityTool) IsReadOnly() bool { return false }

func (t *ReviewDesignFidelityTool) RequiresApproval(_ json.RawMessage) bool { return false }

// fidelityModel returns the model to use for fidelity critique.
// Priority: CLAUDIO_FIDELITY_MODEL env > constructor model param > default.
func (t *ReviewDesignFidelityTool) fidelityModel() string {
	if m := os.Getenv("CLAUDIO_FIDELITY_MODEL"); m != "" {
		return m
	}
	if t.model != "" {
		return t.model
	}
	return "claude-haiku-4-5-20251001"
}

// checkPrerequisites verifies that Node.js and Playwright are available.
func (t *ReviewDesignFidelityTool) checkPrerequisites() error {
	if err := exec.Command("node", "--version").Run(); err != nil {
		return fmt.Errorf(
			"Node.js not found.\n" +
				"To use ReviewDesignFidelity:\n" +
				"  1. Install Node.js >= 18: https://nodejs.org\n" +
				"  2. Run: npx playwright install chromium\n" +
				"  3. Try again",
		)
	}
	if err := exec.Command("npx", "playwright", "--version").Run(); err != nil {
		return fmt.Errorf(
			"Playwright not found.\n" +
				"To use ReviewDesignFidelity:\n" +
				"  1. Ensure Node.js >= 18 is installed: node --version\n" +
				"  2. Run: npx playwright install chromium\n" +
				"  3. Try again",
		)
	}
	return nil
}

// resolveSession finds the design session to compare against.
// If sessionName is provided, looks up that specific session.
// Otherwise picks the newest session by CreatedAt.
func (t *ReviewDesignFidelityTool) resolveSession(sessionName string) (*DesignSession, error) {
	entries, err := os.ReadDir(t.designsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read designs dir %q: %w", t.designsDir, err)
	}

	var sessions []DesignSession
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		sessionDir := filepath.Join(t.designsDir, name)
		manifestPath := filepath.Join(sessionDir, "manifest.json")
		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest ManifestJSON
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			continue
		}
		sessions = append(sessions, DesignSession{
			Session:        name,
			SessionDir:     sessionDir,
			Screens:        manifest.Screens,
			CreatedAt:      manifest.CreatedAt,
			ScreenshotsDir: filepath.Join(sessionDir, "screenshots"),
		})
	}

	if len(sessions) == 0 {
		return nil, fmt.Errorf("no design sessions found in %q", t.designsDir)
	}

	if sessionName != "" {
		for i := range sessions {
			if sessions[i].Session == sessionName {
				return &sessions[i], nil
			}
		}
		return nil, fmt.Errorf("design session %q not found in %q", sessionName, t.designsDir)
	}

	// Pick newest by CreatedAt.
	sort.Slice(sessions, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, sessions[i].CreatedAt)
		tj, _ := time.Parse(time.RFC3339, sessions[j].CreatedAt)
		return ti.After(tj)
	})
	return &sessions[0], nil
}

// captureScreenshot runs the fidelity Node.js script and returns the live screenshot path.
func (t *ReviewDesignFidelityTool) captureScreenshot(ctx context.Context, scriptPath, targetURL, outDir string, viewportW, viewportH int) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	//nolint:gosec // scriptPath is a temp file we just wrote
	cmd := exec.CommandContext(cmdCtx, "node", scriptPath,
		"--url", targetURL,
		"--out-dir", outDir,
		"--viewport-w", strconv.Itoa(viewportW),
		"--viewport-h", strconv.Itoa(viewportH),
		"--scale", "2",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run()

	var nodeOut nodeScriptOutput
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &nodeOut); err != nil {
		return "", fmt.Errorf("Node.js script produced no valid JSON.\nstdout: %s\nstderr: %s",
			stdout.String(), stderr.String())
	}
	if !nodeOut.Success || len(nodeOut.Screenshots) == 0 {
		errMsg := strings.Join(nodeOut.Errors, "; ")
		if errMsg == "" {
			errMsg = "no screenshots captured"
		}
		return "", fmt.Errorf("playwright capture failed: %s", errMsg)
	}
	return nodeOut.Screenshots[0].Path, nil
}

// Execute implements the tool.
func (t *ReviewDesignFidelityTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in ReviewDesignFidelityInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	// 1. Validate input.
	if in.URL == "" && len(in.Screens) == 0 {
		return &Result{Content: "either url or screens is required", IsError: true}, nil
	}
	if in.ViewportWidth == 0 {
		in.ViewportWidth = 1440
	}
	if in.ViewportHeight == 0 {
		in.ViewportHeight = 900
	}

	// 2. Resolve design session.
	session, err := t.resolveSession(in.SessionName)
	if err != nil {
		return &Result{Content: err.Error(), IsError: true}, nil
	}

	// 3. Build screen jobs.
	var jobs []screenJob
	if len(in.Screens) > 0 {
		for _, sm := range in.Screens {
			jobs = append(jobs, screenJob{
				name:                 sm.Name,
				designScreenshotPath: filepath.Join(session.ScreenshotsDir, sm.Name+".png"),
				targetURL:            sm.URL,
			})
		}
	} else {
		for _, screen := range session.Screens {
			jobs = append(jobs, screenJob{
				name:                 screen,
				designScreenshotPath: filepath.Join(session.ScreenshotsDir, screen+".png"),
				targetURL:            in.URL,
			})
		}
	}
	if len(jobs) == 0 {
		return &Result{Content: "design session has no screens; cannot compare", IsError: true}, nil
	}

	// 4. Check prerequisites.
	if err := t.checkPrerequisites(); err != nil {
		return &Result{Content: err.Error(), IsError: true}, nil
	}

	// 5. Write embedded script to temp file once; reuse across all jobs.
	tmpScript, err := os.CreateTemp("", "claudio-fidelity-*.js")
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create temp script: %v", err), IsError: true}, nil
	}
	defer os.Remove(tmpScript.Name())
	if _, err := tmpScript.WriteString(fidelityScript); err != nil {
		tmpScript.Close()
		return &Result{Content: fmt.Sprintf("Failed to write temp script: %v", err), IsError: true}, nil
	}
	tmpScript.Close()

	// Temp dir for live screenshots.
	liveDir, err := os.MkdirTemp("", "claudio-fidelity-live-*")
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create temp dir: %v", err), IsError: true}, nil
	}
	// Note: we don't remove liveDir — paths are returned in result for caller use.

	var results []ScreenFidelityResult
	totalScore := 0

	for _, job := range jobs {
		res := ScreenFidelityResult{
			Name:             job.name,
			DesignScreenshot: job.designScreenshotPath,
		}

		// 5a. Capture live screenshot.
		screenOutDir := filepath.Join(liveDir, job.name)
		livePath, captureErr := t.captureScreenshot(ctx, tmpScript.Name(), job.targetURL, screenOutDir, in.ViewportWidth, in.ViewportHeight)
		if captureErr != nil {
			res.Gaps = []string{fmt.Sprintf("live capture failed: %s", captureErr.Error())}
			res.Suggestions = []string{}
			results = append(results, res)
			continue
		}
		res.LiveScreenshot = livePath

		// 5b. Read design screenshot — remap for worktree support.
		designPath := RemapPathForWorktree(ctx, job.designScreenshotPath)
		designBytes, readErr := os.ReadFile(designPath)
		if readErr != nil {
			res.Gaps = []string{"design screenshot not found"}
			res.Suggestions = []string{}
			results = append(results, res)
			continue
		}

		// 5c. Read live screenshot.
		liveBytes, readErr := os.ReadFile(livePath)
		if readErr != nil {
			res.Gaps = []string{fmt.Sprintf("live screenshot unreadable: %s", readErr.Error())}
			res.Suggestions = []string{}
			results = append(results, res)
			continue
		}

		// 5d. Crop + base64 encode both images.
		designBytes = cropImageIfNeeded(designBytes)
		liveBytes = cropImageIfNeeded(liveBytes)
		designBase64 := base64.StdEncoding.EncodeToString(designBytes)
		liveBase64 := base64.StdEncoding.EncodeToString(liveBytes)

		// 5e. Build vision message with both image blocks.
		contentBlocks := []api.UserContentBlock{
			api.NewImageBlock("image/png", designBase64),
			api.NewImageBlock("image/png", liveBase64),
			api.NewTextBlock("Image 1 is the design mockup. Image 2 is the live implementation. Score fidelity 0-100 and list gaps and suggestions. Respond JSON only: {\"fidelity_score\": <int>, \"gaps\": [], \"suggestions\": []}"),
		}
		contentJSON, marshalErr := json.Marshal(contentBlocks)
		if marshalErr != nil {
			return nil, fmt.Errorf("failed to marshal content blocks: %w", marshalErr)
		}

		messages := []api.Message{
			{Role: "user", Content: json.RawMessage(contentJSON)},
		}

		// 5f. Call LLM.
		resp, llmErr := t.client.SendMessage(ctx, &api.MessagesRequest{
			Model:     t.fidelityModel(),
			System:    "You are a design fidelity reviewer. Compare design mockups to live implementations and return structured JSON.",
			Messages:  messages,
			MaxTokens: 4096,
		})
		if llmErr != nil {
			res.Gaps = []string{fmt.Sprintf("vision API call failed: %s", llmErr.Error())}
			res.Suggestions = []string{}
			results = append(results, res)
			continue
		}

		// 5g. Extract text from response.
		var responseText string
		for _, block := range resp.Content {
			if block.Type == "text" && block.Text != "" {
				responseText += block.Text
			}
		}

		// 5h. Strip markdown fences, parse JSON.
		cleaned := strings.TrimSpace(responseText)
		if strings.HasPrefix(cleaned, "```") {
			if idx := strings.Index(cleaned, "\n"); idx != -1 {
				cleaned = cleaned[idx+1:]
			}
			if idx := strings.LastIndex(cleaned, "```"); idx != -1 {
				cleaned = cleaned[:idx]
			}
			cleaned = strings.TrimSpace(cleaned)
		}

		var vr fidelityVisionResponse
		if parseErr := json.Unmarshal([]byte(cleaned), &vr); parseErr != nil {
			res.Gaps = []string{fmt.Sprintf("failed to parse vision response: %s\nraw: %s", parseErr.Error(), responseText)}
			res.Suggestions = []string{}
			results = append(results, res)
			continue
		}

		// Ensure slices non-nil.
		if vr.Gaps == nil {
			vr.Gaps = []string{}
		}
		if vr.Suggestions == nil {
			vr.Suggestions = []string{}
		}

		res.FidelityScore = vr.FidelityScore
		res.Gaps = vr.Gaps
		res.Suggestions = vr.Suggestions
		totalScore += vr.FidelityScore
		results = append(results, res)
	}

	// 6. Aggregate.
	overallScore := 0
	if len(results) > 0 {
		overallScore = totalScore / len(results)
	}

	out := ReviewDesignFidelityOutput{
		OverallScore: overallScore,
		Pass:         overallScore >= 75,
		SessionName:  session.Session,
		Screens:      results,
	}

	outJSON, _ := json.MarshalIndent(out, "", "  ")
	return &Result{Content: string(outJSON)}, nil
}
