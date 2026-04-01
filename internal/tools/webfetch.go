package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// WebFetchTool fetches and converts web pages to markdown.
type WebFetchTool struct{}

type webFetchInput struct {
	URL    string `json:"url"`
	Prompt string `json:"prompt,omitempty"`
}

func (t *WebFetchTool) Name() string { return "WebFetch" }

func (t *WebFetchTool) Description() string {
	return `Fetches content from a URL and converts it to readable text. Use this to read documentation, web pages, or API responses. Returns cleaned-up text content.`
}

func (t *WebFetchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "The URL to fetch"
			},
			"prompt": {
				"type": "string",
				"description": "What to extract from the page"
			}
		},
		"required": ["url"]
	}`)
}

func (t *WebFetchTool) IsReadOnly() bool                        { return true }
func (t *WebFetchTool) RequiresApproval(_ json.RawMessage) bool { return true }

func (t *WebFetchTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in webFetchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.URL == "" {
		return &Result{Content: "No URL provided", IsError: true}, nil
	}

	// Validate URL
	if !strings.HasPrefix(in.URL, "http://") && !strings.HasPrefix(in.URL, "https://") {
		in.URL = "https://" + in.URL
	}

	content, err := fetchURL(ctx, in.URL)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Fetch failed: %v", err), IsError: true}, nil
	}

	// Convert HTML to readable text
	text := htmlToText(content)

	// Truncate
	const maxLen = 100000
	if len(text) > maxLen {
		text = text[:maxLen] + "\n... (content truncated)"
	}

	if text == "" {
		return &Result{Content: "(empty page)"}, nil
	}

	return &Result{Content: text}, nil
}

func fetchURL(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Claudio/1.0 (Terminal AI Assistant)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,application/json")

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // 2MB limit
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// htmlToText converts HTML to readable plain text.
// This is a simplified version — for production, use github.com/JohannesKaufmann/html-to-markdown.
func htmlToText(html string) string {
	// Remove script and style blocks
	reScript := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	html = reScript.ReplaceAllString(html, "")
	reStyle := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	html = reStyle.ReplaceAllString(html, "")

	// Convert common elements
	html = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>|</li>|</tr>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`(?i)<h[1-6][^>]*>`).ReplaceAllString(html, "\n## ")
	html = regexp.MustCompile(`(?i)</h[1-6]>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`(?i)<li[^>]*>`).ReplaceAllString(html, "- ")
	html = regexp.MustCompile(`(?i)<a[^>]*href="([^"]*)"[^>]*>`).ReplaceAllString(html, "[$1](")
	html = strings.ReplaceAll(html, "</a>", ")")

	// Strip remaining tags
	reTags := regexp.MustCompile(`<[^>]+>`)
	html = reTags.ReplaceAllString(html, "")

	// Decode common entities
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", `"`)
	html = strings.ReplaceAll(html, "&#39;", "'")
	html = strings.ReplaceAll(html, "&nbsp;", " ")

	// Collapse whitespace
	reSpaces := regexp.MustCompile(`[ \t]+`)
	html = reSpaces.ReplaceAllString(html, " ")
	reLines := regexp.MustCompile(`\n{3,}`)
	html = reLines.ReplaceAllString(html, "\n\n")

	return strings.TrimSpace(html)
}
