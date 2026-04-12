package cmd

import (
	"bufio"
	"context"
	"strings"

	"github.com/spf13/cobra"
)

var (
	requestHeaders bool
	requestBody    bool
	requestOffset  int
	requestLimit   int
)

var requestCmd = &cobra.Command{
	Use:   "request <id>",
	Short: "Get request details by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		id := args[0]

		// Get limit from config if not set
		limit := requestLimit
		if limit == 0 {
			limit = Cfg.BodyLimit
		}

		// Call API
		resp, err := Client.Requests.Get(ctx, id)
		if err != nil {
			ErrOut("failed to get request: " + err.Error())
		}

		if resp.Request == nil {
			ErrOut("request not found")
		}

		req := resp.Request

		// Parse request raw to extract headers and body
		reqHeaders := parseHeaders(req.Raw)
		reqBodyStr := extractBody(req.Raw)

		// Parse response raw to extract headers and body
		respHeaders := map[string]string{}
		respBodyStr := ""
		if req.Response != nil {
			respHeaders = parseHeaders(req.Response.Raw)
			respBodyStr = extractBody(req.Response.Raw)
		}

		// Redact headers
		reqHeaders = RedactHeaders(reqHeaders)
		respHeaders = RedactHeaders(respHeaders)

		// Cap bodies
		if requestBody {
			reqBodyStr = CapBody([]byte(reqBodyStr), limit)
		} else {
			reqBodyStr = ""
		}
		if requestBody && req.Response != nil {
			respBodyStr = CapBody([]byte(respBodyStr), limit)
		} else {
			respBodyStr = ""
		}

		// Build output
		type HeaderObj struct {
			Headers map[string]string `json:"headers,omitempty"`
		}

		type RequestObj struct {
			ID       string              `json:"id"`
			Method   string              `json:"method"`
			Host     string              `json:"host"`
			Port     int                 `json:"port"`
			Path     string              `json:"path"`
			Query    string              `json:"query"`
			IsTLS    bool                `json:"is_tls"`
			Raw      string              `json:"raw,omitempty"`
			Headers  map[string]string   `json:"headers,omitempty"`
			Body     string              `json:"body,omitempty"`
		}

		type ResponseObj struct {
			ID            string            `json:"id,omitempty"`
			StatusCode    int               `json:"status_code"`
			RoundtripTime int               `json:"roundtrip_time"`
			Length        int               `json:"length"`
			Raw           string            `json:"raw,omitempty"`
			Headers       map[string]string `json:"headers,omitempty"`
			Body          string            `json:"body,omitempty"`
		}

		reqOut := RequestObj{
			ID:     req.Id,
			Method: req.Method,
			Host:   req.Host,
			Port:   req.Port,
			Path:   req.Path,
			Query:  req.Query,
			IsTLS:  req.IsTls,
		}
		if requestHeaders {
			reqOut.Headers = reqHeaders
		}
		if requestBody {
			reqOut.Body = reqBodyStr
		}

		respOut := ResponseObj{}
		if req.Response != nil {
			respOut = ResponseObj{
				ID:            req.Response.Id,
				StatusCode:    req.Response.StatusCode,
				RoundtripTime: req.Response.RoundtripTime,
				Length:        req.Response.Length,
			}
			if requestHeaders {
				respOut.Headers = respHeaders
			}
			if requestBody {
				respOut.Body = respBodyStr
			}
		}

		out := struct {
			Request  RequestObj   `json:"request"`
			Response ResponseObj  `json:"response"`
		}{
			Request:  reqOut,
			Response: respOut,
		}

		JSONOut(out)
		return nil
	},
}

// parseHeaders extracts HTTP headers from raw HTTP text.
// Assumes first line is request/status line, blank line separates headers from body.
func parseHeaders(raw string) map[string]string {
	headers := make(map[string]string)
	if raw == "" {
		return headers
	}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	inHeaders := false
	for scanner.Scan() {
		line := scanner.Text()

		// Skip first line (request/status line)
		if !inHeaders {
			inHeaders = true
			continue
		}

		// Blank line signals end of headers
		if line == "" {
			break
		}

		// Parse header
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			headers[key] = value
		}
	}

	return headers
}

// extractBody extracts body from raw HTTP text (everything after first blank line).
func extractBody(raw string) string {
	parts := strings.Split(raw, "\r\n\r\n")
	if len(parts) > 1 {
		return parts[1]
	}
	// Try with just newlines
	parts = strings.Split(raw, "\n\n")
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

func init() {
	requestCmd.Flags().BoolVar(&requestHeaders, "headers", false, "include headers in output")
	requestCmd.Flags().BoolVar(&requestBody, "body", false, "include body in output")
	requestCmd.Flags().IntVar(&requestOffset, "offset", 0, "body offset (unused)")
	requestCmd.Flags().IntVar(&requestLimit, "limit", 0, "body limit (default from config)")
	Root.AddCommand(requestCmd)
}
