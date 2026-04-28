package lua

import (
	"os"
	"strings"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/api/provider"
	lua "github.com/yuin/gopher-lua"
)

// LuaProviderConfig holds provider configuration registered via Lua.
type LuaProviderConfig struct {
	Name          string
	Type          string
	BaseURL       string
	APIKey        string
	Models        map[string]string // alias → full model name
	Routes        []string
	ContextWindow int
}

// resolveEnvVar expands "$VAR_NAME" references using os.Getenv.
// If the value doesn't start with "$" it is returned unchanged.
func resolveEnvVar(val string) string {
	if strings.HasPrefix(val, "$") {
		return os.Getenv(val[1:])
	}
	return val
}

// apiRegisterProvider returns the claudio.register_provider(tbl) binding.
//
// Lua usage:
//
//	claudio.register_provider({
//	  name     = "groq",
//	  type     = "openai",
//	  base_url = "https://api.groq.com/openai/v1",
//	  api_key  = "$GROQ_API_KEY",
//	  models   = { llama = "llama-3.3-70b-versatile" },
//	  routes   = { "llama-*" },
//	  context_window = 32768,
//	})
func (r *Runtime) apiRegisterProvider(_ *loadedPlugin) lua.LGFunction {
	return func(L *lua.LState) int {
		tbl := L.CheckTable(1)

		name := tableString(L, tbl, "name")
		if name == "" {
			L.ArgError(1, "register_provider: name required")
			return 0
		}

		cfg := LuaProviderConfig{
			Name:          name,
			Type:          tableString(L, tbl, "type"),
			BaseURL:       tableString(L, tbl, "base_url"),
			APIKey:        tableString(L, tbl, "api_key"),
			ContextWindow: tableInt(L, tbl, "context_window"),
			Models:        make(map[string]string),
		}

		// Parse models table: { alias = "full-model-id", ... }
		if modelsVal := L.GetField(tbl, "models"); modelsVal != lua.LNil {
			if modelsTbl, ok := modelsVal.(*lua.LTable); ok {
				modelsTbl.ForEach(func(k, v lua.LValue) {
					if alias, ok2 := k.(lua.LString); ok2 {
						if modelID, ok3 := v.(lua.LString); ok3 {
							cfg.Models[string(alias)] = string(modelID)
						}
					}
				})
			}
		}

		// Parse routes array: { "pattern-*", ... }
		if routesVal := L.GetField(tbl, "routes"); routesVal != lua.LNil {
			if routesTbl, ok := routesVal.(*lua.LTable); ok {
				routesTbl.ForEach(func(_, v lua.LValue) {
					if s, ok2 := v.(lua.LString); ok2 {
						cfg.Routes = append(cfg.Routes, string(s))
					}
				})
			}
		}

		r.mu.Lock()
		r.pendingProviders = append(r.pendingProviders, cfg)
		r.mu.Unlock()

		return 0
	}
}

// ApplyProviders registers all Lua-configured providers into the API client.
// Call this from app.go after the API client is created and after Lua plugins
// have been loaded (so pendingProviders is fully populated).
func (r *Runtime) ApplyProviders(client *api.Client) {
	r.mu.Lock()
	pending := make([]LuaProviderConfig, len(r.pendingProviders))
	copy(pending, r.pendingProviders)
	r.mu.Unlock()

	for _, p := range pending {
		var prov api.Provider
		key := resolveEnvVar(p.APIKey)

		switch p.Type {
		case "openai":
			op := provider.NewOpenAI(p.Name, p.BaseURL, key)
			if p.ContextWindow > 0 {
				op = op.WithNumCtx(p.ContextWindow)
			}
			prov = op
		case "ollama":
			op := provider.NewOpenAI(p.Name, p.BaseURL, key)
			if p.ContextWindow > 0 {
				op = op.WithNumCtx(p.ContextWindow)
			}
			prov = op
		case "anthropic":
			prov = provider.NewAnthropic(p.BaseURL, key)
		default:
			continue // skip unknown types
		}

		client.RegisterProvider(p.Name, prov)
		for alias, full := range p.Models {
			client.AddModelShortcut(alias, full)
		}
		for _, route := range p.Routes {
			client.AddModelRoute(route, p.Name)
		}
	}
}

// tableInt reads an integer field from a Lua table, returning 0 if absent/not-number.
func tableInt(L *lua.LState, tbl *lua.LTable, key string) int {
	v := L.GetField(tbl, key)
	if n, ok := v.(lua.LNumber); ok {
		return int(n)
	}
	return 0
}
