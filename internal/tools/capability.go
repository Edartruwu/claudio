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

// RegisterCapabilityTools adds capability-gated tools to the registry based on
// the active agent's declared capabilities. Called on both startup and agent switch.
// Each capability maps to a set of tools; agents without that capability never see them.
func RegisterCapabilityTools(registry *Registry, capabilities []string, client *api.Client, pusher ScreenshotPusher, sessionID string, cfg *config.Settings) {
	for _, cap := range capabilities {
		if cap == "design" {
			wd, _ := os.Getwd()
			designsDir := config.ProjectDesignsDir(wd)
			renderTool := NewRenderMockupTool(designsDir)
			if pusher != nil {
				renderTool = renderTool.WithPusher(pusher, sessionID)
			}
			registry.Register(renderTool)
			// Wire pusher into bundle so BundleMockup pushes a clickable link to
			// CC chat after the bundle file is written.
			bundleTool := NewBundleMockupTool(designsDir)
			if settings, err := config.Load(wd); err == nil && settings.PublicURL != "" {
				bundleTool = bundleTool.WithPublicURL(settings.PublicURL)
			}
			if pusher != nil {
				bundleTool = bundleTool.WithPusher(pusher, sessionID)
			}
			registry.Register(bundleTool)
			registry.Register(NewVerifyMockupTool(designsDir, client, ResolveToolModel("VerifyMockup", cfg)))
			registry.Register(NewExportHandoffTool(designsDir))
			registry.Register(NewCreateDesignSessionTool())
			registry.Register(&ExportVideoTool{})
			registry.Register(&ExportDeckPPTXTool{})
			registry.Register(&VerifyPrototypeTool{})
			return
		}
	}

	// ReviewDesignFidelity: available to all agents regardless of capabilities.
	{
		wd, _ := os.Getwd()
		registry.Register(NewReviewDesignFidelityTool(config.ProjectDesignsDir(wd), client, ResolveToolModel("ReviewDesignFidelity", cfg)))
	}
}
