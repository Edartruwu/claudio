package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Abraxas-365/claudio/internal/config"
	lua "github.com/yuin/gopher-lua"
)

// LoadLspConfigs discovers *.lsp.json files in the plugins directory
// and returns the merged LSP server configs.
// Each file should contain a JSON object mapping server names to LspServerConfig.
func LoadLspConfigs(pluginDir string) map[string]config.LspServerConfig {
	result := make(map[string]config.LspServerConfig)

	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return result
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		// Match *.lsp.json
		base := entry.Name()
		if len(base) < len(".lsp.json") || base[len(base)-len(".lsp.json"):] != ".lsp.json" {
			continue
		}

		path := filepath.Join(pluginDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var servers map[string]config.LspServerConfig
		if err := json.Unmarshal(data, &servers); err != nil {
			continue
		}

		for k, v := range servers {
			result[k] = v
		}
	}

	return result
}

// LoadLuaLspConfigs discovers *.lua files in lspDir (~/.claudio/lsp/) and returns
// the merged LSP server configs. Each file must return a table:
//
//	return {
//	  gopls = { command="gopls", args={"serve"}, extensions={".go",".mod"} }
//	}
func LoadLuaLspConfigs(lspDir string) map[string]config.LspServerConfig {
	result := make(map[string]config.LspServerConfig)

	entries, err := os.ReadDir(lspDir)
	if err != nil {
		return result
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".lua" {
			continue
		}

		path := filepath.Join(lspDir, entry.Name())
		L := lua.NewState()
		if err := L.DoFile(path); err != nil {
			L.Close()
			continue
		}

		tbl, ok := L.Get(-1).(*lua.LTable)
		if !ok {
			L.Close()
			continue
		}

		tbl.ForEach(func(key lua.LValue, val lua.LValue) {
			name, ok := key.(lua.LString)
			if !ok {
				return
			}
			srv, ok := val.(*lua.LTable)
			if !ok {
				return
			}
			cfg := config.LspServerConfig{}
			if cmd := srv.RawGetString("command"); cmd != lua.LNil {
				cfg.Command = cmd.String()
			}
			if argsTbl, ok := srv.RawGetString("args").(*lua.LTable); ok {
				argsTbl.ForEach(func(_ lua.LValue, v lua.LValue) {
					cfg.Args = append(cfg.Args, v.String())
				})
			}
			if extTbl, ok := srv.RawGetString("extensions").(*lua.LTable); ok {
				extTbl.ForEach(func(_ lua.LValue, v lua.LValue) {
					cfg.Extensions = append(cfg.Extensions, v.String())
				})
			}
			if envTbl, ok := srv.RawGetString("env").(*lua.LTable); ok {
				cfg.Env = make(map[string]string)
				envTbl.ForEach(func(k lua.LValue, v lua.LValue) {
					cfg.Env[k.String()] = v.String()
				})
			}
			result[string(name)] = cfg
		})

		L.Close()
	}

	return result
}
