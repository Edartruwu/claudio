package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	gen "github.com/caido-community/sdk-go/graphql"
	"github.com/spf13/cobra"
)

var (
	replayHost      string
	replayPort      int
	replayTLS       bool
	replaySessionID string
)

var replayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Send a raw HTTP request via replay",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Read raw HTTP from stdin
		rawHTTP, err := io.ReadAll(os.Stdin)
		if err != nil {
			ErrOut("failed to read stdin: " + err.Error())
		}

		// Default host and port from flags
		host := replayHost
		port := replayPort
		tls := replayTLS

		// Build connection info
		connInfo := gen.ConnectionInfoInput{
			Host:  host,
			Port:  port,
			IsTLS: tls,
		}

		// Build replay input
		input := &gen.StartReplayTaskInput{
			Connection: connInfo,
			Raw:        string(rawHTTP),
			Settings: gen.ReplayEntrySettingsInput{
				ConnectionClose:     false,
				Placeholders:        []gen.ReplayPlaceholderInput{},
				UpdateContentLength: false,
			},
		}

		// Create session if needed
		if replaySessionID == "" {
			// Use a default session or create one
			// For now, we'll send without session
		}

		// Send request
		resp, err := Client.Replay.SendRequest(ctx, replaySessionID, input)
		if err != nil {
			ErrOut("failed to send replay request: " + err.Error())
		}

		// Get entry ID from response
		var entryID string
		if resp.StartReplayTask.Task != nil {
			entryID = resp.StartReplayTask.Task.Id
		}

		// Poll for response (up to 10s)
		var entry *gen.GetReplayEntryReplayEntry
		for i := 0; i < 100; i++ {
			entryResp, err := Client.Replay.GetEntry(ctx, entryID)
			if err == nil && entryResp.ReplayEntry != nil {
				entry = entryResp.ReplayEntry
				// Check if response is ready
				if entry.Request != nil && entry.Request.Response != nil {
					break
				}
			}
			time.Sleep(100 * time.Millisecond)
		}

		// Parse response
		status := 0
		headers := map[string]string{}
		body := ""

		if entry != nil && entry.Request != nil && entry.Request.Response != nil {
			respRaw := entry.Request.Response.Raw
			// Extract status, headers, body from raw response
			headers = parseHeaders(respRaw)
			body = extractBody(respRaw)
			// Parse status code from response line
			if respRaw != "" {
				// First line is like "HTTP/1.1 200 OK"
				statusLine := ""
				for i, c := range respRaw {
					if c == '\n' {
						statusLine = respRaw[:i]
						break
					}
				}
				// Extract status code
				var sc int
				fmt.Sscanf(statusLine, "HTTP/1.%d %d", nil, &sc)
				status = sc
			}
		}

		// Redact headers
		headers = RedactHeaders(headers)

		// Cap body
		limit := Cfg.BodyLimit
		body = CapBody([]byte(body), limit)

		out := struct {
			Status       int               `json:"status"`
			Headers      map[string]string `json:"headers"`
			Body         string            `json:"body"`
			ReplayEntryID string            `json:"replay_entry_id"`
		}{
			Status:        status,
			Headers:       headers,
			Body:          body,
			ReplayEntryID: entryID,
		}

		JSONOut(out)
		return nil
	},
}

func init() {
	replayCmd.Flags().StringVar(&replayHost, "host", "127.0.0.1", "target host")
	replayCmd.Flags().IntVar(&replayPort, "port", 80, "target port")
	replayCmd.Flags().BoolVar(&replayTLS, "tls", true, "use TLS")
	replayCmd.Flags().StringVar(&replaySessionID, "session-id", "", "replay session ID")
	Root.AddCommand(replayCmd)
}
