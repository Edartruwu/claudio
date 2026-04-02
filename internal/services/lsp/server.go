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
)

// ServerConfig defines how to start an LSP server.
type ServerConfig struct {
	Language string   // e.g., "go", "typescript", "python"
	Command  string   // e.g., "gopls", "typescript-language-server"
	Args     []string // additional args
	RootDir  string   // workspace root
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
	mu       sync.RWMutex
	servers  map[string]*ServerInstance // keyed by language
	configs  map[string]ServerConfig    // default configs per language
	idleTimeout time.Duration
}

// DefaultConfigs returns the default LSP server configurations.
func DefaultConfigs() map[string]ServerConfig {
	return map[string]ServerConfig{
		"go": {
			Language: "go",
			Command:  "gopls",
			Args:     []string{"serve"},
		},
		"typescript": {
			Language: "typescript",
			Command:  "typescript-language-server",
			Args:     []string{"--stdio"},
		},
		"python": {
			Language: "python",
			Command:  "pyright-langserver",
			Args:     []string{"--stdio"},
		},
		"rust": {
			Language: "rust",
			Command:  "rust-analyzer",
		},
		"java": {
			Language: "java",
			Command:  "jdtls",
		},
	}
}

// NewServerManager creates a new LSP server manager.
func NewServerManager() *ServerManager {
	return &ServerManager{
		servers:     make(map[string]*ServerInstance),
		configs:     DefaultConfigs(),
		idleTimeout: 5 * time.Minute,
	}
}

// DetectLanguage returns the language for a file path based on extension.
func DetectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx", ".js", ".jsx":
		return "typescript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".c", ".cpp", ".cc", ".h", ".hpp":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	default:
		return ""
	}
}

// IsAvailable checks if an LSP server command is available on PATH.
func IsAvailable(language string) bool {
	configs := DefaultConfigs()
	cfg, ok := configs[language]
	if !ok {
		return false
	}
	_, err := exec.LookPath(cfg.Command)
	return err == nil
}

// StartServer starts an LSP server for the given language.
func (m *ServerManager) StartServer(ctx context.Context, language, rootDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Already running?
	if srv, ok := m.servers[language]; ok && srv.Process != nil && srv.Process.Process != nil {
		srv.LastUsed = time.Now()
		return nil
	}

	cfg, ok := m.configs[language]
	if !ok {
		return fmt.Errorf("no LSP config for language: %s", language)
	}
	cfg.RootDir = rootDir

	// Check if command exists
	if _, err := exec.LookPath(cfg.Command); err != nil {
		return fmt.Errorf("%s not found: %w", cfg.Command, err)
	}

	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	cmd.Dir = rootDir
	cmd.Env = append(os.Environ(), "GOPATH="+os.Getenv("GOPATH"))

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
	m.servers[language] = srv
	return nil
}

// StopServer stops the LSP server for a language.
func (m *ServerManager) StopServer(language string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	srv, ok := m.servers[language]
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

	delete(m.servers, language)
	return nil
}

// StopAll stops all running LSP servers.
func (m *ServerManager) StopAll() {
	m.mu.RLock()
	languages := make([]string, 0, len(m.servers))
	for lang := range m.servers {
		languages = append(languages, lang)
	}
	m.mu.RUnlock()

	for _, lang := range languages {
		m.StopServer(lang)
	}
}

// GetServer returns the running server for a language, starting it if needed.
func (m *ServerManager) GetServer(ctx context.Context, filePath string) (*ServerInstance, error) {
	lang := DetectLanguage(filePath)
	if lang == "" {
		return nil, fmt.Errorf("unknown language for %s", filePath)
	}

	m.mu.RLock()
	srv, ok := m.servers[lang]
	m.mu.RUnlock()

	if ok && srv.Ready {
		srv.LastUsed = time.Now()
		return srv, nil
	}

	// Find root directory
	rootDir := findProjectRoot(filepath.Dir(filePath))
	if err := m.StartServer(ctx, lang, rootDir); err != nil {
		return nil, err
	}

	m.mu.RLock()
	srv = m.servers[lang]
	m.mu.RUnlock()
	return srv, nil
}

// CleanIdle stops servers that haven't been used recently.
func (m *ServerManager) CleanIdle() {
	m.mu.RLock()
	var idle []string
	for lang, srv := range m.servers {
		if time.Since(srv.LastUsed) > m.idleTimeout {
			idle = append(idle, lang)
		}
	}
	m.mu.RUnlock()

	for _, lang := range idle {
		m.StopServer(lang)
	}
}

// Status returns the status of all servers.
func (m *ServerManager) Status() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]string)
	for lang, srv := range m.servers {
		if srv.Ready {
			status[lang] = fmt.Sprintf("running (since %s, last used %s ago)",
				srv.StartedAt.Format("15:04:05"),
				time.Since(srv.LastUsed).Round(time.Second))
		} else {
			status[lang] = "starting"
		}
	}
	return status
}

// --- LSP Protocol Implementation ---

type lspMessage struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      *int        `json:"id,omitempty"`
	Method  string      `json:"method,omitempty"`
	Params  interface{} `json:"params,omitempty"`
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
				"definition":    map[string]interface{}{"dynamicRegistration": false},
				"references":    map[string]interface{}{"dynamicRegistration": false},
				"hover":         map[string]interface{}{"dynamicRegistration": false},
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

	// Read response (simplified — production would need proper message framing)
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
