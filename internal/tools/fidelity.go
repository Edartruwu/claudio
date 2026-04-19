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

// ReviewDesignFidelityTool compares rendered HTML templates against saved
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
	Screens        []ScreenTemplateMapping `json:"screens"`         // required: [{name, template_path}]
	SessionName    string                  `json:"session_name"`    // optional, default = latest
	ViewportWidth  int                     `json:"viewport_width"`  // default 1440
	ViewportHeight int                     `json:"viewport_height"` // default 900
	DeviceScale    int                     `json:"device_scale"`    // default 2
	CSSPaths       []string                `json:"css_paths"`       // optional CSS files to inline before rendering
}

// ScreenTemplateMapping maps a design screen name to a local HTML template file or live URL.
type ScreenTemplateMapping struct {
	Name         string `json:"name"`                    // must match design session screen name
	TemplatePath string `json:"template_path,omitempty"` // abs or relative path to .html template file
	URL          string `json:"url,omitempty"`           // live URL to screenshot via Playwright
}

// urlScreenshotScript is the embedded Node.js script that navigates to a URL and captures a full-page screenshot.
const urlScreenshotScript = `
const { chromium } = require('playwright');
const args = process.argv.slice(2);
const url = args[args.indexOf('--url') + 1];
const outDir = args[args.indexOf('--out-dir') + 1];
const w = parseInt(args[args.indexOf('--viewport-w') + 1] || '1440');
const h = parseInt(args[args.indexOf('--viewport-h') + 1] || '900');
(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  await page.setViewportSize({width: w, height: h});
  await page.goto(url, {waitUntil: 'networkidle', timeout: 30000});
  const p = require('path').join(outDir, 'live.png');
  await page.screenshot({path: p, fullPage: true});
  await browser.close();
  console.log(JSON.stringify({success: true, screenshots: [{name: 'live', path: p}]}));
})().catch(e => { console.log(JSON.stringify({success: false, errors: [e.message]})); process.exit(1); });
`

// ReviewDesignFidelityOutput is the structured result.
type ReviewDesignFidelityOutput struct {
	OverallScore int                    `json:"overall_score"`
	Pass         bool                   `json:"pass"`
	SessionName  string                 `json:"session_name"`
	Screens      []ScreenFidelityResult `json:"screens"`
}

// ScreenFidelityResult holds the fidelity result for one screen.
type ScreenFidelityResult struct {
	Name             string   `json:"name"`
	DesignScreenshot string   `json:"design_screenshot"`
	LiveScreenshot   string   `json:"live_screenshot"` // rendered template screenshot
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

func (t *ReviewDesignFidelityTool) Name() string { return "ReviewDesignFidelity" }

func (t *ReviewDesignFidelityTool) Description() string {
	return `Compares design session screenshots against live pages or rendered HTML templates using vision-capable Haiku. Accepts either a live URL (Playwright screenshots it) or a local template file path (rendered via Playwright). Use URL mode for Go html/template pages served by a running server. Scores visual fidelity 0-100.

Pass threshold: overall_score >= 75.

Requires Node.js >= 18 and Playwright installed:
  npx playwright install chromium`
}

func (t *ReviewDesignFidelityTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
	"type": "object",
	"properties": {
		"screens": {
			"type": "array",
			"description": "Per-screen mappings. Each entry maps a design screen name to a local HTML template file path.",
			"items": {
				"type": "object",
				"properties": {
					"name":          { "type": "string", "description": "Design screen name (must match a screen in the session)." },
					"template_path": { "type": "string", "description": "Absolute or relative path to the HTML template file." },
					"url":           { "type": "string", "description": "URL of live page to screenshot (e.g. http://localhost:8080/sessions). Use for Go template pages — requires server running. Localhost requests bypass auth automatically." }
				},
				"required": ["name"]
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
		},
		"device_scale": {
			"type": "integer",
			"description": "Device pixel ratio for HiDPI rendering. Default: 2."
		},
		"css_paths": {
			"type": "array",
			"items": {"type": "string"},
			"description": "Paths to CSS files to inject into each template before rendering (e.g. Tailwind CSS, app.css). Solves CSS custom property issues when templates depend on vars defined in a layout file."
		}
	},
	"required": ["screens"]
}`)
}

// patchTemplateWithCSS inlines CSS files as <style> blocks prepended to the template HTML.
// Returns (patchedPath, isTempFile, error). If cssPaths is empty, returns original path unchanged.
func patchTemplateWithCSS(ctx context.Context, templatePath string, cssPaths []string) (string, bool, error) {
	if len(cssPaths) == 0 {
		return templatePath, false, nil
	}

	originalHTML, err := os.ReadFile(templatePath)
	if err != nil {
		return templatePath, false, fmt.Errorf("patchTemplateWithCSS: read template: %w", err)
	}

	var styleBlocks strings.Builder
	for _, p := range cssPaths {
		remapped := RemapPathForWorktree(ctx, p)
		cssBytes, readErr := os.ReadFile(remapped)
		if readErr != nil {
			// non-fatal: skip missing CSS file
			continue
		}
		styleBlocks.WriteString("<style>\n")
		styleBlocks.Write(cssBytes)
		styleBlocks.WriteString("\n</style>\n")
	}

	combined := append([]byte(styleBlocks.String()), originalHTML...)

	tmp, err := os.CreateTemp("", "claudio-fidelity-patched-*.html")
	if err != nil {
		return templatePath, false, fmt.Errorf("patchTemplateWithCSS: create temp: %w", err)
	}
	if _, err := tmp.Write(combined); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return templatePath, false, fmt.Errorf("patchTemplateWithCSS: write temp: %w", err)
	}
	tmp.Close()

	return tmp.Name(), true, nil
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

// Execute implements the tool.
func (t *ReviewDesignFidelityTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in ReviewDesignFidelityInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	// 1. Validate input and apply defaults.
	if len(in.Screens) == 0 {
		return &Result{Content: "screens is required and must not be empty", IsError: true}, nil
	}
	if in.ViewportWidth == 0 {
		in.ViewportWidth = 1440
	}
	if in.ViewportHeight == 0 {
		in.ViewportHeight = 900
	}
	if in.DeviceScale == 0 {
		in.DeviceScale = 2
	}

	// 2. Resolve design session.
	session, err := t.resolveSession(in.SessionName)
	if err != nil {
		return &Result{Content: err.Error(), IsError: true}, nil
	}

	// 3. Check prerequisites.
	if err := t.checkPrerequisites(); err != nil {
		return &Result{Content: err.Error(), IsError: true}, nil
	}

	// 4. Write renderScript (package var from render_script.go) to a temp file once; reuse across all screens.
	tmpScript, err := os.CreateTemp("", "claudio-fidelity-*.js")
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create temp script: %v", err), IsError: true}, nil
	}
	defer os.Remove(tmpScript.Name())
	if _, err := tmpScript.WriteString(renderScript); err != nil {
		tmpScript.Close()
		return &Result{Content: fmt.Sprintf("Failed to write temp script: %v", err), IsError: true}, nil
	}
	tmpScript.Close()

	// Write urlScreenshotScript to a temp file once; used for URL-mode screens.
	tmpURLScript, err := os.CreateTemp("", "claudio-fidelity-url-*.js")
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create URL screenshot script: %v", err), IsError: true}, nil
	}
	defer os.Remove(tmpURLScript.Name())
	if _, err := tmpURLScript.WriteString(urlScreenshotScript); err != nil {
		tmpURLScript.Close()
		return &Result{Content: fmt.Sprintf("Failed to write URL screenshot script: %v", err), IsError: true}, nil
	}
	tmpURLScript.Close()

	var results []ScreenFidelityResult
	totalScore := 0

	for _, screen := range in.Screens {
		res := ScreenFidelityResult{
			Name:             screen.Name,
			DesignScreenshot: filepath.Join(session.ScreenshotsDir, screen.Name+".png"),
		}

		// 4a. Create per-screen temp output dir.
		tmpOutDir, mkErr := os.MkdirTemp("", "claudio-fidelity-render-*")
		if mkErr != nil {
			res.Gaps = []string{fmt.Sprintf("failed to create temp dir: %s", mkErr.Error())}
			res.Suggestions = []string{}
			results = append(results, res)
			continue
		}

		// 4b. Determine rendered path: URL mode or template mode.
		var renderedPath string
		if screen.URL != "" {
			// URL mode — screenshot live page via urlScreenshotScript.
			cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			//nolint:gosec // script path is a temp file we just wrote
			cmd := exec.CommandContext(cmdCtx, "node", tmpURLScript.Name(),
				"--url", screen.URL,
				"--out-dir", tmpOutDir,
				"--viewport-w", strconv.Itoa(in.ViewportWidth),
				"--viewport-h", strconv.Itoa(in.ViewportHeight),
			)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			_ = cmd.Run()
			cancel()

			var nodeOut nodeScriptOutput
			if jsonErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &nodeOut); jsonErr != nil {
				os.RemoveAll(tmpOutDir)
				res.Gaps = []string{fmt.Sprintf("url screenshot script produced no valid JSON.\nstdout: %s\nstderr: %s",
					stdout.String(), stderr.String())}
				res.Suggestions = []string{}
				results = append(results, res)
				continue
			}
			if !nodeOut.Success || len(nodeOut.Screenshots) == 0 {
				os.RemoveAll(tmpOutDir)
				errMsg := strings.Join(nodeOut.Errors, "; ")
				if errMsg == "" {
					errMsg = "no screenshots captured"
				}
				res.Gaps = []string{fmt.Sprintf("playwright url screenshot failed: %s", errMsg)}
				res.Suggestions = []string{}
				results = append(results, res)
				continue
			}
			renderedPath = nodeOut.Screenshots[0].Path
		} else if screen.TemplatePath != "" {
			// Template mode — existing patchTemplateWithCSS + renderScript path.
			renderPath, isTemp, patchErr := patchTemplateWithCSS(ctx, RemapPathForWorktree(ctx, screen.TemplatePath), in.CSSPaths)
			if patchErr != nil {
				// non-fatal: fall back to original
				renderPath = RemapPathForWorktree(ctx, screen.TemplatePath)
				isTemp = false
			}
			if isTemp {
				defer os.Remove(renderPath)
			}
			cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			//nolint:gosec // script path is a temp file we just wrote
			cmd := exec.CommandContext(cmdCtx, "node", tmpScript.Name(),
				"--html", renderPath,
				"--out-dir", tmpOutDir,
				"--viewport-w", strconv.Itoa(in.ViewportWidth),
				"--viewport-h", strconv.Itoa(in.ViewportHeight),
				"--scale", strconv.Itoa(in.DeviceScale),
				"--capture-screens", "false",
			)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			_ = cmd.Run()
			cancel()

			var nodeOut nodeScriptOutput
			if jsonErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &nodeOut); jsonErr != nil {
				os.RemoveAll(tmpOutDir)
				res.Gaps = []string{fmt.Sprintf("render script produced no valid JSON.\nstdout: %s\nstderr: %s",
					stdout.String(), stderr.String())}
				res.Suggestions = []string{}
				results = append(results, res)
				continue
			}
			if !nodeOut.Success || len(nodeOut.Screenshots) == 0 {
				os.RemoveAll(tmpOutDir)
				errMsg := strings.Join(nodeOut.Errors, "; ")
				if errMsg == "" {
					errMsg = "no screenshots captured"
				}
				res.Gaps = []string{fmt.Sprintf("playwright render failed: %s", errMsg)}
				res.Suggestions = []string{}
				results = append(results, res)
				continue
			}
			renderedPath = nodeOut.Screenshots[0].Path
			for _, s := range nodeOut.Screenshots {
				if s.Name == "full-canvas" {
					renderedPath = s.Path
					break
				}
			}
		} else {
			os.RemoveAll(tmpOutDir)
			results = append(results, ScreenFidelityResult{Name: screen.Name, Gaps: []string{"neither url nor template_path provided"}, Suggestions: []string{}})
			continue
		}
		res.LiveScreenshot = renderedPath

		// 4d. Load design screenshot.
		designPath := RemapPathForWorktree(ctx, res.DesignScreenshot)
		designBytes, readErr := os.ReadFile(designPath)
		if readErr != nil {
			os.RemoveAll(tmpOutDir)
			res.Gaps = []string{"design screenshot not found for screen"}
			res.Suggestions = []string{}
			results = append(results, res)
			continue
		}

		// 4e. Read rendered screenshot, crop + base64 encode both images.
		renderedBytes, readErr := os.ReadFile(renderedPath)
		if readErr != nil {
			os.RemoveAll(tmpOutDir)
			res.Gaps = []string{fmt.Sprintf("rendered screenshot unreadable: %s", readErr.Error())}
			res.Suggestions = []string{}
			results = append(results, res)
			continue
		}
		designBytes = cropImageIfNeeded(designBytes)
		renderedBytes = cropImageIfNeeded(renderedBytes)
		designBase64 := base64.StdEncoding.EncodeToString(designBytes)
		renderedBase64 := base64.StdEncoding.EncodeToString(renderedBytes)

		// 4f. Build vision message: two image blocks + text prompt.
		contentBlocks := []api.UserContentBlock{
			api.NewImageBlock("image/png", designBase64),
			api.NewImageBlock("image/png", renderedBase64),
			api.NewTextBlock(`Score the VISUAL DESIGN fidelity between Image 1 (design mockup) and Image 2 (implementation). Ignore all text content, usernames, session names, timestamps, and data values — these will differ and are not design issues. Only evaluate: color palette, typography (font family/size/weight), spacing and padding, layout structure, component shapes, border styles, shadows, icons, and visual hierarchy. Respond JSON only: {"fidelity_score": 0-100, "gaps": ["specific visual gap"], "suggestions": ["specific fix"]}`),
		}
		contentJSON, marshalErr := json.Marshal(contentBlocks)
		if marshalErr != nil {
			os.RemoveAll(tmpOutDir)
			return nil, fmt.Errorf("failed to marshal content blocks: %w", marshalErr)
		}

		messages := []api.Message{
			{Role: "user", Content: json.RawMessage(contentJSON)},
		}

		resp, llmErr := t.client.SendMessage(ctx, &api.MessagesRequest{
			Model:     t.fidelityModel(),
			System:    "You are a design fidelity reviewer. Evaluate only visual design properties — never penalize for differences in text content, data values, or real vs mock data.",
			Messages:  messages,
			MaxTokens: 4096,
		})
		if llmErr != nil {
			os.RemoveAll(tmpOutDir)
			res.Gaps = []string{fmt.Sprintf("vision API call failed: %s", llmErr.Error())}
			res.Suggestions = []string{}
			results = append(results, res)
			continue
		}

		// 4g. Extract text from response.
		var responseText string
		for _, block := range resp.Content {
			if block.Type == "text" && block.Text != "" {
				responseText += block.Text
			}
		}

		// Strip markdown fences, parse JSON.
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
			os.RemoveAll(tmpOutDir)
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

		// 6. Clean up rendered screenshot temp dir.
		os.RemoveAll(tmpOutDir)
	}

	// 5. Aggregate.
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
