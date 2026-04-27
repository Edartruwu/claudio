package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/auth/oauth"
	"github.com/Abraxas-365/claudio/internal/auth/storage"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to Claude via OAuth",
	RunE: func(cmd *cobra.Command, args []string) error {
		profile, _ := cmd.Flags().GetString("profile")
		if profile == "" {
			profile = appInstance.Profile
		}

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		// Add a timeout for the login flow
		ctx, cancelTimeout := context.WithTimeout(ctx, 5*time.Minute)
		defer cancelTimeout()

		cfg := oauth.DefaultConfig()
		svc := oauth.NewService(cfg)

		tokens, err := svc.Login(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
			return fmt.Errorf("login failed: %w", err)
		}

		// Use profile-specific storage
		store := storage.NewDefaultStorage(profile)

		// Save tokens to secure storage
		if err := store.SaveTokens(tokens); err != nil {
			return fmt.Errorf("failed to save credentials: %w", err)
		}

		// Save API key if one was created
		if tokens.APIKey != "" {
			if err := store.SaveAPIKey(tokens.APIKey); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save API key: %v\n", err)
			}
		}

		if tokens.Account != nil {
			fmt.Printf("Logged in as: %s\n", tokens.Account.EmailAddress)
		}
		if tokens.SubscriptionType != "" {
			fmt.Printf("Subscription: %s\n", tokens.SubscriptionType)
		}
		fmt.Printf("Saved to profile: %s\n", profile)

		activeProfile := config.GetActiveProfile()
		if profile != activeProfile {
			fmt.Printf("Tip: run `claudio auth use %s` to switch to this profile.\n", profile)
		}

		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and clear stored credentials",
	RunE: func(cmd *cobra.Command, args []string) error {
		profile, _ := cmd.Flags().GetString("profile")
		if profile == "" {
			profile = appInstance.Profile
		}

		store := storage.NewDefaultStorage(profile)
		if err := store.Delete(); err != nil {
			return fmt.Errorf("failed to clear credentials: %w", err)
		}
		fmt.Printf("Logged out from profile: %s\n", profile)
		return nil
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	RunE: func(cmd *cobra.Command, args []string) error {
		profile, _ := cmd.Flags().GetString("profile")
		if profile == "" {
			profile = appInstance.Profile
		}

		store := storage.NewDefaultStorage(profile)
		result := appInstance.Auth.Resolve()

		// If checking a non-active profile, resolve against that store directly
		if profile != appInstance.Profile {
			tokens, _ := store.ReadTokens()
			apiKey, _ := store.ReadAPIKey()
			if tokens == nil && apiKey == "" {
				fmt.Printf("Profile %q: not logged in.\n", profile)
				return nil
			}
			fmt.Printf("Profile: %s\n", profile)
			if tokens != nil {
				if tokens.Account != nil {
					fmt.Printf("Email: %s\n", tokens.Account.EmailAddress)
				}
				if tokens.SubscriptionType != "" {
					fmt.Printf("Subscription: %s\n", tokens.SubscriptionType)
				}
			} else if apiKey != "" {
				if len(apiKey) > 12 {
					fmt.Printf("API key: %s...%s\n", apiKey[:8], apiKey[len(apiKey)-4:])
				}
			}
			return nil
		}

		if result.Source == "none" {
			fmt.Println("Not logged in.")
			fmt.Println("Run: claudio auth login")
			return nil
		}

		fmt.Printf("Profile: %s\n", profile)
		fmt.Printf("Logged in: yes\n")
		fmt.Printf("Auth source: %s\n", result.Source)
		fmt.Printf("Auth type: %s\n", authType(result.IsOAuth))

		// Show additional details for OAuth
		if result.IsOAuth {
			tokens, err := appInstance.Storage.ReadTokens()
			if err == nil && tokens != nil {
				if tokens.Account != nil {
					fmt.Printf("Email: %s\n", tokens.Account.EmailAddress)
				}
				if tokens.SubscriptionType != "" {
					fmt.Printf("Subscription: %s\n", tokens.SubscriptionType)
				}
				if !tokens.ExpiresAt.IsZero() {
					remaining := time.Until(tokens.ExpiresAt)
					if remaining > 0 {
						fmt.Printf("Token expires in: %s\n", remaining.Round(time.Second))
					} else {
						fmt.Printf("Token expired: %s ago (will auto-refresh)\n", (-remaining).Round(time.Second))
					}
				}
			}
		} else {
			// Mask API key
			if len(result.Token) > 12 {
				fmt.Printf("API key: %s...%s\n", result.Token[:8], result.Token[len(result.Token)-4:])
			}
		}

		return nil
	},
}

var authUseCmd = &cobra.Command{
	Use:   "use <profile>",
	Short: "Switch active auth profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		profile := args[0]

		// Verify profile exists (has credentials)
		store := storage.NewDefaultStorage(profile)
		tokens, _ := store.ReadTokens()
		apiKey, _ := store.ReadAPIKey()
		if tokens == nil && apiKey == "" {
			return fmt.Errorf("profile %q has no credentials — run `claudio auth login --profile %s` first", profile, profile)
		}

		paths := config.GetPaths()
		if err := os.WriteFile(paths.ActiveProfileFile, []byte(profile), 0600); err != nil {
			return fmt.Errorf("failed to write active profile: %w", err)
		}
		fmt.Printf("Active profile: %s\n", profile)
		return nil
	},
}

var authListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all auth profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		activeProfile := config.GetActiveProfile()
		paths := config.GetPaths()

		// Collect profile names: always include "default", then scan profiles/ dir
		seen := map[string]bool{"default": true}
		profileNames := []string{"default"}

		profilesDir := filepath.Join(paths.Home, "profiles")
		entries, err := os.ReadDir(profilesDir)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() && !seen[e.Name()] {
					seen[e.Name()] = true
					profileNames = append(profileNames, e.Name())
				}
			}
		}

		for _, name := range profileNames {
			store := storage.NewDefaultStorage(name)
			tokens, _ := store.ReadTokens()
			apiKey, _ := store.ReadAPIKey()

			marker := "  "
			if name == activeProfile {
				marker = "* "
			}

			var detail string
			if tokens != nil {
				email := ""
				if tokens.Account != nil {
					email = tokens.Account.EmailAddress
				}
				sub := tokens.SubscriptionType
				parts := []string{}
				if email != "" {
					parts = append(parts, email)
				}
				if sub != "" {
					parts = append(parts, sub)
				}
				if len(parts) > 0 {
					detail = strings.Join(parts, "  ")
				} else {
					detail = "(oauth)"
				}
			} else if apiKey != "" {
				if len(apiKey) > 12 {
					detail = fmt.Sprintf("API key %s...%s", apiKey[:8], apiKey[len(apiKey)-4:])
				} else {
					detail = "API key"
				}
			} else {
				detail = "(no credentials)"
			}

			if name == activeProfile {
				fmt.Printf("%s%-20s (active)  %s\n", marker, name, detail)
			} else {
				fmt.Printf("%s%-20s           %s\n", marker, name, detail)
			}
		}
		return nil
	},
}

func authType(isOAuth bool) string {
	if isOAuth {
		return "OAuth"
	}
	return "API Key"
}

func init() {
	// Login flags
	authLoginCmd.Flags().String("profile", "", "Profile to save credentials to (default: active profile)")

	// Logout flags
	authLogoutCmd.Flags().String("profile", "", "Profile to log out from (default: active profile)")

	// Status flags
	authStatusCmd.Flags().String("profile", "", "Profile to check (default: active profile)")

	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authUseCmd)
	authCmd.AddCommand(authListCmd)
	rootCmd.AddCommand(authCmd)
}
