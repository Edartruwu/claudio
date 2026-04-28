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
