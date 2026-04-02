package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/Abraxas-365/claudio/internal/auth/oauth"
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

		// Save tokens to secure storage
		if err := appInstance.Storage.SaveTokens(tokens); err != nil {
			return fmt.Errorf("failed to save credentials: %w", err)
		}

		// Save API key if one was created
		if tokens.APIKey != "" {
			if err := appInstance.Storage.SaveAPIKey(tokens.APIKey); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save API key: %v\n", err)
			}
		}

		if tokens.Account != nil {
			fmt.Printf("Logged in as: %s\n", tokens.Account.EmailAddress)
		}
		if tokens.SubscriptionType != "" {
			fmt.Printf("Subscription: %s\n", tokens.SubscriptionType)
		}

		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and clear stored credentials",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := appInstance.Storage.Delete(); err != nil {
			return fmt.Errorf("failed to clear credentials: %w", err)
		}
		fmt.Println("Logged out successfully.")
		return nil
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	RunE: func(cmd *cobra.Command, args []string) error {
		result := appInstance.Auth.Resolve()

		if result.Source == "none" {
			fmt.Println("Not logged in.")
			fmt.Println("Run: claudio auth login")
			return nil
		}

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

func authType(isOAuth bool) string {
	if isOAuth {
		return "OAuth"
	}
	return "API Key"
}

func init() {
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	rootCmd.AddCommand(authCmd)
}
