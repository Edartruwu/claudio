package lua

import (
	"log"
	"strings"

	"github.com/Abraxas-365/claudio/internal/cli/commands"
	lua "github.com/yuin/gopher-lua"
)

// apiRegisterCommand returns the claudio.register_command(tbl) binding.
//
// Lua usage:
//
//	claudio.register_command({
//	  name        = "jira",
//	  description = "Open Jira issue",
//	  aliases     = { "j" },
//	  execute     = function(args)
//	    claudio.notify("Opening: " .. args, "info")
//	    return "opened"
//	  end
//	})
//
// If commandRegistry is not yet set, the command is queued and flushed when
// SetCommandRegistry is called.
func (r *Runtime) apiRegisterCommand(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		tbl := L.CheckTable(1)

		name := tableString(L, tbl, "name")
		desc := tableString(L, tbl, "description")
		aliases := tableStringSlice(L, tbl, "aliases")
		executeFn := L.GetField(tbl, "execute")

		if name == "" {
			L.ArgError(1, "register_command: name required")
			return 0
		}

		var handler func(args string) (string, error)
		if fn, ok := executeFn.(*lua.LFunction); ok {
			handler = func(args string) (string, error) {
				plugin.mu.Lock()
				defer plugin.mu.Unlock()

				defer func() {
					if rv := recover(); rv != nil {
						log.Printf("[lua] plugin %s command %s panic: %v", plugin.name, name, rv)
					}
				}()

				err := plugin.L.CallByParam(lua.P{
					Fn:      fn,
					NRet:    1,
					Protect: true,
				}, lua.LString(args))
				if err != nil {
					return "", err
				}

				ret := plugin.L.Get(-1)
				plugin.L.Pop(1)
				if ret == lua.LNil {
					return "", nil
				}
				return ret.String(), nil
			}
		} else {
			handler = func(args string) (string, error) { return "", nil }
		}

		cmd := &commands.Command{
			Name:        name,
			Aliases:     aliases,
			Description: desc,
			Execute:     handler,
		}

		r.mu.Lock()
		if r.commandRegistry != nil {
			r.commandRegistry.Register(cmd)
		} else {
			r.pendingCommands = append(r.pendingCommands, &pendingCommand{cmd: cmd})
		}
		r.mu.Unlock()

		return 0
	}
}

// apiCmd returns the claudio.cmd(cmdline) binding.
//
// Lua usage:
//
//	claudio.cmd("colorscheme tokyonight")
//	claudio.cmd("set model claude-opus-4-6")
//
// Splits on first space → name + args, then calls Registry.Get + Execute.
// If registry not set, logs a warning.
func (r *Runtime) apiCmd(plugin *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		cmdline := L.CheckString(1)
		cmdline = strings.TrimSpace(cmdline)

		var cmdName, args string
		if idx := strings.IndexByte(cmdline, ' '); idx >= 0 {
			cmdName = cmdline[:idx]
			args = strings.TrimSpace(cmdline[idx+1:])
		} else {
			cmdName = cmdline
		}

		r.mu.Lock()
		reg := r.commandRegistry
		r.mu.Unlock()

		if reg == nil {
			log.Printf("[lua] plugin %s: claudio.cmd(%q) called but no commandRegistry wired", plugin.name, cmdline)
			return 0
		}

		cmd, ok := reg.Get(cmdName)
		if !ok {
			log.Printf("[lua] plugin %s: claudio.cmd: unknown command %q", plugin.name, cmdName)
			return 0
		}

		result, err := cmd.Execute(args)
		if err != nil {
			log.Printf("[lua] plugin %s: claudio.cmd(%q) error: %v", plugin.name, cmdline, err)
			return 0
		}
		if result != "" {
			L.Push(lua.LString(result))
			return 1
		}
		return 0
	}
}
