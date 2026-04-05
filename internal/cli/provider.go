package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage model providers",
}

var providerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured providers and model routes",
	RunE: func(cmd *cobra.Command, args []string) error {
		if appInstance == nil || appInstance.API == nil {
			return fmt.Errorf("not initialized — run 'claudio auth login' or set ANTHROPIC_API_KEY")
		}

		providers := appInstance.API.GetProviderNames()
		routes := appInstance.API.GetProviderModels()

		if len(providers) == 0 {
			fmt.Println("No external providers configured.")
			fmt.Println("\nConfigure providers in ~/.claudio/settings.json:")
			fmt.Println(`  "providers": {`)
			fmt.Println(`    "groq": {`)
			fmt.Println(`      "apiBase": "https://api.groq.com/openai/v1",`)
			fmt.Println(`      "apiKey": "$GROQ_API_KEY",`)
			fmt.Println(`      "type": "openai",`)
			fmt.Println(`      "models": { "llama": "llama-3.3-70b-versatile" }`)
			fmt.Println(`    }`)
			fmt.Println(`  }`)
			return nil
		}

		fmt.Println("Registered providers:")
		for _, name := range providers {
			fmt.Printf("  - %s\n", name)
		}

		if len(routes) > 0 {
			fmt.Println("\nModel routes:")
			for pattern, provName := range routes {
				fmt.Printf("  %s → %s\n", pattern, provName)
			}
		}

		shortcuts := appInstance.API.GetModelShortcuts()
		if len(shortcuts) > 0 {
			fmt.Println("\nModel shortcuts:")
			for shortcut, modelID := range shortcuts {
				fmt.Printf("  /%s → %s\n", shortcut, modelID)
			}
		}

		return nil
	},
}

var providerTestCmd = &cobra.Command{
	Use:   "test [provider-name]",
	Short: "Test connectivity to providers",
	Long:  "Sends a minimal request to each configured provider to verify connectivity and latency.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if appInstance == nil || appInstance.API == nil {
			return fmt.Errorf("not initialized")
		}

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
		defer cancelTimeout()

		providers := appInstance.API.GetProviderNames()
		if len(providers) == 0 {
			fmt.Println("No providers configured.")
			return nil
		}

		// Filter by name if argument given.
		if len(args) > 0 {
			name := args[0]
			if !appInstance.API.HasProvider(name) {
				return fmt.Errorf("provider %q not found", name)
			}
			providers = []string{name}
		}

		fmt.Println("Testing providers...")
		fmt.Println()
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "PROVIDER\tSTATUS\tLATENCY\tMODEL\tNOTE\n")

		for _, name := range providers {
			result := appInstance.API.HealthCheck(ctx, name)
			status := "✓ OK"
			if !result.OK {
				status = "✗ FAIL"
			}
			note := ""
			if result.Error != "" {
				note = result.Error
				if len(note) > 60 {
					note = note[:60] + "..."
				}
			}
			fmt.Fprintf(w, "%s\t%s\t%dms\t%s\t%s\n",
				name, status, result.Latency.Milliseconds(), result.Model, note)
		}
		w.Flush()
		return nil
	},
}

var providerModelsCmd = &cobra.Command{
	Use:   "models [provider-name]",
	Short: "Discover available models from providers",
	Long:  "Queries the /v1/models endpoint of OpenAI-compatible providers to list available models.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if appInstance == nil || appInstance.API == nil {
			return fmt.Errorf("not initialized")
		}

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		ctx, cancelTimeout := context.WithTimeout(ctx, 15*time.Second)
		defer cancelTimeout()

		models := appInstance.API.DiscoverModels(ctx)
		if len(models) == 0 {
			fmt.Println("No models discovered. (Only OpenAI-compatible providers support model listing.)")
			return nil
		}

		// Filter by provider if argument given.
		if len(args) > 0 {
			name := args[0]
			var filtered []struct{ ID, Provider string }
			for _, m := range models {
				if m.ProviderName == name {
					filtered = append(filtered, struct{ ID, Provider string }{m.ID, m.ProviderName})
				}
			}
			if len(filtered) == 0 {
				fmt.Printf("No models found for provider %q.\n", name)
				return nil
			}
			fmt.Printf("Models from %s:\n", name)
			for _, m := range filtered {
				fmt.Printf("  %s\n", m.ID)
			}
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "MODEL\tPROVIDER\tOWNER\n")
		for _, m := range models {
			fmt.Fprintf(w, "%s\t%s\t%s\n", m.ID, m.ProviderName, m.OwnedBy)
		}
		w.Flush()
		return nil
	},
}

func init() {
	providerCmd.AddCommand(providerListCmd)
	providerCmd.AddCommand(providerTestCmd)
	providerCmd.AddCommand(providerModelsCmd)
	rootCmd.AddCommand(providerCmd)
}
