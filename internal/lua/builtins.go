package lua

import "log"

// sidebarLua is the built-in sidebar plugin that registers the 4 default
// sidebar blocks (Files, Session, Tasks, Tokens) using the data provider APIs.
// It runs after plugins load so user plugins can override individual blocks.
const sidebarLua = `
-- Built-in sidebar plugin — registers default blocks via data provider APIs.
-- User plugins can override by registering blocks with the same id.

-- Files block (weight=3)
claudio.ui.register_sidebar_block({
  id     = "files",
  title  = "Files",
  weight = 3,
  render = function(w, h)
    local files = claudio.files.list()
    if #files == 0 then return "  No files yet" end
    local lines = {}
    for i, f in ipairs(files) do
      if i > h then
        table.insert(lines, "  +" .. (#files - h) .. " more")
        break
      end
      local prefix = f.is_dir and "  📁 " or "  📄 "
      table.insert(lines, prefix .. f.name)
    end
    return table.concat(lines, "\n")
  end
})

-- Session block (weight=1)
claudio.ui.register_sidebar_block({
  id     = "session",
  title  = "Session",
  weight = 1,
  render = function(w, h)
    local s = claudio.session.current()
    if s == nil then return "" end
    local lines = {}
    if s.name ~= "" then
      table.insert(lines, s.name)
    end
    if s.model ~= "" then
      table.insert(lines, s.model)
    end
    return table.concat(lines, "\n")
  end
})

-- Tasks block (weight=2)
claudio.ui.register_sidebar_block({
  id     = "todos",
  title  = "Tasks",
  weight = 2,
  render = function(w, h)
    local tasks = claudio.tasks.list()
    if #tasks == 0 then return "No tasks" end
    local lines = {}
    for i, t in ipairs(tasks) do
      if i > h then break end
      local icon = "○"
      if t.status == "completed" then
        icon = "✓"
      elseif t.status == "in_progress" then
        icon = "●"
      end
      table.insert(lines, icon .. " " .. t.title)
    end
    return table.concat(lines, "\n")
  end
})

-- Tokens block (weight=1)
claudio.ui.register_sidebar_block({
  id     = "tokens",
  title  = "Tokens",
  weight = 1,
  render = function(w, h)
    local u = claudio.tokens.usage()
    if u == nil then return "" end
    local pct = 0
    if u.max > 0 then
      pct = math.floor(u.used / u.max * 100)
    end
    return string.format("%d / %d (%d%%)\n$%.4f", u.used, u.max, pct, u.cost)
  end
})
`

// LoadBuiltins executes embedded built-in Lua plugins (e.g. sidebar.lua).
// Call after plugins have been loaded so built-in blocks act as defaults
// that user plugins can override by registering blocks with the same id.
func (r *Runtime) LoadBuiltins() {
	if err := r.execString(sidebarLua, "builtin:sidebar.lua"); err != nil {
		log.Printf("[lua] builtin sidebar.lua: %v", err)
	}
}
