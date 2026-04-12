package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	replayEntryOffset int
	replayEntryLimit  int
)

var replayEntryCmd = &cobra.Command{
	Use:   "replay-entry <id>",
	Short: "Get replay entry details by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		id := args[0]

		// Get limit from config if not set
		limit := replayEntryLimit
		if limit == 0 {
			limit = Cfg.BodyLimit
		}

		// Call API
		resp, err := Client.Replay.GetEntry(ctx, id)
		if err != nil {
			ErrOut("failed to get replay entry: " + err.Error())
		}

		if resp.ReplayEntry == nil {
			ErrOut("replay entry not found")
		}

		entry := resp.ReplayEntry

		// Extract response info
		respStatus := 0
		respHeaders := map[string]string{}
		respBody := ""
		if entry.Request != nil && entry.Request.Response != nil {
			respHeaders = parseHeaders(entry.Request.Response.Raw)
			respBody = extractBody(entry.Request.Response.Raw)
			// Parse status code (simple extraction)
			respRaw := entry.Request.Response.Raw
			if respRaw != "" {
				statusLine := ""
				for i, c := range respRaw {
					if c == '\n' {
						statusLine = respRaw[:i]
						break
					}
				}
				// Extract status code from response line
				var minor int
				fmt.Sscanf(statusLine, "HTTP/1.%d %d", &minor, &respStatus)
			}
		}

		// Redact headers
		respHeaders = RedactHeaders(respHeaders)

		// Cap body
		respBody = CapBody([]byte(respBody), limit)

		// Build output
		type RequestInfo struct {
			ID    string `json:"id"`
			Raw   string `json:"raw,omitempty"`
		}

		type ResponseInfo struct {
			StatusCode int               `json:"status_code"`
			Headers    map[string]string `json:"headers,omitempty"`
			Body       string            `json:"body,omitempty"`
		}

		type ConnectionInfo struct {
			Host  string `json:"host"`
			Port  int    `json:"port"`
			IsTLS bool   `json:"is_tls"`
		}

		reqInfo := RequestInfo{}
		if entry.Request != nil {
			reqInfo.ID = entry.Request.Id
		}

		respInfo := ResponseInfo{
			StatusCode: respStatus,
			Headers:    respHeaders,
			Body:       respBody,
		}

		connInfo := ConnectionInfo{
			Host:  entry.Connection.Host,
			Port:  entry.Connection.Port,
			IsTLS: entry.Connection.IsTLS,
		}

		out := struct {
			ID         string        `json:"id"`
			Request    RequestInfo   `json:"request"`
			Response   ResponseInfo  `json:"response"`
			Connection ConnectionInfo `json:"connection"`
			Error      *string       `json:"error,omitempty"`
		}{
			ID:         entry.Id,
			Request:    reqInfo,
			Response:   respInfo,
			Connection: connInfo,
			Error:      entry.Error,
		}

		JSONOut(out)
		return nil
	},
}



func init() {
	replayEntryCmd.Flags().IntVar(&replayEntryOffset, "offset", 0, "body offset (unused)")
	replayEntryCmd.Flags().IntVar(&replayEntryLimit, "limit", 0, "body limit (default from config)")
	Root.AddCommand(replayEntryCmd)
}
