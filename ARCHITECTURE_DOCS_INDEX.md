# Claudio Architecture Documentation Index

Complete reference for all architecture documentation files. **Start here.**

## 📚 Main Architecture Docs (NEW)

These four documents form the core architecture reference:

### 1. [ARCHITECTURE_MAP.md](ARCHITECTURE_MAP.md) — 969 lines, 31 KB
**Complete architectural blueprint**

- 5-layer architecture overview (entry → app → execution → tools → UI)
- Application startup flow (cmd/claudio → cli → app.New)
- App initialization (20+ subsystems wired)
- Query engine loop (streaming, tool execution, stop reasons)
- Tool system (registry, 50+ tools, deferred loading, security)
- Permission system (content-pattern rule evaluation)
- Wiring patterns (event bus, hooks, security context)
- Multi-provider API (OpenAI, Anthropic, Ollama)
- Session & memory management
- Teams & agents coordination
- Learning & analytics
- All 3 UI layers (CLI, TUI, Web)
- Security model & error handling
- Performance optimizations
- Full request/response flow diagram

**Best for:** Comprehensive architectural understanding

---

### 2. [PACKAGE_INDEX.md](PACKAGE_INDEX.md) — 267 lines, 9.6 KB
**Quick reference for all 35 packages**

- Foundational packages (config, auth, storage, security)
- API & LLM integration (api, provider, models)
- Execution engine (query, tools, permissions, orchestrator)
- Agents & teams (agents, teams, session, tasks)
- Services (memory, skills, lsp, mcp, analytics, etc.)
- Events & extensibility (bus, hooks, plugins, learning)
- Utility packages (git, rules, snippets, etc.)
- UI layers (cli, tui, web)
- Simplified dependency graph
- 50+ tools categorized by function
- Configuration file reference
- Common architectural patterns (6 key patterns)

**Best for:** Quick lookups, package reference, "where does X live?"

---

### 3. [LIFECYCLE_AND_PATTERNS.md](LIFECYCLE_AND_PATTERNS.md) — 750 lines, 26 KB
**Runtime behavior & interaction sequences**

- Session lifecycle (new and resume)
- Single turn anatomy (6 phases from setup to cleanup)
- All stop reason handlers (tool_use, end_turn, max_tokens)
- Tool execution lifecycle (lookup → validation → execution → capture)
- Security injection details
- Hook firing with context variables
- Memory extraction and injection
- Sub-agent spawning with context decoration
- Team coordination (creation, spawning, mailbox polling)
- Auto-learning/instinct lifecycle
- Prompt caching setup and expiry
- Conversation compaction (95% context trigger)
- Configuration merging and trust flow
- 10 key interaction patterns summary

**Best for:** Understanding "what happens when X occurs?" and runtime flows

---

### 4. [EXPLORATION_GUIDE.md](EXPLORATION_GUIDE.md) — 272 lines, 11 KB
**Navigation guide for architecture documentation**

- Quick start (which doc to read when)
- Documentation map (what each doc contains)
- Quick navigation by task (11 common tasks)
- Architecture patterns reference with code sketches
- Code exploration workflow (5-step process)
- Key takeaways (10 core concepts)
- Navigation tips and grep recipes
- Getting help guide

**Best for:** Figuring out which doc to read and how to navigate the codebase

---

## 🎯 Using This Documentation

### Choose Your Path

**Path A: New to Claudio**
1. Read EXPLORATION_GUIDE.md → Quick Start
2. Read ARCHITECTURE_MAP.md → Executive Summary
3. Skim PACKAGE_INDEX.md → find interesting packages
4. Read LIFECYCLE_AND_PATTERNS.md → understand request flow

**Path B: Need Specific Knowledge**
1. Go to EXPLORATION_GUIDE.md → Quick Navigation by Task
2. Find your task
3. Follow the links provided

**Path C: Quick Reference**
1. Use PACKAGE_INDEX.md for package lookups
2. Use ARCHITECTURE_MAP.md for concept lookups
3. Use LIFECYCLE_AND_PATTERNS.md for flow lookups

---

## 📖 Supporting Documentation

Additional specialized documentation (created in previous explorations):

- **HOOK_SYSTEM_ARCHITECTURE.md** — Deep dive on hook system
- **SESSION_MANAGEMENT_ARCHITECTURE.md** — Session lifecycle details
- **RTK_EXPLORATION_REPORT.md** — Tools system exploration
- **EXPLORATION_SUMMARY.md** — High-level overview

---

## 🗺️ 35 Internal Packages at a Glance

### Foundational (4)
config, auth, storage, security

### API & LLM (3)
api, api/provider, models

### Execution Engine (4)
query, tools, permissions, orchestrator

### Agents & Teams (4)
agents, teams, session, tasks

### Services (12)
memory, skills, lsp, mcp, analytics, cachetracker, compact, difftracker, filtersavings, notifications, naming, toolcache

### Events & Extensions (4)
bus, hooks, plugins, learning

### Utilities (6)
git, ratelimit, rules, snippets, keybindings, utils

### Bridge & Infrastructure (2)
bridge, query

### UI Layers (3)
cli, tui, web

### Meta (1)
app

---

## 🔍 Key Architecture Concepts

### Single App Struct
`internal/app/App` is the dependency injection root—all subsystems are fields of this struct.

### Query Engine Loop
`internal/query/engine.go` contains the main conversation loop:
- Builds system prompt
- Sends to LLM
- Streams response
- Handles tool calls
- Continues until end_turn

### Tool Registry
`internal/tools/registry.go` manages 50+ tools:
- Deferred loading for large tools
- Security context injection
- Input validation against schema
- Result capture (inline or disk-offloaded)

### Permission System
Content-pattern rules evaluated before tool execution:
- Extract content from tool input
- Match against rules
- Return auto/manual/deny behavior

### Wiring Patterns
Key architectural patterns used throughout:
1. Event bus (pub/sub)
2. Hooks (lifecycle shell commands)
3. Security context injection
4. Skill auto-detection
5. Agent overrides
6. Team context decoration

---

## 📊 Documentation Statistics

| Document | Lines | Size | Topic |
|----------|-------|------|-------|
| ARCHITECTURE_MAP.md | 969 | 31 KB | Complete blueprint |
| LIFECYCLE_AND_PATTERNS.md | 750 | 26 KB | Runtime flows |
| EXPLORATION_GUIDE.md | 272 | 11 KB | Navigation |
| PACKAGE_INDEX.md | 267 | 9.6 KB | Quick reference |
| **New Docs Total** | **2,258** | **77.6 KB** | Architecture documentation |

---

## 🚀 Getting Started with Code

### Entry Point
```
cmd/claudio/main.go (7 lines)
  → cli.Execute()
    → internal/cli/root.go (Cobra command)
      → app.New() [dependency injection]
```

### Main Loop
```
query.Engine.Run(ctx, userMessage)
  → Build system prompt
  → Stream from LLM
  → Handle tool calls
  → Continue until end_turn
```

### For Exploring Specific Areas

**Add a new tool:** Read ARCHITECTURE_MAP.md (Tool System), look at bash.go or read.go

**Understand permissions:** Read ARCHITECTURE_MAP.md (Permission System) + LIFECYCLE_AND_PATTERNS.md (Permission Evaluation)

**Add a hook:** Read ARCHITECTURE_MAP.md (Hooks), look at ~/.claudio/hooks.json

**Understand agents:** Read ARCHITECTURE_MAP.md (Agent System) + LIFECYCLE_AND_PATTERNS.md (Sub-Agent Spawning)

**Understand memory:** Read ARCHITECTURE_MAP.md (Session & Memory) + LIFECYCLE_AND_PATTERNS.md (Memory Lifecycle)

---

## 💡 Pro Tips

1. **Start with EXPLORATION_GUIDE.md** — It tells you exactly what to read for your task
2. **Use grep to find types** — `grep "type App struct"` finds the App definition
3. **Follow imports** — The import statements show dependencies clearly
4. **Look for callback functions** — onTurnEnd, onAutoCompact, mailboxPoller are key extension points
5. **Check tool implementations** — bash.go, read.go, agent.go show the pattern
6. **Use PACKAGE_INDEX.md tables** — Quick way to find what package owns what

---

## ❓ Common Questions & Where to Find Answers

| Question | Document | Section |
|----------|----------|---------|
| What is the overall architecture? | ARCHITECTURE_MAP | Executive Summary |
| Which package owns X? | PACKAGE_INDEX | Package tables |
| How does a request flow through the system? | LIFECYCLE_AND_PATTERNS | Request/Response Sequence |
| What packages depend on each other? | PACKAGE_INDEX | Dependency Graph |
| How do tools work? | ARCHITECTURE_MAP | Tool System |
| How do permissions work? | ARCHITECTURE_MAP | Permission System |
| How do teams coordinate? | LIFECYCLE_AND_PATTERNS | Team Coordination |
| Where is the main loop? | ARCHITECTURE_MAP | Query Engine Loop |
| How is memory managed? | LIFECYCLE_AND_PATTERNS | Memory Lifecycle |
| How do hooks execute? | LIFECYCLE_AND_PATTERNS | Hook Execution |
| What is the startup sequence? | ARCHITECTURE_MAP | App Initialization |
| How are tools registered? | ARCHITECTURE_MAP | Tool System |
| What's the database schema? | ARCHITECTURE_MAP | Storage |
| Which agent should I use? | ARCHITECTURE_MAP | Agent System |
| How do I trace a request? | LIFECYCLE_AND_PATTERNS | Request/Response Sequence |

---

## 📝 Document Contents at a Glance

### ARCHITECTURE_MAP.md
✅ Executive summary  
✅ 5-layer architecture  
✅ Startup flow  
✅ App wiring  
✅ Query loop  
✅ Tools  
✅ Permissions  
✅ Hooks  
✅ Memory  
✅ Teams  
✅ API  
✅ Security  
✅ Config  
✅ Full request/response flow  

### PACKAGE_INDEX.md
✅ 35 package summary  
✅ Quick reference tables  
✅ Dependency graph  
✅ 50+ tools list  
✅ Config files  
✅ 6 key patterns  
✅ Entry points  

### LIFECYCLE_AND_PATTERNS.md
✅ Session lifecycle  
✅ Turn anatomy  
✅ Stop reason handlers  
✅ Tool execution  
✅ Security injection  
✅ Hooks firing  
✅ Memory extraction  
✅ Sub-agents  
✅ Teams  
✅ Learning  
✅ Caching  
✅ Compaction  
✅ Config loading  

### EXPLORATION_GUIDE.md
✅ Quick start  
✅ Doc map  
✅ 11 task guides  
✅ Patterns reference  
✅ Exploration workflow  
✅ Takeaways  
✅ Navigation tips  

---

## 🎓 Learning Path

### Level 1: Orientation (30 min)
- Read EXPLORATION_GUIDE.md → Quick Start
- Read ARCHITECTURE_MAP.md → Executive Summary
- Skim PACKAGE_INDEX.md → get a feel for packages

### Level 2: Core Understanding (1-2 hours)
- Read ARCHITECTURE_MAP.md → Application Startup Flow
- Read ARCHITECTURE_MAP.md → Query Engine Loop
- Read LIFECYCLE_AND_PATTERNS.md → Request/Response Sequence
- Understand the main loop

### Level 3: Deep Dive (as needed)
- Pick a subsystem (tools, permissions, teams, memory, etc.)
- Read relevant section from ARCHITECTURE_MAP.md
- Read relevant section from LIFECYCLE_AND_PATTERNS.md
- Look at relevant code files

### Level 4: Implementation (hands-on)
- Follow EXPLORATION_GUIDE.md task guides
- Use PACKAGE_INDEX.md to find relevant files
- Read actual code for that subsystem
- Implement your change

---

## 📞 Using This as a Reference

Bookmark this file and use it as your entry point:
1. Find your question in "Common Questions" table
2. Read the recommended document section
3. Use "Key Takeaways" to solidify understanding
4. Read actual code for details

---

**Last Updated:** April 13, 2025
**Documentation Coverage:** 35 internal packages, 50+ tools, complete startup/request/response flows
**Total Architecture Docs:** 4 core documents + 4 supporting documents

