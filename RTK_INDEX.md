# RTK Project Exploration - Complete Index

## 📋 Overview

This directory contains a comprehensive exploration of the **RTK (Rust Token Killer)** project — a high-performance Rust CLI proxy that minimizes LLM token consumption by filtering and compressing command outputs.

**Source**: `/Users/abraxas/Personal/rtk/` (main repository)  
**Version**: 0.35.0  
**Language**: 100% Rust (~21,000 lines)  
**Binaries**: <5MB stripped, <10ms startup overhead

---

## 📁 Documentation Files

### 1. **RTK_EXPLORATION_REPORT.md** (644 lines)
Complete architectural overview and project documentation.

**Contents**:
- Executive summary & key metrics
- Full directory structure (src/ with 11 modules)
- Detailed module breakdown (analytics, cmds, core, discover, hooks, learn, parser)
- 8 command ecosystems (Git, Rust, JS/TS, Python, Ruby, Go, .NET, Cloud)
- 100+ supported commands with features
- Token savings benchmarks (60-90%)
- Technical architecture & design patterns
- Build process & development workflow
- Future roadmap

**Best for**: Understanding project architecture, module organization, design patterns

---

### 2. **RTK_COMMANDS_MATRIX.md** (415 lines)
Quick-reference matrix of all 100+ commands organized by category.

**Contents**:
- 50+ command tables by category (File, Git, Rust, JS, Python, Ruby, Go, .NET, Cloud)
- Each command shows: arguments, output, token savings, special features
- Token savings benchmarks per operation (e.g., `ls` -80%, `cargo test` -90%)
- Global flags & argument types
- Command routing decision tree
- Filter coverage by ecosystem
- Installation methods comparison
- Hook agent support matrix (6 agents)
- Error recovery features
- Configuration sections

**Best for**: Quick command lookup, finding token savings, agent support details

---

### 3. **RTK_INDEX.md** (this file)
Navigation guide and summary of exploration documents.

---

## 🗂️ Source Code Reference

### Key Files Referenced

| File | Lines | Purpose |
|------|-------|---------|
| `src/main.rs` | ~800 | CLI router, 50+ command variants |
| `src/cmds/mod.rs` | 10 | Module organization |
| `src/cmds/*/` | ~100 files | 8 language ecosystem filters |
| `src/core/mod.rs` | ~20 | Infrastructure exports |
| `src/core/tracking.rs` | ~500 | SQLite token tracking |
| `src/core/filter.rs` | ~400 | TOML filter engine |
| `src/core/utils.rs` | ~300 | Shared utilities |
| `src/analytics/gain.rs` | ~600 | Token savings dashboard |
| `src/discover/mod.rs` | ~5 files | Command rewriting engine |
| `src/hooks/init.rs` | ~1000 | Hook installation |
| `src/parser/types.rs` | ~200 | Output parsing trait |
| `src/filters/` | 70+ files | Built-in TOML filters |

### Source Locations

**Main RTK Repository**:
```
/Users/abraxas/Personal/rtk/
├── src/
│   ├── main.rs              ← CLI router
│   ├── cmds/                ← Command implementations (100+ supported)
│   │   ├── git/, rust/, js/, python/, ruby/, go/, dotnet/, cloud/, system/
│   ├── core/                ← Infrastructure (config, tracking, filters)
│   ├── discover/            ← Command rewriting engine
│   ├── analytics/           ← Token savings dashboards
│   ├── hooks/               ← LLM agent integration
│   ├── learn/               ← CLI correction detection
│   ├── parser/              ← Output parsing framework
│   └── filters/             ← 70+ built-in TOML filters
├── Cargo.toml               ← Dependencies, metadata
├── CLAUDE.md                ← Claude Code guidance
├── README.md                ← Project README
├── CONTRIBUTING.md          ← Contribution guide
└── docs/                    ← Architecture docs
```

---

## 🎯 Quick Navigation

### I want to...

#### Understand the project structure
→ Read **RTK_EXPLORATION_REPORT.md** sections:
- "Directory Structure"
- "Module Breakdown"
- "Technical Architecture"

#### Find a specific command
→ Use **RTK_COMMANDS_MATRIX.md** and CTRL+F:
- Search by command name (e.g., "rtk cargo test")
- Search by ecosystem (e.g., "JavaScript")
- Search by output type (e.g., "Failures only")

#### Learn about token savings
→ Check **RTK_COMMANDS_MATRIX.md**:
- "Token Savings Benchmark" table
- Per-command savings in command tables
- Installation method comparison

#### Understand how hooks work
→ Read **RTK_EXPLORATION_REPORT.md** sections:
- "src/hooks/" module details
- "Hook Agent Support Matrix" in MATRIX doc
- Installation modes details

#### Find architecture details
→ Read **RTK_EXPLORATION_REPORT.md**:
- "Technical Architecture"
- "Design Patterns"
- Module-specific sections in "Module Breakdown"

#### Learn about filtering strategies
→ Read **RTK_EXPLORATION_REPORT.md**:
- "Key Features & Capabilities" → "Filtering Strategies"
- Individual ecosystem module details
- "Core Infrastructure" section

#### Understand the rewrite system
→ Read **RTK_EXPLORATION_REPORT.md**:
- "src/discover/" module breakdown
- "Command Rewriting Flow" in architecture section

---

## 📊 Statistics Summary

| Metric | Value |
|--------|-------|
| Total Rust lines | ~21,000 |
| Rust source files | 100+ |
| Built-in TOML filters | 70+ |
| Supported commands | 100+ |
| Subcommands | 50+ |
| Regex rewrite rules | 60+ |
| Command ecosystems | 8 |
| LLM agent hooks | 6+ |
| Startup time | <10ms |
| Binary size (stripped) | <5MB |
| Token savings | 60-90% |

---

## 🏗️ Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        RTK (main.rs)                        │
│                      CLI Command Router                      │
└────────┬────────┬────────┬────────┬────────┬────────┬────────┘
         │        │        │        │        │        │
    ┌────▼──┬─────▼──┬────▼──┬────▼──┬────▼──┬────▼──┬────▼──┐
    │ Git   │ Rust   │  JS   │ Python│ Ruby  │  Go   │.NET   │
    │ (8)   │  (4)   │ (10)  │  (5)  │  (3)  │  (4)  │  (4)  │
    └────┬──┴────┬───┴───┬───┴───┬───┴───┬───┴───┬───┴───┬───┘
         │       │       │       │       │       │       │
    ┌────▼───────▼───────▼───────▼───────▼───────▼───────▼──┐
    │                  CORE INFRASTRUCTURE                   │
    │  ┌─────────────┬──────────┬───────────┬──────────────┐ │
    │  │  Tracking   │ Filtering │ Config    │ Display      │ │
    │  │  (SQLite)   │  (TOML)   │  (TOML)   │ Helpers      │ │
    │  └─────────────┴──────────┴───────────┴──────────────┘ │
    └────┬────────────────────────────────────────────────┬───┘
         │                                                │
    ┌────▼──────────────┐  ┌─────────────────────────────▼─┐
    │   DISCOVER        │  │      ANALYTICS                 │
    │ (Rewriting)       │  │ (Dashboards, Reporting)        │
    │ ┌──────────────┐  │  │ ┌──────────────────────────┐   │
    │ │ Lexer        │  │  │ │ Gain (savings)           │   │
    │ │ Registry     │  │  │ │ Discover (opportunities) │   │
    │ │ Rules        │  │  │ │ Session (adoption)       │   │
    │ │ Provider     │  │  │ │ Learn (corrections)      │   │
    │ └──────────────┘  │  │ └──────────────────────────┘   │
    └───────────────────┘  └────────────────────────────────┘
         │
    ┌────▼─────────────────────────────────────────────────┐
    │                      HOOKS                           │
    │  ┌─────────────┬──────────┬──────────┬─────────────┐ │
    │  │ Init        │ Integrity│ Rewrite  │ Permissions │ │
    │  │ Trust       │ (SHA256) │  Bridge  │ (deny/ask)  │ │
    │  └─────────────┴──────────┴──────────┴─────────────┘ │
    │  Supports: Claude, Cursor, Windsurf, Cline,         │
    │            Gemini, Copilot, Codex                    │
    └─────────────────────────────────────────────────────┘
```

---

## 🔑 Key Concepts

### 1. **Token Counting**
- Algorithm: `ceil(chars / 4.0)` (approximate)
- Tracks: Original vs filtered token counts
- Stored: SQLite tracking database

### 2. **TOML Filter Pipeline**
8-stage sequential processing:
1. Strip ANSI codes
2. Regex replacements
3. Short-circuit rules
4. Strip/keep lines by regex
5. Truncate long lines
6. Head/tail line selection
7. Absolute line cap
8. Empty fallback message

### 3. **Command Rewriting**
- Lexer tokenizes shell syntax
- Registry classifies against 60+ patterns
- Splits on operators (`&&`, `||`, `;`) and pipes (`|`)
- Rewrites each segment independently
- Preserves env vars, redirects, quotes

### 4. **Three-Tier Parsing**
- **Tier 1 (Full)**: Complete JSON parsing
- **Tier 2 (Degraded)**: Regex fallback
- **Tier 3 (Passthrough)**: Truncated raw output

### 5. **Hook Integrity**
- SHA-256 verification
- Tamper detection
- Permission model (deny > ask > allow)
- Exit code contract

---

## 📚 Related Documentation in RTK Repo

| File | Purpose |
|------|---------|
| `CLAUDE.md` | Claude Code integration guide |
| `README.md` | Project overview & quick start |
| `CONTRIBUTING.md` | Contribution guidelines |
| `docs/contributing/ARCHITECTURE.md` | Full architecture doc |
| `docs/contributing/TECHNICAL.md` | Technical deep dive |
| `CHANGELOG.md` | Release history (63KB) |
| `src/cmds/README.md` | Command module patterns |
| `src/core/README.md` | Infrastructure details |
| `src/analytics/README.md` | Analytics module design |
| `src/hooks/README.md` | Hook system details |
| `src/discover/README.md` | Rewrite engine details |
| `src/parser/README.md` | Parser infrastructure |
| `src/filters/README.md` | TOML filter guide |
| `src/learn/README.md` | Correction detection |

---

## 🛠️ Development Commands

```bash
# Build & run
cargo build                          # Debug
cargo build --release                # Optimized
cargo run -- <cmd>                   # Direct execution

# Testing & quality
cargo test                           # All tests
cargo test <test_name>               # Specific test
cargo fmt                            # Format
cargo clippy --all-targets           # Lint
rtk cargo build                       # Token-optimized

# Pre-commit gate
cargo fmt --all && cargo clippy --all-targets && cargo test --all

# Performance testing
hyperfine 'rtk git log -10' --warmup 3

# Installation
cargo install --path .               # Local install
cargo install --git https://github.com/rtk-ai/rtk  # From repo
```

---

## 🌳 Directory Tree (src/)

```
src/
├── main.rs                          # CLI entry point
├── analytics/
│   ├── mod.rs
│   ├── gain.rs                      # Token savings dashboard
│   ├── cc_economics.rs              # Cost reduction
│   ├── ccusage.rs                   # Claude Code spending
│   └── session_cmd.rs               # Adoption metrics
├── cmds/
│   ├── mod.rs
│   ├── README.md
│   ├── cloud/                       # 5 cloud commands
│   │   ├── aws_cmd.rs
│   │   ├── container.rs
│   │   ├── curl_cmd.rs
│   │   ├── psql_cmd.rs
│   │   └── wget_cmd.rs
│   ├── dotnet/                      # 5 .NET commands
│   ├── git/                         # 8 Git subcommands
│   │   ├── git.rs
│   │   ├── diff_cmd.rs
│   │   ├── gh_cmd.rs
│   │   └── gt_cmd.rs
│   ├── go/                          # 4 Go commands
│   ├── js/                          # 10 JS/TS commands
│   │   ├── npm_cmd.rs
│   │   ├── pnpm_cmd.rs
│   │   ├── vitest_cmd.rs
│   │   ├── playwright_cmd.rs
│   │   ├── tsc_cmd.rs
│   │   ├── lint_cmd.rs
│   │   ├── prettier_cmd.rs
│   │   ├── prisma_cmd.rs
│   │   └── next_cmd.rs
│   ├── python/                      # 5 Python commands
│   ├── ruby/                        # 3 Ruby commands
│   ├── rust/                        # 4 Rust commands
│   │   ├── cargo_cmd.rs
│   │   ├── runner.rs
│   │   └── mod.rs
│   └── system/                      # 15 system commands
│       ├── ls.rs, tree.rs, read.rs
│       ├── find_cmd.rs, grep_cmd.rs
│       ├── json_cmd.rs, log_cmd.rs
│       ├── env_cmd.rs, deps.rs
│       └── ...
├── core/
│   ├── mod.rs
│   ├── config.rs                    # Configuration loading
│   ├── tracking.rs                  # SQLite database
│   ├── filter.rs                    # TOML filter engine
│   ├── runner.rs                    # Command execution
│   ├── tee.rs                       # Output recovery
│   ├── telemetry.rs                 # Usage tracking
│   ├── utils.rs                     # Shared utilities
│   └── ...
├── discover/                        # Command rewriting
│   ├── lexer.rs                     # Shell tokenization
│   ├── registry.rs                  # Classifier
│   ├── rules.rs                     # 60+ rewrite rules
│   ├── provider.rs                  # Session extraction
│   └── report.rs                    # Reporting
├── filters/                         # 70+ TOML filters
│   ├── README.md
│   ├── brew-install.toml
│   ├── docker-compose.toml
│   ├── terraform-plan.toml
│   └── ... (60+ more)
├── hooks/                           # LLM agent integration
│   ├── init.rs                      # Installation flows
│   ├── integrity.rs                 # SHA-256 verification
│   ├── permissions.rs               # Permission model
│   ├── rewrite_cmd.rs               # CLI bridge
│   ├── hook_cmd.rs                  # Gemini/Copilot
│   └── ...
├── learn/                           # Correction detection
│   ├── detector.rs
│   └── report.rs
└── parser/                          # Output parsing
    ├── types.rs                     # Parser trait
    ├── formatter.rs                 # Format modes
    └── mod.rs
```

---

## 📖 How to Use These Documents

### For Architecture Questions
1. Start with **RTK_EXPLORATION_REPORT.md** "Technical Architecture" section
2. Dig into specific module sections for details
3. Check source code links for implementation

### For Command Usage Questions
1. Use **RTK_COMMANDS_MATRIX.md** and search for command
2. Check savings %, features, and arguments
3. Look at examples in command tables

### For Integration Questions
1. Check "Hook Agent Support Matrix" in MATRIX doc
2. Read "src/hooks/" section in EXPLORATION doc
3. See installation modes comparison in MATRIX doc

### For Development/Contribution
1. Read CONTRIBUTING.md in RTK repo
2. Check relevant module README.md files
3. Review CLAUDE.md for coding patterns
4. See "Coding Rules" in CLAUDE.md

---

## 🔗 External References

**Official**:
- Website: https://www.rtk-ai.app
- GitHub: https://github.com/rtk-ai/rtk
- Releases: https://github.com/rtk-ai/rtk/releases
- Discord: https://discord.gg/RySmvNF5kF

**Installation**:
- Homebrew: `brew install rtk`
- Cargo: `cargo install --git https://github.com/rtk-ai/rtk`
- Download: See releases page

---

## ✅ Exploration Completeness

This exploration covers:

- ✅ All 11 source modules
- ✅ 100+ supported commands
- ✅ 8 language ecosystems
- ✅ 70+ built-in TOML filters
- ✅ 6+ LLM agent hooks
- ✅ Technical architecture
- ✅ Design patterns & infrastructure
- ✅ Token savings analytics
- ✅ Build & development process
- ✅ Command rewriting engine
- ✅ Configuration & permissions

**Not covered** (available in source):
- Implementation details (see source files)
- Inline code comments (see source)
- Test fixtures (see tests/ directory)
- Git history/changelog details (see CHANGELOG.md)

---

## 📝 Notes

- All documentation reflects RTK version **0.35.0**
- Source location: `/Users/abraxas/Personal/rtk/`
- Analysis date: April 2024
- Exploration performed via code analysis, README examination, and architecture documentation review

---

**Last Updated**: April 11, 2024  
**Exploration Type**: Complete codebase analysis  
**Documentation**: 3 comprehensive markdown files (1,472 total lines)
