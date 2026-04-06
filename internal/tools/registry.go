package tools

import (
	"encoding/json"
	"fmt"

	"github.com/Abraxas-365/claudio/internal/services/lsp"
	"github.com/Abraxas-365/claudio/internal/snippets"
	"github.com/Abraxas-365/claudio/internal/tools/grepcache"
	"github.com/Abraxas-365/claudio/internal/tools/readcache"
)

// Registry holds all registered tools.
type Registry struct {
	tools map[string]Tool
	order []string // preserve insertion order
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
	r.order = append(r.order, t.Name())
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return t, nil
}

// All returns all registered tools in order.
func (r *Registry) All() []Tool {
	result := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.tools[name])
	}
	return result
}

// APIDefinitions returns tool definitions with full schemas (no deferral).
func (r *Registry) APIDefinitions() json.RawMessage {
	defs := make([]APIToolDef, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		defs = append(defs, APIToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		})
	}
	data, _ := json.Marshal(defs)
	return data
}

// APIDefinitionsWithDeferral returns tool definitions with deferred loading support.
// Tools that implement DeferrableTool and return ShouldDefer()=true are sent with
// only their name (defer_loading: true), unless they appear in discoveredTools.
func (r *Registry) APIDefinitionsWithDeferral(discoveredTools map[string]bool) json.RawMessage {
	defs := make([]APIToolDef, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		shouldDefer := false
		if dt, ok := t.(DeferrableTool); ok && dt.ShouldDefer() {
			// Defer unless the model already discovered this tool via ToolSearch
			if discoveredTools == nil || !discoveredTools[name] {
				shouldDefer = true
			}
			// Auto-activate if the tool's backing service is available
			if shouldDefer {
				if aa, ok := t.(AutoActivatable); ok && aa.AutoActivate() {
					shouldDefer = false
				}
			}
		}

		if shouldDefer {
			// Completely omit undiscovered deferred tools from the tools array.
			// Their names are already listed in the system reminder so the model
			// knows they exist and can call ToolSearch to load them on demand.
			continue
		} else {
			defs = append(defs, APIToolDef{
				Name:        name,
				Description: t.Description(),
				InputSchema: t.InputSchema(),
			})
		}
	}
	data, _ := json.Marshal(defs)
	return data
}

// DeferredToolNames returns the names of tools that support deferred loading
// and are not auto-activated (i.e. their backing service is unavailable).
func (r *Registry) DeferredToolNames() []string {
	var names []string
	for _, name := range r.order {
		t := r.tools[name]
		dt, ok := t.(DeferrableTool)
		if !ok || !dt.ShouldDefer() {
			continue
		}
		// Skip tools that auto-activate — they are sent with full schema already.
		if aa, ok := t.(AutoActivatable); ok && aa.AutoActivate() {
			continue
		}
		names = append(names, name)
	}
	return names
}

// ToolSearchHints returns a map of tool name → search hint for all deferrable tools.
func (r *Registry) ToolSearchHints() map[string]string {
	hints := make(map[string]string)
	for _, name := range r.order {
		if dt, ok := r.tools[name].(DeferrableTool); ok && dt.ShouldDefer() {
			hints[name] = dt.SearchHint()
		}
	}
	return hints
}

// ReadCache returns the shared ReadCache used by the FileReadTool, or nil if not present.
func (r *Registry) ReadCache() *readcache.Cache {
	if t, ok := r.tools["Read"]; ok {
		if ft, ok := t.(*FileReadTool); ok {
			return ft.ReadCache
		}
	}
	return nil
}

// Remove removes a tool from the registry by name.
func (r *Registry) Remove(name string) {
	delete(r.tools, name)
	for i, n := range r.order {
		if n == name {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

// Clone creates a copy of the registry (for sub-agent filtered registries).
// File read/write tools get a fresh ReadCache so the sub-agent's reads don't
// pollute the parent cache — otherwise the parent sees "File unchanged since
// last read" for files it never read itself (the sub-agent did).
func (r *Registry) Clone() *Registry {
	clone := NewRegistry()
	for _, name := range r.order {
		clone.Register(r.tools[name])
	}

	// Give the sub-agent its own caches so it doesn't share state with the
	// parent session — otherwise the parent sees spurious cache hits for files
	// or searches it never performed itself (the sub-agent did).
	freshRC := readcache.New(256)
	freshGC := grepcache.New(512)
	if t, err := clone.Get("Read"); err == nil {
		if ft, ok := t.(*FileReadTool); ok {
			cloned := *ft
			cloned.ReadCache = freshRC
			clone.tools["Read"] = &cloned
		}
	}
	if t, err := clone.Get("Write"); err == nil {
		if ft, ok := t.(*FileWriteTool); ok {
			cloned := *ft
			cloned.ReadCache = freshRC
			clone.tools["Write"] = &cloned
		}
	}
	if t, err := clone.Get("Grep"); err == nil {
		if gt, ok := t.(*GrepTool); ok {
			cloned := *gt
			cloned.Cache = freshGC
			clone.tools["Grep"] = &cloned
		}
	}

	return clone
}

// SetLSPManager injects the LSP server manager into the LSP tool.
func (r *Registry) SetLSPManager(m *lsp.ServerManager) {
	if t, err := r.Get("LSP"); err == nil {
		if lt, ok := t.(*LSPTool); ok {
			lt.SetLSPManager(m)
		}
	}
}

// SetSnippetConfig configures snippet expansion on the Write and Edit tools.
func (r *Registry) SetSnippetConfig(cfg *snippets.Config) {
	if cfg == nil {
		return
	}
	if t, err := r.Get("Write"); err == nil {
		if ft, ok := t.(*FileWriteTool); ok {
			ft.SnippetConfig = cfg
		}
	}
	if t, err := r.Get("Edit"); err == nil {
		if ft, ok := t.(*FileEditTool); ok {
			ft.SnippetConfig = cfg
		}
	}
}

// Names returns the names of all registered tools.
func (r *Registry) Names() []string {
	result := make([]string, len(r.order))
	copy(result, r.order)
	return result
}

// DefaultRegistry creates a registry with all core tools.
func DefaultRegistry() *Registry {
	r := NewRegistry()

	// Shared caches for deduplicating repeated tool calls within a session.
	rc := readcache.New(256)
	gc := grepcache.New(512)

	// Core file & shell tools (always loaded — never deferred)
	r.Register(&BashTool{})
	r.Register(&FileReadTool{ReadCache: rc})
	r.Register(&FileWriteTool{ReadCache: rc})
	r.Register(&FileEditTool{ReadCache: rc})
	r.Register(&GlobTool{})
	r.Register(&GrepTool{Cache: gc})

	// Agent (always loaded)
	r.Register(&AgentTool{})

	// Plan mode (always loaded — AI must see full description to enter proactively)
	r.Register(&EnterPlanModeTool{})
	r.Register(&ExitPlanModeTool{})

	// ToolSearch (always loaded — needed to discover deferred tools)
	ts := &ToolSearchTool{}
	r.Register(ts)

	// --- Deferred tools (sent with defer_loading: true to save tokens) ---

	// Web tools
	r.Register(&WebSearchTool{deferrable: newDeferrable("web search current events information")})
	r.Register(&WebFetchTool{deferrable: newDeferrable("fetch URL content webpage")})

	// Code intelligence
	r.Register(&LSPTool{deferrable: newDeferrable("LSP code intelligence definitions references")})

	// Notebook
	r.Register(&NotebookEditTool{deferrable: newDeferrable("jupyter notebook cell edit")})

	// Task management
	r.Register(&TaskCreateTool{deferrable: newDeferrable("create task todo list tracking")})
	r.Register(&TaskListTool{deferrable: newDeferrable("list tasks todo progress")})
	r.Register(&TaskGetTool{deferrable: newDeferrable("get task details by ID")})
	r.Register(&TaskUpdateTool{deferrable: newDeferrable("update task status progress")})

	// Workspace
	r.Register(&EnterWorktreeTool{deferrable: newDeferrable("git worktree isolated branch")})
	r.Register(&ExitWorktreeTool{deferrable: newDeferrable("exit leave worktree")})


	// Background task management (Runtime injected later)
	r.Register(&TaskStopTool{deferrable: newDeferrable("stop cancel background task")})
	r.Register(&TaskOutputTool{deferrable: newDeferrable("read background task output")})

	// Team management (Manager injected later)
	r.Register(&TeamCreateTool{deferrable: newDeferrable("create team multi-agent collaboration")})
	r.Register(&TeamDeleteTool{deferrable: newDeferrable("delete remove team")})
	r.Register(&SendMessageTool{deferrable: newDeferrable("send message to team agent")})

	// Memory access (Store injected later)
	r.Register(&MemoryTool{deferrable: newDeferrable("memory search read list persistent context")})

	// Cron/scheduled tasks (Store injected later)
	r.Register(&CronCreateTool{})
	r.Register(&CronDeleteTool{})
	r.Register(&CronListTool{})

	// AskUser (channels injected by TUI layer)
	r.Register(&AskUserTool{deferrable: newDeferrable("ask user question options structured")})

	// Inject registry into ToolSearch so it can look up tools
	ts.SetRegistry(r)

	return r
}
