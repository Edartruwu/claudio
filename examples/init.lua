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

-- ── Sidebar (via claudio.win.new_panel) ──────────────────────────────────────
-- The sidebar is a panel created by defaults.lua via claudio.win.new_panel.
-- Four sections ship by default: session, tasks, files, tokens.
--
-- Sections have weight (vertical order), min_height, and a render(w, h) fn.
-- You can create additional panels at any position: "left", "right", "bottom".

-- Create a custom sidebar panel (overrides the default):
-- local my_sidebar = claudio.win.new_panel({ position = "left", width = 30 })
-- my_sidebar:add_section({
--   id = "session", title = "Session", weight = 1, min_height = 3,
--   render = function(w, h)
--     local s = claudio.session.current()
--     if s == nil then return "" end
--     local lines = {}
--     if s.name  and s.name  ~= "" then table.insert(lines, "  " .. s.name)  end
--     if s.model and s.model ~= "" then table.insert(lines, "  " .. s.model) end
--     local t = claudio.tokens.usage()
--     if t then table.insert(lines, string.format("  $%.4f", t.cost)) end
--     return table.concat(lines, "\n")
--   end,
-- })

-- Add a git status section:
-- my_sidebar:add_section({
--   id = "git", title = "Git", weight = 5, min_height = 3,
--   render = function(w, h)
--     local branch = claudio.cmd("git rev-parse --abbrev-ref HEAD 2>/dev/null"):gsub("\n","")
--     local stats  = claudio.cmd("git diff --shortstat 2>/dev/null"):gsub("\n","")
--     if branch == "" then return "  not a repo" end
--     local lines = { "   " .. branch }
--     if stats ~= "" then table.insert(lines, "  " .. stats) end
--     return table.concat(lines, "\n")
--   end,
-- })

-- Hide/show a panel at runtime:
-- my_sidebar:hide()
-- my_sidebar:show()
-- my_sidebar:toggle()

-- Remove a section:
-- my_sidebar:remove_section("tokens")

-- Bottom panel example:
-- local bottom = claudio.win.new_panel({ position = "bottom", height = 8 })
-- bottom:add_section({
--   id = "clock", title = "Time", weight = 1, min_height = 1,
--   render = function(w, h) return "  " .. os.date("%H:%M") end,
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
