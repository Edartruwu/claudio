// Package plugins bridges the plugin registry with the tool system,
// exposing discovered plugins as invocable tools.
package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// PluginProxyTool wraps a plugin executable as a tools.Tool implementation.
type PluginProxyTool struct {
	PluginName string
	PluginPath string
	Desc       string
}

// Name returns the tool name (prefixed with "plugin_").
func (t *PluginProxyTool) Name() string {
	return "plugin_" + t.PluginName
}

// Description returns the plugin's description.
func (t *PluginProxyTool) Description() string {
	if t.Desc != "" {
		return t.Desc
	}
	return fmt.Sprintf("Plugin: %s (run with arguments via 'args' parameter)", t.PluginName)
}

// InputSchema returns the JSON schema for plugin input.
func (t *PluginProxyTool) InputSchema() json.RawMessage {
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

// IsReadOnly returns false — plugins may modify state.
func (t *PluginProxyTool) IsReadOnly() bool { return false }

// RequiresApproval returns true — plugins always need approval.
func (t *PluginProxyTool) RequiresApproval(_ json.RawMessage) bool { return true }

// Execute runs the plugin with the given arguments.
func (t *PluginProxyTool) Execute(ctx context.Context, input json.RawMessage) (*ToolResult, error) {
	var in struct {
		Args  string `json:"args"`
		Input string `json:"input"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return &ToolResult{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
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
		return &ToolResult{
			Content: fmt.Sprintf("Plugin error: %v\n%s", err, string(output)),
			IsError: true,
		}, nil
	}

	result := string(output)
	if result == "" {
		result = "(no output)"
	}

	return &ToolResult{Content: result}, nil
}

// ToolResult matches tools.Result for compatibility.
type ToolResult struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error,omitempty"`
}

// PluginInfo describes a discovered plugin for listing.
type PluginInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description"`
	IsScript    bool   `json:"is_script"`
}
