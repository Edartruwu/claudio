package plugins

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Abraxas-365/claudio/internal/tools"
)

// PluginProxyTool wraps a plugin executable as a tools.Tool implementation.
type PluginProxyTool struct {
	PluginName string
	PluginPath string
	Desc       string
	schema     json.RawMessage
	deferrable
}

// NewProxyTool creates a PluginProxyTool from a discovered Plugin.
func NewProxyTool(p *Plugin) *PluginProxyTool {
	hint := "plugin " + p.Name
	if p.Description != "" {
		hint = p.Description
	}
	return &PluginProxyTool{
		PluginName: p.Name,
		PluginPath: p.Path,
		Desc:       p.Description,
		schema:     p.Schema,
		deferrable: deferrable{hint: hint},
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
		args = append(args, in.Command)
	}
	if in.Args != "" {
		args = append(args, strings.Fields(in.Args)...)
	}

	cmd := exec.CommandContext(ctx, t.PluginPath, args...)
	if in.Input != "" {
		cmd.Stdin = strings.NewReader(in.Input)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &tools.Result{
			Content: fmt.Sprintf("Plugin error: %v\n%s", err, string(output)),
			IsError: true,
		}, nil
	}

	result := string(output)

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
		if mt, ok := mediaTypes[ext]; ok {
			if data, err := os.ReadFile(trimmed); err == nil {
				b64 := base64.StdEncoding.EncodeToString(data)
				return &tools.Result{
					Content: result,
					Images:  []tools.ImageData{{MediaType: mt, Data: b64}},
				}, nil
			}
		}
	}

	if result == "" {
		result = "(no output)"
	}

	return &tools.Result{Content: result}, nil
}

// deferrable implements tools.DeferrableTool so plugin tools are deferred by default.
type deferrable struct {
	hint string
}

func (d deferrable) ShouldDefer() bool  { return true }
func (d deferrable) SearchHint() string { return d.hint }
