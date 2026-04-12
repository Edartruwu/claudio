package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

var replaySessionsCmd = &cobra.Command{
	Use:   "replay-sessions",
	Short: "List replay sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Call API
		resp, err := Client.Replay.ListSessions(ctx, nil)
		if err != nil {
			ErrOut("failed to list replay sessions: " + err.Error())
		}

		// Extract sessions
		type Session struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}

		var sessions []Session
		for _, edge := range resp.ReplaySessions.Edges {
			node := edge.Node
			sessions = append(sessions, Session{
				ID:   node.Id,
				Name: node.Name,
			})
		}

		JSONOut(sessions)
		return nil
	},
}

func init() {
	Root.AddCommand(replaySessionsCmd)
}
