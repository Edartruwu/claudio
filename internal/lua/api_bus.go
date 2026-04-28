package lua

import (
	"encoding/json"
	"log"
	"time"

	"github.com/Abraxas-365/claudio/internal/bus"
	lua "github.com/yuin/gopher-lua"
)

// apiSubscribe returns the claudio.subscribe(eventType, handler) binding.
//
// Lua usage:
//
//	claudio.subscribe("message.sent", function(event)
//	  -- event.type, event.payload
//	end)
func (r *Runtime) apiSubscribe(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		eventType := L.CheckString(1)
		handler := L.CheckFunction(2)

		unsub := r.bus.Subscribe(eventType, func(e bus.Event) {
			plugin.mu.Lock()
			defer plugin.mu.Unlock()

			defer func() {
				if rv := recover(); rv != nil {
					log.Printf("[lua] plugin %s subscribe handler panic: %v", plugin.name, rv)
				}
			}()

			// Build event table: { type = "...", payload = <table> }
			evtTbl := plugin.L.NewTable()
			plugin.L.SetField(evtTbl, "type", lua.LString(e.Type))

			if len(e.Payload) > 0 {
				payloadVal, err := jsonBytesToLuaTable(plugin.L, e.Payload)
				if err != nil {
					log.Printf("[lua] plugin %s: decode event payload: %v", plugin.name, err)
					plugin.L.SetField(evtTbl, "payload", lua.LNil)
				} else {
					plugin.L.SetField(evtTbl, "payload", payloadVal)
				}
			}

			if err := plugin.L.CallByParam(lua.P{
				Fn:      handler,
				NRet:    0,
				Protect: true,
			}, evtTbl); err != nil {
				log.Printf("[lua] plugin %s subscribe handler error: %v", plugin.name, err)
			}
		})

		plugin.mu.Lock()
		plugin.unsubs = append(plugin.unsubs, unsub)
		plugin.mu.Unlock()

		return 0
	}
}

// apiPublish returns the claudio.publish(eventType, payload) binding.
//
// Lua usage:
//
//	claudio.publish("plugin.my-plugin.custom-event", { key = "value" })
func (r *Runtime) apiPublish(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		eventType := L.CheckString(1)

		var payload json.RawMessage
		if L.GetTop() >= 2 {
			payloadVal := L.Get(2)
			data, err := luaValueToJSON(payloadVal)
			if err != nil {
				log.Printf("[lua] plugin %s: publish encode error: %v", plugin.name, err)
				return 0
			}
			payload = data
		}

		r.bus.Publish(bus.Event{
			Type:      eventType,
			Payload:   payload,
			Timestamp: time.Now(),
		})
		return 0
	}
}
