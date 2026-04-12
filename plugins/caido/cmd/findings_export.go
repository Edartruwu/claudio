package cmd

import (
	"context"
	"strings"

	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/spf13/cobra"
)

// findingsExportCmd exports findings
var findingsExportCmd = &cobra.Command{
	Use:   "findings-export",
	Short: "Export findings",
	RunE: func(cmd *cobra.Command, args []string) error {
		idsStr, _ := cmd.Flags().GetString("ids")
		reporter, _ := cmd.Flags().GetString("reporter")

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

		// Create export input
		input := &gen.ExportFindingsInput{
			Ids: ids,
		}

		// Add filter if reporter is specified
		if reporter != "" {
			input.Filter = &gen.FilterClauseFindingInput{
				Reporter: &reporter,
			}
		}

		resp, err := Client.Findings.Export(ctx, input)
		if err != nil {
			ErrOut("failed to export findings: " + err.Error())
		}

		// Return export result
		if resp.ExportFindings.Export == nil {
			JSONOut(map[string]interface{}{})
			return nil
		}

		result := map[string]interface{}{
			"export_id": resp.ExportFindings.Export.Id,
		}

		JSONOut(result)
		return nil
	},
}

func init() {
	findingsExportCmd.Flags().StringP("ids", "", "", "Comma-separated finding IDs to export")
	findingsExportCmd.Flags().StringP("reporter", "", "", "Export findings from this reporter")

	Root.AddCommand(findingsExportCmd)
}
