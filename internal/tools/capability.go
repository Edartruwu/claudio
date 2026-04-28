package tools

import (
	"os"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/config"
)

// ResolveToolModel returns the model to use for a named tool.
// Fallback chain: toolModels.<name> → smallModel → hardcoded default.
func ResolveToolModel(toolName string, cfg *config.Settings) string {
	if cfg != nil {
		if m, ok := cfg.ToolModels[toolName]; ok && m != "" {
			return m
		}
		if cfg.SmallModel != "" {
			return cfg.SmallModel
		}
	}
	return "claude-haiku-4-5-20251001"
}

// CapabilityRegistrar is implemented by capabilities.Registry and injected via
// SetCapabilityRegistry. Using an interface avoids a circular import between
// internal/tools and internal/capabilities.
type CapabilityRegistrar interface {
	// IsKnown reports whether a capability token is registered.
	IsKnown(name string) bool
	// ApplyToRegistry runs tool factories for every known cap in caps, registering
	// them into reg. Returns true if at least one capability was applied.
	ApplyToRegistry(caps []string, reg *Registry, client *api.Client, pusher ScreenshotPusher, sessionID string, cfg *config.Settings) bool
}

// globalCapReg is set during app initialization by SetCapabilityRegistry.
// Nil until then — tests that don't call New() still get the fallback path.
var globalCapReg CapabilityRegistrar

// SetCapabilityRegistry wires the dynamic capability registry into the tools
// package. Must be called during App.New() before any agent spawns.
func SetCapabilityRegistry(r CapabilityRegistrar) {
	globalCapReg = r
}

// RegisterCapabilityTools adds capability-gated tools to the registry based on
// the active agent's declared capabilities. Called on both startup and agent switch.
// Signature is unchanged from the original so all call sites need no modification.
func RegisterCapabilityTools(registry *Registry, capabilities []string, client *api.Client, pusher ScreenshotPusher, sessionID string, cfg *config.Settings) {
	if globalCapReg != nil {
		applied := globalCapReg.ApplyToRegistry(capabilities, registry, client, pusher, sessionID, cfg)
		if applied {
			return
		}
	}

	// ReviewDesignFidelity: available to all agents that have no capability tools
	// applied (i.e. agents that don't hold a gated capability like "design").
	{
		wd, _ := os.Getwd()
		registry.Register(NewReviewDesignFidelityTool(config.ProjectDesignsDir(wd), client, ResolveToolModel("ReviewDesignFidelity", cfg)))
	}
}
