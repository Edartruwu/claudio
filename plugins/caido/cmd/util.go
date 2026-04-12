package cmd

import (
	"encoding/json"
	"fmt"
	"os"
)

// RedactHeaders redacts sensitive headers from a map.
// Redacted headers: Authorization, Cookie, Set-Cookie, X-Api-Key, X-Auth-Token
func RedactHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}

	redacted := make(map[string]string)
	sensitiveHeaders := map[string]bool{
		"Authorization":  true,
		"Cookie":         true,
		"Set-Cookie":     true,
		"X-Api-Key":      true,
		"X-Auth-Token":   true,
	}

	for k, v := range headers {
		if sensitiveHeaders[k] {
			redacted[k] = "[REDACTED]"
		} else {
			redacted[k] = v
		}
	}

	return redacted
}

// CapBody truncates body to limit bytes, appending "...(truncated)" if cut.
func CapBody(body []byte, limit int) string {
	if len(body) <= limit {
		return string(body)
	}
	return string(body[:limit]) + "...(truncated)"
}

// JSONOut marshals v to JSON and writes to stdout.
// On error, prints {"error":"marshal failed"} and exits with code 1.
func JSONOut(v interface{}) {
	out, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stdout, `{"error":"marshal failed"}`)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "%s\n", string(out))
}

// ErrOut prints msg to stderr and exits with code 1.
func ErrOut(msg string) {
	fmt.Fprintf(os.Stderr, "%s\n", msg)
	os.Exit(1)
}
