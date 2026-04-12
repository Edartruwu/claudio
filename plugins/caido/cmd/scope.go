package cmd

import (
	"context"
	"strings"

	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/spf13/cobra"
)

var (
	scopeName  string
	scopeAllow string
	scopeDeny  string
)

var scopesCmd = &cobra.Command{
	Use:   "scopes",
	Short: "List all scopes",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		resp, err := Client.Scopes.List(ctx)
		if err != nil {
			ErrOut("failed to list scopes: " + err.Error())
		}

		type ScopeItem struct {
			ID        string   `json:"id"`
			Name      string   `json:"name"`
			Allowlist []string `json:"allowlist"`
			Denylist  []string `json:"denylist"`
		}

		var scopes []ScopeItem
		for _, scope := range resp.Scopes {
			scopes = append(scopes, ScopeItem{
				ID:        scope.Id,
				Name:      scope.Name,
				Allowlist: scope.Allowlist,
				Denylist:  scope.Denylist,
			})
		}

		JSONOut(scopes)
		return nil
	},
}

var scopeCreateCmd = &cobra.Command{
	Use:   "scope-create",
	Short: "Create a new scope",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Validate required flags
		if scopeName == "" {
			ErrOut("--name is required")
		}
		if scopeAllow == "" {
			ErrOut("--allow is required")
		}

		ctx := context.Background()

		// Parse comma-separated allowlist and denylist
		allowlist := strings.Split(scopeAllow, ",")
		denylist := []string{}
		if scopeDeny != "" {
			denylist = strings.Split(scopeDeny, ",")
		}

		input := &gen.CreateScopeInput{
			Name:      scopeName,
			Allowlist: allowlist,
			Denylist:  denylist,
		}

		resp, err := Client.Scopes.Create(ctx, input)
		if err != nil {
			ErrOut("failed to create scope: " + err.Error())
		}

		type ScopeCreated struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}

		out := ScopeCreated{
			ID:   resp.CreateScope.Scope.Id,
			Name: resp.CreateScope.Scope.Name,
		}

		JSONOut(out)
		return nil
	},
}

func init() {
	scopeCreateCmd.Flags().StringVar(&scopeName, "name", "", "scope name (required)")
	scopeCreateCmd.Flags().StringVar(&scopeAllow, "allow", "", "comma-separated allowlist (required)")
	scopeCreateCmd.Flags().StringVar(&scopeDeny, "deny", "", "comma-separated denylist (optional)")

	Root.AddCommand(scopesCmd)
	Root.AddCommand(scopeCreateCmd)
}
