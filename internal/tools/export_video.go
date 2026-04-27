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
	"strings"
	"time"
)

// ExportVideoTool wraps huashu-design's render-video.js to export an HTML
// animation as MP4 or GIF. Requires node and ffmpeg on PATH.
type ExportVideoTool struct{}

type exportVideoInput struct {
	HTMLPath   string `json:"html_path"`
	Format     string `json:"format"`      // "mp4" or "gif"; default "mp4"
	FPS        int    `json:"fps"`         // output framerate; default 60
	DurationMS int    `json:"duration_ms"` // animation duration in ms; default 3000
	OutputPath string `json:"output_path"` // optional; default same dir as html_path
}

type exportVideoOutput struct {
	OutputPath    string `json:"output_path"`
	Format        string `json:"format"`
	FileSizeBytes int64  `json:"file_size_bytes"`
	DurationMS    int    `json:"duration_ms"`
}

func (t *ExportVideoTool) Name() string { return "ExportVideo" }

func (t *ExportVideoTool) Description() string {
	return "Export an HTML animation as MP4 or GIF video. Requires node and ffmpeg."
}

func (t *ExportVideoTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"html_path": {
				"type": "string",
				"description": "Path to the HTML file to render as video."
			},
			"format": {
				"type": "string",
				"enum": ["mp4", "gif"],
				"description": "Output format. Default: mp4."
			},
			"fps": {
				"type": "integer",
				"description": "Output framerate. Default: 60."
			},
			"duration_ms": {
				"type": "integer",
				"description": "Animation duration in milliseconds. Default: 3000."
			},
			"output_path": {
				"type": "string",
				"description": "Where to write the output file. Default: same directory as html_path."
			}
		},
		"required": ["html_path"]
	}`)
}

func (t *ExportVideoTool) IsReadOnly() bool                        { return false }
func (t *ExportVideoTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *ExportVideoTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	// 1. Parse input.
	var in exportVideoInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}
	if in.HTMLPath == "" {
		return &Result{Content: "html_path is required", IsError: true}, nil
	}
	if in.Format == "" {
		in.Format = "mp4"
	}
	in.Format = strings.ToLower(in.Format)
	if in.Format != "mp4" && in.Format != "gif" {
		return &Result{Content: fmt.Sprintf("format must be mp4 or gif, got: %q", in.Format), IsError: true}, nil
	}
	if in.FPS <= 0 {
		in.FPS = 60
	}
	if in.DurationMS <= 0 {
		in.DurationMS = 3000
	}

	// Resolve absolute HTML path.
	htmlAbs, err := filepath.Abs(in.HTMLPath)
	if err != nil {
		return &Result{Content: fmt.Sprintf("cannot resolve html_path: %v", err), IsError: true}, nil
	}
	if _, err := os.Stat(htmlAbs); err != nil {
		return &Result{Content: fmt.Sprintf("html_path not found: %s", htmlAbs), IsError: true}, nil
	}

	// 2. Check prerequisites.
	if err := checkNodeAvailable(); err != nil {
		return &Result{Content: err.Error(), IsError: true}, nil
	}
	if err := checkFfmpegAvailable(); err != nil {
		return &Result{Content: err.Error(), IsError: true}, nil
	}

	// 3. Locate render-video.js script.
	scriptPath := filepath.Join(huashuDesignDir(), "scripts", "render-video.js")
	if _, err := os.Stat(scriptPath); err != nil {
		return &Result{Content: fmt.Sprintf(
			"render-video.js not found at %s\nSet HUASHU_DESIGN_DIR env var to the huashu-design directory.", scriptPath), IsError: true}, nil
	}

	// 4. Build command. Script takes duration in seconds.
	durationSec := float64(in.DurationMS) / 1000.0

	cmdCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	args := []string{
		scriptPath,
		htmlAbs,
		"--duration=" + strconv.FormatFloat(durationSec, 'f', -1, 64),
	}

	//nolint:gosec // scriptPath is resolved from known config path
	cmd := exec.CommandContext(cmdCtx, "node", args...)

	// Set NODE_PATH so global playwright is found.
	cmd.Env = append(os.Environ(), "NODE_PATH="+nodeGlobalModulesDir())

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	// Script outputs MP4 next to HTML file: same basename, .mp4 extension.
	basename := strings.TrimSuffix(filepath.Base(htmlAbs), filepath.Ext(htmlAbs))
	mp4Path := filepath.Join(filepath.Dir(htmlAbs), basename+".mp4")

	if runErr != nil {
		errMsg := fmt.Sprintf("render-video.js failed.\nstdout: %s\nstderr: %s\nexit: %v",
			stdout.String(), stderr.String(), runErr)
		return &Result{Content: errMsg, IsError: true}, nil
	}

	// 5. Determine final output path.
	finalPath := mp4Path
	if in.OutputPath != "" {
		finalPath, _ = filepath.Abs(in.OutputPath)
	}

	// 6. If GIF requested, convert MP4 → GIF via ffmpeg.
	if in.Format == "gif" {
		gifPath := finalPath
		if filepath.Ext(gifPath) != ".gif" {
			gifPath = strings.TrimSuffix(gifPath, filepath.Ext(gifPath)) + ".gif"
		}

		gifCtx, gifCancel := context.WithTimeout(ctx, 60*time.Second)
		defer gifCancel()

		// Two-pass: palette generation then encode for quality GIF.
		palettePath := mp4Path + ".palette.png"
		defer os.Remove(palettePath)

		//nolint:gosec // ffmpeg args are controlled
		paletteCmd := exec.CommandContext(gifCtx, "ffmpeg", "-y",
			"-i", mp4Path,
			"-vf", fmt.Sprintf("fps=%d,scale=-1:-1:flags=lanczos,palettegen", in.FPS),
			palettePath,
		)
		if out, err := paletteCmd.CombinedOutput(); err != nil {
			return &Result{Content: fmt.Sprintf("ffmpeg palette generation failed: %v\n%s", err, string(out)), IsError: true}, nil
		}

		//nolint:gosec // ffmpeg args are controlled
		gifCmd := exec.CommandContext(gifCtx, "ffmpeg", "-y",
			"-i", mp4Path,
			"-i", palettePath,
			"-lavfi", fmt.Sprintf("fps=%d,scale=-1:-1:flags=lanczos[x];[x][1:v]paletteuse", in.FPS),
			gifPath,
		)
		if out, err := gifCmd.CombinedOutput(); err != nil {
			return &Result{Content: fmt.Sprintf("ffmpeg GIF conversion failed: %v\n%s", err, string(out)), IsError: true}, nil
		}

		// Clean up intermediate MP4 if output is GIF and paths differ.
		if gifPath != mp4Path {
			os.Remove(mp4Path)
		}
		finalPath = gifPath
	} else if finalPath != mp4Path {
		// Move MP4 to requested output_path.
		if err := os.Rename(mp4Path, finalPath); err != nil {
			// Rename fails across filesystems — fallback to copy.
			data, readErr := os.ReadFile(mp4Path)
			if readErr != nil {
				return &Result{Content: fmt.Sprintf("cannot read MP4 output: %v", readErr), IsError: true}, nil
			}
			if writeErr := os.WriteFile(finalPath, data, 0644); writeErr != nil {
				return &Result{Content: fmt.Sprintf("cannot write to output_path: %v", writeErr), IsError: true}, nil
			}
			os.Remove(mp4Path)
		}
	}

	// 7. Stat final file.
	info, err := os.Stat(finalPath)
	if err != nil {
		return &Result{Content: fmt.Sprintf("output file missing after render: %v", err), IsError: true}, nil
	}

	out := exportVideoOutput{
		OutputPath:    finalPath,
		Format:        in.Format,
		FileSizeBytes: info.Size(),
		DurationMS:    in.DurationMS,
	}

	resultJSON, _ := json.MarshalIndent(out, "", "  ")

	// Include script stdout/stderr as context.
	var sb strings.Builder
	sb.WriteString(string(resultJSON))
	if s := strings.TrimSpace(stdout.String()); s != "" {
		sb.WriteString("\n\n--- render log ---\n")
		sb.WriteString(s)
	}
	if s := strings.TrimSpace(stderr.String()); s != "" {
		sb.WriteString("\n\n--- render warnings ---\n")
		sb.WriteString(s)
	}

	return &Result{Content: sb.String()}, nil
}
