# Claudio Architecture Exploration Guide

This guide directs you to the right documentation for different exploration tasks.

## Quick Start

1. **New to the codebase?** Start with [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) → Executive Summary
2. **Need quick reference?** See [PACKAGE_INDEX.md](PACKAGE_INDEX.md) for tables and patterns
3. **Understanding lifecycle?** Read [LIFECYCLE_AND_PATTERNS.md](LIFECYCLE_AND_PATTERNS.md)

---

## Documentation Map

### ARCHITECTURE_MAP.md (969 lines, 31 KB)

**Best for:** Understanding overall structure, major components, wiring patterns

**Contains:**
- Executive summary of 5-layer architecture
- Application startup flow (cmd → cli → app.New → UI)
- App initialization (20+ subsystems wired)
- Query engine loop with all stop reasons
- Tool system (registry, 50+ tools, deferred loading, security injection)
- Permission system (content-pattern rules evaluation)
- Wiring patterns (event bus, hooks, security context, skills, agents, teams)
- Multi-provider API abstraction
- Session & memory management
- Teams & agents coordination
- Learning & analytics
- Plugins & extensibility
- All 3 UI layers (CLI, TUI, Web)
- Config system & file layout
- Storage (SQLite schema)
- Detailed request/response flow
- Key integration points
- Error handling & resilience
- Security model
- Performance optimizations
- Summary connection map

**Read when:** You need to understand "what does this codebase do?" and "how do the pieces fit?"

---

### PACKAGE_INDEX.md (267 lines, 9.6 KB)

**Best for:** Quick lookups, package roles, tool categories, common patterns

**Contains:**
- 35 internal packages organized by tier (foundational, API, execution, services, UI)
- Table of each package's responsibility and key files
- Simplified dependency graph
- 50+ tools categorized by function
- Configuration file reference
- 6 common architectural patterns with code sketches
- Entry points (commands and their handlers)

**Read when:** You need to find "where does X live?" or "how do A and B interact?"

---

### LIFECYCLE_AND_PATTERNS.md (750 lines, 26 KB)

**Best for:** Understanding runtime behavior, interactions, event sequences

**Contains:**
- Session lifecycle (new and resume)
- Complete single turn anatomy (6 major phases)
- All stop reason handlers (tool_use, end_turn, max_tokens)
- Tool execution sequence (lookup, validation, execution, security)
- Security injection details
- Hook firing with environment variables
- Memory extraction and injection
- Sub-agent spawning with context decoration
- Team coordination (creation, spawning, mailbox polling)
- Auto-learning lifecycle
- Prompt caching setup and expiry
- Conversation compaction (95% context trigger)
- Config loading and trust flow
- 10 key interaction patterns

**Read when:** You need to understand "what happens when X occurs?" or "how does Y flow through the system?"

---

## Quick Navigation by Task

### "I want to add a new tool"
1. Read: [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) → Tool System section
2. Read: [PACKAGE_INDEX.md](PACKAGE_INDEX.md) → Common Patterns → Tool Invocation
3. Look at: `internal/tools/bash.go` or `internal/tools/read.go` as examples
4. Register in: `internal/tools/registry.go` → DefaultRegistry()

### "I want to understand permission rules"
1. Read: [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) → Permission System section
2. Read: [LIFECYCLE_AND_PATTERNS.md](LIFECYCLE_AND_PATTERNS.md) → Permission Evaluation
3. Check: `internal/config/config.go` for PermissionRule definition
4. Check: `internal/permissions/rules.go` for evaluation logic

### "I want to understand how agents work"
1. Read: [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) → Agent System section
2. Read: [LIFECYCLE_AND_PATTERNS.md](LIFECYCLE_AND_PATTERNS.md) → Sub-Agent Spawning
3. Check: `internal/agents/agents.go` for agent definitions
4. Check: `internal/tools/agent.go` for agent tool implementation

### "I want to understand memory"
1. Read: [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) → Session & Memory section
2. Read: [LIFECYCLE_AND_PATTERNS.md](LIFECYCLE_AND_PATTERNS.md) → Memory Lifecycle
3. Check: `internal/services/memory/memory.go` for storage
4. Check: `internal/services/memory/extract.go` for extraction

### "I want to understand teams"
1. Read: [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) → Teams & Agents section
2. Read: [LIFECYCLE_AND_PATTERNS.md](LIFECYCLE_AND_PATTERNS.md) → Team Coordination
3. Check: `internal/teams/team.go` for team definition
4. Check: `internal/teams/runner.go` for execution

### "I want to understand how hooks work"
1. Read: [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) → Hooks section
2. Read: [LIFECYCLE_AND_PATTERNS.md](LIFECYCLE_AND_PATTERNS.md) → Hook Execution
3. Check: `internal/hooks/hooks.go` for implementation
4. Check: `~/.claudio/hooks.json` for example configuration

### "I want to trace a request from user input to response"
1. Read: [LIFECYCLE_AND_PATTERNS.md](LIFECYCLE_AND_PATTERNS.md) → Request/Response Sequence
2. Check: `cmd/claudio/main.go` → `cli/root.go` → `app/app.go` → `query/engine.go`
3. Understand: System prompt construction, API request, streaming, stop reasons, tool execution

### "I want to understand startup"
1. Read: [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) → Application Startup Flow
2. Read: [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) → App Initialization
3. Check: `cmd/claudio/main.go` (7 lines)
4. Check: `internal/cli/root.go` (PersistentPreRunE hook)

### "I want to understand TUI/Web/CLI layers"
1. Read: [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) → UI Layers section
2. Check: `internal/cli/root.go` for Cobra command setup
3. Check: `internal/tui/` for Bubble Tea components
4. Check: `internal/web/server.go` for HTTP server

### "I want to understand database schema"
1. Read: [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) → Storage section
2. Check: `internal/storage/db.go` for schema setup
3. Check: `internal/storage/sessions.go` for session queries

### "I want to understand multi-provider API support"
1. Read: [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) → Multi-Provider API section
2. Check: `internal/api/provider/` directory for implementations
3. Check: `internal/api/client.go` for routing

### "I want to understand config system"
1. Read: [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) → Config System section
2. Read: [LIFECYCLE_AND_PATTERNS.md](LIFECYCLE_AND_PATTERNS.md) → Configuration Merging
3. Check: `internal/config/config.go` for Settings struct
4. Check: `~/.claudio/claudio.json` for example configuration

---

## Architecture Patterns Reference

### From PACKAGE_INDEX.md

1. **Tool Invocation**
   - Get tool by name from registry
   - Validate input against schema
   - Execute tool with injected context
   - Return result (inline or reference)

2. **Sub-Agent Spawning**
   - Get agent definition
   - Filter/clone tool registry
   - Create sub-engine with agent's system prompt
   - Run and capture response

3. **Memory Extraction**
   - Turn ends (end_turn reason)
   - Parse facts/concepts from response
   - Save to memory files and FTS
   - Next session loads lean index

4. **Team Coordination**
   - Create team with members
   - Spawn each member as sub-agent
   - Poll mailbox each turn
   - Coordinate via messages

5. **Permission Evaluation**
   - Extract content from tool input
   - Match against pattern rules
   - Return auto/manual/deny behavior
   - Prompt or execute accordingly

6. **Skill Resolution**
   - Look up skill by name
   - Append to system prompt
   - Re-invoke LLM
   - Return response as result

---

## Code Exploration Workflow

### Step 1: Understand the Layer
- Example: "I want to understand how tools work"
- Read: [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) → Tool System
- Learn: Registry pattern, deferred loading, security injection

### Step 2: Find the Key Package
- Use [PACKAGE_INDEX.md](PACKAGE_INDEX.md) to locate package
- Example: `internal/tools/registry.go` is the hub

### Step 3: Trace the Flow
- Use [LIFECYCLE_AND_PATTERNS.md](LIFECYCLE_AND_PATTERNS.md) for sequence
- Example: Tool Execution Lifecycle shows lookup → validation → execution → capture

### Step 4: Read Key Files
- Start with the main type definition
- Example: `tools.Registry` in `internal/tools/registry.go`
- Follow to related types (Tool interface, ToolUse, Result)

### Step 5: See Examples
- Look at simple tools first
- Example: `internal/tools/bash.go` or `internal/tools/read.go`
- Compare with complex tools: `internal/tools/agent.go`, `internal/tools/skill.go`

---

## Key Takeaways

1. **Single App struct** – `internal/app/App` is the DI root, passed everywhere
2. **Query engine loop** – `internal/query/engine.go` is the main loop, handles all stop reasons
3. **Tool registry** – `internal/tools/registry.go` manages 50+ tools with deferred loading
4. **Permission rules** – Content-pattern matchers in config, evaluated before tool execution
5. **Hooks system** – Shell commands fired at lifecycle points with context variables
6. **Memory system** – Markdown files in `~/.claudio/memory/` with FTS indexing
7. **Teams** – Multi-agent coordination via mailbox polling and context decoration
8. **Security** – Path/command validation injected at tool construction
9. **Caching** – Frozen system prompts for prompt caching, token savings
10. **Compaction** – Auto-compaction at 95% context, refreshes memory index

---

## Document Statistics

| Document | Lines | Size | Purpose |
|----------|-------|------|---------|
| ARCHITECTURE_MAP.md | 969 | 31 KB | Complete architecture overview |
| PACKAGE_INDEX.md | 267 | 9.6 KB | Quick reference tables & patterns |
| LIFECYCLE_AND_PATTERNS.md | 750 | 26 KB | Runtime behavior & sequences |
| **Total** | **1,986** | **67 KB** | Complete architecture documentation |

---

## Tips for Navigating the Codebase

1. **Follow the imports** – Start from `cmd/claudio/main.go` and trace imports
2. **Use grep for types** – `grep "type App struct"` to find definitions
3. **Look for Execute methods** – Most tools have an `Execute(input string) (string, error)` method
4. **Check for interfaces** – Tool, Provider, EventHandler are key interfaces
5. **Watch for callbacks** – onTurnEnd, onAutoCompact, mailboxPoller are key callbacks
6. **Use package names as hints** – `internal/services/` contains support services, `internal/tools/` contains tools

---

## Getting Help

- **How does X work?** → Check [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) for your X
- **What does package Y do?** → Check [PACKAGE_INDEX.md](PACKAGE_INDEX.md) table
- **What happens when Z occurs?** → Check [LIFECYCLE_AND_PATTERNS.md](LIFECYCLE_AND_PATTERNS.md)
- **Where is [file/type]?** → Use grep or [PACKAGE_INDEX.md](PACKAGE_INDEX.md) file column

