package lua

import (
	"encoding/json"
	"log"
	"time"

	"github.com/Abraxas-365/claudio/internal/bus"
	lua "github.com/yuin/gopher-lua"
)

// apiNotify returns the claudio.notify(message, level) binding.
//
// Lua usage:
//
//	claudio.notify("Build complete", "info")   -- level: info | warn | error
func (r *Runtime) apiNotify(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		msg := L.CheckString(1)
		level := "info"
		if L.GetTop() >= 2 {
			level = L.CheckString(2)
		}

		payload, _ := json.Marshal(map[string]string{
			"message": msg,
			"level":   level,
			"source":  "plugin:" + plugin.name,
		})

		r.bus.Publish(bus.Event{
			Type:      "notification",
			Payload:   payload,
			Timestamp: time.Now(),
		})
		return 0
	}
}

// apiLog returns the claudio.log(message) binding.
//
// Lua usage:
//
//	claudio.log("debug message")
func (r *Runtime) apiLog(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		msg := L.CheckString(1)
		log.Printf("[lua:%s] %s", plugin.name, msg)
		return 0
	}
}
