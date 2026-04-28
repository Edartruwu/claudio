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

-- Default colorscheme
claudio.ui.set_theme({
  primary     = "#7aa2f7",
  secondary   = "#bb9af7",
  success     = "#9ece6a",
  warning     = "#e0af68",
  error       = "#f7768e",
  muted       = "#565f89",
  surface     = "#1a1b26",
  surface_alt = "#24283b",
  text        = "#c0caf5",
  dim         = "#414868",
  subtle      = "#9aa5ce",
  orange      = "#ff9e64",
  aqua        = "#73daca",
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
claudio.keymap.map("<space>a",  "picker.agents")
claudio.keymap.map("<space>/",  "picker.search")
claudio.keymap.map("<space>;",  "picker.recent")
claudio.keymap.map("<space>.",  "picker.buffers")
claudio.keymap.map("<space>p",  "picker.commands")

-- Editor
claudio.keymap.map("<space>e",  "editor.edit")
claudio.keymap.map("<space>ev", "editor.view")

-- Misc
claudio.keymap.map("<space>t",  "todo.toggle")
