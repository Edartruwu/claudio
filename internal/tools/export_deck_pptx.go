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
	"strings"
	"time"
)

// ExportDeckPPTXTool wraps huashu-design's export_deck_pptx.mjs to export an
// HTML slide deck as an editable PowerPoint (.pptx) file with text frames
// preserved. Requires node on PATH.
type ExportDeckPPTXTool struct{}

type exportDeckPPTXInput struct {
	HTMLPath         string `json:"html_path"`
	OutputPath       string `json:"output_path"`
	SpeakerNotesPath string `json:"speaker_notes_path"`
}

type exportDeckPPTXOutput struct {
	OutputPath     string `json:"output_path"`
	SlideCount     int    `json:"slide_count"`
	TextFrameCount int    `json:"text_frame_count"`
	FileSizeBytes  int64  `json:"file_size_bytes"`
}

func (t *ExportDeckPPTXTool) Name() string { return "ExportDeckPPTX" }

func (t *ExportDeckPPTXTool) Description() string {
	return "Export an HTML slide deck as an editable PowerPoint (.pptx) file with text frames preserved."
}

func (t *ExportDeckPPTXTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"html_path": {
				"type": "string",
				"description": "Path to the HTML slide deck directory (containing numbered .html files) or a single .html slide file (its parent directory will be used)."
			},
			"output_path": {
				"type": "string",
				"description": "Where to write the .pptx file. Default: same directory as html_path with .pptx extension."
			},
			"speaker_notes_path": {
				"type": "string",
				"description": "Optional path to a markdown file with speaker notes per slide."
			}
		},
		"required": ["html_path"]
	}`)
}

func (t *ExportDeckPPTXTool) IsReadOnly() bool                        { return false }
func (t *ExportDeckPPTXTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *ExportDeckPPTXTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	// 1. Parse input.
	var in exportDeckPPTXInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}
	if in.HTMLPath == "" {
		return &Result{Content: "html_path is required", IsError: true}, nil
	}

	// Resolve slides directory: if html_path is a file, use its parent dir.
	slidesDir, err := filepath.Abs(in.HTMLPath)
	if err != nil {
		return &Result{Content: fmt.Sprintf("cannot resolve html_path: %v", err), IsError: true}, nil
	}
	info, err := os.Stat(slidesDir)
	if err != nil {
		return &Result{Content: fmt.Sprintf("html_path not found: %s", slidesDir), IsError: true}, nil
	}
	if !info.IsDir() {
		// Single file — use its directory.
		slidesDir = filepath.Dir(slidesDir)
	}

	// Resolve output path.
	outPath := in.OutputPath
	if outPath == "" {
		outPath = filepath.Join(slidesDir, filepath.Base(slidesDir)+".pptx")
	}
	outPath, _ = filepath.Abs(outPath)
	if !strings.HasSuffix(strings.ToLower(outPath), ".pptx") {
		outPath += ".pptx"
	}

	// 2. Check prerequisites.
	if err := checkNodeAvailable(); err != nil {
		return &Result{Content: err.Error(), IsError: true}, nil
	}

	// 3. Locate export_deck_pptx.mjs script.
	scriptPath := filepath.Join(huashuDesignDir(), "scripts", "export_deck_pptx.mjs")
	if _, err := os.Stat(scriptPath); err != nil {
		return &Result{Content: fmt.Sprintf(
			"export_deck_pptx.mjs not found at %s\nSet HUASHU_DESIGN_DIR env var to the huashu-design directory.", scriptPath), IsError: true}, nil
	}

	// 4. Build command.
	cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	args := []string{
		scriptPath,
		"--slides", slidesDir,
		"--out", outPath,
	}

	//nolint:gosec // scriptPath is resolved from known config path
	cmd := exec.CommandContext(cmdCtx, "node", args...)

	// Set NODE_PATH for global module resolution and working dir to script's
	// parent so relative requires (html2pptx.js) resolve correctly.
	cmd.Env = append(os.Environ(), "NODE_PATH="+nodeGlobalModulesDir())
	cmd.Dir = filepath.Dir(scriptPath)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	if runErr != nil {
		errMsg := fmt.Sprintf("export_deck_pptx.mjs failed.\nstdout: %s\nstderr: %s\nexit: %v",
			stdout.String(), stderr.String(), runErr)
		return &Result{Content: errMsg, IsError: true}, nil
	}

	// 5. Parse counts from stdout.
	combined := stdout.String() + stderr.String()
	slideCount := parseSlideCount(combined)
	textFrameCount := 0 // script doesn't report this directly

	// 6. Stat output file.
	outInfo, err := os.Stat(outPath)
	if err != nil {
		return &Result{Content: fmt.Sprintf("output file missing after export: %v\nstdout: %s\nstderr: %s",
			err, stdout.String(), stderr.String()), IsError: true}, nil
	}

	out := exportDeckPPTXOutput{
		OutputPath:     outPath,
		SlideCount:     slideCount,
		TextFrameCount: textFrameCount,
		FileSizeBytes:  outInfo.Size(),
	}

	resultJSON, _ := json.MarshalIndent(out, "", "  ")

	var sb strings.Builder
	sb.WriteString(string(resultJSON))
	if s := strings.TrimSpace(stdout.String()); s != "" {
		sb.WriteString("\n\n--- export log ---\n")
		sb.WriteString(s)
	}
	if s := strings.TrimSpace(stderr.String()); s != "" {
		sb.WriteString("\n\n--- export warnings ---\n")
		sb.WriteString(s)
	}

	return &Result{Content: sb.String()}, nil
}

// slideCountRe matches "Converting N slides" or "N/M slides" patterns from script output.
var slideCountRe = regexp.MustCompile(`(?i)converting\s+(\d+)\s+slides`)

func parseSlideCount(output string) int {
	m := slideCountRe.FindStringSubmatch(output)
	if len(m) >= 2 {
		n := 0
		fmt.Sscanf(m[1], "%d", &n)
		return n
	}
	return 0
}
