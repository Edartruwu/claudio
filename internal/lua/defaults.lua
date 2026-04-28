-- Claudio default configuration
-- This runs before ~/.claudio/init.lua — user can override anything here

claudio.config.set("model",             "claude-sonnet-4-6")
claudio.config.set("smallModel",        "claude-haiku-4-5-20251001")
claudio.config.set("permissionMode",    "default")
claudio.config.set("compactMode",       "strategic")
claudio.config.set("compactKeepN",      5)
claudio.config.set("sessionPersist",    true)
claudio.config.set("hookProfile",       "standard")
claudio.config.set("autoCompact",       false)
claudio.config.set("caveman",           false)
claudio.config.set("outputStyle",       "normal")
claudio.config.set("outputFilter",      true)
claudio.config.set("autoMemoryExtract", false)
claudio.config.set("memorySelection",   "none")

-- Default colorscheme: Gruvbox Dark
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
claudio.ui.set_border("rounded")

-- Leader keymaps (Space as leader)
-- Users can override or remove any binding via claudio.keymap.map / claudio.keymap.unmap.

-- Windows
claudio.keymap.map("<space>ww", "window.cycle")
claudio.keymap.map("<space>wh", "window.focus.left")
claudio.keymap.map("<space>wj", "window.focus.down")
claudio.keymap.map("<space>wk", "window.focus.up")
claudio.keymap.map("<space>wl", "window.focus.right")
claudio.keymap.map("<space>wp", "window.focus.down")
claudio.keymap.map("<space>wv", "window.split.v")
claudio.keymap.map("<space>wq", "window.close")
claudio.keymap.map("<space>wc", "window.float.close")
claudio.keymap.map("<space>wo", "window.float.hint")

-- Buffers / Sessions
claudio.keymap.map("<space>bn", "buffer.next")
claudio.keymap.map("<space>bp", "buffer.prev")
claudio.keymap.map("<space>bc", "buffer.new")
claudio.keymap.map("<space>bk", "buffer.close")
claudio.keymap.map("<space>br", "buffer.rename")
claudio.keymap.map("<space>bl", "buffer.list")

-- Panels
claudio.keymap.map("<space>K",  "panel.skills")
claudio.keymap.map("<space>M",  "panel.memory")
claudio.keymap.map("<space>T",  "panel.tasks")
claudio.keymap.map("<space>O",  "panel.tools")
claudio.keymap.map("<space>A",  "panel.analytics")
claudio.keymap.map("<space>f",  "panel.files")
claudio.keymap.map("<space>op", "panel.session_tree")
claudio.keymap.map("<space>oa", "panel.agent_gui")
claudio.keymap.map("<space>ik", "panel.skills")
claudio.keymap.map("<space>im", "panel.memory")
claudio.keymap.map("<space>ia", "panel.analytics")
claudio.keymap.map("<space>it", "panel.tasks")
claudio.keymap.map("<space>io", "panel.tools")

-- Pickers
claudio.keymap.map("<space>/",  "picker.search")
claudio.keymap.map("<space>;",  "picker.recent")
claudio.keymap.map("<space>p",  "picker.commands")

-- Editor
claudio.keymap.map("<space>e",  "editor.edit")
claudio.keymap.map("<space>ev", "editor.view")

-- Misc
claudio.keymap.map("<space>t",  "todo.toggle")


-- Default sidebar via claudio.win.new_panel
-- Remove or override sections in your ~/.claudio/init.lua.

local sidebar = claudio.win.new_panel({ position = "left", width = 30 })

sidebar:add_section({
  id = "session", title = "Session", weight = 1, min_height = 3,
  render = function(w, h)
    local s = claudio.session.current()
    if s == nil then return "" end
    local lines = {}
    if s.name  and s.name  ~= "" then table.insert(lines, "  " .. s.name)  end
    if s.model and s.model ~= "" then table.insert(lines, "  " .. s.model) end
    return table.concat(lines, "\n")
  end,
})

sidebar:add_section({
  id = "todos", title = "Tasks", weight = 2, min_height = 3,
  render = function(w, h)
    local tasks = claudio.tasks.list()
    if tasks == nil or #tasks == 0 then return "  no tasks" end
    local lines = {}
    for _, t in ipairs(tasks) do
      local icon = t.status == "completed" and "✓" or (t.status == "in_progress" and "●" or "○")
      table.insert(lines, "  " .. icon .. " " .. t.title)
    end
    return table.concat(lines, "\n")
  end,
})

sidebar:add_section({
  id = "files", title = "Files", weight = 3, min_height = 3,
  render = function(w, h)
    local files = claudio.files.list()
    if files == nil or #files == 0 then return "  no files" end
    local lines = {}
    for _, f in ipairs(files) do
      table.insert(lines, "  " .. f)
    end
    return table.concat(lines, "\n")
  end,
})

sidebar:add_section({
  id = "tokens", title = "Tokens", weight = 4, min_height = 2,
  render = function(w, h)
    local t = claudio.tokens.usage()
    if t == nil then return "" end
    return string.format("  %d tok  $%.4f", t.total, t.cost)
  end,
})

-- Branching
claudio.keymap.map("<space>gb", "branch.session")
claudio.keymap.map("<space>gp", "branch.parent-jump")
