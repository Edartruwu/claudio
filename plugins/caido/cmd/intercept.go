package cmd

import (
	"context"
	"encoding/base64"

	caido "github.com/caido-community/sdk-go"
	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/spf13/cobra"
)

var (
	interceptFilter string
	interceptLimit  int
	interceptAfter  string
	interceptAction string
	interceptRaw    string
)

var interceptStatusCmd = &cobra.Command{
	Use:   "intercept-status",
	Short: "Get intercept status",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Get status
		resp, err := Client.Intercept.GetStatus(ctx)
		if err != nil {
			ErrOut("failed to get intercept status: " + err.Error())
		}

		// Map status to string
		statusStr := string(resp.InterceptStatus)

		out := struct {
			State string `json:"state"`
		}{
			State: statusStr,
		}

		JSONOut(out)
		return nil
	},
}

var interceptControlCmd = &cobra.Command{
	Use:   "intercept-control",
	Short: "Control intercept state",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Validate --action is set
		if interceptAction == "" {
			ErrOut("flag --action is required")
		}

		// Validate action value
		validActions := map[string]bool{
			"pause":  true,
			"resume": true,
		}
		if !validActions[interceptAction] {
			ErrOut("invalid --action value: " + interceptAction)
		}

		// Execute action
		switch interceptAction {
		case "pause":
			_, err := Client.Intercept.Pause(ctx)
			if err != nil {
				ErrOut("failed to pause intercept: " + err.Error())
			}

			out := struct {
				Status string `json:"status"`
			}{
				Status: "paused",
			}
			JSONOut(out)

		case "resume":
			_, err := Client.Intercept.Resume(ctx)
			if err != nil {
				ErrOut("failed to resume intercept: " + err.Error())
			}

			out := struct {
				Status string `json:"status"`
			}{
				Status: "resumed",
			}
			JSONOut(out)
		}

		return nil
	},
}

var interceptListCmd = &cobra.Command{
	Use:   "intercept-list",
	Short: "List intercepted entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Validate limit
		if interceptLimit < 1 || interceptLimit > 100 {
			ErrOut("flag -n must be between 1 and 100")
		}

		// Build options
		opts := &caido.ListInterceptEntriesOptions{
			First: &interceptLimit,
		}
		if interceptFilter != "" {
			opts.Filter = &interceptFilter
		}
		if interceptAfter != "" {
			opts.After = &interceptAfter
		}

		// List entries
		resp, err := Client.Intercept.ListEntries(ctx, opts)
		if err != nil {
			ErrOut("failed to list intercept entries: " + err.Error())
		}

		// Extract entries
		type EntryItem struct {
			ID             string `json:"id"`
			Method         string `json:"method"`
			Host           string `json:"host"`
			Path           string `json:"path"`
			ResponseStatus int    `json:"response_status"`
		}

		var entries []EntryItem
		for _, edge := range resp.InterceptEntries.Edges {
			node := edge.Node
			req := node.Request
			status := 0
			if req.Response != nil {
				status = req.Response.StatusCode
			}
			entries = append(entries, EntryItem{
				ID:             node.Id,
				Method:         req.Method,
				Host:           req.Host,
				Path:           req.Path,
				ResponseStatus: status,
			})
		}

		// Build output
		nextCursor := ""
		if len(resp.InterceptEntries.Edges) > 0 && len(resp.InterceptEntries.Edges) == interceptLimit {
			nextCursor = resp.InterceptEntries.Edges[len(resp.InterceptEntries.Edges)-1].Cursor
		}

		out := struct {
			Entries    []EntryItem `json:"entries"`
			NextCursor string      `json:"next_cursor"`
			Total      int         `json:"total"`
		}{
			Entries:    entries,
			NextCursor: nextCursor,
			Total:      resp.InterceptEntries.Count.Value,
		}

		JSONOut(out)
		return nil
	},
}

var interceptForwardCmd = &cobra.Command{
	Use:   "intercept-forward <id>",
	Short: "Forward an intercepted request with optional modification",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		entryID := args[0]

		// Prepare input
		input := &gen.ForwardInterceptMessageInput{}

		// If --raw is provided, decode and use it
		if interceptRaw != "" {
			decoded, err := base64.StdEncoding.DecodeString(interceptRaw)
			if err != nil {
				ErrOut("failed to decode base64 --raw: " + err.Error())
			}
			// Create request message with the decoded raw data
			reqMsg := &gen.ForwardInterceptRequestMessageInput{
				UpdateRaw:           string(decoded),
				UpdateContentLength: false,
			}
			input.Request = reqMsg
		}

		// Forward
		_, err := Client.Intercept.Forward(ctx, entryID, input)
		if err != nil {
			ErrOut("failed to forward intercept: " + err.Error())
		}

		out := struct {
			Status string `json:"status"`
		}{
			Status: "forwarded",
		}
		JSONOut(out)
		return nil
	},
}

var interceptDropCmd = &cobra.Command{
	Use:   "intercept-drop <id>",
	Short: "Drop an intercepted request",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		entryID := args[0]

		// Drop
		_, err := Client.Intercept.Drop(ctx, entryID)
		if err != nil {
			ErrOut("failed to drop intercept: " + err.Error())
		}

		out := struct {
			Status string `json:"status"`
		}{
			Status: "dropped",
		}
		JSONOut(out)
		return nil
	},
}

func init() {
	// intercept-status
	Root.AddCommand(interceptStatusCmd)

	// intercept-control
	interceptControlCmd.Flags().StringVar(&interceptAction, "action", "", "action: pause|resume (required)")
	Root.AddCommand(interceptControlCmd)

	// intercept-list
	interceptListCmd.Flags().StringVarP(&interceptFilter, "filter", "f", "", "HTTPQL filter expression")
	interceptListCmd.Flags().IntVarP(&interceptLimit, "limit", "n", 20, "number of entries to return (1-100)")
	interceptListCmd.Flags().StringVar(&interceptAfter, "after", "", "pagination cursor")
	Root.AddCommand(interceptListCmd)

	// intercept-forward
	interceptForwardCmd.Flags().StringVar(&interceptRaw, "raw", "", "base64-encoded modified request")
	Root.AddCommand(interceptForwardCmd)

	// intercept-drop
	Root.AddCommand(interceptDropCmd)
}
