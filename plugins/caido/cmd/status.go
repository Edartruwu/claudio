package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Abraxas-365/claudio-plugin-caido/client"
	"github.com/Abraxas-365/claudio-plugin-caido/config"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check Caido connection status",
	Long:  "Output JSON with connection health and instance info",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load config (don't use root prerun, handle errors gracefully)
		cfg, err := config.Load()
		if err != nil {
			output := map[string]interface{}{
				"connected": false,
				"error":     fmt.Sprintf("failed to load config: %s", err.Error()),
			}
			JSONOut(output)
			return nil
		}

		// Try to connect and get health
		client, err := client.New(cfg)
		if err != nil {
			output := map[string]interface{}{
				"connected":    false,
				"error":        fmt.Sprintf("failed to connect: %s", err.Error()),
				"url":          cfg.URL,
				"authenticated": false,
			}
			JSONOut(output)
			return nil
		}

		// Get health info
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		health, err := client.Health(ctx)
		if err != nil {
			output := map[string]interface{}{
				"connected":    false,
				"error":        fmt.Sprintf("failed to get health: %s", err.Error()),
				"url":          cfg.URL,
				"authenticated": false,
			}
			JSONOut(output)
			return nil
		}

		// Get runtime info for platform
		runtime, err := client.Instance.GetRuntime(ctx)
		
		// Build output with all info
		output := map[string]interface{}{
			"connected":     true,
			"url":           cfg.URL,
			"caido_version": health.Version,
			"authenticated": true,
		}

		// Add platform if available
		if err == nil && runtime != nil {
			runtimeData := runtime.GetRuntime()
			if runtimeData.GetPlatform() != "" {
				output["platform"] = runtimeData.GetPlatform()
			}
		}

		JSONOut(output)
		return nil
	},
}

func init() {
	Root.AddCommand(statusCmd)
}
