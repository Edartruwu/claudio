// Package capabilities provides a dynamic registry for agent capability tokens.
// Plugins can register new capabilities at runtime; the hardcoded "design"
// capability is registered by internal/app during startup.
package capabilities

import (
	"os"
	"sync"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// ToolDeps bundles the runtime dependencies a ToolFactory receives when the
// capability system builds tools for a specific agent session.
type ToolDeps struct {
	Client    *api.Client
	Pusher    tools.ScreenshotPusher
	SessionID string
	Cfg       *config.Settings
}

// ToolFactory creates and registers tools into reg for a single agent session.
// It receives all runtime deps via ToolDeps so factories can be declared once
// and instantiated many times (once per agent switch / spawn).
type ToolFactory func(reg *tools.Registry, deps ToolDeps)

// Registry maps capability tokens to their ToolFactory slices.
// Safe for concurrent use — Lua plugin init runs in parallel with app startup.
type Registry struct {
	mu   sync.RWMutex
	caps map[string][]ToolFactory
}

// New returns an empty, ready-to-use Registry.
func New() *Registry {
	return &Registry{caps: make(map[string][]ToolFactory)}
}

// Register declares (or extends) a capability with one or more tool factories.
// Calling Register multiple times with the same name appends factories — useful
// for plugins that extend a built-in capability.
func (r *Registry) Register(name string, factories ...ToolFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.caps[name] = append(r.caps[name], factories...)
}

// IsKnown reports whether a capability token has been registered.
func (r *Registry) IsKnown(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.caps[name]
	return ok
}

// ApplyToRegistry runs all factories for every cap in caps that the registry
// knows about, registering their tools into reg.
// Returns true if at least one capability was applied (i.e., any cap matched).
// Signature mirrors RegisterCapabilityTools so the tools package can call it
// via the CapabilityRegistrar interface without importing this package.
func (r *Registry) ApplyToRegistry(
	caps []string,
	reg *tools.Registry,
	client *api.Client,
	pusher tools.ScreenshotPusher,
	sessionID string,
	cfg *config.Settings,
) bool {
	deps := ToolDeps{
		Client:    client,
		Pusher:    pusher,
		SessionID: sessionID,
		Cfg:       cfg,
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	applied := false
	for _, cap := range caps {
		factories, ok := r.caps[cap]
		if !ok {
			continue
		}
		for _, factory := range factories {
			factory(reg, deps)
		}
		applied = true
	}
	return applied
}

// DesignFactories returns the ToolFactory slice for the built-in "design"
// capability. Called by app.go to register "design" without duplicating
// the constructor calls here.
func DesignFactories() []ToolFactory {
	return []ToolFactory{
		func(reg *tools.Registry, deps ToolDeps) {
			wd, _ := os.Getwd()
			designsDir := config.ProjectDesignsDir(wd)

			renderTool := tools.NewRenderMockupTool(designsDir)
			if deps.Pusher != nil {
				renderTool = renderTool.WithPusher(deps.Pusher, deps.SessionID)
			}
			reg.Register(renderTool)

			bundleTool := tools.NewBundleMockupTool(designsDir)
			if settings, err := config.Load(wd); err == nil && settings.PublicURL != "" {
				bundleTool = bundleTool.WithPublicURL(settings.PublicURL)
			}
			if deps.Pusher != nil {
				bundleTool = bundleTool.WithPusher(deps.Pusher, deps.SessionID)
			}
			reg.Register(bundleTool)

			reg.Register(tools.NewVerifyMockupTool(designsDir, deps.Client, tools.ResolveToolModel("VerifyMockup", deps.Cfg)))
			reg.Register(tools.NewExportHandoffTool(designsDir))
			reg.Register(tools.NewCreateDesignSessionTool())
			reg.Register(&tools.ExportVideoTool{})
			reg.Register(&tools.ExportDeckPPTXTool{})
			reg.Register(&tools.VerifyPrototypeTool{})
		},
	}
}
