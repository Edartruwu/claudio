package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ScreenshotPusher is called after each screenshot is saved to disk.
// Implement in the wiring layer to push images into the ComandCenter chat.
// sessionID is the local session identifier forwarded for context; routing on
// the server side uses the hub's session binding, not this value.
type ScreenshotPusher interface {
	PushScreenshot(sessionID, filePath, filename string) error
	// PushBundleLink sends a clickable link bubble to CC chat pointing at a
	// self-contained bundle HTML file served by the CC server.
	// filePath is the absolute disk path so the server can copy it into uploads.
	PushBundleLink(sessionID, bundleURL, sessionName, filePath string) error
}

// RenderMockupTool renders an HTML mockup using Playwright via Node.js and
// captures screenshots. Pure Go wrapper — no CGO. Browser invoked via
// exec.Command("node", ...) with an embedded Node.js script.
type RenderMockupTool struct {
	designsDir string
	pusher     ScreenshotPusher // nil = no push (CLI/TUI-only mode)
	sessionID  string
}

// NewRenderMockupTool creates a RenderMockupTool that saves screenshots under designsDir.
func NewRenderMockupTool(designsDir string) *RenderMockupTool {
	return &RenderMockupTool{designsDir: designsDir}
}

// WithPusher wires a ScreenshotPusher so screenshots are auto-pushed to chat
// after being saved. sessionID is forwarded to PushScreenshot as context.
// Returns the receiver for fluent chaining.
func (t *RenderMockupTool) WithPusher(pusher ScreenshotPusher, sessionID string) *RenderMockupTool {
	t.pusher = pusher
	t.sessionID = sessionID
	return t
}

// RenderMockupInput is the JSON input schema for this tool.
type RenderMockupInput struct {
	HTMLPath       string `json:"html_path"`
	SessionDir     string `json:"session_dir"`     // optional: reuse existing session dir instead of creating new timestamp
	ForceNew       bool   `json:"force_new"`       // if true, always create a new timestamped session dir even if one exists
	ViewportWidth  int    `json:"viewport_width"`  // default: 1440
	ViewportHeight int    `json:"viewport_height"` // default: 900
	DeviceScale    int    `json:"device_scale"`    // default: 3
	CaptureScreens *bool  `json:"capture_screens"` // pointer so we can detect omission; default true
}

// ScreenshotInfo holds the name and file path of a captured screenshot.
type ScreenshotInfo struct {
	Name                string `json:"name"`
	Path                string `json:"path"`
	RenderedHTMLPath    string `json:"rendered_html_path,omitempty"`
	InteractionsPath    string `json:"interactions_path,omitempty"`
}

// ViewportSize holds the width and height of a browser viewport in CSS pixels.
type ViewportSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// ScreenManifest holds metadata for a single artboard screen in a design session.
type ScreenManifest struct {
	Name        string       `json:"name"`
	Type        string       `json:"type,omitempty"`        // "screen"|"foundation"|"component"|"state" — default "screen"
	Breakpoint  string       `json:"breakpoint"`            // "mobile" | "desktop" | "tablet" | "unknown"
	Viewport    ViewportSize `json:"viewport"`
	Description string       `json:"description,omitempty"`
}

// inferScreenManifest returns a ScreenManifest for name, inferring type,
// breakpoint and viewport from name keywords.
func inferScreenManifest(name string) ScreenManifest {
	lower := strings.ToLower(name)

	// Infer screen type from name keywords.
	var screenType string
	switch {
	case strings.Contains(lower, "foundation"):
		screenType = "foundation"
	case strings.Contains(lower, "component"):
		screenType = "component"
	case strings.Contains(lower, "state") || strings.Contains(lower, "states"):
		screenType = "state"
	default:
		screenType = "screen"
	}

	// Infer breakpoint from name keywords.
	switch {
	case strings.Contains(lower, "mobile"):
		return ScreenManifest{Name: name, Type: screenType, Breakpoint: "mobile", Viewport: ViewportSize{390, 844}}
	case strings.Contains(lower, "desktop"):
		return ScreenManifest{Name: name, Type: screenType, Breakpoint: "desktop", Viewport: ViewportSize{1440, 900}}
	case strings.Contains(lower, "tablet"):
		return ScreenManifest{Name: name, Type: screenType, Breakpoint: "tablet", Viewport: ViewportSize{768, 1024}}
	default:
		return ScreenManifest{Name: name, Type: screenType, Breakpoint: "unknown", Viewport: ViewportSize{390, 844}}
	}
}

// ManifestJSON represents the design session manifest.
type ManifestJSON struct {
	ProjectPath string           `json:"project_path"`
	SessionDir  string           `json:"session_dir"`
	CreatedAt   string           `json:"created_at"`
	Screens     []ScreenManifest `json:"screens"`
}

// UnmarshalJSON implements backward-compat deserialization for ManifestJSON.
// Old manifests have "screens": ["name1", "name2"]; new ones have full objects.
func (m *ManifestJSON) UnmarshalJSON(data []byte) error {
	// Use an alias to avoid infinite recursion.
	type manifestAlias struct {
		ProjectPath string          `json:"project_path"`
		SessionDir  string          `json:"session_dir"`
		CreatedAt   string          `json:"created_at"`
		Screens     json.RawMessage `json:"screens"`
	}
	var alias manifestAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	m.ProjectPath = alias.ProjectPath
	m.SessionDir = alias.SessionDir
	m.CreatedAt = alias.CreatedAt

	if len(alias.Screens) == 0 || string(alias.Screens) == "null" {
		m.Screens = nil
		return nil
	}

	// Try new format first: []ScreenManifest
	var screens []ScreenManifest
	if err := json.Unmarshal(alias.Screens, &screens); err == nil {
		// Validate: if any element has a non-empty Name we assume it's the new format.
		// (An empty array or all-zero structs from string unmarshal would fail below anyway.)
		m.Screens = screens
		return nil
	}

	// Fall back to old format: []string
	var names []string
	if err := json.Unmarshal(alias.Screens, &names); err != nil {
		return fmt.Errorf("manifest screens field is neither []ScreenManifest nor []string: %w", err)
	}
	m.Screens = make([]ScreenManifest, 0, len(names))
	for _, n := range names {
		m.Screens = append(m.Screens, inferScreenManifest(n))
	}
	return nil
}

// RenderMockupOutput is the JSON result returned by this tool.
type RenderMockupOutput struct {
	Success         bool             `json:"success"`
	ConsoleErrors   []string         `json:"console_errors"`
	ConsoleWarnings []string         `json:"console_warnings"`
	Screenshots     []ScreenshotInfo `json:"screenshots"`
	RenderTimeMs    int64            `json:"render_time_ms"`
	OutputDir       string           `json:"output_dir"`
	SessionDir      string           `json:"session_dir"`
}

// nodeScriptOutput matches the JSON the Node.js script writes to stdout.
type nodeScriptOutput struct {
	Success         bool             `json:"success"`
	Errors          []string         `json:"errors"`
	Warnings        []string         `json:"warnings"`
	Screenshots     []ScreenshotInfo `json:"screenshots"`
	FragmentWarning string           `json:"fragment_warning"` // non-empty when URL returned an HTML fragment with no stylesheets
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
				"description": "Session directory to write screenshots into ({session_dir}/screenshots/). Pass the same session_dir used for BundleMockup to keep all outputs together. If omitted, the tool reuses the most recent existing session for this project, or creates a new one if none exists."
			},
			"force_new": {
				"type": "boolean",
				"description": "If true, always create a new timestamped session directory even if one already exists. Use this only when the user explicitly wants to start a brand new design from scratch. Default: false."
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
				"description": "Device pixel ratio (1 = normal, 2 = retina, 3 = high-res phone). Default: 3."
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

// inlineLocalScripts rewrites an HTML file so that every
//
//	<script ... src="relative/local/path.jsx">
//
// tag is replaced by an inline <script ...> block with the file's content.
// Remote URLs (http/https//) are left untouched.
// Returns a path to a temp file containing the inlined HTML; the caller is
// responsible for removing it.
var scriptSrcRe = regexp.MustCompile(`(?i)<script([^>]*?)\ssrc="([^"]+)"([^>]*)>\s*</script>`)


func inlineLocalScripts(htmlPath string) (string, error) {
	htmlBytes, err := os.ReadFile(htmlPath)
	if err != nil {
		return "", fmt.Errorf("read html: %w", err)
	}
	baseDir := filepath.Dir(htmlPath)

	replaced := scriptSrcRe.ReplaceAllFunc(htmlBytes, func(match []byte) []byte {
		m := scriptSrcRe.FindSubmatch(match)
		if m == nil {
			return match
		}
		before, src, after := string(m[1]), string(m[2]), string(m[3])

		// Leave remote URLs alone.
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "//") {
			return match
		}

		absPath := src
		if !filepath.IsAbs(src) {
			absPath = filepath.Join(baseDir, src)
		}
		content, err := os.ReadFile(absPath)
		if err != nil {
			// Can't read the file — leave the original tag; Playwright will
			// surface the actual error in console_errors.
			return match
		}

		// Rebuild tag without src, with inlined content.
		// Preserve all other attributes (e.g. type="text/babel").
		attrs := strings.TrimSpace(before + " " + after)
		return []byte(fmt.Sprintf("<script %s>\n%s\n</script>", attrs, string(content)))
	})

	tmp, err := os.CreateTemp("", "claudio-inlined-*.html")
	if err != nil {
		return "", fmt.Errorf("create temp html: %w", err)
	}
	if _, err := tmp.Write(replaced); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("write temp html: %w", err)
	}
	tmp.Close()
	return tmp.Name(), nil
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

	// Guard: extension check
	if !strings.EqualFold(filepath.Ext(in.HTMLPath), ".html") {
		return &Result{Content: fmt.Sprintf("html_path must be an .html file, got: %q", in.HTMLPath), IsError: true}, nil
	}

	// Guard: content sniff
	if f, err := os.Open(in.HTMLPath); err == nil {
		peek := make([]byte, 512)
		n, _ := f.Read(peek)
		f.Close()
		lower := strings.ToLower(string(peek[:n]))
		if !strings.Contains(lower, "<html") && !strings.Contains(lower, "<!doctype") {
			return &Result{Content: fmt.Sprintf("html_path does not look like HTML (no <html> or <!DOCTYPE> in first 512 bytes): %q", in.HTMLPath), IsError: true}, nil
		}
	}

	// Apply defaults.
	if in.ViewportWidth == 0 {
		in.ViewportWidth = 1440
	}
	if in.ViewportHeight == 0 {
		in.ViewportHeight = 900
	}
	if in.DeviceScale == 0 {
		in.DeviceScale = 3
	}
	captureScreens := true
	if in.CaptureScreens != nil {
		captureScreens = *in.CaptureScreens
	}

	// 3. Resolve session directory.
	// Priority: explicit session_dir > canonical "session" dir (single per project).
	sessionDir := in.SessionDir
	if sessionDir == "" {
		sessionDir = filepath.Join(t.designsDir, "session")
	}
	outDir := filepath.Join(sessionDir, "screenshots")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create output dir: %v", err), IsError: true}, nil
	}

	// 3b. Inline any local <script src="..."> references to avoid file:// CORS.
	inlinedHTML, err := inlineLocalScripts(in.HTMLPath)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to inline scripts: %v", err), IsError: true}, nil
	}
	defer os.Remove(inlinedHTML)

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
		"--html", inlinedHTML,
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
		SessionDir:      sessionDir,
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

	// Populate rendered HTML and interactions paths for each screenshot
	renderedBaseDir := filepath.Join(sessionDir, "rendered")
	for i := range out.Screenshots {
		s := &out.Screenshots[i]
		if s.Name != "" && s.Name != "full-canvas" {
			s.RenderedHTMLPath = filepath.Join(renderedBaseDir, s.Name+".html")
			s.InteractionsPath = filepath.Join(renderedBaseDir, s.Name+".interactions.json")
		}
	}

	// Push each screenshot to ComandCenter chat if a pusher is configured.
	// Skip full-canvas when artboard shots exist — artboards are the deliverable
	// and full-canvas can be enormous (crashes vision API at >8000px).
	if t.pusher != nil {
		hasArtboards := false
		for _, s := range out.Screenshots {
			if s.Name != "full-canvas" {
				hasArtboards = true
				break
			}
		}
		for _, s := range out.Screenshots {
			if hasArtboards && s.Name == "full-canvas" {
				continue
			}
			_ = t.pusher.PushScreenshot(t.sessionID, s.Path, filepath.Base(s.Path))
		}
	}

	// Write/merge manifest.json to sessionDir
	if err := t.writeManifest(sessionDir, out.Screenshots); err != nil {
		// Manifest write failure is not fatal — don't block render result
		// but could log if needed in future
		_ = err
	}

	outJSON, _ := json.MarshalIndent(out, "", "  ")

	isError := !nodeOut.Success
	return &Result{Content: string(outJSON), IsError: isError}, nil
}

// writeManifest writes or merges manifest.json in sessionDir.
// Creates new manifest on first write, merges screens on subsequent calls.
// Preserves existing ScreenManifest data (breakpoint/viewport/description);
// only infers metadata for screens not already present in the manifest.
func (t *RenderMockupTool) writeManifest(sessionDir string, screenshots []ScreenshotInfo) error {
	manifestPath := filepath.Join(sessionDir, "manifest.json")

	// Collect unique new screen names from screenshots.
	var newNames []string
	seenNames := make(map[string]bool)
	for _, s := range screenshots {
		if s.Name == "" || s.Name == "full-canvas" {
			continue
		}
		if !seenNames[s.Name] {
			seenNames[s.Name] = true
			newNames = append(newNames, s.Name)
		}
	}

	projectPath, _ := os.Getwd()

	var manifest ManifestJSON
	if data, err := os.ReadFile(manifestPath); err == nil {
		// Manifest exists — unmarshal (handles both old []string and new []ScreenManifest).
		if err := json.Unmarshal(data, &manifest); err != nil {
			// Corrupt manifest — start fresh.
			manifest = ManifestJSON{
				ProjectPath: projectPath,
				SessionDir:  sessionDir,
				CreatedAt:   time.Now().UTC().Format(time.RFC3339),
			}
		}
		// Build index of existing screens by name to preserve their metadata.
		existing := make(map[string]bool, len(manifest.Screens))
		for _, sm := range manifest.Screens {
			existing[sm.Name] = true
		}
		// Append only new screens, inferring metadata for each.
		for _, name := range newNames {
			if !existing[name] {
				existing[name] = true
				manifest.Screens = append(manifest.Screens, inferScreenManifest(name))
			}
		}
	} else {
		// New manifest — infer metadata for all screens.
		screens := make([]ScreenManifest, 0, len(newNames))
		for _, name := range newNames {
			screens = append(screens, inferScreenManifest(name))
		}
		manifest = ManifestJSON{
			ProjectPath: projectPath,
			SessionDir:  sessionDir,
			CreatedAt:   time.Now().UTC().Format(time.RFC3339),
			Screens:     screens,
		}
	}

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath, manifestJSON, 0644)
}
