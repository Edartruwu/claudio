package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// BundleMockupTool reads an HTML entry file, inlines local <script src="...">
// references, optionally fetches and embeds CDN script tags, then writes a
// single self-contained HTML file. Pure Go — no CGO, no exec.Command.
type BundleMockupTool struct {
	designsDir string
}

// NewBundleMockupTool creates a BundleMockupTool that defaults output under designsDir.
func NewBundleMockupTool(designsDir string) *BundleMockupTool {
	return &BundleMockupTool{designsDir: designsDir}
}

// BundleMockupInput is the JSON input schema for this tool.
type BundleMockupInput struct {
	EntryHTML  string            `json:"entry_html"`
	OutputPath string            `json:"output_path"`
	Files      map[string]string `json:"files"`
	EmbedCDN   *bool             `json:"embed_cdn"` // pointer so we can detect omission
}

// BundleMockupOutput is the JSON result returned by this tool.
type BundleMockupOutput struct {
	OutputPath     string   `json:"output_path"`
	SizeBytes      int64    `json:"size_bytes"`
	EmbeddedDeps   []string `json:"embedded_deps"`
	OfflineCapable bool     `json:"offline_capable"`
}

func (t *BundleMockupTool) Name() string { return "BundleMockup" }

func (t *BundleMockupTool) Description() string {
	return `Bundle an HTML mockup into a single self-contained file.

Reads an HTML entry file, inlines all local <script src="..."> references
(JSX, JS, etc.), optionally fetches CDN script tags (React, ReactDOM, Babel)
via HTTP and embeds them inline, then writes the result to output_path.
Returns the output path, file size, list of embedded CDN deps, and whether
the result is offline-capable.

Pure Go — works without Node.js, Playwright, or any external dependencies.`
}

func (t *BundleMockupTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"entry_html": {
				"type": "string",
				"description": "Absolute or relative path to the HTML entry file."
			},
			"output_path": {
				"type": "string",
				"description": "Where to write the bundled HTML. Defaults to {designsDir}/{timestamp}/bundle/mockup.html."
			},
			"files": {
				"type": "object",
				"description": "Optional explicit file map: {\"tokens.jsx\": \"/path/to/tokens.jsx\", ...}. Overrides automatic resolution.",
				"additionalProperties": {"type": "string"}
			},
			"embed_cdn": {
				"type": "boolean",
				"description": "Fetch CDN <script> URLs and embed inline. Default: true."
			}
		},
		"required": ["entry_html"]
	}`)
}

func (t *BundleMockupTool) IsReadOnly() bool { return false }

func (t *BundleMockupTool) RequiresApproval(_ json.RawMessage) bool { return false }

// localScriptRe matches any <script src="...">; local vs CDN distinguished in code.
var localScriptRe = regexp.MustCompile(`(?i)<script([^>]*)\bsrc="([^"]+)"([^>]*)>(\s*</script>)?`)

// cdnScriptRe matches <script src="https://..."> or <script src="http://...">.
var cdnScriptRe = regexp.MustCompile(`(?i)<script([^>]*)\bsrc="(https?://[^"]+)"([^>]*)>(\s*</script>)?`)

func (t *BundleMockupTool) Execute(_ context.Context, input json.RawMessage) (*Result, error) {
	var in BundleMockupInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.EntryHTML == "" {
		return &Result{Content: "entry_html is required", IsError: true}, nil
	}

	// Default embed_cdn = true when omitted.
	embedCDN := true
	if in.EmbedCDN != nil {
		embedCDN = *in.EmbedCDN
	}

	// Read entry HTML.
	htmlBytes, err := os.ReadFile(in.EntryHTML)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to read entry_html %q: %v", in.EntryHTML, err), IsError: true}, nil
	}
	html := string(htmlBytes)
	htmlDir := filepath.Dir(in.EntryHTML)

	var warnings []string

	// --- 1. Inline local <script src="..."> tags ---
	html = localScriptRe.ReplaceAllStringFunc(html, func(match string) string {
		groups := localScriptRe.FindStringSubmatch(match)
		if groups == nil {
			return match
		}
		srcRef := groups[2] // e.g. "./tokens.jsx" or "tokens.jsx"
		// Skip CDN URLs — handled by cdnScriptRe below.
		if strings.HasPrefix(srcRef, "http://") || strings.HasPrefix(srcRef, "https://") {
			return match
		}
		srcName := filepath.Base(srcRef)

		// Determine actual file path: explicit map takes priority.
		var srcPath string
		if in.Files != nil {
			if mapped, ok := in.Files[srcName]; ok {
				srcPath = mapped
			} else if mapped, ok := in.Files[srcRef]; ok {
				srcPath = mapped
			}
		}
		if srcPath == "" {
			// Resolve relative to the HTML file's directory.
			srcPath = filepath.Join(htmlDir, filepath.FromSlash(srcRef))
		}

		content, err := os.ReadFile(srcPath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("could not read local script %q (%s): %v", srcRef, srcPath, err))
			return match // leave original tag unchanged
		}

		// Preserve any other attributes (e.g. type="text/babel") but strip src.
		attrs := strings.TrimSpace(groups[1] + " " + groups[3])
		attrs = removeSrcAttr(attrs)
		if attrs != "" {
			return fmt.Sprintf("<script %s>\n%s\n</script>", strings.TrimSpace(attrs), string(content))
		}
		return fmt.Sprintf("<script>\n%s\n</script>", string(content))
	})

	// --- 2. Optionally embed CDN <script src="https://..."> tags ---
	var embeddedDeps []string
	remainingCDN := 0

	if embedCDN {
		client := &http.Client{Timeout: 30 * time.Second}

		html = cdnScriptRe.ReplaceAllStringFunc(html, func(match string) string {
			groups := cdnScriptRe.FindStringSubmatch(match)
			if groups == nil {
				return match
			}
			url := groups[2]

			resp, err := client.Get(url) //nolint:noctx
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("CDN fetch failed for %q: %v — leaving as-is", url, err))
				remainingCDN++
				return match
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				warnings = append(warnings, fmt.Sprintf("CDN fetch %q returned HTTP %d — leaving as-is", url, resp.StatusCode))
				remainingCDN++
				return match
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("CDN read failed for %q: %v — leaving as-is", url, err))
				remainingCDN++
				return match
			}

			dep := extractDepName(url)
			if dep != "" {
				embeddedDeps = append(embeddedDeps, dep)
			}

			attrs := strings.TrimSpace(groups[1] + " " + groups[3])
			attrs = removeSrcAttr(attrs)
			if attrs != "" {
				return fmt.Sprintf("<script %s>\n%s\n</script>", strings.TrimSpace(attrs), string(body))
			}
			return fmt.Sprintf("<script>\n%s\n</script>", string(body))
		})
	}

	// --- 3. Resolve output path ---
	outPath := in.OutputPath
	if outPath == "" {
		ts := time.Now().Format("20060102-150405")
		outPath = filepath.Join(t.designsDir, ts, "bundle", "mockup.html")
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create output dir: %v", err), IsError: true}, nil
	}

	if err := os.WriteFile(outPath, []byte(html), 0644); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to write output: %v", err), IsError: true}, nil
	}

	info, err := os.Stat(outPath)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to stat output: %v", err), IsError: true}, nil
	}

	offlineCapable := embedCDN && remainingCDN == 0

	out := BundleMockupOutput{
		OutputPath:     outPath,
		SizeBytes:      info.Size(),
		EmbeddedDeps:   embeddedDeps,
		OfflineCapable: offlineCapable,
	}

	outJSON, _ := json.MarshalIndent(out, "", "  ")

	var sb strings.Builder
	sb.Write(outJSON)
	if len(warnings) > 0 {
		sb.WriteString("\n\nWarnings:\n")
		for _, w := range warnings {
			sb.WriteString("  - ")
			sb.WriteString(w)
			sb.WriteString("\n")
		}
	}

	return &Result{Content: sb.String()}, nil
}

// removeSrcAttr strips src="..." from an attribute string.
var srcAttrRe = regexp.MustCompile(`(?i)\s*\bsrc="[^"]*"`)

func removeSrcAttr(attrs string) string {
	return strings.TrimSpace(srcAttrRe.ReplaceAllString(attrs, ""))
}

// extractDepName attempts to extract "pkg@version" from a CDN URL.
// Examples:
//
//	https://unpkg.com/react@18.3.1/umd/react.development.js → react@18.3.1
//	https://cdn.jsdelivr.net/npm/react-dom@18.3.1/+esm       → react-dom@18.3.1
func extractDepName(rawURL string) string {
	// Strip protocol + host to get path segment
	path := rawURL
	if idx := strings.Index(rawURL, "://"); idx != -1 {
		rest := rawURL[idx+3:]
		if slash := strings.Index(rest, "/"); slash != -1 {
			path = rest[slash+1:]
		}
	}

	// jsdelivr: /npm/pkg@ver/... or /gh/...
	path = strings.TrimPrefix(path, "npm/")
	path = strings.TrimPrefix(path, "gh/")

	// Take first path segment (before next /)
	if slash := strings.Index(path, "/"); slash != -1 {
		path = path[:slash]
	}

	// path should now be "react@18.3.1" or similar
	if strings.Contains(path, "@") && path != "" {
		return path
	}
	return ""
}
