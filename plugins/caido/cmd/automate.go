package cmd

import (
	"context"

	caido "github.com/caido-community/sdk-go"
	"github.com/spf13/cobra"
)

var (
	automateEntryLimit  int
	automateEntryAfter  string
	automateAction      string
	automateSessionID   string
	automateTaskID      string
)

var automateSessionsCmd = &cobra.Command{
	Use:   "automate-sessions",
	Short: "List fuzzing sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// List sessions with default options
		resp, err := Client.Automate.ListSessions(ctx, nil)
		if err != nil {
			ErrOut("failed to list sessions: " + err.Error())
		}

		// Extract sessions
		type SessionItem struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			CreatedAt int64  `json:"created_at"`
		}

		var sessions []SessionItem
		for _, edge := range resp.AutomateSessions.Edges {
			node := edge.Node
			sessions = append(sessions, SessionItem{
				ID:        node.Id,
				Name:      node.Name,
				CreatedAt: node.CreatedAt,
			})
		}

		JSONOut(sessions)
		return nil
	},
}

var automateSessionCmd = &cobra.Command{
	Use:   "automate-session <id>",
	Short: "Get session details and entry list",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		sessionID := args[0]

		// Get session details
		resp, err := Client.Automate.GetSession(ctx, sessionID)
		if err != nil {
			ErrOut("failed to get session: " + err.Error())
		}

		session := resp.AutomateSession
		if session == nil {
			ErrOut("session not found")
		}

		// Extract entries
		type EntryItem struct {
			ID        string `json:"id"`
			CreatedAt int64  `json:"created_at"`
		}

		var entries []EntryItem
		for _, e := range session.Entries {
			entries = append(entries, EntryItem{
				ID:        e.Id,
				CreatedAt: e.CreatedAt,
			})
		}

		out := struct {
			ID        string      `json:"id"`
			Name      string      `json:"name"`
			Raw       string      `json:"raw"`
			CreatedAt int64       `json:"created_at"`
			Entries   []EntryItem `json:"entries"`
		}{
			ID:        session.Id,
			Name:      session.Name,
			Raw:       session.Raw,
			CreatedAt: session.CreatedAt,
			Entries:   entries,
		}

		JSONOut(out)
		return nil
	},
}

var automateEntryCmd = &cobra.Command{
	Use:   "automate-entry <id>",
	Short: "Get fuzzing entry results and payloads",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		entryID := args[0]

		// Validate limit
		if automateEntryLimit < 1 || automateEntryLimit > 100 {
			ErrOut("flag -n must be between 1 and 100")
		}

		// Build options for listing requests
		opts := &caido.ListEntryRequestsOptions{
			First: &automateEntryLimit,
		}
		if automateEntryAfter != "" {
			opts.After = &automateEntryAfter
		}

		// Get entry requests
		resp, err := Client.Automate.GetEntryRequests(ctx, entryID, opts)
		if err != nil {
			ErrOut("failed to get entry requests: " + err.Error())
		}

		// Extract requests
		type PayloadItem struct {
			Position *int    `json:"position"`
			Raw      *string `json:"raw"`
		}

		type RequestItem struct {
			SequenceID string         `json:"sequence_id"`
			Payloads   []PayloadItem  `json:"payloads"`
		}

		var requests []RequestItem
		for _, edge := range resp.AutomateEntry.Requests.Edges {
			node := edge.Node

			// Extract payloads
			var payloads []PayloadItem
			for _, p := range node.Payloads {
				payloads = append(payloads, PayloadItem{
					Position: p.Position,
					Raw:      p.Raw,
				})
			}

			requests = append(requests, RequestItem{
				SequenceID: node.SequenceId,
				Payloads:   payloads,
			})
		}

		// Build output
		nextCursor := ""
		if len(resp.AutomateEntry.Requests.Edges) > 0 && len(resp.AutomateEntry.Requests.Edges) == automateEntryLimit {
			nextCursor = resp.AutomateEntry.Requests.Edges[len(resp.AutomateEntry.Requests.Edges)-1].Cursor
		}

		out := struct {
			Requests   []RequestItem `json:"requests"`
			NextCursor string        `json:"next_cursor"`
			Total      int           `json:"total"`
		}{
			Requests:   requests,
			NextCursor: nextCursor,
			Total:      resp.AutomateEntry.Requests.Count.Value,
		}

		JSONOut(out)
		return nil
	},
}

var automateControlCmd = &cobra.Command{
	Use:   "automate-control",
	Short: "Control automate tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Validate --action is set
		if automateAction == "" {
			ErrOut("flag --action is required")
		}

		// Validate action value
		validActions := map[string]bool{
			"start":  true,
			"pause":  true,
			"resume": true,
			"cancel": true,
		}
		if !validActions[automateAction] {
			ErrOut("invalid --action value: " + automateAction)
		}

		// Validate required flags per action
		if automateAction == "start" {
			if automateSessionID == "" {
				ErrOut("--session is required for start action")
			}
		} else {
			// pause, resume, cancel require --task
			if automateTaskID == "" {
				ErrOut("--task is required for " + automateAction + " action")
			}
		}

		// Execute action
		switch automateAction {
		case "start":
			resp, err := Client.Automate.StartTask(ctx, automateSessionID)
			if err != nil {
				ErrOut("failed to start task: " + err.Error())
			}

			task := resp.StartAutomateTask.AutomateTask
			taskID := ""
			if task != nil {
				taskID = task.Id
			}
			out := struct {
				ID string `json:"id"`
			}{
				ID: taskID,
			}
			JSONOut(out)

		case "pause":
			_, err := Client.Automate.PauseTask(ctx, automateTaskID)
			if err != nil {
				ErrOut("failed to pause task: " + err.Error())
			}

			out := struct {
				Status string `json:"status"`
			}{
				Status: "paused",
			}
			JSONOut(out)

		case "resume":
			_, err := Client.Automate.ResumeTask(ctx, automateTaskID)
			if err != nil {
				ErrOut("failed to resume task: " + err.Error())
			}

			out := struct {
				Status string `json:"status"`
			}{
				Status: "resumed",
			}
			JSONOut(out)

		case "cancel":
			_, err := Client.Automate.CancelTask(ctx, automateTaskID)
			if err != nil {
				ErrOut("failed to cancel task: " + err.Error())
			}

			out := struct {
				Status string `json:"status"`
			}{
				Status: "cancelled",
			}
			JSONOut(out)
		}

		return nil
	},
}

func init() {
	// automate-sessions
	Root.AddCommand(automateSessionsCmd)

	// automate-session
	Root.AddCommand(automateSessionCmd)

	// automate-entry
	automateEntryCmd.Flags().IntVarP(&automateEntryLimit, "limit", "n", 20, "number of payloads to return (1-100)")
	automateEntryCmd.Flags().StringVar(&automateEntryAfter, "after", "", "pagination cursor")
	Root.AddCommand(automateEntryCmd)

	// automate-control
	automateControlCmd.Flags().StringVar(&automateAction, "action", "", "action: start|pause|resume|cancel (required)")
	automateControlCmd.Flags().StringVar(&automateSessionID, "session", "", "session ID (required for start)")
	automateControlCmd.Flags().StringVar(&automateTaskID, "task", "", "task ID (required for pause|resume|cancel)")
	Root.AddCommand(automateControlCmd)
}
