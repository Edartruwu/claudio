package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// RenderMockupTool renders an HTML mockup using Playwright via Node.js and
// captures screenshots. Pure Go wrapper — no CGO. Browser invoked via
// exec.Command("node", ...) with an embedded Node.js script.
type RenderMockupTool struct {
	designsDir string
}

// NewRenderMockupTool creates a RenderMockupTool that saves screenshots under designsDir.
func NewRenderMockupTool(designsDir string) *RenderMockupTool {
	return &RenderMockupTool{designsDir: designsDir}
}

// RenderMockupInput is the JSON input schema for this tool.
type RenderMockupInput struct {
	HTMLPath       string `json:"html_path"`
	SessionDir     string `json:"session_dir"`     // optional: reuse existing session dir instead of creating new timestamp
	ViewportWidth  int    `json:"viewport_width"`  // default: 1440
	ViewportHeight int    `json:"viewport_height"` // default: 900
	DeviceScale    int    `json:"device_scale"`    // default: 2
	CaptureScreens *bool  `json:"capture_screens"` // pointer so we can detect omission; default true
}

// ScreenshotInfo holds the name and file path of a captured screenshot.
type ScreenshotInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// RenderMockupOutput is the JSON result returned by this tool.
type RenderMockupOutput struct {
	Success         bool             `json:"success"`
	ConsoleErrors   []string         `json:"console_errors"`
	ConsoleWarnings []string         `json:"console_warnings"`
	Screenshots     []ScreenshotInfo `json:"screenshots"`
	RenderTimeMs    int64            `json:"render_time_ms"`
	OutputDir       string           `json:"output_dir"`
}

// nodeScriptOutput matches the JSON the Node.js script writes to stdout.
type nodeScriptOutput struct {
	Success     bool             `json:"success"`
	Errors      []string         `json:"errors"`
	Warnings    []string         `json:"warnings"`
	Screenshots []ScreenshotInfo `json:"screenshots"`
}

func (t *RenderMockupTool) Name() string { return "RenderMockup" }

func (t *RenderMockupTool) Description() string {
	return `Render an HTML mockup using Playwright (headless Chromium) and capture screenshots.

Loads the HTML file in a headless browser, waits for fonts and animations to
settle, then captures a full-page screenshot plus individual screenshots for
each element marked with data-artboard="<name>".

Returns paths to all screenshots, any console errors/warnings emitted during
render, and the total render time.

Requires Node.js >= 18 and Playwright installed:
  npm install -g playwright  (or npx playwright install chromium)`
}

func (t *RenderMockupTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"html_path": {
				"type": "string",
				"description": "Absolute or relative path to the HTML file to render."
			},
			"session_dir": {
				"type": "string",
				"description": "Session directory to write screenshots into ({session_dir}/screenshots/). Pass the same session_dir used for BundleMockup to keep all outputs together. Defaults to a new {designsDir}/{timestamp} dir."
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
				"description": "Device pixel ratio (1 = normal, 2 = retina). Default: 2."
			},
			"capture_screens": {
				"type": "boolean",
				"description": "If true, screenshot each [data-artboard] element in addition to the full page. Default: true."
			}
		},
		"required": ["html_path"]
	}`)
}

func (t *RenderMockupTool) IsReadOnly() bool { return false }

func (t *RenderMockupTool) RequiresApproval(_ json.RawMessage) bool { return false }

// checkPrerequisites verifies that Node.js and Playwright are available.
func (t *RenderMockupTool) checkPrerequisites() error {
	if err := exec.Command("node", "--version").Run(); err != nil {
		return fmt.Errorf(
			"Node.js not found.\n" +
				"To use RenderMockup:\n" +
				"  1. Install Node.js >= 18: https://nodejs.org\n" +
				"  2. Run: npx playwright install chromium\n" +
				"  3. Try again",
		)
	}

	// npx playwright --version exits 0 when playwright is installed.
	if err := exec.Command("npx", "playwright", "--version").Run(); err != nil {
		return fmt.Errorf(
			"Playwright not found.\n" +
				"To use RenderMockup:\n" +
				"  1. Ensure Node.js >= 18 is installed: node --version\n" +
				"  2. Run: npx playwright install chromium\n" +
				"  3. Try again",
		)
	}

	return nil
}

func (t *RenderMockupTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	// 1. Prerequisites.
	if err := t.checkPrerequisites(); err != nil {
		return &Result{Content: err.Error(), IsError: true}, nil
	}

	// 2. Parse input.
	var in RenderMockupInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}
	if in.HTMLPath == "" {
		return &Result{Content: "html_path is required", IsError: true}, nil
	}

	// Apply defaults.
	if in.ViewportWidth == 0 {
		in.ViewportWidth = 1440
	}
	if in.ViewportHeight == 0 {
		in.ViewportHeight = 900
	}
	if in.DeviceScale == 0 {
		in.DeviceScale = 2
	}
	captureScreens := true
	if in.CaptureScreens != nil {
		captureScreens = *in.CaptureScreens
	}

	// 3. Create output dir: {sessionDir}/screenshots/ or {designsDir}/{timestamp}/screenshots/
	sessionDir := in.SessionDir
	if sessionDir == "" {
		sessionDir = filepath.Join(t.designsDir, time.Now().Format("20060102-150405"))
	}
	outDir := filepath.Join(sessionDir, "screenshots")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create output dir: %v", err), IsError: true}, nil
	}

	// 4. Write the embedded Node.js script to a temp file.
	tmpScript, err := os.CreateTemp("", "claudio-render-*.js")
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create temp script: %v", err), IsError: true}, nil
	}
	defer os.Remove(tmpScript.Name())

	if _, err := tmpScript.WriteString(renderScript); err != nil {
		tmpScript.Close()
		return &Result{Content: fmt.Sprintf("Failed to write temp script: %v", err), IsError: true}, nil
	}
	tmpScript.Close()

	// 5. Build and run the command with a 60 s deadline.
	cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	captureArg := "false"
	if captureScreens {
		captureArg = "true"
	}

	//nolint:gosec // script path is a temp file we just wrote
	cmd := exec.CommandContext(cmdCtx, "node", tmpScript.Name(),
		"--html", in.HTMLPath,
		"--out-dir", outDir,
		"--viewport-w", strconv.Itoa(in.ViewportWidth),
		"--viewport-h", strconv.Itoa(in.ViewportHeight),
		"--scale", strconv.Itoa(in.DeviceScale),
		"--capture-screens", captureArg,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start).Milliseconds()

	// 6. Parse JSON from stdout regardless of exit code — the script always
	//    emits JSON before exiting non-zero.
	var nodeOut nodeScriptOutput
	if jsonErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &nodeOut); jsonErr != nil {
		// stdout was not valid JSON — surface raw output as error.
		errMsg := fmt.Sprintf("Node.js script produced no valid JSON.\nstdout: %s\nstderr: %s",
			stdout.String(), stderr.String())
		if runErr != nil {
			errMsg += fmt.Sprintf("\nexit error: %v", runErr)
		}
		return &Result{Content: errMsg, IsError: true}, nil
	}

	// 7. Build structured result.
	out := RenderMockupOutput{
		Success:         nodeOut.Success,
		ConsoleErrors:   nodeOut.Errors,
		ConsoleWarnings: nodeOut.Warnings,
		Screenshots:     nodeOut.Screenshots,
		RenderTimeMs:    elapsed,
		OutputDir:       outDir,
	}
	if out.ConsoleErrors == nil {
		out.ConsoleErrors = []string{}
	}
	if out.ConsoleWarnings == nil {
		out.ConsoleWarnings = []string{}
	}
	if out.Screenshots == nil {
		out.Screenshots = []ScreenshotInfo{}
	}

	outJSON, _ := json.MarshalIndent(out, "", "  ")

	isError := !nodeOut.Success
	return &Result{Content: string(outJSON), IsError: isError}, nil
}
