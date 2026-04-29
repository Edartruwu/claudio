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


-- No default sidebar. Create your own in ~/.claudio/init.lua:
--   local sidebar = claudio.win.new_panel({ position = "left", width = 30 })
--   sidebar:add_section({ id = "session", title = "Session", ... })

-- Branching
claudio.keymap.map("<space>gb", "branch.session")
claudio.keymap.map("<space>gp", "branch.parent-jump")

-- Panes (split-pane multi-session)
claudio.keymap.map("<space>wn", "pane.next")
claudio.keymap.map("<space>wN", "pane.prev")
claudio.keymap.map("<space>wx", "pane.close")
