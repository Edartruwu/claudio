package cmd

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/spf13/cobra"
)

var (
	historyFilter string
	historyLimit  int
	historyAfter  string
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "List HTTP requests from history",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Validate limit
		if historyLimit < 1 || historyLimit > 100 {
			ErrOut("flag -n must be between 1 and 100")
		}

		// Build options
		opts := &caido.ListRequestsOptions{
			First: &historyLimit,
		}
		if historyFilter != "" {
			opts.Filter = &historyFilter
		}
		if historyAfter != "" {
			opts.After = &historyAfter
		}

		// Call API
		resp, err := Client.Requests.List(ctx, opts)
		if err != nil {
			ErrOut("failed to list requests: " + err.Error())
		}

		// Extract requests and cursor
		type RequestItem struct {
			ID              string `json:"id"`
			Method          string `json:"method"`
			Host            string `json:"host"`
			Path            string `json:"path"`
			ResponseStatus  int    `json:"response_status"`
			ResponseLength  int    `json:"response_length"`
		}

		var requests []RequestItem
		for _, edge := range resp.Requests.Edges {
			node := edge.Node
			status := 0
			length := 0
			if node.Response != nil {
				status = node.Response.StatusCode
				length = node.Response.Length
			}
			requests = append(requests, RequestItem{
				ID:             node.Id,
				Method:         node.Method,
				Host:           node.Host,
				Path:           node.Path,
				ResponseStatus: status,
				ResponseLength: length,
			})
		}

		// Build output
		nextCursor := ""
		if len(resp.Requests.Edges) > 0 && len(resp.Requests.Edges) == historyLimit {
			nextCursor = resp.Requests.Edges[len(resp.Requests.Edges)-1].Cursor
		}

		out := struct {
			Requests   []RequestItem `json:"requests"`
			NextCursor string        `json:"next_cursor"`
			Total      int           `json:"total"`
		}{
			Requests:   requests,
			NextCursor: nextCursor,
			Total:      resp.Requests.Count.Value,
		}

		JSONOut(out)
		return nil
	},
}

func init() {
	historyCmd.Flags().StringVarP(&historyFilter, "filter", "f", "", "HTTPQL filter expression")
	historyCmd.Flags().IntVarP(&historyLimit, "limit", "n", 20, "number of requests to return (1-100)")
	historyCmd.Flags().StringVar(&historyAfter, "after", "", "pagination cursor")
	Root.AddCommand(historyCmd)
}
