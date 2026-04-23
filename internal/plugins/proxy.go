package plugins

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tools/outputfilter"
)

// PluginProxyTool wraps a plugin executable as a tools.Tool implementation.
type PluginProxyTool struct {
	PluginName          string
	PluginPath          string
	Desc                string
	Instructions        string
	schema              json.RawMessage
	OutputFilterEnabled bool
	FilterRecorder      func(cmd string, bytesIn, bytesOut int)
	deferrable
}

// NewProxyTool creates a PluginProxyTool from a discovered Plugin.
func NewProxyTool(p *Plugin) *PluginProxyTool {
	hint := "plugin " + p.Name
	if p.Description != "" {
		hint = p.Description
	}
	return &PluginProxyTool{
		PluginName:   p.Name,
		PluginPath:   p.Path,
		Desc:         p.Description,
		Instructions: p.Instructions,
		schema:       p.Schema,
		deferrable:   deferrable{hint: hint},
	}
}

func (t *PluginProxyTool) Name() string {
	return "plugin_" + t.PluginName
}

func (t *PluginProxyTool) Description() string {
	if t.Desc != "" {
		return t.Desc
	}
	return fmt.Sprintf("Plugin: %s (run with arguments via 'args' parameter)", t.PluginName)
}

func (t *PluginProxyTool) InputSchema() json.RawMessage {
	if t.schema != nil {
		return t.schema
	}
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"args": {
				"type": "string",
				"description": "Arguments to pass to the plugin"
			},
			"input": {
				"type": "string",
				"description": "Input data to pass via stdin"
			}
		}
	}`)
}

func (t *PluginProxyTool) IsReadOnly() bool { return false }

func (t *PluginProxyTool) RequiresApproval(_ json.RawMessage) bool { return true }

// PluginInstructions returns the plugin's markdown instructions for lazy delivery via ToolSearch.
func (t *PluginProxyTool) PluginInstructions() string {
	return t.Instructions
}

// resizeImage uses nearest-neighbor scaling to resize an image to a maximum width.
func resizeImage(src image.Image, maxWidth int) image.Image {
	b := src.Bounds()
	w, h := b.Max.X, b.Max.Y
	if w <= maxWidth {
		return src
	}
	nw := maxWidth
	nh := h * maxWidth / w
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			dst.Set(x, y, src.At(x*w/nw, y*h/nh))
		}
	}
	return dst
}

// compressImage reads an image file, decodes it, resizes if needed, and re-encodes as JPEG.
func compressImage(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		// unsupported format — return raw bytes as fallback
		return raw, nil
	}
	img = resizeImage(img, 1024)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 72}); err != nil {
		return raw, nil
	}
	return buf.Bytes(), nil
}

func (t *PluginProxyTool) Execute(ctx context.Context, input json.RawMessage) (*tools.Result, error) {
	var in struct {
		Command string `json:"command"`
		Args    string `json:"args"`
		Input   string `json:"input"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	var args []string
	if in.Command != "" {
		args = append(args, shellSplit(in.Command)...)
	}
	if in.Args != "" {
		args = append(args, shellSplit(in.Args)...)
	}

	cmd := exec.CommandContext(ctx, t.PluginPath, args...)
	// Always forward raw input JSON as stdin so plugins can parse their schema fields.
	cmd.Stdin = bytes.NewReader(input)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &tools.Result{
			Content: fmt.Sprintf("Plugin error: %v\n%s", err, string(output)),
			IsError: true,
		}, nil
	}

	result := string(output)

	// Apply output filter to reduce token usage when enabled.
	if t.OutputFilterEnabled && result != "" {
		result = outputfilter.FilterAndRecord(in.Command, result, t.FilterRecorder)
	}

	// Detect if the output is a path to an image file.
	trimmed := strings.TrimSpace(result)
	if !strings.Contains(trimmed, "\n") {
		ext := strings.ToLower(filepath.Ext(trimmed))
		mediaTypes := map[string]string{
			".jpg":  "image/jpeg",
			".jpeg": "image/jpeg",
			".png":  "image/png",
			".gif":  "image/gif",
			".webp": "image/webp",
		}
		if _, ok := mediaTypes[ext]; ok {
			if compressed, err := compressImage(trimmed); err == nil {
				b64 := base64.StdEncoding.EncodeToString(compressed)
				return &tools.Result{
					Content: result,
					Images:  []tools.ImageData{{MediaType: "image/jpeg", Data: b64}},
				}, nil
			}
		}
	}

	if result == "" {
		result = "(no output)"
	}

	return &tools.Result{Content: result}, nil
}

// shellSplit splits s into tokens the same way a POSIX shell would, respecting
// single-quoted and double-quoted strings so that multi-word flag values like
// --subject "Hello World" are kept as a single token.
func shellSplit(s string) []string {
	var tokens []string
	var cur strings.Builder
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			} else {
				cur.WriteByte(c)
			}
		case inDouble:
			if c == '"' {
				inDouble = false
			} else if c == '\\' && i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
			} else {
				cur.WriteByte(c)
			}
		case c == '\'':
			inSingle = true
		case c == '"':
			inDouble = true
		case c == ' ' || c == '\t' || c == '\n':
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// deferrable implements tools.DeferrableTool so plugin tools are deferred by default.
type deferrable struct {
	hint string
}

func (d deferrable) ShouldDefer() bool  { return true }
func (d deferrable) SearchHint() string { return d.hint }
