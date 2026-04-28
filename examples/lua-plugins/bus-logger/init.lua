-- bus-logger: event-driven plugin example
--
-- Subscribes to Claudio bus events and logs them. Demonstrates:
--   - claudio.subscribe for reactive, event-driven plugins
--   - claudio.get_config / claudio.set_config for lightweight state
--   - claudio.notify to surface info to the user
--
-- Install:
--   claudio plugin install ./examples/lua-plugins/bus-logger

claudio.log("bus-logger: loading")

-- Track how many events we've seen across this session.
-- Note: this is in-memory only — resets on restart.
local event_count = 0

-- Subscribe to message.sent — fires whenever a chat message is sent.
claudio.subscribe("message.sent", function(event)
  event_count = event_count + 1

  -- event.type    — the event type string
  -- event.payload — Lua table decoded from the JSON payload (may be nil)
  local payload_info = "no payload"
  if event.payload then
    payload_info = "payload received"
  end

  claudio.log(string.format(
    "bus-logger: [%s] event #%d — %s",
    event.type, event_count, payload_info
  ))
end)

-- Subscribe to tool.executed — fires after any tool runs.
claudio.subscribe("tool.executed", function(event)
  event_count = event_count + 1

  local tool_name = "unknown"
  if event.payload and event.payload.tool_name then
    tool_name = event.payload.tool_name
  end

  claudio.log(string.format(
    "bus-logger: tool executed — %s (event #%d)",
    tool_name, event_count
  ))
end)

-- Register a tool so agents can ask for a session summary.
claudio.register_tool({
  name        = "bus_logger_stats",
  description = "Returns how many bus events the bus-logger plugin has observed this session.",
  execute     = function(_input)
    return {
      content = string.format(
        "bus-logger has observed %d event(s) this session.",
        event_count
      )
    }
  end,
})

-- Also register a hook to log every Write tool call for audit purposes.
claudio.register_hook("PostToolUse", "Write", function(ctx)
  claudio.log(string.format(
    "bus-logger: Write tool ran in session %s",
    ctx.session_id
  ))

  -- Optionally surface a notification in the TUI.
  -- Comment this out if it's too noisy.
  -- claudio.notify("Write tool executed", "info")
end)

claudio.log("bus-logger: ready — subscribed to message.sent, tool.executed; hook on Write")
