// Package outputfilter provides RTK-style output filtering to reduce token
// usage for command outputs. It detects the command being run and applies
// command-specific filters, then applies generic filters on top.
package outputfilter

import (
	"log"
	"strings"

	"github.com/Abraxas-365/claudio/internal/filters/luaregistry"
	"github.com/Abraxas-365/claudio/internal/tools/outputfilter/tomlfilter"
	lua "github.com/yuin/gopher-lua"
)

func init() {
	tomlfilter.SetBuiltinFS(BuiltinFiltersFS())
}

// Filter applies output filtering to a command's combined stdout+stderr.
// It detects the command type and applies the most specific filter available,
// then applies generic filters on top.
func Filter(command, output string) string {
	if output == "" {
		return output
	}

	cmd := normalizeCommand(command)

	// Priority 1: Lua-registered filters (highest priority)
	if filtered, ok := applyLuaFilter(cmd, output); ok {
		return filtered
	}

	// Priority 2: TOML-defined filters
	if filtered, ok := tomlfilter.DefaultRegistry().Apply(cmd, output); ok {
		return filtered
	}

	// Try command-specific filters first
	if filtered, ok := filterByCommand(cmd, output); ok {
		return filtered
	}

	// Fall back to generic filters only
	return Generic(output)
}

// filterByCommand dispatches to a command-specific filter if one exists.
func filterByCommand(cmd, output string) (string, bool) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", false
	}

	base := parts[0]
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}

	switch base {
	case "git":
		return filterGit(sub, output)
	case "go":
		return filterGo(sub, output)
	case "cargo":
		return filterCargo(sub, output)
	case "npm", "pnpm", "yarn":
		return filterNpm(sub, output)
	case "pip", "pip3":
		return filterPip(sub, output)
	case "docker":
		return filterDocker(sub, output)
	case "make":
		return filterMake(output)
	case "gh":
		return filterGh(sub, output)
	case "aws":
		return filterAws(sub, output)
	case "kubectl":
		return filterKubectl(sub, output)
	case "rake":
		return filterRake(sub, output)
	case "rspec":
		return filterRspec(sub, output)
	case "rubocop":
		return filterRubocop(sub, output)
	case "bundle":
		return filterBundle(sub, output)
	case "prisma":
		return filterPrisma(sub, output)
	case "terraform":
		return filterTerraform(sub, output)
	case "ansible-playbook", "ansible-inventory":
		return filterAnsible(base, output)
	case "ansible":
		return filterAnsible(base, output)
	case "eslint":
		return filterEslint(output), true
	case "tsc":
		return filterTsc(output), true
	case "curl":
		return filterCurl(output)
	case "jest":
		return filterJest(sub, output)
	case "vitest":
		return filterJest(sub, output)
	case "pytest":
		return filterPytest(sub, output)
	}

	return "", false
}

// FilterAndRecord applies output filtering and calls rec with the byte counts.
// rec receives a normalized key (base command + first subcommand, e.g. "git diff"),
// bytes before filtering, and bytes after. rec may be nil (no-op).
// The existing Filter function is unchanged.
func FilterAndRecord(command, output string, rec func(cmd string, bytesIn, bytesOut int)) string {
	result := Filter(command, output)
	if rec != nil {
		cmd := normalizeCommand(command)
		parts := strings.Fields(cmd)
		key := cmd
		if len(parts) >= 2 {
			key = parts[0] + " " + parts[1]
		} else if len(parts) == 1 {
			key = parts[0]
		}
		rec(key, len(output), len(result))
	}
	return result
}

// normalizeCommand strips leading env vars, shell prefixes, etc.
func normalizeCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	// Strip leading env vars: FOO=bar cmd -> cmd
	for {
		if idx := strings.Index(cmd, "="); idx > 0 {
			// Check there's no space before the =
			prefix := cmd[:idx]
			if !strings.Contains(prefix, " ") {
				// Skip past the value
				rest := cmd[idx+1:]
				if spaceIdx := strings.Index(rest, " "); spaceIdx >= 0 {
					cmd = strings.TrimSpace(rest[spaceIdx:])
					continue
				}
			}
		}
		break
	}
	return cmd
}

// applyLuaFilter checks the Lua registry for a matching filter and runs
// the declarative pipeline + optional transform function.
func applyLuaFilter(cmd, output string) (string, bool) {
	entry, ok := luaregistry.Lookup(cmd)
	if !ok {
		return "", false
	}

	// Run 8-stage declarative pipeline
	result, err := tomlfilter.ApplyPipeline(entry.Def, output)
	if err != nil {
		log.Printf("[claudio-filter] lua filter %q pipeline error: %v", entry.Name, err)
		return output, true // return original on error
	}

	// Run optional transform function under VM mutex
	if entry.Transform != nil {
		entry.Mu.Lock()
		defer entry.Mu.Unlock()
		if err := entry.VM.CallByParam(lua.P{
			Fn:      entry.Transform,
			NRet:    1,
			Protect: true,
		}, lua.LString(result)); err != nil {
			log.Printf("[claudio-filter] lua filter %q transform error: %v", entry.Name, err)
			return result, true
		}
		ret := entry.VM.Get(-1)
		entry.VM.Pop(1)
		if s, ok := ret.(lua.LString); ok {
			result = string(s)
		}
	}

	return result, true
}
