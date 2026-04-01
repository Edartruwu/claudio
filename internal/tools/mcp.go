package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// MCPClient manages a connection to an MCP server via stdio.
type MCPClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
	nextID int
	tools  []MCPToolDef
}

// MCPToolDef is a tool definition from an MCP server.
type MCPToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// MCPRequest is a JSON-RPC request to an MCP server.
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// MCPResponse is a JSON-RPC response from an MCP server.
type MCPResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *MCPError       `json:"error,omitempty"`
}

// MCPError is a JSON-RPC error.
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewMCPClient starts an MCP server process and connects to it via stdio.
func NewMCPClient(ctx context.Context, command string, args []string, env map[string]string) (*MCPClient, error) {
	cmd := exec.CommandContext(ctx, command, args...)

	// Set environment
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start MCP server: %w", err)
	}

	client := &MCPClient{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdoutPipe),
	}

	// Initialize the connection
	if err := client.initialize(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("MCP initialization failed: %w", err)
	}

	// Discover tools
	if err := client.discoverTools(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("MCP tool discovery failed: %w", err)
	}

	return client, nil
}

// Tools returns the available MCP tools.
func (c *MCPClient) Tools() []MCPToolDef {
	return c.tools
}

// CallTool invokes a tool on the MCP server.
func (c *MCPClient) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	params := map[string]interface{}{
		"name":      name,
		"arguments": json.RawMessage(args),
	}

	resp, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return "", err
	}

	// Parse tool result
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return string(resp), nil
	}

	var texts []string
	for _, c := range result.Content {
		if c.Type == "text" {
			texts = append(texts, c.Text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

// Close shuts down the MCP server.
func (c *MCPClient) Close() error {
	c.stdin.Close()
	return c.cmd.Process.Kill()
}

func (c *MCPClient) initialize(ctx context.Context) error {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":   map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "claudio",
			"version": "0.1.0",
		},
	}

	_, err := c.call(ctx, "initialize", params)
	if err != nil {
		return err
	}

	// Send initialized notification
	return c.notify("notifications/initialized", nil)
}

func (c *MCPClient) discoverTools(ctx context.Context) error {
	resp, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return err
	}

	var result struct {
		Tools []MCPToolDef `json:"tools"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse tools list: %w", err)
	}

	c.tools = result.Tools
	return nil
}

func (c *MCPClient) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()

	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	_, err = fmt.Fprintf(c.stdin, "%s\n", data)
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response (simplified — production would handle notifications)
	line, err := c.stdout.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var resp MCPResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

func (c *MCPClient) notify(method string, params interface{}) error {
	req := struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params,omitempty"`
	}{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	c.mu.Lock()
	_, err = fmt.Fprintf(c.stdin, "%s\n", data)
	c.mu.Unlock()
	return err
}

// MCPProxyTool wraps an MCP server tool as a Claudio tool.
type MCPProxyTool struct {
	client     *MCPClient
	toolDef    MCPToolDef
	serverName string
}

func (t *MCPProxyTool) Name() string {
	return fmt.Sprintf("mcp_%s_%s", t.serverName, t.toolDef.Name)
}

func (t *MCPProxyTool) Description() string {
	return fmt.Sprintf("[MCP:%s] %s", t.serverName, t.toolDef.Description)
}

func (t *MCPProxyTool) InputSchema() json.RawMessage {
	return t.toolDef.InputSchema
}

func (t *MCPProxyTool) IsReadOnly() bool { return false }

func (t *MCPProxyTool) RequiresApproval(_ json.RawMessage) bool { return true }

func (t *MCPProxyTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	output, err := t.client.CallTool(ctx, t.toolDef.Name, input)
	if err != nil {
		return &Result{Content: fmt.Sprintf("MCP tool error: %v", err), IsError: true}, nil
	}
	return &Result{Content: output}, nil
}
