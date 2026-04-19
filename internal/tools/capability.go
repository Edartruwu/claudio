package tools

import (
	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/config"
)

// RegisterCapabilityTools adds capability-gated tools to the registry based on
// the active agent's declared capabilities. Called on both startup and agent switch.
// Each capability maps to a set of tools; agents without that capability never see them.
func RegisterCapabilityTools(registry *Registry, capabilities []string, client *api.Client, pusher ScreenshotPusher, sessionID string) {
	for _, cap := range capabilities {
		if cap == "design" {
			paths := config.GetPaths()
			renderTool := NewRenderMockupTool(paths.Designs)
			if pusher != nil {
				renderTool = renderTool.WithPusher(pusher, sessionID)
			}
			registry.Register(renderTool)
			// Wire renderer into bundle so BundleMockup auto-renders the final
			// file and pushes fresh screenshots — CC chat shows exact bundle output.
			registry.Register(NewBundleMockupTool(paths.Designs).WithRenderer(renderTool))
			registry.Register(NewVerifyMockupTool(paths.Designs, client, ""))
			registry.Register(NewExportHandoffTool(paths.Designs))
			return
		}
	}
}
