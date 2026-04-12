package cmd

import (
	"context"

	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/spf13/cobra"
)

// findingCreateCmd creates a new finding
var findingCreateCmd = &cobra.Command{
	Use:   "finding-create",
	Short: "Create a finding",
	RunE: func(cmd *cobra.Command, args []string) error {
		requestID, _ := cmd.Flags().GetString("request-id")
		title, _ := cmd.Flags().GetString("title")
		description, _ := cmd.Flags().GetString("description")

		// Validate required flags
		if requestID == "" {
			ErrOut("--request-id is required")
		}
		if title == "" {
			ErrOut("--title is required")
		}

		ctx := context.Background()

		// Create finding
		input := &gen.CreateFindingInput{
			Title:       title,
			Description: &description,
			Reporter:    "claudio",
		}

		resp, err := Client.Findings.Create(ctx, requestID, input)
		if err != nil {
			ErrOut("failed to create finding: " + err.Error())
		}

		// Extract created finding from response
		if resp.CreateFinding.Finding == nil {
			ErrOut("failed to create finding: no finding in response")
		}

		finding := map[string]interface{}{
			"id":          resp.CreateFinding.Finding.Id,
			"title":       resp.CreateFinding.Finding.Title,
			"description": resp.CreateFinding.Finding.Description,
			"host":        resp.CreateFinding.Finding.Host,
			"path":        resp.CreateFinding.Finding.Path,
			"reporter":    resp.CreateFinding.Finding.Reporter,
		}

		JSONOut(finding)
		return nil
	},
}

func init() {
	findingCreateCmd.Flags().StringP("request-id", "", "", "Request ID to attach finding to (required)")
	findingCreateCmd.Flags().StringP("title", "", "", "Finding title (required)")
	findingCreateCmd.Flags().StringP("description", "", "", "Finding description (optional)")

	Root.AddCommand(findingCreateCmd)
}
