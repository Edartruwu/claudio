package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// ServerState tracks a running MCP server.
type ServerState struct {
	Name       string
	Config     config.MCPServerConfig
	Client     *tools.MCPClient
	StartedAt  time.Time
	LastUsed   time.Time
	ToolCount  int
	Status     string // "running", "stopped", "error"
	Error      string
}

// Manager manages MCP server lifecycles.
type Manager struct {
	mu       sync.RWMutex
	servers  map[string]*ServerState
	configs  map[string]config.MCPServerConfig
	registry *tools.Registry
	bus      *bus.Bus
	idleTime time.Duration
}

// NewManager creates a new MCP server manager.
func NewManager(configs map[string]config.MCPServerConfig, registry *tools.Registry, eventBus *bus.Bus) *Manager {
	return &Manager{
		servers:  make(map[string]*ServerState),
		configs:  configs,
		registry: registry,
		bus:      eventBus,
		idleTime: 5 * time.Minute,
	}
}

// StartServer starts an MCP server by name (on-demand).
func (m *Manager) StartServer(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Already running?
	if s, ok := m.servers[name]; ok && s.Status == "running" {
		s.LastUsed = time.Now()
		return nil
	}

	cfg, ok := m.configs[name]
	if !ok {
		return fmt.Errorf("MCP server %q not configured", name)
	}

	client, err := tools.NewMCPClient(ctx, cfg.Command, cfg.Args, cfg.Env)
	if err != nil {
		m.servers[name] = &ServerState{
			Name:   name,
			Config: cfg,
			Status: "error",
			Error:  err.Error(),
		}
		return fmt.Errorf("failed to start MCP server %q: %w", name, err)
	}

	state := &ServerState{
		Name:      name,
		Config:    cfg,
		Client:    client,
		StartedAt: time.Now(),
		LastUsed:  time.Now(),
		ToolCount: len(client.Tools()),
		Status:    "running",
	}
	m.servers[name] = state

	// Register MCP tools into the main registry
	_ = client.Tools() // Tools are available via state.Client.Tools()

	m.bus.Publish(bus.Event{
		Type:    bus.EventMCPConnect,
		Payload: mustJSON(map[string]any{"server": name, "tools": len(client.Tools())}),
	})

	return nil
}

// StopServer stops a running MCP server.
func (m *Manager) StopServer(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.servers[name]
	if !ok || state.Status != "running" {
		return nil
	}

	if state.Client != nil {
		state.Client.Close()
	}
	state.Status = "stopped"

	m.bus.Publish(bus.Event{
		Type:    bus.EventMCPDisconnect,
		Payload: mustJSON(map[string]any{"server": name}),
	})

	return nil
}

// Status returns the status of all configured servers.
func (m *Manager) Status() []ServerState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []ServerState
	for name, cfg := range m.configs {
		if state, ok := m.servers[name]; ok {
			result = append(result, *state)
		} else {
			result = append(result, ServerState{
				Name:   name,
				Config: cfg,
				Status: "stopped",
			})
		}
	}
	return result
}

// StopIdle shuts down servers that haven't been used within the idle timeout.
func (m *Manager) StopIdle() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, state := range m.servers {
		if state.Status == "running" && time.Since(state.LastUsed) > m.idleTime {
			if state.Client != nil {
				state.Client.Close()
			}
			state.Status = "stopped"
			m.bus.Publish(bus.Event{
				Type:    bus.EventMCPDisconnect,
				Payload: mustJSON(map[string]any{"server": name, "reason": "idle"}),
			})
		}
	}
}

// StopAll shuts down all running servers.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, state := range m.servers {
		if state.Status == "running" && state.Client != nil {
			state.Client.Close()
			state.Status = "stopped"
		}
	}
}

// MCPToolNames returns the names of all tools discovered from running MCP servers.
func (m *Manager) MCPToolNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var names []string
	for _, state := range m.servers {
		if state.Status == "running" && state.Client != nil {
			for _, def := range state.Client.Tools() {
				names = append(names, def.Name)
			}
		}
	}
	return names
}

// FilterTools returns the subset of allTools that match any pattern in allowedTools.
// Patterns follow filepath.Match glob semantics (e.g. "caido-*").
// If allowedTools is empty, all tools are returned unfiltered.
func FilterTools(allowedTools []string, allTools []string) []string {
	if len(allowedTools) == 0 {
		return allTools
	}
	var result []string
	for _, tool := range allTools {
		for _, pattern := range allowedTools {
			if matched, err := filepath.Match(pattern, tool); err == nil && matched {
				result = append(result, tool)
				break
			}
		}
	}
	return result
}

func mustJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}
