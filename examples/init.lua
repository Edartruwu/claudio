-- examples/init.lua
-- Reference config for ~/.claudio/init.lua
-- Copy what you want — everything is optional.
-- Load order: defaults.lua → ~/.claudio/init.lua → plugins → .claudio/init.lua

-- ── Model ────────────────────────────────────────────────────────────────────
claudio.config.set("model",       "claude-sonnet-4-6")
claudio.config.set("smallModel",  "claude-haiku-4-5-20251001")

-- ── Behavior ─────────────────────────────────────────────────────────────────
claudio.config.set("compactMode",       "strategic") -- "off" | "strategic" | "aggressive"
claudio.config.set("compactKeepN",      5)           -- recent messages kept on compact
claudio.config.set("autoCompact",       false)
claudio.config.set("autoMemoryExtract", false)
claudio.config.set("outputFilter",      true)        -- filter noisy tool output
claudio.config.set("caveman",           false)        -- ultra-terse response mode
claudio.config.set("sessionPersist",    true)

-- ── Theme: Gruvbox Dark ───────────────────────────────────────────────────────
claudio.ui.set_theme({
  primary     = "#83a598",
  secondary   = "#d3869b",
  success     = "#b8bb26",
  warning     = "#fabd2f",
  error       = "#fb4934",
  muted       = "#928374",
  surface     = "#282828",
  surface_alt = "#3c3836",
  text        = "#ebdbb2",
  dim         = "#665c54",
  subtle      = "#504945",
  orange      = "#fe8019",
  aqua        = "#8ec07c",
})

-- ── Theme: Tokyo Night (alternative — uncomment to use) ──────────────────────
-- claudio.ui.set_theme({
--   primary     = "#7aa2f7",
--   secondary   = "#bb9af7",
--   success     = "#9ece6a",
--   warning     = "#e0af68",
--   error       = "#f7768e",
--   muted       = "#565f89",
--   surface     = "#1a1b26",
--   surface_alt = "#24283b",
--   text        = "#c0caf5",
--   dim         = "#414868",
--   subtle      = "#9aa5ce",
--   orange      = "#ff9e64",
--   aqua        = "#73daca",
-- })

-- ── Theme: Catppuccin Mocha (alternative) ────────────────────────────────────
-- claudio.ui.set_theme({
--   primary     = "#89b4fa",
--   secondary   = "#cba6f7",
--   success     = "#a6e3a1",
--   warning     = "#f9e2af",
--   error       = "#f38ba8",
--   muted       = "#6c7086",
--   surface     = "#1e1e2e",
--   surface_alt = "#313244",
--   text        = "#cdd6f4",
--   dim         = "#585b70",
--   subtle      = "#45475a",
--   orange      = "#fab387",
--   aqua        = "#94e2d5",
-- })

-- Border: "rounded" (default), "block", "double", "normal", "hidden"
claudio.ui.set_border("rounded")

-- ── Sidebar ───────────────────────────────────────────────────────────────────
-- The sidebar is pure Lua. Four blocks ship in defaults.lua:
--   "files"   (weight 3) — tracked files in the current session
--   "session" (weight 1) — current session name + model
--   "todos"   (weight 2) — active tasks (○ pending  ● in_progress  ✓ done)
--   "tokens"  (weight 1) — token usage + cost
--
-- weight controls vertical order — higher weight = rendered lower.
-- render(w, h) receives available width/height; return a plain string.
-- Re-registering with the same id replaces the default block.

-- Disable the sidebar entirely:
-- claudio.ui.set_sidebar_enabled(false)

-- Remove a single default block (e.g. hide token counter):
-- claudio.ui.remove_sidebar_block("tokens")

-- Override the session block to show more detail:
-- claudio.ui.register_sidebar_block({
--   id     = "session",
--   title  = "Session",
--   weight = 1,
--   render = function(w, h)
--     local s = claudio.session.current()
--     if s == nil then return "" end
--     local lines = {}
--     if s.name  ~= "" then table.insert(lines, "  " .. s.name)  end
--     if s.model ~= "" then table.insert(lines, "  " .. s.model) end
--     local t = claudio.tokens.usage()
--     if t then
--       table.insert(lines, string.format("  $%.4f", t.cost))
--     end
--     return table.concat(lines, "\n")
--   end,
-- })

-- Add a git status block:
-- claudio.ui.register_sidebar_block({
--   id     = "git",
--   title  = "Git",
--   weight = 4,
--   render = function(w, h)
--     local branch = claudio.cmd("git rev-parse --abbrev-ref HEAD 2>/dev/null"):gsub("\n","")
--     local stats  = claudio.cmd("git diff --shortstat 2>/dev/null"):gsub("\n","")
--     if branch == "" then return "  not a repo" end
--     local lines = { "   " .. branch }
--     if stats ~= "" then table.insert(lines, "  " .. stats) end
--     local staged = claudio.cmd("git diff --cached --shortstat 2>/dev/null"):gsub("\n","")
--     if staged ~= "" then table.insert(lines, "  staged: " .. staged) end
--     return table.concat(lines, "\n")
--   end,
-- })

-- Add a clock block:
-- claudio.ui.register_sidebar_block({
--   id     = "clock",
--   title  = "Time",
--   weight = 10,
--   render = function(w, h)
--     return "  " .. os.date("%H:%M")
--   end,
-- })

-- Add a weather block (requires curl):
-- claudio.ui.register_sidebar_block({
--   id     = "weather",
--   title  = "Weather",
--   weight = 10,
--   render = function(w, h)
--     local out = claudio.cmd("curl -sf 'wttr.in/?format=3' 2>/dev/null")
--     return out ~= "" and ("  " .. out) or "  unavailable"
--   end,
-- })

-- ── Keymaps ──────────────────────────────────────────────────────────────────
-- Override or extend the default Space-leader bindings from defaults.lua.
-- Full list of actions: https://github.com/Abraxas-365/claudio/blob/main/docs/actions.md

-- claudio.keymap.map("<space>gh", "git.history")
-- claudio.keymap.unmap("<space>T")          -- remove a default binding

-- ── Skills ───────────────────────────────────────────────────────────────────
-- Personal skills injected into every agent session.
-- claudio.register_skill({
--   name    = "my-standards",
--   content = "Always write tests. Always document public APIs.",
-- })

-- ── Hooks ────────────────────────────────────────────────────────────────────
-- React to Claudio events (tool.executed, session.started, agent.completed, …)
-- claudio.subscribe("tool.executed", function(event)
--   claudio.log("ran tool: " .. tostring(event.tool_name))
-- end)

-- ── Custom Provider ──────────────────────────────────────────────────────────
-- Add an OpenAI-compatible provider (Groq, Ollama, Together, etc.)
-- claudio.register_provider({
--   name     = "groq",
--   type     = "openai",
--   base_url = "https://api.groq.com/openai/v1",
--   api_key  = "$GROQ_API_KEY",
--   routes   = { "llama-*", "mixtral-*" },
-- })

-- ── Plugins ──────────────────────────────────────────────────────────────────
-- Community plugins live in ~/.claudio/plugins/*/init.lua
-- local myplugin = require("plugins.myplugin")
-- myplugin.setup({ token = claudio.get_config("myplugin", "token") })
