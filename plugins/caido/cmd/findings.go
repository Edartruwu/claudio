package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

// findingsCmd lists all findings
var findingsCmd = &cobra.Command{
	Use:   "findings",
	Short: "List findings",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// List findings
		resp, err := Client.Findings.List(ctx, nil)
		if err != nil {
			ErrOut("failed to list findings: " + err.Error())
		}

		// Extract findings from response
		var findings []map[string]interface{}
		for _, edge := range resp.Findings.Edges {
			finding := map[string]interface{}{
				"id":          edge.Node.Id,
				"title":       edge.Node.Title,
				"description": edge.Node.Description,
				"request_id":  edge.Node.Request.Id,
				"reporter":    edge.Node.Reporter,
				"severity":    nil, // SDK doesn't expose severity in this version
			}
			findings = append(findings, finding)
		}

		JSONOut(findings)
		return nil
	},
}

func init() {
	Root.AddCommand(findingsCmd)
}
