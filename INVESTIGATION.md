# Claudio Built-In Agent Architecture Investigation

## Subject
Investigated how built-in agents are defined, registered, invoked, and tool-controlled in Claudio; how custom agents load from files; how skills and plugins integrate; how TUI agent selection works; and how CLI `--agent` flag flows through the system.

---

## Codebase Overview

**Entry points:**
- CLI: `internal/cli/root.go` — agent init via `--agent` flag (line 221), calls `applyAgentOverrides()` (line 255, 381, 857)
- Agent system: `internal/agents/agents.go` — definitions, registration, loading
- Skills: `internal/services/skills/loader.go` — skill loading, registry, bundled skills
- TUI: `internal/tui/root.go` — agent persona application during runtime, `internal/tui/agentselector/selector.go` — UI component

**Key structures:**
- `AgentDefinition` struct (agents.go:36) — defines agent capabilities, prompts, tools
- `Skill` struct (skills/loader.go:14) — skill definitions with hooks
- `Frontmatter` map (utils/frontmatter.go:9) — simple key-value parser for YAML-like config

---

## Key Findings

### 1. AgentDefinition Struct — Complete Field Map

**Location:** `internal/agents/agents.go:36`

```go
type AgentDefinition struct {
  Type                string      // unique identifier (e.g., "general-purpose", "Explore")
  WhenToUse          string      // description of when to use this agent
  SystemPrompt       string      // system prompt injected when agent is active
  Tools              []string    // allowed tools (empty or "*" for all)
  DisallowedTools    []string    // explicitly denied tools (blocks these)
  Model              string      // model override (haiku, sonnet, opus, or "")
  ReadOnly           bool        // if true, Edit/Write/NotebookEdit blocked
  MemoryDir          string      // agent's own memory directory (crystallized agents)
  ExtraSkillsDir     string      // agent-specific skills directory (merged with global)
  ExtraPluginsDir    string      // agent-specific plugins directory (merged with global)
  SourceSession      string      // session ID if agent was crystallized from a session
  SourceProject      string      // project dir if agent was originally created in a project
  MaxTurns           int         // max agentic turns (API calls); 0 = unlimited
}
```

**Data flow:** `GetAgent(agentType)` → searches `AllAgents()` → returns definition or falls back to `GeneralPurposeAgent()`.

---

### 2. Built-In Agent Definitions

**Location:** `internal/agents/agents.go:79–302`

Four built-in agents, each a function that returns a populated `AgentDefinition`:

| Agent | Type | MaxTurns | Model | ReadOnly | DisallowedTools | Role |
|-------|------|----------|-------|----------|-----------------|------|
| `GeneralPurposeAgent()` | `"general-purpose"` | 50 | (inherit) | false | none | Default multi-step agent, full tool access |
| `ExploreAgent()` | `"Explore"` | 25 | haiku | true | Agent, ExitPlanMode, Edit, Write, NotebookEdit | Codebase exploration specialist (read-only) |
| `PlanAgent()` | `"Plan"` | 30 | (inherit) | true | Agent, ExitPlanMode, Edit, Write, NotebookEdit | Implementation architect (design only, no code) |
| `VerificationAgent()` | `"verification"` | 20 | (inherit) | true | Edit, Write, NotebookEdit | Test + validate implementation (bash + read-only) |

**Call chain:** `BuiltInAgents()` → returns all 4 as a slice → used by `AllAgents()` + `GetAgent()`.

---

### 3. Custom Agent Loading — File Format & Directory Structure

**Location:** `internal/agents/agents.go:304–488`

**Custom agent file formats:**

1. **Flat markdown** — single `.md` file per agent:
   ```
   agents/
   └── my-agent.md
   ```
   - Filename (minus `.md`) = agent Type
   - Frontmatter keys: `description`, `name`, `tools`, `disallowedTools`, `model`, `sourceSession`, `sourceProject`
   - Markdown body after `---` → SystemPrompt

2. **Directory-form** — dedicated folder per agent (preferred, wins over flat):
   ```
   agents/
   └── my-agent/
       ├── AGENT.md         (priority 1)
       ├── agent.md         (priority 2)
       ├── my-agent.md      (priority 3)
       ├── memory/          (auto-detected)
       ├── skills/          (auto-detected)
       └── plugins/         (auto-detected)
   ```
   - Directory name = agent Type
   - Looks for definition .md in order: `AGENT.md` > `agent.md` > `<dirname>.md`
   - Subdirectories auto-populate `MemoryDir`, `ExtraSkillsDir`, `ExtraPluginsDir`

**Frontmatter format:**
```yaml
---
description: Brief description of when to use this agent
name: Display name (optional, falls back to description)
tools: "Skill, Agent, Read"  # or "*" for all
disallowedTools: "Edit, Write"
model: "sonnet"              # haiku, sonnet, opus, or empty
sourceSession: "<session-id>"
sourceProject: "/path/to/project"
---

Your system prompt here. Can span multiple paragraphs.
```

**Loading logic** (`LoadCustomAgents(dirs...)`):
1. Read directory entries
2. Directory-form agents loaded first, mark types as "seen"
3. Flat-file `.md` agents loaded, skip if type already seen (directory wins)
4. For each agent: parse frontmatter, extract fields, detect subdirs, append to custom slice
5. `AllAgents()` merges built-in + custom, custom overrides built-in by Type

---

### 4. Tool Filtering & DisallowedTools

**Mechanism:** `registry.Clone().Remove(name)` removes tools by name.

**Where applied:**
1. **CLI (`--agent` flag)** — `applyAgentOverrides()` (root.go:525):
   - Clones registry at startup
   - Removes each tool in `DisallowedTools` slice
   - Returns filtered registry + model override + plugin infos
   - Calls registered before engine creation

2. **TUI (runtime selection)** — `applyAgentPersona()` (root.go:1735):
   - Same clone-and-remove logic
   - Updates live engine registry mid-session via `engine.SetRegistry(filtered)`

**Call chain for CLI:**
```
rootCmd.PersistentPreRunE → runInteractive()/runSinglePrompt()
  → applyAgentOverrides(appInstance.Tools)
    → agents.GetAgent(flagAgent)
    → registry.Clone()
    → registry.Remove(...DisallowedTools)
    → returns (filtered registry, modelOverride, extraPluginInfos)
  → passes filtered registry to query.NewEngineWithConfig()
```

---

### 5. Skills System — Loading, Registry, Bundled Skills

**Location:** `internal/services/skills/loader.go`

**Skill struct** (line 14):
```go
type Skill struct {
  Name        string      // unique skill name
  Description string      // brief description
  Content     string      // the prompt/instruction content
  Source      string      // "bundled", "user", "project", "plugin"
  FilePath    string      // path to skill file (optional)
  SkillDir    string      // directory containing skill (optional)
  Paths       []string    // paths this skill applies to (optional)
  Hooks       []SkillHook // auto-register hooks
}
```

**SkillHook** (line 26):
```go
type SkillHook struct {
  Event   string  // "PreToolUse", "PostToolUse", etc.
  Matcher string  // tool name glob (e.g., "Write|Edit" or "*")
  Command string  // shell command to run
  Timeout int     // milliseconds
  Async   bool    // non-blocking if true
}
```

**Registry** (line 35):
```go
type Registry struct {
  mu     sync.RWMutex
  skills map[string]*Skill
}
```
Methods: `Get(name)`, `Register(skill)`, `All()` (sorted deterministically).

**Loading hierarchy** — `LoadAll(userDir, projectDir)` (line 80):
1. Load bundled skills first (hardcoded in `bundledSkills()`)
2. Load user skills from `~/.claudio/skills/` (if dir provided)
3. Load project skills from `.claudio/skills/` (if dir provided)
4. Later registrations override earlier ones by skill name

**Bundled skills** (line 217–1700): Defined as `var skillContent` constants, compiled into binary. Currently: `commit`, `review`, `simplify`, `updateConfig`, `debug`, `batch`, `pr`, `test`, `securityReview`, `setupSnippets`, `refactor`, `init`, `harness`, `caveman`, `cavemanCommit`, `cavemanReview`.

**Per-agent skills:** `applyAgentOverrides()` (root.go:536–558) loads `agentDef.ExtraSkillsDir` via `skills.LoadAll()` and merges into SkillTool's registry without mutating the global one.

---

### 6. CLI `--agent` Flag Flow

**Definition:** `internal/cli/root.go:45` (var), `init()` line 221 (flag registration):
```go
flagAgent string  // Run as a specific agent persona (e.g., prab, backend-senior)
rootCmd.PersistentFlags().StringVar(&flagAgent, "agent", "", "...")
```

**Flow at startup:**

```
PersistentPreRunE (root.go:74)
  ↓
runSinglePrompt() or runInteractive()
  ↓
applyAgentOverrides(appInstance.Tools) [line 255, 381, 857]
  ├─ agents.GetAgent(flagAgent) → AgentDefinition
  ├─ registry.Clone() → filtered registry
  ├─ filtered.Remove(...DisallowedTools)
  ├─ Load ExtraSkillsDir skills (if present)
  ├─ Load ExtraPluginsDir plugins (if present)
  └─ Return (filtered, modelOverride, pluginInfos)

appInstance.Config.Model = modelOverride (if not "")
appInstance.API.SetModel(modelOverride)

query.NewEngineWithConfig(..., filtered, ...)

buildFullSystemPrompt() returns base system + rules + context
engine.SetSystem(base) — agent's SystemPrompt NOT appended at CLI startup
```

**Note:** CLI `--agent` applies tool filtering but NOT system prompt injection at this stage (that's TUI-only).

---

### 7. TUI Agent Selection & Runtime Application

**Components:**

1. **AgentSelectedMsg** (agentselector/selector.go:16):
```go
type AgentSelectedMsg struct {
  AgentType       string
  DisplayName     string
  SystemPrompt    string
  Model           string
  DisallowedTools []string
}
```

2. **Agent selector UI** (agentselector/selector.go):
   - Model holds list of all agents (built-in + custom)
   - User navigates with arrow keys, filters with text input
   - On selection, sends `AgentSelectedMsg` up the event chain

3. **Persona application** — Two methods on TUI Model:

   **`applyAgentPersona(msg)` (root.go:1735)** — runtime, after engine created:
   - If `msg.AgentType == ""` → clear agent, restore base state
   - Append `msg.SystemPrompt` to `baseSystemPrompt` → new system prompt
   - Clone registry, remove `DisallowedTools`, set as `m.registry`
   - Apply model override
   - Call `engine.SetSystem(newSystem)` + `engine.SetRegistry(filtered)` — updates live engine
   - Add system message to chat

   **`ApplyAgentPersonaAtStartup(msg)` (root.go:1890)** — before engine created:
   - Same logic as `applyAgentPersona` but skips system message + viewport refresh
   - Called when `--agent` flag is set at CLI startup (root.go:948)
   - Ensures system prompt and registry are prepared before engine creation

**Call chain for TUI selection:**
```
User selects agent in selector UI
  → AgentSelectedMsg sent
  → root.Update() handles agentselector.AgentSelectedMsg case (root.go:1335)
    → m.applyAgentPersona(msg)
      ├─ m.systemPrompt = baseSystemPrompt + msg.SystemPrompt
      ├─ filtered = m.registry.Clone()
      ├─ filtered.Remove(DisallowedTools...)
      ├─ m.registry = filtered
      ├─ engine.SetSystem(newSystem)
      ├─ engine.SetRegistry(filtered)
      └─ addMessage("Agent persona: ...")
```

---

### 8. Crystallized Sessions → Agent Definitions

**Location:** `internal/agents/crystallize.go:14`

**Function:** `CrystallizeSession(agentsDir, name, description, sessionID, sourceProject, summary string, memories []*memory.Entry) (*AgentDefinition, error)`

**Process:**
1. Sanitize agent name for filesystem
2. Create agent directory: `agentsDir/<safeName>/`
3. Create memory subdirectory: `agentsDir/<safeName>/memory/`
4. Build system prompt from name, description, summary, key memory entries
5. Write agent markdown file: `agentsDir/<safeName>.md` with frontmatter + prompt body
6. Copy session memories into agent's memory directory
7. Return new `AgentDefinition` with MemoryDir set

**Generated frontmatter includes:**
- `description: <description>`
- `sourceSession: <sessionID>`
- `sourceProject: <sourceProject>`
- `tools: "*"`

**Result:** Crystallized agents can be reloaded from disk as custom agents; `LoadCustomAgents()` auto-detects their MemoryDir and SourceSession fields.

---

### 9. Agent Registry Operations — Clone, Remove, Merge

**Methods on `*tools.Registry`:**

- **`Clone() *Registry`** — deep copy of registry (all tools copied by reference, map cloned)
- **`Remove(name string)`** — delete tool from registry by name
- **`Get(name string) (Tool, error)`** — retrieve tool
- **`Register(tool Tool)`** — add/overwrite tool
- **`All() []Tool`** — list all registered tools

**Skill merge pattern** (root.go:536–558):
```go
// Global registry
globalSkillReg := appInstance.Skill.SkillsRegistry

// Agent wants extra skills
if agentDef.ExtraSkillsDir != "" {
  mergedReg := skills.NewRegistry()
  // Copy global skills
  for _, s := range globalSkillReg.All() {
    mergedReg.Register(s)
  }
  // Load + add agent-specific skills
  extraReg := skills.LoadAll("", agentDef.ExtraSkillsDir)
  for _, s := range extraReg.All() {
    mergedReg.Register(s)
  }
  // Replace SkillTool with new instance using merged registry
  filtered.Register(newSkillToolWithMergedRegistry)
}
```
Result: Agent sees global skills + its own, global registry unmodified.

---

### 10. Model Override Resolution & Application

**Shortcut resolution** (root.go:595):
```go
model := agentDef.Model
if resolved, ok := appInstance.API.ResolveModelShortcut(model); ok {
  model = resolved  // e.g., "sonnet" → "claude-3-5-sonnet-20241022"
}
```

**Precedence:**
1. Agent's `Model` field (if set)
2. Default from `appInstance.Config.Model` (if agent.Model == "")
3. CLI `--model` flag (sets config.Model during init)

**Applied at:**
- CLI startup: `appInstance.API.SetModel(modelOverride)` (root.go:258)
- TUI runtime: `m.apiClient.SetModel(model)` + engine via active sessions

---

## Symbol Map

| Symbol | File | Role |
|--------|------|------|
| `AgentDefinition` | `internal/agents/agents.go:36` | Core struct describing agent capabilities, tools, prompts |
| `BuiltInAgents()` | `internal/agents/agents.go:79` | Returns slice of 4 built-in agent definitions |
| `GeneralPurposeAgent()` | `internal/agents/agents.go:128` | Default general-purpose agent (no restrictions) |
| `ExploreAgent()` | `internal/agents/agents.go:162` | Read-only codebase explorer (haiku model) |
| `PlanAgent()` | `internal/agents/agents.go:217` | Architecture/design agent (no code write) |
| `VerificationAgent()` | `internal/agents/agents.go:257` | Test/validation agent (bash + read-only) |
| `GetAgent(agentType)` | `internal/agents/agents.go:90` | Fetch agent definition by type; fallback to general-purpose |
| `LoadCustomAgents(dirs...)` | `internal/agents/agents.go:312` | Load custom agents from markdown files |
| `AllAgents(customDirs...)` | `internal/agents/agents.go:462` | Merge built-in + custom agents |
| `CrystallizeSession()` | `internal/agents/crystallize.go:14` | Create agent definition from session knowledge |
| `applyAgentOverrides()` | `internal/cli/root.go:525` | CLI: clone registry, apply DisallowedTools, merge skills/plugins |
| `applyAgentPersona()` | `internal/tui/root.go:1735` | TUI runtime: apply agent system prompt + filtered registry |
| `ApplyAgentPersonaAtStartup()` | `internal/tui/root.go:1890` | TUI pre-engine: prepare agent for engine creation |
| `Skill` | `internal/services/skills/loader.go:14` | Skill definition: name, description, content, hooks |
| `SkillHook` | `internal/services/skills/loader.go:26` | Tool hook: event, matcher, command, timeout |
| `Registry` | `internal/services/skills/loader.go:35` | Skill registry: map[string]*Skill + thread-safe Get/Register/All |
| `LoadAll(userDir, projectDir)` | `internal/services/skills/loader.go:80` | Load skills from bundled + user + project sources |
| `bundledSkills()` | `internal/services/skills/loader.go:217` | Return hardcoded bundled skill definitions |
| `ParseFrontmatter(content)` | `internal/utils/frontmatter.go:13` | Parse YAML-like frontmatter from markdown |
| `AgentSelectedMsg` | `internal/tui/agentselector/selector.go:16` | TUI message: agent type, prompt, model, disallowed tools |

---

## Dependencies & Data Flow

### Agent Initialization Pipeline (Startup)

```
CLI root command (--agent flag set)
  ├─ PersistentPreRunE → Initialize app
  │   ├─ Load config, auth, plugins, rules, skills
  │   └─ appInstance.Tools = registry (global, all tools)
  │
  ├─ applyAgentOverrides(appInstance.Tools)
  │   ├─ agents.GetAgent(flagAgent) → AgentDefinition
  │   ├─ Clone registry → filtered
  │   ├─ Remove DisallowedTools from filtered
  │   ├─ Merge ExtraSkillsDir (if set)
  │   ├─ Merge ExtraPluginsDir (if set)
  │   └─ Return (filtered registry, model override, plugin infos)
  │
  ├─ Update appInstance.API.Model if override present
  │
  ├─ buildFullSystemPrompt() → base system (rules + context, NO agent prompt here)
  │
  ├─ query.NewEngineWithConfig(
  │     api, 
  │     filtered_registry,  ← Tool filtering applied
  │     handler, 
  │     config
  │   )
  │
  └─ engine.SetSystem(base)
```

**Note:** CLI startup does NOT inject agent's SystemPrompt; that's TUI-only. The `--agent` flag's main effect is tool filtering via DisallowedTools.

### Agent Selection Pipeline (TUI Runtime)

```
User selects agent in TUI agent picker
  ├─ Selector builds AgentSelectedMsg
  │   ├─ AgentType (e.g., "Explore")
  │   ├─ DisplayName
  │   ├─ SystemPrompt (full text)
  │   ├─ Model (override or "")
  │   └─ DisallowedTools []string
  │
  ├─ root.Update() → applyAgentPersona(msg)
  │   ├─ Append msg.SystemPrompt to baseSystemPrompt
  │   ├─ Clone m.registry → filtered
  │   ├─ filtered.Remove(msg.DisallowedTools...)
  │   ├─ Set m.registry = filtered
  │   ├─ Apply model override (resolve shortcuts)
  │   ├─ engine.SetSystem(newSystem)
  │   ├─ engine.SetRegistry(filtered)
  │   └─ addMessage("Agent persona: X")
  │
  └─ Next turn uses filtered registry + new system prompt
```

### Custom Agent Loading

```
User registers custom agent dir: agents.SetCustomDirs("/path/to/agents")
  ├─ LoadCustomAgents("/path/to/agents")
  │   ├─ Scan for directories (directory-form agents)
  │   │   ├─ Look for AGENT.md (priority 1)
  │   │   ├─ Look for agent.md (priority 2)
  │   │   ├─ Look for <dirname>.md (priority 3)
  │   │   ├─ Parse frontmatter (keys: tools, disallowedTools, model, etc.)
  │   │   ├─ Extract markdown body → SystemPrompt
  │   │   ├─ Auto-detect memory/, skills/, plugins/ subdirs
  │   │   └─ Create AgentDefinition
  │   │
  │   ├─ Scan for flat .md files
  │   │   ├─ Skip if directory-form agent with same name already loaded
  │   │   ├─ Parse frontmatter
  │   │   ├─ Check for sibling subdirs
  │   │   └─ Create AgentDefinition
  │   │
  │   └─ Return []AgentDefinition
  │
  ├─ AllAgents(customDirs) merges
  │   ├─ Start with BuiltInAgents()
  │   ├─ Overlay LoadCustomAgents() results
  │   ├─ Custom overrides built-in by Type
  │   └─ Return merged slice (built-in first, then new custom types)
  │
  └─ GetAgent(type) searches merged slice, fallback to general-purpose
```

---

## Risks & Observations

### 1. Agent System Prompt NOT Applied at CLI Startup

**Issue:** When `--agent` flag is used, the agent's `SystemPrompt` is NOT injected into the base system prompt at startup. Tool filtering (DisallowedTools) is applied, but the agent's instructions are ignored at CLI level.

**Why:** The `--agent` design targets TUI, where `ApplyAgentPersonaAtStartup()` prepares the model + registry, but the system prompt is only set on the live engine after creation.

**Impact:** CLI `--agent` usage may feel "incomplete" for headless/script scenarios. Consider whether CLI-level agents should also inject SystemPrompt.

### 2. Registry Cloning Creates References, Not Deep Copies

**Observation:** `registry.Clone()` copies the map but tool instances are stored by reference. Modifying a tool's internal state affects all clones.

**Risk:** Low — tools are generally immutable. But if a tool's state is modified post-clone, all copies see it.

### 3. Custom Agents Can Override Built-In by Type

**Design:** Custom agents with `Type == "general-purpose"` override the built-in general-purpose agent.

**Risk:** User could accidentally shadow a built-in agent. Consider warning or preventing overrides of built-in types.

### 4. Crystallized Agents Store Memories on Disk

**Observation:** `CrystallizeSession()` writes agent's memory entries to disk (`memory/` subdir). These are NOT loaded automatically — agent must have `MemoryDir` field set for memories to be accessible.

**Potential issue:** If the agent's `MemoryDir` isn't set correctly, crystallized knowledge is lost.

### 5. SkillHook Command Execution

**Risk:** Skill hooks can run arbitrary shell commands. Ensure hooks are validated before execution, especially for user-provided skills.

### 6. MaxTurns Field on Built-In Agents

**Observation:** Built-in agents have `MaxTurns` set (e.g., 50, 25, 30, 20). This prevents runaway agents.

**Impact:** Good safety feature, but custom agents default to `MaxTurns == 0` (unlimited). Consider requiring explicit `maxTurns` in custom agent frontmatter.

### 7. Tool Filtering via DisallowedTools is All-or-Nothing

**Observation:** Tools are either fully removed or fully accessible. No per-tool function-level restrictions.

**Impact:** Fine-grained permissions (e.g., "Edit but not Write") require custom tooling.

---

## Open Questions

1. **Does CLI `--agent` pass the agent's SystemPrompt to the engine?**
   - Answer: NO — CLI startup skips SystemPrompt injection. Only TUI applies it at runtime.
   - **Follow-up:** Is this intentional? Should headless mode support agent system prompts?

2. **Can an agent define its own memory location?**
   - Answer: YES — `MemoryDir` field. But it must be set in the agent definition; it's not auto-populated from a custom agent's memory/ subdir during TUI runtime (only at crystallization).
   - **Follow-up:** Should `ApplyAgentPersona()` also load + set memory context?

3. **How are agent-specific plugins discovered?**
   - Answer: `applyAgentOverrides()` loads `ExtraPluginsDir` and registers each as a PluginProxyTool. But custom agents must have the plugins/ subdir pre-created.
   - **Follow-up:** Should the loader auto-create plugins/ during agent discovery?

4. **Can multiple agents share a skills/ directory?**
   - Answer: YES — `ExtraSkillsDir` is just a path. Multiple agents can point to the same skills directory.
   - **Impact:** Useful for shared skill libraries.

5. **What happens if an agent's SystemPrompt is empty?**
   - Answer: No system prompt appended; base prompt used as-is.
   - **Implication:** Valid design — agent can be tool-restricted without custom instructions.

---

## Summary for Planning a New Built-In Agent

To add a new built-in agent, you need:

1. **New function in `internal/agents/agents.go`:**
   ```go
   func MyNewAgent() AgentDefinition {
     return AgentDefinition{
       Type:            "my-agent",
       MaxTurns:        20,
       WhenToUse:       "Description of when to use this agent",
       Tools:           []string{"*"},
       DisallowedTools: []string{"Edit", "Write"},  // Or empty for full access
       Model:           "haiku",  // Or "" to inherit
       ReadOnly:        true,     // Or false
       SystemPrompt:    `Your detailed system prompt here...`,
     }
   }
   ```

2. **Register in `BuiltInAgents()` slice** (agents.go:79–85):
   ```go
   return []AgentDefinition{
     GeneralPurposeAgent(),
     ExploreAgent(),
     PlanAgent(),
     VerificationAgent(),
     MyNewAgent(),  // Add here
   }
   ```

3. **System prompt best practices:**
   - Define agent role, constraints, process, tool guidance
   - Follow existing patterns (Explore, Plan, Verification) for tone/structure
   - Include escalation rules if agent should stop and ask

4. **Testing:**
   - Check `internal/agents/agents_test.go` for existing test patterns
   - Test `GetAgent("my-agent")` returns correct definition
   - Test tool filtering works (`DisallowedTools` actually removed from registry)

5. **Optional — custom skills/plugins/memory:**
   - If agent needs agent-specific behavior, create a custom agent in `.claudio/agents/` instead
   - Custom agents can have `ExtraSkillsDir` pointing to agent-specific skills
   - Store agent-specific knowledge in `.claudio/agents/my-agent/memory/`

---

**Report generated:** Static analysis of agent system; runtime behavior not verified.
