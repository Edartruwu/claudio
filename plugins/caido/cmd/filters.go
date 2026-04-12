package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

var filtersCmd = &cobra.Command{
	Use:   "filters",
	Short: "List saved HTTPQL filter presets",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// List filter presets
		resp, err := Client.Filters.List(ctx)
		if err != nil {
			ErrOut("failed to list filters: " + err.Error())
		}

		// Extract filters from response
		var filters []map[string]interface{}
		for _, filter := range resp.FilterPresets {
			f := map[string]interface{}{
				"id":    filter.Id,
				"name":  filter.Name,
				"alias": filter.Alias,
				"clause": filter.Clause,
			}
			filters = append(filters, f)
		}

		JSONOut(filters)
		return nil
	},
}

func init() {
	Root.AddCommand(filtersCmd)
}
