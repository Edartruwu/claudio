package tools

import (
	"os"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/config"
)

// RegisterCapabilityTools adds capability-gated tools to the registry based on
// the active agent's declared capabilities. Called on both startup and agent switch.
// Each capability maps to a set of tools; agents without that capability never see them.
func RegisterCapabilityTools(registry *Registry, capabilities []string, client *api.Client, pusher ScreenshotPusher, sessionID string) {
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
			if pusher != nil {
				bundleTool = bundleTool.WithPusher(pusher, sessionID)
			}
			registry.Register(bundleTool)
			registry.Register(NewVerifyMockupTool(designsDir, client, ""))
			registry.Register(NewExportHandoffTool(designsDir))
			return
		}
	}

	// ReviewDesignFidelity: available to all agents regardless of capabilities.
	{
		wd, _ := os.Getwd()
		registry.Register(NewReviewDesignFidelityTool(config.ProjectDesignsDir(wd), client, ""))
	}
}
