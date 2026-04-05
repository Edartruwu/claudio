// Package lsp provides Language Server Protocol server lifecycle management.
// It manages starting, stopping, and communicating with LSP servers for
// code intelligence features like go-to-definition, find-references, hover.
package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/config"
)

// ServerConfig defines how to start an LSP server (extension-based routing).
type ServerConfig struct {
	Name       string            // server name (e.g., "gopls")
	Command    string            // executable
	Args       []string          // additional args
	Extensions []string          // file extensions this server handles
	Env        map[string]string // extra environment variables
	RootDir    string            // workspace root (set at start time)
}

// FromLspServerConfig converts a config.LspServerConfig to a ServerConfig.
func FromLspServerConfig(name string, cfg config.LspServerConfig) ServerConfig {
	return ServerConfig{
		Name:       name,
		Command:    cfg.Command,
		Args:       cfg.Args,
		Extensions: cfg.Extensions,
		Env:        cfg.Env,
	}
}

// ServerInstance represents a running LSP server process.
type ServerInstance struct {
	Config    ServerConfig
	Process   *exec.Cmd
	Stdin     io.WriteCloser
	Stdout    *bufio.Reader
	Ready     bool
	StartedAt time.Time
	LastUsed  time.Time
	mu        sync.Mutex
	nextID    int
}

// ServerManager manages LSP server lifecycles.
type ServerManager struct {
	mu          sync.RWMutex
	servers     map[string]*ServerInstance    // keyed by server name
	configs     map[string]ServerConfig       // keyed by server name
	extMap      map[string]string             // extension -> server name
	idleTimeout time.Duration
}

// NewServerManager creates a new LSP server manager with the given configs.
// No servers are started until requested. Pass nil for no servers.
func NewServerManager(cfgs map[string]config.LspServerConfig) *ServerManager {
	m := &ServerManager{
		servers:     make(map[string]*ServerInstance),
		configs:     make(map[string]ServerConfig),
		extMap:      make(map[string]string),
		idleTimeout: 5 * time.Minute,
	}

	for name, cfg := range cfgs {
		sc := FromLspServerConfig(name, cfg)
		m.configs[name] = sc
		for _, ext := range sc.Extensions {
			ext = strings.ToLower(ext)
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			m.extMap[ext] = name
		}
	}

	return m
}

// HasServers returns true if at least one LSP server is configured.
func (m *ServerManager) HasServers() bool {
	return len(m.configs) > 0
}

// HasConnected returns true if at least one LSP server is currently running.
func (m *ServerManager) HasConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.servers) > 0
}

// ServerForFile returns the server name that handles the given file extension,
// or empty string if none configured.
func (m *ServerManager) ServerForFile(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	return m.extMap[ext]
}

// StartServer starts the named LSP server.
func (m *ServerManager) StartServer(ctx context.Context, name, rootDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Already running?
	if srv, ok := m.servers[name]; ok && srv.Process != nil && srv.Process.Process != nil {
		srv.LastUsed = time.Now()
		return nil
	}

	cfg, ok := m.configs[name]
	if !ok {
		return fmt.Errorf("no LSP config for server: %s", name)
	}
	cfg.RootDir = rootDir

	// Check if command exists
	if _, err := exec.LookPath(cfg.Command); err != nil {
		return fmt.Errorf("%s not found on PATH: %w", cfg.Command, err)
	}

	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	cmd.Dir = rootDir

	// Build environment
	env := os.Environ()
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", cfg.Command, err)
	}

	srv := &ServerInstance{
		Config:    cfg,
		Process:   cmd,
		Stdin:     stdin,
		Stdout:    bufio.NewReader(stdout),
		StartedAt: time.Now(),
		LastUsed:  time.Now(),
	}

	// Send initialize request
	if err := srv.initialize(rootDir); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("initialize: %w", err)
	}

	srv.Ready = true
	m.servers[name] = srv
	return nil
}

// StopServer stops the named LSP server.
func (m *ServerManager) StopServer(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	srv, ok := m.servers[name]
	if !ok {
		return nil
	}

	// Send shutdown request
	srv.sendRequest("shutdown", nil)
	srv.sendNotification("exit", nil)

	// Give it a moment to exit gracefully
	done := make(chan error, 1)
	go func() { done <- srv.Process.Wait() }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		srv.Process.Process.Kill()
	}

	delete(m.servers, name)
	return nil
}

// StopAll stops all running LSP servers.
func (m *ServerManager) StopAll() {
	m.mu.RLock()
	names := make([]string, 0, len(m.servers))
	for name := range m.servers {
		names = append(names, name)
	}
	m.mu.RUnlock()

	for _, name := range names {
		m.StopServer(name)
	}
}

// GetServer returns the running server for a file, starting it if needed.
func (m *ServerManager) GetServer(ctx context.Context, filePath string) (*ServerInstance, error) {
	name := m.ServerForFile(filePath)
	if name == "" {
		return nil, fmt.Errorf("no LSP server configured for %s", filepath.Ext(filePath))
	}

	m.mu.RLock()
	srv, ok := m.servers[name]
	m.mu.RUnlock()

	if ok && srv.Ready {
		srv.LastUsed = time.Now()
		return srv, nil
	}

	// Find root directory
	rootDir := findProjectRoot(filepath.Dir(filePath))
	if err := m.StartServer(ctx, name, rootDir); err != nil {
		return nil, err
	}

	m.mu.RLock()
	srv = m.servers[name]
	m.mu.RUnlock()
	return srv, nil
}

// CleanIdle stops servers that haven't been used recently.
func (m *ServerManager) CleanIdle() {
	m.mu.RLock()
	var idle []string
	for name, srv := range m.servers {
		if time.Since(srv.LastUsed) > m.idleTimeout {
			idle = append(idle, name)
		}
	}
	m.mu.RUnlock()

	for _, name := range idle {
		m.StopServer(name)
	}
}

// Status returns the status of all configured servers.
func (m *ServerManager) Status() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]string)
	for name := range m.configs {
		srv, running := m.servers[name]
		if running && srv.Ready {
			status[name] = fmt.Sprintf("running (since %s, last used %s ago)",
				srv.StartedAt.Format("15:04:05"),
				time.Since(srv.LastUsed).Round(time.Second))
		} else {
			status[name] = "configured (not running)"
		}
	}
	return status
}

// --- LSP Protocol Implementation ---

type lspMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  interface{}     `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

func (s *ServerInstance) initialize(rootDir string) error {
	absRoot, _ := filepath.Abs(rootDir)
	uri := "file://" + absRoot

	params := map[string]interface{}{
		"processId": os.Getpid(),
		"rootUri":   uri,
		"capabilities": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"definition":     map[string]interface{}{"dynamicRegistration": false},
				"references":     map[string]interface{}{"dynamicRegistration": false},
				"hover":          map[string]interface{}{"dynamicRegistration": false},
				"documentSymbol": map[string]interface{}{"dynamicRegistration": false},
			},
		},
	}

	_, err := s.sendRequest("initialize", params)
	if err != nil {
		return err
	}

	// Send initialized notification
	return s.sendNotification("initialized", map[string]interface{}{})
}

func (s *ServerInstance) sendRequest(method string, params interface{}) (json.RawMessage, error) {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.mu.Unlock()

	msg := lspMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := s.Stdin.Write([]byte(header)); err != nil {
		return nil, err
	}
	if _, err := s.Stdin.Write(data); err != nil {
		return nil, err
	}

	return s.readResponse()
}

func (s *ServerInstance) sendNotification(method string, params interface{}) error {
	msg := lspMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := s.Stdin.Write([]byte(header)); err != nil {
		return err
	}
	_, err = s.Stdin.Write(data)
	return err
}

func (s *ServerInstance) readResponse() (json.RawMessage, error) {
	// Read Content-Length header
	var contentLength int
	for {
		line, err := s.Stdout.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			fmt.Sscanf(line, "Content-Length: %d", &contentLength)
		}
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("no Content-Length in response")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(s.Stdout, body); err != nil {
		return nil, err
	}

	var resp lspMessage
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	return resp.Result, nil
}

// GoToDefinition finds the definition of a symbol.
func (s *ServerInstance) GoToDefinition(filePath string, line, character int) (json.RawMessage, error) {
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": "file://" + filePath},
		"position":     map[string]interface{}{"line": line, "character": character},
	}
	return s.sendRequest("textDocument/definition", params)
}

// FindReferences finds all references to a symbol.
func (s *ServerInstance) FindReferences(filePath string, line, character int) (json.RawMessage, error) {
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": "file://" + filePath},
		"position":     map[string]interface{}{"line": line, "character": character},
		"context":      map[string]interface{}{"includeDeclaration": true},
	}
	return s.sendRequest("textDocument/references", params)
}

// Hover returns hover information for a symbol.
func (s *ServerInstance) Hover(filePath string, line, character int) (json.RawMessage, error) {
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": "file://" + filePath},
		"position":     map[string]interface{}{"line": line, "character": character},
	}
	return s.sendRequest("textDocument/hover", params)
}

// DocumentSymbols returns all symbols in a document.
func (s *ServerInstance) DocumentSymbols(filePath string) (json.RawMessage, error) {
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": "file://" + filePath},
	}
	return s.sendRequest("textDocument/documentSymbol", params)
}

func findProjectRoot(dir string) string {
	markers := []string{".git", "go.mod", "package.json", "Cargo.toml", "pyproject.toml"}
	for {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}
