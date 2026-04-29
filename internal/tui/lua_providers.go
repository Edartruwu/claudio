package tui

// lua_providers.go — lightweight adapters wiring live Model state into the
// four Lua data-provider interfaces (SessionProvider, FilesProvider,
// TasksProvider, TokensProvider) declared in internal/lua/api_data.go.
//
// All adapters hold only pointer/shared state so they remain live after
// the initial wiring call in New().

import (
	"path/filepath"
	"sync"

	luart "github.com/Abraxas-365/claudio/internal/lua"
	"github.com/Abraxas-365/claudio/internal/session"
	sidebarblocks "github.com/Abraxas-365/claudio/internal/tui/sidebar/blocks"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// ---------------------------------------------------------------------------
// luaTokenState — shared mutable box for token/cost data.
// A *luaTokenState is stored on Model so pointer receiver methods and
// BubbleTea Update() (value receiver) can all write to the same box.
// ---------------------------------------------------------------------------

type luaTokenState struct {
	mu   sync.RWMutex
	used int
	cost float64
}

func (b *luaTokenState) set(used int, cost float64) {
	b.mu.Lock()
	b.used = used
	b.cost = cost
	b.mu.Unlock()
}

// Usage implements luart.TokensProvider.
func (b *luaTokenState) Usage() luart.TokenUsage {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return luart.TokenUsage{Used: b.used, Max: 0, Cost: b.cost}
}

// ---------------------------------------------------------------------------
// sessionProviderAdapter
// ---------------------------------------------------------------------------

type sessionProviderAdapter struct{ s *session.Session }

func (a sessionProviderAdapter) CurrentID() string {
	if a.s == nil {
		return ""
	}
	if c := a.s.Current(); c != nil {
		return c.ID
	}
	return ""
}

func (a sessionProviderAdapter) CurrentName() string {
	if a.s == nil {
		return ""
	}
	if c := a.s.Current(); c != nil {
		return c.Title
	}
	return ""
}

func (a sessionProviderAdapter) CurrentModel() string {
	if a.s == nil {
		return ""
	}
	if c := a.s.Current(); c != nil {
		return c.Model
	}
	return ""
}

// ---------------------------------------------------------------------------
// filesProviderAdapter
// ---------------------------------------------------------------------------

type filesProviderAdapter struct{ b *sidebarblocks.FilesBlock }

func (a filesProviderAdapter) List() []luart.FileEntry {
	if a.b == nil {
		return nil
	}
	raw := a.b.Entries()
	result := make([]luart.FileEntry, len(raw))
	for i, e := range raw {
		result[i] = luart.FileEntry{
			Path:  e.Path,
			Name:  filepath.Base(e.Path),
			IsDir: false,
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// tasksProviderAdapter — reads from the package-level GlobalTaskStore.
// ---------------------------------------------------------------------------

type tasksProviderAdapter struct{}

func (tasksProviderAdapter) List() []luart.TaskEntry {
	raw := tools.GlobalTaskStore.List()
	result := make([]luart.TaskEntry, len(raw))
	for i, t := range raw {
		result[i] = luart.TaskEntry{
			ID:         t.ID,
			Title:      t.Subject,
			Status:     string(t.Status),
			ActiveForm: t.ActiveForm,
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// wireLuaDataProviders — called once from New() after LuaRuntime is ready.
// ---------------------------------------------------------------------------

func wireLuaDataProviders(rt *luart.Runtime, sess *session.Session, files *sidebarblocks.FilesBlock, tokens *luaTokenState) {
	if rt == nil {
		return
	}
	rt.SetSessionProvider(sessionProviderAdapter{s: sess})
	rt.SetFilesProvider(filesProviderAdapter{b: files})
	rt.SetTasksProvider(tasksProviderAdapter{})
	rt.SetTokensProvider(tokens)
}
