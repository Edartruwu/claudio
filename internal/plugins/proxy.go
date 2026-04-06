package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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
		Args  string `json:"args"`
		Input string `json:"input"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	var args []string
	if in.Args != "" {
		args = strings.Fields(in.Args)
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
