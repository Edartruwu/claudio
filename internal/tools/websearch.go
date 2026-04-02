package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/prompts"
)

// WebSearchTool searches the web.
type WebSearchTool struct {
	deferrable
}

type webSearchInput struct {
	Query          string   `json:"query"`
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	BlockedDomains []string `json:"blocked_domains,omitempty"`
}

func (t *WebSearchTool) Name() string { return "WebSearch" }

func (t *WebSearchTool) Description() string {
	return prompts.WebSearchDescription()
}

func (t *WebSearchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "The search query"
			},
			"allowed_domains": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Only return results from these domains"
			},
			"blocked_domains": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Exclude results from these domains"
			}
		},
		"required": ["query"]
	}`)
}

func (t *WebSearchTool) IsReadOnly() bool                        { return true }
func (t *WebSearchTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *WebSearchTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in webSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.Query == "" {
		return &Result{Content: "No query provided", IsError: true}, nil
	}

	// Use DuckDuckGo HTML search as a free fallback
	results, err := duckduckgoSearch(ctx, in.Query)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Search failed: %v", err), IsError: true}, nil
	}

	if results == "" {
		return &Result{Content: "No results found"}, nil
	}

	return &Result{Content: results}, nil
}

func duckduckgoSearch(ctx context.Context, query string) (string, error) {
	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Claudio/1.0 (Terminal AI Assistant)")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", err
	}

	// Simple HTML parsing for DuckDuckGo results
	return extractDDGResults(string(body)), nil
}

func extractDDGResults(html string) string {
	var results []string
	// Look for result snippets between class="result__snippet" tags
	parts := strings.Split(html, `class="result__a"`)
	for i, part := range parts {
		if i == 0 || i > 10 {
			continue
		}
		// Extract href
		href := extractAttr(part, `href="`, `"`)
		// Extract title text (rough)
		title := extractText(part, ">", "</a>")
		// Extract snippet
		snippet := ""
		if idx := strings.Index(part, `class="result__snippet"`); idx > 0 {
			snippet = extractText(part[idx:], ">", "</")
		}

		if href != "" && title != "" {
			result := fmt.Sprintf("%d. %s\n   %s\n   %s", i, strings.TrimSpace(title), href, strings.TrimSpace(snippet))
			results = append(results, result)
		}
	}

	if len(results) == 0 {
		return "No results could be parsed"
	}

	return strings.Join(results, "\n\n")
}

func extractAttr(s, prefix, suffix string) string {
	start := strings.Index(s, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)
	end := strings.Index(s[start:], suffix)
	if end < 0 {
		return ""
	}
	return s[start : start+end]
}

func extractText(s, prefix, suffix string) string {
	start := strings.Index(s, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)
	end := strings.Index(s[start:], suffix)
	if end < 0 {
		return ""
	}
	return s[start : start+end]
}

