package cmd

import (
	"context"
	"strings"

	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/spf13/cobra"
)

// findingsDeleteCmd deletes findings by IDs or reporter
var findingsDeleteCmd = &cobra.Command{
	Use:   "findings-delete",
	Short: "Delete findings",
	RunE: func(cmd *cobra.Command, args []string) error {
		idsStr, _ := cmd.Flags().GetString("ids")
		reporter, _ := cmd.Flags().GetString("reporter")

		// Validate at least one flag is provided
		if idsStr == "" && reporter == "" {
			ErrOut("at least one of --ids or --reporter must be provided")
		}

		ctx := context.Background()

		// Parse IDs from comma-separated string
		var ids []string
		if idsStr != "" {
			ids = strings.Split(idsStr, ",")
			// Trim whitespace from each ID
			for i, id := range ids {
				ids[i] = strings.TrimSpace(id)
			}
		}

		// Create delete input
		input := &gen.DeleteFindingsInput{
			Ids: ids,
		}
		if reporter != "" {
			input.Reporter = &reporter
		}

		resp, err := Client.Findings.Delete(ctx, input)
		if err != nil {
			ErrOut("failed to delete findings: " + err.Error())
		}

		result := map[string]interface{}{
			"deleted": len(resp.DeleteFindings.DeletedIds),
		}

		JSONOut(result)
		return nil
	},
}

func init() {
	findingsDeleteCmd.Flags().StringP("ids", "", "", "Comma-separated finding IDs to delete")
	findingsDeleteCmd.Flags().StringP("reporter", "", "", "Delete all findings from this reporter")

	Root.AddCommand(findingsDeleteCmd)
}
