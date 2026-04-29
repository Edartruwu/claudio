package tools

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/services/lsp"
	"github.com/Abraxas-365/claudio/internal/snippets"
	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/Abraxas-365/claudio/internal/tools/grepcache"
	"github.com/Abraxas-365/claudio/internal/tools/readcache"
)

// Registry holds all registered tools.
type Registry struct {
	tools map[string]Tool
	order []string // preserve insertion order

	// deferOverride lets the user manually pin a normally-deferred tool to be
	// always loaded (value=false) or force a normally-eager *deferrable* tool
	// to be deferred (value=true). Tools that do not implement DeferrableTool
	// cannot be force-deferred and overrides for them are ignored.
	deferOverride map[string]bool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools:         make(map[string]Tool),
		deferOverride: make(map[string]bool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	if _, exists := r.tools[t.Name()]; !exists {
		r.order = append(r.order, t.Name())
	}
	r.tools[t.Name()] = t
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
		shouldDefer := r.resolveDeferral(name, t, discoveredTools)

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
// User overrides are honored: pinned tools are excluded, force-deferred tools
// are included.
func (r *Registry) DeferredToolNames() []string {
	var names []string
	for _, name := range r.order {
		t := r.tools[name]
		if r.resolveDeferral(name, t, nil) {
			names = append(names, name)
		}
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

// resolveDeferral applies override + auto-activate + discovery rules and returns
// whether `name` should be deferred for this request.
func (r *Registry) resolveDeferral(name string, t Tool, discoveredTools map[string]bool) bool {
	dt, isDeferrable := t.(DeferrableTool)
	if !isDeferrable {
		return false // non-deferrable tools are always eager
	}

	// Start from the tool's own opinion, then apply user override if present.
	wantDefer := dt.ShouldDefer()
	if override, ok := r.deferOverride[name]; ok {
		wantDefer = override
	}
	if !wantDefer {
		return false
	}

	// Already discovered via ToolSearch — load the full schema.
	if discoveredTools != nil && discoveredTools[name] {
		return false
	}

	// Auto-activate if the tool's backing service is available, but only when
	// the user has not explicitly forced it deferred.
	if _, userOverrode := r.deferOverride[name]; !userOverrode {
		if aa, ok := t.(AutoActivatable); ok && aa.AutoActivate() {
			return false
		}
	}

	return true
}

// IsDeferrable reports whether the named tool implements DeferrableTool.
// Non-deferrable (always-eager) tools cannot be toggled by the user.
func (r *Registry) IsDeferrable(name string) bool {
	t, ok := r.tools[name]
	if !ok {
		return false
	}
	_, ok = t.(DeferrableTool)
	return ok
}

// IsDeferred reports whether the named tool will be deferred for the next API
// request given current overrides. discoveredTools may be nil.
func (r *Registry) IsDeferred(name string) bool {
	t, ok := r.tools[name]
	if !ok {
		return false
	}
	return r.resolveDeferral(name, t, nil)
}

// SetDeferOverride pins a tool's deferral state. Pass deferred=false to load
// it eagerly even if it normally defers; pass deferred=true to force a
// deferrable tool to defer even if it would auto-activate. No effect on tools
// that do not implement DeferrableTool.
func (r *Registry) SetDeferOverride(name string, deferred bool) {
	if !r.IsDeferrable(name) {
		return
	}
	r.deferOverride[name] = deferred
}

// ClearDeferOverride removes any user override for the tool, restoring its
// natural (tool-defined) behavior.
func (r *Registry) ClearDeferOverride(name string) {
	delete(r.deferOverride, name)
}

// HasDeferOverride reports whether the user has set an explicit override for
// this tool.
func (r *Registry) HasDeferOverride(name string) bool {
	_, ok := r.deferOverride[name]
	return ok
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
	for k, v := range r.deferOverride {
		clone.deferOverride[k] = v
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

	// Shallow-copy BashTool so sub-agent gets its own instance.
	// Without this, sub-agent shares parent's BashTool by reference
	// and background task completions land on the wrong runtime.
	if t, err := clone.Get("Bash"); err == nil {
		if bt, ok := t.(*BashTool); ok {
			cloned := *bt
			clone.tools["Bash"] = &cloned
		}
	}

	return clone
}

// SetTaskRuntime injects a task runtime into the registry's BashTool.
// Used to give sub-agents their own runtime so background task completions
// are routed to the correct engine.
func (r *Registry) SetTaskRuntime(rt *tasks.Runtime) {
	if t, err := r.Get("Bash"); err == nil {
		if bt, ok := t.(*BashTool); ok {
			bt.TaskRuntime = rt
		}
	}
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

// SetBus injects the event bus into task tools for event publishing.
func (r *Registry) SetBus(b *bus.Bus) {
	if b == nil {
		return
	}
	if t, err := r.Get("TaskCreate"); err == nil {
		if tc, ok := t.(*TaskCreateTool); ok {
			tc.bus = b
		}
	}
	if t, err := r.Get("TaskUpdate"); err == nil {
		if tu, ok := t.(*TaskUpdateTool); ok {
			tu.bus = b
		}
	}
	// Inject bus into TaskStore for task completion events
	GlobalTaskStore.bus = b
}

// SetSmallModelClient injects the API client and small model name into TaskUpdateTool
// for the verification nudge feature.
func (r *Registry) SetSmallModelClient(client *api.Client, smallModel string) {
	if t, err := r.Get("TaskUpdate"); err == nil {
		if tu, ok := t.(*TaskUpdateTool); ok {
			tu.APIClient = client
			tu.SmallModel = smallModel
		}
	}
}

// FilterByNames returns a cloned registry containing only tools whose names
// appear in the provided list. Unknown names are silently ignored.
func (r *Registry) FilterByNames(names []string) *Registry {
	clone := r.Clone()
	keep := make(map[string]struct{}, len(names))
	for _, n := range names {
		keep[n] = struct{}{}
	}
	for _, name := range clone.Names() {
		if _, ok := keep[name]; !ok {
			clone.Remove(name)
		}
	}
	return clone
}

// Names returns the names of all registered tools.
func (r *Registry) Names() []string {
	result := make([]string, len(r.order))
	copy(result, r.order)
	return result
}

// TeamToolNames lists the team collaboration tools that are only available
// to the principal agent when a team template or ephemeral team is active.
var TeamToolNames = []string{
	"SendMessage",
	"SpawnTeammate",
	"InstantiateTeam",
	"PurgeTeammates",
	"ListTeammates",
}

// TeamEagerToolNames lists all tools that should be force-eager (non-deferred)
// when a team context is active. Includes team tools + task management + AskUser
// so agents never need to ToolSearch for them during team workflows.
var TeamEagerToolNames = []string{
	"SendMessage",
	"SpawnTeammate",
	"InstantiateTeam",
	"PurgeTeammates",
	"ListTeammates",
	"TaskCreate",
	"TaskList",
	"TaskGet",
	"TaskUpdate",
	"BgTaskList",
	"TaskStop",
	"TaskOutput",
	"AskUser",
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

	// Skill tool — always loaded so the LLM sees available skills for auto-detection
	r.Register(&SkillTool{})

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
	r.Register(&BgTaskListTool{deferrable: newDeferrable("list background tasks running shell server")})
	r.Register(&TaskStopTool{deferrable: newDeferrable("stop cancel background task")})
	r.Register(&TaskOutputTool{deferrable: newDeferrable("read background task output")})

	// Team management (Manager injected later)
	r.Register(&SendMessageTool{deferrable: newDeferrable("send message to team agent")})
	r.Register(&SpawnTeammateTool{deferrable: newDeferrable("spawn teammate agent background parallel named")})
	r.Register(&InstantiateTeamTool{deferrable: newDeferrable("instantiate team template load roster")})
	r.Register(&PurgeTeammatesTool{deferrable: newDeferrable("purge remove completed failed agents worktrees")})
	r.Register(&ListTeammatesTool{deferrable: newDeferrable("list teammates agents spawned running status")})


	// Master session coordination (AttachClient/AttachURL injected later)
	r.Register(&SendToSessionTool{deferrable: newDeferrable("send message to master session ComandCenter")})
	r.Register(&SpawnSessionTool{deferrable: newDeferrable("spawn new session attach ComandCenter parallel")})

	// Memory access (Store injected later)
	r.Register(&MemoryTool{})
	r.Register(&RecallTool{})

	// Cron/scheduled tasks (Store injected later)
	r.Register(&CronCreateTool{})
	r.Register(&CronDeleteTool{})
	r.Register(&CronListTool{})

	// AskUser (channels injected by TUI layer)
	r.Register(&AskUserTool{deferrable: newDeferrable("ask user question options structured")})

	// Design discovery tool (always available, not capability-gated)
	wd, _ := os.Getwd()
	r.Register(NewListDesignsTool(config.ProjectDesignsDir(wd)))

	// Inject registry into ToolSearch so it can look up tools
	ts.SetRegistry(r)

	return r
}
