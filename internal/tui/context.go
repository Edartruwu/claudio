package tui

import (
	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/hooks"
	"github.com/Abraxas-365/claudio/internal/learning"
	luart "github.com/Abraxas-365/claudio/internal/lua"
	"github.com/Abraxas-365/claudio/internal/rules"
	"github.com/Abraxas-365/claudio/internal/security"
	"github.com/Abraxas-365/claudio/internal/services/analytics"
	"github.com/Abraxas-365/claudio/internal/services/filtersavings"
	"github.com/Abraxas-365/claudio/internal/services/memory"
	"github.com/Abraxas-365/claudio/internal/session"
	"github.com/Abraxas-365/claudio/internal/storage"
	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/Abraxas-365/claudio/internal/teams"
	"github.com/Abraxas-365/claudio/internal/tui/windows"
)

// AppContext provides shared application state to TUI panels.
// It aggregates all backend services that panels may need to display
// or interact with, avoiding the need for individual option functions.
type AppContext struct {
	Bus         *bus.Bus
	Session     *session.Session
	Memory      *memory.ScopedStore
	Config      *config.Settings
	Analytics     *analytics.Tracker
	FilterSavings *filtersavings.Service
	Learning      *learning.Store
	TaskRuntime *tasks.Runtime
	DB          *storage.DB
	Hooks       *hooks.Manager
	Rules       *rules.Registry
	Auditor     *security.Auditor
	TeamManager   *teams.Manager
	TeamRunner    *teams.TeammateRunner
	LuaRuntime    *luart.Runtime  // optional; nil when Lua plugin system is disabled
	WindowManager *windows.Manager // optional; nil falls back to TUI-local instance
}

// WithAppContext sets the shared application context for TUI panels.
func WithAppContext(ctx *AppContext) ModelOption {
	return func(m *Model) { m.appCtx = ctx }
}
