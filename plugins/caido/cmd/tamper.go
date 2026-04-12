package cmd

import (
	"context"
	"strings"

	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/spf13/cobra"
)

var tamperCmd = &cobra.Command{
	Use:   "tamper",
	Short: "List tamper rule collections",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// List tamper collections
		resp, err := Client.Tamper.ListCollections(ctx)
		if err != nil {
			ErrOut("failed to list tamper collections: " + err.Error())
		}

		// Extract collections with nested rules
		var collections []map[string]interface{}
		for _, collection := range resp.TamperRuleCollections {
			var rules []map[string]interface{}
			for _, rule := range collection.Rules {
				enabled := false
				if rule.Enable != nil {
					enabled = true
				}
				r := map[string]interface{}{
					"id":        rule.Id,
					"name":      rule.Name,
					"enabled":   enabled,
					"condition": rule.Condition,
				}
				rules = append(rules, r)
			}

			c := map[string]interface{}{
				"id":    collection.Id,
				"name":  collection.Name,
				"rules": rules,
			}
			collections = append(collections, c)
		}

		JSONOut(collections)
		return nil
	},
}

var tamperCreateCmd = &cobra.Command{
	Use:   "tamper-create",
	Short: "Create a tamper rule",
	RunE: func(cmd *cobra.Command, args []string) error {
		collectionID, _ := cmd.Flags().GetString("collection-id")
		name, _ := cmd.Flags().GetString("name")
		condition, _ := cmd.Flags().GetString("condition")
		sourcesStr, _ := cmd.Flags().GetString("sources")

		// Validate required flags
		if collectionID == "" {
			ErrOut("--collection-id is required")
		}
		if name == "" {
			ErrOut("--name is required")
		}

		// Parse and validate sources
		var sources []gen.Source
		if sourcesStr != "" {
			sourceStrs := strings.Split(sourcesStr, ",")
			validSources := map[string]gen.Source{
				"INTERCEPT": gen.SourceIntercept,
				"REPLAY":    gen.SourceReplay,
				"AUTOMATE":  gen.SourceAutomate,
				"IMPORT":    gen.SourceImport,
				"PLUGIN":    gen.SourcePlugin,
				"WORKFLOW":  gen.SourceWorkflow,
				"SAMPLE":    gen.SourceSample,
			}

			for _, s := range sourceStrs {
				s = strings.TrimSpace(s)
				if source, ok := validSources[s]; ok {
					sources = append(sources, source)
				} else {
					ErrOut("invalid source: " + s + " (valid: INTERCEPT, REPLAY, AUTOMATE, IMPORT, PLUGIN, WORKFLOW, SAMPLE)")
				}
			}
		}

		ctx := context.Background()

		// Create tamper rule
		input := &gen.CreateTamperRuleInput{
			CollectionId: collectionID,
			Name:         name,
			Condition:    &condition,
			Sources:      sources,
			Section:      gen.TamperSectionInput{}, // Default empty section
		}

		resp, err := Client.Tamper.CreateRule(ctx, input)
		if err != nil {
			ErrOut("failed to create tamper rule: " + err.Error())
		}

		if resp.CreateTamperRule.Rule == nil {
			ErrOut("failed to create tamper rule: no rule in response")
		}

		rule := map[string]interface{}{
			"id":   resp.CreateTamperRule.Rule.Id,
			"name": resp.CreateTamperRule.Rule.Name,
		}

		JSONOut(rule)
		return nil
	},
}

var tamperToggleCmd = &cobra.Command{
	Use:   "tamper-toggle",
	Short: "Enable or disable a tamper rule",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, _ := cmd.Flags().GetString("id")
		enabled, _ := cmd.Flags().GetBool("enabled")

		// Validate required flags
		if id == "" {
			ErrOut("--id is required")
		}

		ctx := context.Background()

		// Toggle tamper rule
		resp, err := Client.Tamper.ToggleRule(ctx, id, enabled)
		if err != nil {
			ErrOut("failed to toggle tamper rule: " + err.Error())
		}

		if resp.ToggleTamperRule.Rule == nil {
			ErrOut("failed to toggle tamper rule: no rule in response")
		}

		isEnabled := false
		if resp.ToggleTamperRule.Rule.Enable != nil {
			isEnabled = true
		}

		rule := map[string]interface{}{
			"id":      resp.ToggleTamperRule.Rule.Id,
			"name":    resp.ToggleTamperRule.Rule.Name,
			"enabled": isEnabled,
		}

		JSONOut(rule)
		return nil
	},
}

var tamperDeleteCmd = &cobra.Command{
	Use:   "tamper-delete",
	Short: "Delete a tamper rule",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, _ := cmd.Flags().GetString("id")

		// Validate required flags
		if id == "" {
			ErrOut("--id is required")
		}

		ctx := context.Background()

		// Delete tamper rule
		_, err := Client.Tamper.DeleteRule(ctx, id)
		if err != nil {
			ErrOut("failed to delete tamper rule: " + err.Error())
		}

		JSONOut(map[string]interface{}{
			"success": true,
		})
		return nil
	},
}

func init() {
	// tamper command
	Root.AddCommand(tamperCmd)

	// tamper-create command
	tamperCreateCmd.Flags().StringP("collection-id", "", "", "Collection ID (required)")
	tamperCreateCmd.Flags().StringP("name", "", "", "Rule name (required)")
	tamperCreateCmd.Flags().StringP("condition", "", "", "Rule condition (optional)")
	tamperCreateCmd.Flags().StringP("sources", "", "", "Comma-separated sources (INTERCEPT,REPLAY,AUTOMATE,IMPORT,PLUGIN,WORKFLOW,SAMPLE)")
	Root.AddCommand(tamperCreateCmd)

	// tamper-toggle command
	tamperToggleCmd.Flags().StringP("id", "", "", "Rule ID (required)")
	tamperToggleCmd.Flags().BoolP("enabled", "", false, "Enable or disable the rule")
	Root.AddCommand(tamperToggleCmd)

	// tamper-delete command
	tamperDeleteCmd.Flags().StringP("id", "", "", "Rule ID (required)")
	Root.AddCommand(tamperDeleteCmd)
}
