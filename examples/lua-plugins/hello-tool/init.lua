-- hello-tool: minimal example plugin
--
-- Registers a single "hello" tool that returns a greeting.
-- This is the simplest possible Claudio plugin — a good starting point.
--
-- Install:
--   claudio plugin install ./examples/lua-plugins/hello-tool
--
-- Usage (in a session):
--   Use the hello tool with name="Alice"

claudio.log("hello-tool: loading")

claudio.register_tool({
  name        = "hello",
  description = "Returns a personalized greeting. Pass { name: string } as input.",

  -- JSON Schema describing the expected input.
  -- Agents use this to know what fields to pass.
  schema = '{"type":"object","properties":{"name":{"type":"string","description":"Name to greet"}},"required":[]}',

  -- execute is called every time an agent invokes this tool.
  -- `input` is a Lua table decoded from the JSON input the agent sends.
  execute = function(input)
    local name = input.name or "world"

    -- Return a table with a `content` string.
    -- Set is_error = true to signal failure to the agent.
    return { content = "Hello, " .. name .. "! Greetings from a Lua plugin." }
  end,
})

claudio.log("hello-tool: registered 'hello' tool")
