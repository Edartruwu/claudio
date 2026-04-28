# Lua Plugin System

Claudio supports a Neovim-style plugin system using Lua. Plugins live in
`~/.claudio/lua-plugins/` and are loaded automatically at startup. Each plugin
gets an isolated sandbox with access to the `claudio` global API.

## Quick Start

```bash
claudio plugin install https://github.com/you/my-claudio-plugin
```

Or drop a folder manually:

```
~/.claudio/lua-plugins/
  my-plugin/
    init.lua      ← required entry point
```

## Plugin Structure

Every plugin is a directory containing `init.lua`. The file is executed once at
startup. You can register tools, skills, hooks, and event listeners directly at
the top level — no `setup()` wrapper required, though it is a common convention:

```lua
-- init.lua
local M = {}

claudio.register_tool({
  name        = "greet",
  description = "Returns a greeting",
  execute     = function(input)
    return { content = "Hello, " .. (input.name or "world") .. "!" }
  end,
})

return M
```

## CLI Commands

```bash
# Install a plugin from a Git URL or local path
claudio plugin install https://github.com/user/claudio-myplugin
claudio plugin install ./path/to/local-plugin

# List installed plugins
claudio plugin list

# Show info about a plugin
claudio plugin info myplugin

# Remove a plugin
claudio plugin remove myplugin
```

## `claudio` API Reference

All functions are available via the `claudio` global table injected into every
plugin's sandbox.

---

### `claudio.register_tool(tbl)`

Registers a new tool that agents can call.

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Unique tool name (snake_case) |
| `description` | string | no | Shown to the agent/LLM |
| `schema` | string | no | JSON Schema string for inputs. Defaults to `{}` |
| `execute` | function | yes | Called when the tool runs. Receives a Lua table of inputs, returns a result |

**Return value from `execute`:** a string, or a table with `content` (string) and
optionally `is_error` (boolean).

```lua
claudio.register_tool({
  name        = "read_env",
  description = "Returns a safe environment variable",
  schema      = '{"type":"object","properties":{"key":{"type":"string"}}}',
  execute     = function(input)
    local allowed = { HOME = true, USER = true, SHELL = true }
    if not allowed[input.key] then
      return { content = "not allowed", is_error = true }
    end
    return { content = os.getenv and os.getenv(input.key) or "" }
  end,
})
```

---

### `claudio.register_skill(tbl)`

Registers a skill (instruction set) that agents can use.

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Skill name (used in `claudio skill run <name>`) |
| `description` | string | no | Short description |
| `content` | string | yes | Full skill content (Markdown) |
| `capabilities` | table | no | List of capability names that enable this skill |

```lua
claudio.register_skill({
  name        = "my-workflow",
  description = "My custom workflow",
  content     = [[
# My Workflow

1. Do X
2. Do Y
3. Verify Z
]],
  capabilities = { "design" },
})
```

---

### `claudio.subscribe(eventType, handler)`

Subscribes to an event bus event. The handler is called every time the event
fires. Returns nothing (unsubscription happens automatically on plugin teardown).

```lua
claudio.subscribe("message.sent", function(event)
  -- event.type    string  — event type
  -- event.payload table   — decoded JSON payload (may be nil)
  claudio.log("message sent: " .. tostring(event.payload))
end)
```

Common event types (non-exhaustive):

| Event | Description |
|---|---|
| `message.sent` | A chat message was sent |
| `tool.executed` | A tool finished execution |
| `session.started` | A new session started |
| `notification` | A notification was published |

---

### `claudio.publish(eventType, payload)`

Publishes a custom event on the bus. Use namespaced types to avoid collisions.

```lua
claudio.publish("plugin.my-plugin.task-done", {
  task    = "build",
  success = true,
})
```

---

### `claudio.get_config(key)`

Reads a value from this plugin's config namespace. Returns `nil` if not set.
Config is scoped per plugin — two plugins cannot read each other's config.

```lua
local threshold = claudio.get_config("max_retries") or 3
```

---

### `claudio.set_config(key, value)`

Writes a value to this plugin's config namespace. Accepts strings, numbers,
booleans, and tables.

```lua
claudio.set_config("last_run", "2025-01-01")
claudio.set_config("options", { verbose = true, limit = 10 })
```

---

### `claudio.register_hook(event, matcher, handler)`

Registers a hook that fires before or after a tool runs.

| Arg | Type | Description |
|---|---|---|
| `event` | string | Hook event: `"PreToolUse"` or `"PostToolUse"` |
| `matcher` | string | Tool name to match, or `"*"` for all tools |
| `handler` | function | Called with a context table |

Context table fields:
- `event` — hook event name
- `tool_name` — name of the tool
- `tool_input` — raw input JSON string
- `tool_output` — raw output string (empty for PreToolUse)
- `session_id` — current session ID

```lua
claudio.register_hook("PostToolUse", "Write", function(ctx)
  claudio.log("Write ran in session " .. ctx.session_id)
  claudio.log("Input: " .. ctx.tool_input)
end)

-- Match all tools
claudio.register_hook("PreToolUse", "*", function(ctx)
  claudio.log("About to run: " .. ctx.tool_name)
end)
```

---

### `claudio.notify(message, level)`

Publishes a notification on the bus. The TUI displays these as status messages.

| Arg | Type | Default | Description |
|---|---|---|---|
| `message` | string | — | Notification text |
| `level` | string | `"info"` | `"info"`, `"warn"`, or `"error"` |

```lua
claudio.notify("Plugin loaded successfully")
claudio.notify("Something went wrong", "error")
claudio.notify("Disk space low", "warn")
```

---

### `claudio.log(message)`

Writes a line to the Claudio log (stderr / log file). Prefixed with
`[lua:<plugin-name>]`.

```lua
claudio.log("plugin initializing...")
```

---

## Sandboxing

Each plugin runs in an isolated Lua VM. Only the following stdlib modules are
available:

| Module | Available |
|---|---|
| `base` (print, tostring, pairs, …) | yes |
| `table` | yes |
| `string` | yes |
| `math` | yes |
| `io` | **no** |
| `os` | **no** |
| `package` / `require` | **no** |
| `debug` | **no** |
| `coroutine` | **no** |

Dangerous base functions (`dofile`, `loadfile`, `load`, `loadstring`) are
explicitly removed.

If your plugin needs filesystem access, register a Go-backed tool on the host
side and call it — or request the capability be added to the Claudio core API.

---

## Declaring a Capability

Plugins can declare new capabilities that agents can opt into via their
`capabilities:` field. Use `claudio.publish` to notify the capability registry,
or register tools and group them by naming convention.

The idiomatic pattern is to register all tools for a capability and then publish
a capability-declared event:

```lua
-- Declare the capability by publishing an event
claudio.publish("capability.declared", {
  name        = "database",
  description = "SQL query and schema inspection tools",
  tools       = { "sql_query", "schema_inspect" },
})

-- Register the tools that back it
claudio.register_tool({ name = "sql_query", ... })
claudio.register_tool({ name = "schema_inspect", ... })
```

Agents that list `capabilities: ["database"]` in their definition will then have
access to these tools.

---

## Plugin Development Tips

- **Fail loudly during init.** If your plugin can't start (missing config, etc.),
  call `error("my-plugin: missing required config 'api_key'")` — Claudio will
  log it and skip the plugin cleanly without crashing.

- **Namespace your event types.** Use `plugin.<your-plugin-name>.<event>` to
  avoid conflicts with core events.

- **Config is in-memory only.** `set_config` writes to the in-memory settings
  map — it does not persist across restarts. Use `claudio.publish` to write to
  the plugin_data KV store via a future persistent config API.

- **One VM per plugin.** Plugins cannot communicate via shared Lua globals.
  Use the event bus (`publish`/`subscribe`) for inter-plugin communication.

- **Bus handlers are synchronous.** A slow handler will block bus.Publish.
  Keep handlers fast; offload heavy work if possible.
