-- custom-capability: demonstrates declaring a new agent capability from Lua
--
-- This plugin registers two tools and declares them as a "notepad" capability.
-- Agents that list `capabilities: ["notepad"]` in their definition will have
-- access to `notepad_write` and `notepad_read`.
--
-- Install:
--   claudio plugin install ./examples/lua-plugins/custom-capability

claudio.log("custom-capability: loading")

-- In-memory notepad — a simple key/value store for the session.
local notes = {}

-- Tool 1: write a note
claudio.register_tool({
  name        = "notepad_write",
  description = "Write a note to the in-session notepad. Input: { key: string, value: string }",
  schema      = [[{
    "type": "object",
    "properties": {
      "key":   { "type": "string", "description": "Note key" },
      "value": { "type": "string", "description": "Note content" }
    },
    "required": ["key", "value"]
  }]],
  execute = function(input)
    if not input.key or input.key == "" then
      return { content = "error: key is required", is_error = true }
    end
    notes[input.key] = input.value
    claudio.log("custom-capability: wrote note '" .. input.key .. "'")
    return { content = "Saved note '" .. input.key .. "'" }
  end,
})

-- Tool 2: read a note
claudio.register_tool({
  name        = "notepad_read",
  description = "Read a note from the in-session notepad. Input: { key: string }",
  schema      = [[{
    "type": "object",
    "properties": {
      "key": { "type": "string", "description": "Note key to read" }
    },
    "required": ["key"]
  }]],
  execute = function(input)
    if not input.key or input.key == "" then
      return { content = "error: key is required", is_error = true }
    end
    local val = notes[input.key]
    if val == nil then
      return { content = "No note found for key '" .. input.key .. "'" }
    end
    return { content = val }
  end,
})

-- Declare the "notepad" capability on the event bus.
-- The capability registry listens for this event and makes the capability
-- available for agents to opt into via `capabilities: ["notepad"]`.
claudio.publish("capability.declared", {
  name        = "notepad",
  description = "In-session notepad: read and write ephemeral notes",
  tools       = { "notepad_write", "notepad_read" },
})

-- Optionally register a skill that teaches agents how to use this capability.
claudio.register_skill({
  name         = "use-notepad",
  description  = "How to use the notepad capability to persist notes across tool calls",
  capabilities = { "notepad" },
  content      = [[
# Using the Notepad

The notepad capability gives you two tools for ephemeral in-session storage:

- **notepad_write** `{ key, value }` — save a string value under a key
- **notepad_read** `{ key }` — retrieve a previously saved value

## Example workflow

1. After gathering information, save it: `notepad_write { key: "summary", value: "..." }`
2. Later in the session, retrieve it: `notepad_read { key: "summary" }`
3. Notes are lost when the session ends — do not rely on them for persistence.
]],
})

claudio.notify("Notepad capability ready", "info")
claudio.log("custom-capability: registered notepad_write, notepad_read, and 'use-notepad' skill")
