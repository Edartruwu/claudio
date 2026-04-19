package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// ExportHandoffTool packages a mockup directory into a developer handoff bundle.
// It parses HTML files to extract component inventory, asset references, and
// interaction points, then writes spec.md and tokens-used.json.
type ExportHandoffTool struct {
	designsDir string
}

// NewExportHandoffTool creates an ExportHandoffTool that defaults output under designsDir.
func NewExportHandoffTool(designsDir string) *ExportHandoffTool {
	return &ExportHandoffTool{designsDir: designsDir}
}

// ExportHandoffInput is the JSON input schema for this tool.
type ExportHandoffInput struct {
	MockupDir    string `json:"mockup_dir"`    // path to dir containing index.html + screen HTMLs
	SessionDir   string `json:"session_dir"`   // optional: reuse existing session dir for handoff output
	Framework    string `json:"framework"`     // "react" | "vue" | "svelte" | "vanilla" — default: "react"
	DesignTokens string `json:"design_tokens"` // optional path to design-system.json
	ProjectName  string `json:"project_name"`  // used in spec header
}

// ExportHandoffOutput is the JSON result returned by this tool.
type ExportHandoffOutput struct {
	Success        bool     `json:"success"`
	HandoffDir     string   `json:"handoff_dir"`      // path to handoff/ subdir
	SpecPath       string   `json:"spec_path"`        // handoff/spec.md
	TokensUsedPath string   `json:"tokens_used_path"` // handoff/tokens-used.json
	TokensJsonPath string   `json:"tokens_json_path"` // handoff/tokens.json (copied from session dir)
	TokensCSSPath  string   `json:"tokens_css_path"`  // handoff/tokens.css (generated CSS vars)
	ComponentCount int      `json:"component_count"`
	AssetCount     int      `json:"asset_count"`
	Warnings       []string `json:"warnings"`
}

func (t *ExportHandoffTool) Name() string { return "ExportHandoff" }

func (t *ExportHandoffTool) Description() string {
	return `Package a mockup directory into a developer handoff bundle.

Reads index.html and screen-*.html files from mockup_dir, parses component
inventory (Tailwind classes), asset references, and interaction points, then
writes handoff/spec.md and handoff/tokens-used.json. Optionally cross-references
a design-system.json to list which tokens are actually used.

Returns paths to all generated artifacts and counts of components and assets.`
}

func (t *ExportHandoffTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
	"type": "object",
	"properties": {
		"mockup_dir": {
			"type": "string",
			"description": "Absolute path to the mockup directory containing index.html and screen-*.html files."
		},
		"session_dir": {
			"type": "string",
			"description": "Session directory to write handoff into ({session_dir}/handoff/). Pass the same session_dir used for RenderMockup/BundleMockup to keep all outputs together."
		},
		"framework": {
			"type": "string",
			"description": "Target framework: react | vue | svelte | vanilla. Default: react.",
			"enum": ["react", "vue", "svelte", "vanilla"]
		},
		"design_tokens": {
			"type": "string",
			"description": "Optional path to design-system.json. Used to compute which tokens appear in the HTML."
		},
		"project_name": {
			"type": "string",
			"description": "Project name used in spec header."
		}
	},
	"required": ["mockup_dir"]
}`)
}

func (t *ExportHandoffTool) IsReadOnly() bool { return false }

func (t *ExportHandoffTool) RequiresApproval(_ json.RawMessage) bool { return false }

// ---- regex patterns for HTML parsing ----

var (
	// data-artboard="screen-name"
	artboardRe = regexp.MustCompile(`(?i)data-artboard="([^"]+)"`)
	// class="..." — captures full class string
	classAttrRe = regexp.MustCompile(`(?i)\bclass="([^"]+)"`)
	// <img src="...">
	imgSrcRe = regexp.MustCompile(`(?i)<img[^>]+\bsrc="([^"]+)"`)
	// <link href="...">
	linkHrefRe = regexp.MustCompile(`(?i)<link[^>]+\bhref="([^"]+)"`)
	// @import url("...") or @import url('...') or @import url(...)
	cssImportRe = regexp.MustCompile(`@import\s+url\(["']?([^"')]+)["']?\)`)
	// href="..." on <a> tags
	anchorHrefRe = regexp.MustCompile(`(?i)<a[^>]+\bhref="([^"#][^"]*)"`)
	// onclick="..."
	onclickRe = regexp.MustCompile(`(?i)\bonclick="([^"]+)"`)
	// data-action="..."
	dataActionRe = regexp.MustCompile(`(?i)\bdata-action="([^"]+)"`)
	// font-family or @import of Google Fonts
	fontFamilyRe = regexp.MustCompile(`(?i)font-family:\s*['"]?([A-Za-z][A-Za-z0-9 _-]+)['"]?`)
	// CDN script src for icon libraries
	iconCDNRe = regexp.MustCompile(`(?i)<script[^>]+\bsrc="(https?://[^"]*(?:feather|lucide|heroicon|fontawesome|ionicon|bootstrap-icon)[^"]*)"`)
)

// componentPattern maps component names to Tailwind class keywords.
type componentPattern struct {
	name     string
	keywords []string
}

var componentPatterns = []componentPattern{
	{name: "Button", keywords: []string{"btn", "button"}},
	{name: "Input", keywords: []string{"input", "form-input", "text-input", "form-control"}},
	{name: "Card", keywords: []string{"card"}},
	{name: "Nav", keywords: []string{"nav", "navbar", "navigation"}},
	{name: "Modal", keywords: []string{"modal", "dialog", "overlay"}},
	{name: "Table", keywords: []string{"table", "thead", "tbody"}},
	{name: "Badge", keywords: []string{"badge", "tag", "chip"}},
	{name: "Form", keywords: []string{"form"}},
	{name: "Header", keywords: []string{"header"}},
	{name: "Sidebar", keywords: []string{"sidebar"}},
	{name: "Footer", keywords: []string{"footer"}},
	{name: "Dropdown", keywords: []string{"dropdown", "select", "combobox"}},
	{name: "Tabs", keywords: []string{"tabs", "tab-"}},
	{name: "Alert", keywords: []string{"alert", "toast", "notification"}},
	{name: "Avatar", keywords: []string{"avatar", "profile-pic"}},
}

// ---- internal data types ----

type screenInfo struct {
	Name string
	File string
}

type componentInfo struct {
	Name    string
	Count   int
	Classes []string // representative Tailwind classes
}

type assetRef struct {
	Path  string
	Type  string // img | css | font | icon-cdn | js
}

type interactionPoint struct {
	Element  string
	Trigger  string
	Behavior string
}

// Execute implements the tool.
func (t *ExportHandoffTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in ExportHandoffInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.MockupDir == "" {
		return &Result{Content: "mockup_dir is required", IsError: true}, nil
	}

	mockupDir := RemapPathForWorktree(ctx, in.MockupDir)

	// Defaults
	if in.Framework == "" {
		in.Framework = "react"
	}
	if in.ProjectName == "" {
		in.ProjectName = filepath.Base(mockupDir)
	}

	// 1. Validate mockup_dir contains index.html
	indexPath := filepath.Join(mockupDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return &Result{Content: fmt.Sprintf("mockup_dir %q must contain index.html: %v", mockupDir, err), IsError: true}, nil
	}

	var warnings []string

	// 2. Collect HTML files: index.html + screen-*.html
	htmlFiles := []string{indexPath}
	entries, err := os.ReadDir(mockupDir)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to read mockup_dir %q: %v", mockupDir, err), IsError: true}, nil
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "screen-") && strings.HasSuffix(name, ".html") {
			htmlFiles = append(htmlFiles, filepath.Join(mockupDir, name))
		}
	}

	// 3. Read all HTML into combined string and also track per-file content
	var files []fileContent
	var combined strings.Builder
	for _, p := range htmlFiles {
		b, err := os.ReadFile(p)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("could not read %q: %v", p, err))
			continue
		}
		files = append(files, fileContent{path: p, name: filepath.Base(p), content: string(b)})
		combined.Write(b)
		combined.WriteByte('\n')
	}
	allHTML := combined.String()

	// 4. Parse screens from data-artboard attributes
	screens := parseScreens(files)

	// 5. Parse component inventory from Tailwind classes
	components := parseComponents(allHTML)

	// 6. Parse asset references
	assets := parseAssets(allHTML)

	// 7. Parse interaction points
	interactions := parseInteractions(allHTML)

	// 8. Parse font families and icon CDN references
	fonts := parseFonts(allHTML)
	iconCDNs := parseIconCDNs(allHTML)

	// 9. Load and cross-reference design tokens (optional)
	tokensUsed := map[string]interface{}{}
	if in.DesignTokens != "" {
		tokensPath := RemapPathForWorktree(ctx, in.DesignTokens)
		raw, err := os.ReadFile(tokensPath)
		if err != nil {
			return &Result{Content: fmt.Sprintf("could not read design_tokens %q: %v", tokensPath, err), IsError: true}, nil
		} else {
			var allTokens map[string]interface{}
			if err := json.Unmarshal(raw, &allTokens); err != nil {
				warnings = append(warnings, fmt.Sprintf("could not parse design_tokens JSON: %v", err))
			} else {
				// Find which token names appear in the HTML
				for k, v := range allTokens {
					if strings.Contains(allHTML, k) {
						tokensUsed[k] = v
					}
				}
			}
		}
	}

	// 10. Create handoff dir — prefer session_dir if provided, else nest under mockupDir
	handoffBase := mockupDir
	if in.SessionDir != "" {
		handoffBase = in.SessionDir
	}
	handoffDir := filepath.Join(handoffBase, "handoff")
	if err := os.MkdirAll(handoffDir, 0755); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create handoff dir: %v", err), IsError: true}, nil
	}

	// 11. Find and copy tokens.json from session/mockup dir.
	tokensJsonDest := filepath.Join(handoffDir, "tokens.json")
	tokensJsonSrc := findTokensJson(mockupDir, in.SessionDir)
	var tokensJsonData map[string]interface{}
	if tokensJsonSrc != "" {
		if err := copyFile(tokensJsonSrc, tokensJsonDest); err != nil {
			warnings = append(warnings, fmt.Sprintf("tokens.json found at %q but copy failed: %v", tokensJsonSrc, err))
			tokensJsonSrc = ""
		} else {
			// Parse for inline summary in spec.md.
			raw, readErr := os.ReadFile(tokensJsonDest)
			if readErr == nil {
				_ = json.Unmarshal(raw, &tokensJsonData)
			}
		}
	} else {
		warnings = append(warnings, "tokens.json not found — design agent may not have written it yet")
	}

	// 11b. Generate tokens.css from tokensJsonData
	tokensCSSPath := filepath.Join(handoffDir, "tokens.css")
	if tokensJsonData != nil {
		cssContent := generateTokensCSS(tokensJsonData)
		if err := os.WriteFile(tokensCSSPath, []byte(cssContent), 0644); err != nil {
			warnings = append(warnings, fmt.Sprintf("tokens.css write failed: %v", err))
			tokensCSSPath = ""
		}
	} else {
		tokensCSSPath = ""
	}

	// 12. Write spec.md
	sessionDirForSpec := in.SessionDir
	specPath := filepath.Join(handoffDir, "spec.md")
	specContent := buildSpecMarkdown(in.ProjectName, in.Framework, sessionDirForSpec, screens, components, tokensUsed, tokensJsonData, assets, interactions, fonts, iconCDNs)
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to write spec.md: %v", err), IsError: true}, nil
	}

	// 13. Write tokens-used.json
	tokensPath := filepath.Join(handoffDir, "tokens-used.json")
	tokensJSON, err := json.MarshalIndent(tokensUsed, "", "  ")
	if err != nil {
		tokensJSON = []byte("{}")
	}
	if err := os.WriteFile(tokensPath, tokensJSON, 0644); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to write tokens-used.json: %v", err), IsError: true}, nil
	}

	// 13b. Copy rendered/ → handoff/rendered/ (skip silently if absent)
	if in.SessionDir != "" {
		renderedSrc := filepath.Join(in.SessionDir, "rendered")
		if _, err := os.Stat(renderedSrc); err == nil {
			renderedDst := filepath.Join(handoffDir, "rendered")
			if err := copyRenderedDir(renderedSrc, renderedDst); err != nil {
				warnings = append(warnings, fmt.Sprintf("rendered/ copy failed: %v", err))
			}
		}
	}

	// 14. Build and return output
	out := ExportHandoffOutput{
		Success:        true,
		HandoffDir:     handoffDir,
		SpecPath:       specPath,
		TokensUsedPath: tokensPath,
		TokensJsonPath: tokensJsonDest,
		TokensCSSPath:  tokensCSSPath,
		ComponentCount: len(components),
		AssetCount:     len(assets),
		Warnings:       warnings,
	}
	if out.Warnings == nil {
		out.Warnings = []string{}
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

// ---- helpers ----

// renderedInteraction is one interactive element from {screen}.interactions.json.
type renderedInteraction struct {
	Tag  string `json:"tag"`
	Text string `json:"text"`
}

// tagToType maps HTML tag name to a human-readable type label.
func tagToType(tag string) string {
	switch strings.ToLower(tag) {
	case "a":
		return "link"
	case "input":
		return "input field"
	case "select":
		return "dropdown"
	default:
		return tag
	}
}

// sortedKeys returns sorted keys of a map[string]interface{}.
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// camelCaseToKebab converts camelCase to kebab-case (e.g. brandBlue → brand-blue).
func camelCaseToKebab(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			result = append(result, '-')
		}
		result = append(result, unicode.ToLower(r))
	}
	return string(result)
}

// generateTokensCSS emits a tokens.css file from a parsed tokens.json map.
func generateTokensCSS(tokens map[string]interface{}) string {
	var sb strings.Builder
	sb.WriteString("/* Design System Tokens — generated by Claudio Design */\n:root {\n")

	// Colors
	if v, ok := tokens["colors"]; ok {
		if m, ok := v.(map[string]interface{}); ok && len(m) > 0 {
			sb.WriteString("  /* Colors */\n")
			for _, k := range sortedKeys(m) {
				sb.WriteString(fmt.Sprintf("  --color-%s: %v;\n", camelCaseToKebab(k), m[k]))
			}
		}
	}

	// Spacing
	if v, ok := tokens["spacing"]; ok {
		if m, ok := v.(map[string]interface{}); ok && len(m) > 0 {
			sb.WriteString("  /* Spacing */\n")
			for _, k := range sortedKeys(m) {
				sb.WriteString(fmt.Sprintf("  --spacing-%s: %v;\n", camelCaseToKebab(k), m[k]))
			}
		}
	}

	// Radii
	if v, ok := tokens["radii"]; ok {
		if m, ok := v.(map[string]interface{}); ok && len(m) > 0 {
			sb.WriteString("  /* Radii */\n")
			for _, k := range sortedKeys(m) {
				sb.WriteString(fmt.Sprintf("  --radius-%s: %v;\n", camelCaseToKebab(k), m[k]))
			}
		}
	}

	// Shadows
	if v, ok := tokens["shadows"]; ok {
		if m, ok := v.(map[string]interface{}); ok && len(m) > 0 {
			sb.WriteString("  /* Shadows */\n")
			for _, k := range sortedKeys(m) {
				sb.WriteString(fmt.Sprintf("  --shadow-%s: %v;\n", camelCaseToKebab(k), m[k]))
			}
		}
	}

	// Typography — emit size/weight/lineHeight per style
	if v, ok := tokens["typography"]; ok {
		if m, ok := v.(map[string]interface{}); ok && len(m) > 0 {
			sb.WriteString("  /* Typography */\n")
			for _, k := range sortedKeys(m) {
				kk := camelCaseToKebab(k)
				if styleMap, ok := m[k].(map[string]interface{}); ok {
					if size, ok := styleMap["fontSize"]; ok {
						sb.WriteString(fmt.Sprintf("  --font-size-%s: %v;\n", kk, size))
					}
					if weight, ok := styleMap["fontWeight"]; ok {
						sb.WriteString(fmt.Sprintf("  --font-weight-%s: %v;\n", kk, weight))
					}
					if lh, ok := styleMap["lineHeight"]; ok {
						sb.WriteString(fmt.Sprintf("  --line-height-%s: %v;\n", kk, lh))
					}
				}
			}
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

// copyRenderedDir copies all files (non-recursive) from srcDir to dstDir.
func copyRenderedDir(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		src := filepath.Join(srcDir, e.Name())
		dst := filepath.Join(dstDir, e.Name())
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", e.Name(), err)
		}
	}
	return nil
}

type fileContent struct {
	path    string
	name    string
	content string
}

func parseScreens(files []fileContent) []screenInfo {
	var screens []screenInfo
	seen := map[string]bool{}
	for _, f := range files {
		// Try data-artboard first
		matches := artboardRe.FindAllStringSubmatch(f.content, -1)
		for _, m := range matches {
			name := m[1]
			if !seen[name] {
				seen[name] = true
				screens = append(screens, screenInfo{Name: name, File: f.name})
			}
		}
		// Fall back to filename for screen-*.html
		if strings.HasPrefix(f.name, "screen-") && strings.HasSuffix(f.name, ".html") {
			stem := strings.TrimSuffix(strings.TrimPrefix(f.name, "screen-"), ".html")
			if !seen[stem] {
				seen[stem] = true
				screens = append(screens, screenInfo{Name: stem, File: f.name})
			}
		}
	}
	return screens
}

func parseComponents(html string) []componentInfo {
	// Collect all class values
	classMatches := classAttrRe.FindAllStringSubmatch(html, -1)

	// Count keyword hits per component pattern
	type hit struct {
		count   int
		classes map[string]int
	}
	hits := make(map[string]*hit)

	for _, m := range classMatches {
		classes := strings.Fields(m[1])
		for _, cls := range classes {
			clsLower := strings.ToLower(cls)
			for _, pat := range componentPatterns {
				for _, kw := range pat.keywords {
					if strings.Contains(clsLower, kw) {
						if hits[pat.name] == nil {
							hits[pat.name] = &hit{classes: map[string]int{}}
						}
						hits[pat.name].count++
						hits[pat.name].classes[cls]++
					}
				}
			}
		}
	}

	var components []componentInfo
	for _, pat := range componentPatterns {
		h, ok := hits[pat.name]
		if !ok {
			continue
		}
		// Collect top representative classes (up to 5)
		type kv struct {
			k string
			v int
		}
		var pairs []kv
		for k, v := range h.classes {
			pairs = append(pairs, kv{k, v})
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })
		var topClasses []string
		for i, p := range pairs {
			if i >= 5 {
				break
			}
			topClasses = append(topClasses, p.k)
		}
		components = append(components, componentInfo{
			Name:    pat.name,
			Count:   h.count,
			Classes: topClasses,
		})
	}
	return components
}

func parseAssets(html string) []assetRef {
	seen := map[string]bool{}
	var assets []assetRef

	add := func(path, typ string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		assets = append(assets, assetRef{Path: path, Type: typ})
	}

	for _, m := range imgSrcRe.FindAllStringSubmatch(html, -1) {
		add(m[1], "img")
	}
	for _, m := range linkHrefRe.FindAllStringSubmatch(html, -1) {
		href := m[1]
		if strings.HasSuffix(strings.ToLower(href), ".css") {
			add(href, "css")
		} else {
			add(href, "link")
		}
	}
	for _, m := range cssImportRe.FindAllStringSubmatch(html, -1) {
		add(m[1], "css-import")
	}

	return assets
}

func parseInteractions(html string) []interactionPoint {
	var interactions []interactionPoint

	for _, m := range anchorHrefRe.FindAllStringSubmatch(html, -1) {
		href := m[1]
		if strings.HasPrefix(href, "http") || strings.HasPrefix(href, "//") {
			continue
		}
		interactions = append(interactions, interactionPoint{
			Element:  fmt.Sprintf(`<a href="%s">`, href),
			Trigger:  "click",
			Behavior: fmt.Sprintf("navigate to %s", href),
		})
	}
	for _, m := range onclickRe.FindAllStringSubmatch(html, -1) {
		interactions = append(interactions, interactionPoint{
			Element:  "element",
			Trigger:  "onclick",
			Behavior: truncateStr(m[1], 80),
		})
	}
	for _, m := range dataActionRe.FindAllStringSubmatch(html, -1) {
		interactions = append(interactions, interactionPoint{
			Element:  "element",
			Trigger:  "data-action",
			Behavior: m[1],
		})
	}

	// Deduplicate
	seen := map[string]bool{}
	var deduped []interactionPoint
	for _, ia := range interactions {
		key := ia.Trigger + ":" + ia.Behavior
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, ia)
		}
	}
	return deduped
}

func parseFonts(html string) []string {
	seen := map[string]bool{}
	var fonts []string
	for _, m := range fontFamilyRe.FindAllStringSubmatch(html, -1) {
		f := strings.TrimSpace(m[1])
		if f != "" && !seen[f] {
			seen[f] = true
			fonts = append(fonts, f)
		}
	}
	return fonts
}

func parseIconCDNs(html string) []string {
	seen := map[string]bool{}
	var cdns []string
	for _, m := range iconCDNRe.FindAllStringSubmatch(html, -1) {
		u := m[1]
		if !seen[u] {
			seen[u] = true
			cdns = append(cdns, u)
		}
	}
	return cdns
}

// findTokensJson looks for tokens.json in mockupDir, then parent of mockupDir, then sessionDir.
// Returns the first path found, or empty string if not found.
func findTokensJson(mockupDir, sessionDir string) string {
	candidates := []string{
		filepath.Join(mockupDir, "tokens.json"),
		filepath.Join(filepath.Dir(mockupDir), "tokens.json"),
	}
	if sessionDir != "" {
		candidates = append(candidates, filepath.Join(sessionDir, "tokens.json"))
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// copyFile copies src to dst, creating dst if needed.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// buildSpecMarkdown assembles the spec.md content.
func buildSpecMarkdown(
	projectName, framework, sessionDir string,
	screens []screenInfo,
	components []componentInfo,
	tokensUsed map[string]interface{},
	tokensJsonData map[string]interface{},
	assets []assetRef,
	interactions []interactionPoint,
	fonts []string,
	iconCDNs []string,
) string {
	var sb strings.Builder
	ts := time.Now().Format("2006-01-02 15:04:05")

	sb.WriteString(fmt.Sprintf("# Design Spec: %s\n\n", projectName))
	sb.WriteString(fmt.Sprintf("Generated: %s\n", ts))
	sb.WriteString(fmt.Sprintf("Framework target: %s\n\n", framework))

	// Reference Files
	sb.WriteString("## Reference Files\n\n")
	sb.WriteString("- Bundle: bundle/mockup.html\n")
	sb.WriteString("- Screenshots: screenshots/\n")
	sb.WriteString("- Tokens JSON: handoff/tokens.json\n")
	sb.WriteString("- Tokens CSS: handoff/tokens.css\n")
	sb.WriteString("- Rendered screens: handoff/rendered/\n")
	sb.WriteString("\n")

	// Screens — with per-screen interaction tables
	sb.WriteString("## Screens\n\n")
	if len(screens) == 0 {
		sb.WriteString("- (none detected)\n")
	}
	for _, s := range screens {
		sb.WriteString(fmt.Sprintf("## Screen: %s\n\n", s.Name))
		// Load interactions.json for this screen if available
		if sessionDir != "" {
			ijPath := filepath.Join(sessionDir, "rendered", s.Name+".interactions.json")
			if raw, err := os.ReadFile(ijPath); err == nil {
				var elems []renderedInteraction
				if json.Unmarshal(raw, &elems) == nil && len(elems) > 0 {
					sb.WriteString("### Interactive Elements\n\n")
					sb.WriteString("| Element | Text | Type |\n")
					sb.WriteString("|---------|------|------|\n")
					for _, el := range elems {
						sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", el.Tag, el.Text, tagToType(el.Tag)))
					}
					sb.WriteString("\n")
				}
			}
		}
	}
	sb.WriteString("\n")

	// Design Tokens — inline color summary from tokens.json
	sb.WriteString("## Design Tokens\n\n")
	if tokensJsonData != nil {
		// Colors
		if colors, ok := tokensJsonData["colors"]; ok {
			if colorMap, ok := colors.(map[string]interface{}); ok && len(colorMap) > 0 {
				sb.WriteString("**Colors:**\n\n")
				for _, k := range sortedKeys(colorMap) {
					sb.WriteString(fmt.Sprintf("  %s: %v\n", k, colorMap[k]))
				}
				sb.WriteString("\n")
			}
		}
		// Typography
		if typo, ok := tokensJsonData["typography"]; ok {
			if typoMap, ok := typo.(map[string]interface{}); ok && len(typoMap) > 0 {
				sb.WriteString("**Typography:**\n\n")
				for _, k := range sortedKeys(typoMap) {
					v, _ := json.Marshal(typoMap[k])
					sb.WriteString(fmt.Sprintf("  %s: %s\n", k, string(v)))
				}
				sb.WriteString("\n")
			}
		}
		// Spacing
		if spacing, ok := tokensJsonData["spacing"]; ok {
			if spacingMap, ok := spacing.(map[string]interface{}); ok && len(spacingMap) > 0 {
				sb.WriteString("**Spacing:**\n\n")
				keys := sortedKeys(spacingMap)
				for _, k := range keys {
					sb.WriteString(fmt.Sprintf("  %s: %v\n", k, spacingMap[k]))
				}
				sb.WriteString("\n")
			}
		}
		// Radii
		if radii, ok := tokensJsonData["radii"]; ok {
			if radiiMap, ok := radii.(map[string]interface{}); ok && len(radiiMap) > 0 {
				sb.WriteString("**Radii:**\n\n")
				keys := sortedKeys(radiiMap)
				for _, k := range keys {
					sb.WriteString(fmt.Sprintf("  %s: %v\n", k, radiiMap[k]))
				}
				sb.WriteString("\n")
			}
		}
		// Shadows
		if shadows, ok := tokensJsonData["shadows"]; ok {
			if shadowMap, ok := shadows.(map[string]interface{}); ok && len(shadowMap) > 0 {
				sb.WriteString("**Shadows:**\n\n")
				keys := sortedKeys(shadowMap)
				for _, k := range keys {
					sb.WriteString(fmt.Sprintf("  %s: %v\n", k, shadowMap[k]))
				}
				sb.WriteString("\n")
			}
		}
	} else {
		sb.WriteString("See tokens.json (not yet generated)\n\n")
	}

	// Component Inventory
	sb.WriteString("## Components\n\n")
	sb.WriteString("| Component | Count | Tailwind Classes | Notes |\n")
	sb.WriteString("|-----------|-------|-----------------|-------|\n")
	if len(components) == 0 {
		sb.WriteString("| (none detected) | — | — | — |\n")
	}
	for _, c := range components {
		classes := strings.Join(c.Classes, ", ")
		sb.WriteString(fmt.Sprintf("| %s | %d | %s | |\n", c.Name, c.Count, classes))
	}
	sb.WriteString("\n")

	// Asset References
	sb.WriteString("## Asset References\n\n")
	sb.WriteString("| Asset | Type |\n")
	sb.WriteString("|-------|------|\n")
	if len(assets) == 0 {
		sb.WriteString("| (none detected) | — |\n")
	}
	for _, a := range assets {
		sb.WriteString(fmt.Sprintf("| %s | %s |\n", a.Path, a.Type))
	}
	sb.WriteString("\n")

	// Interaction Spec
	sb.WriteString("## Interaction Spec\n\n")
	sb.WriteString("| Element | Trigger | Expected Behavior |\n")
	sb.WriteString("|---------|---------|-------------------|\n")
	if len(interactions) == 0 {
		sb.WriteString("| (none detected) | — | — |\n")
	}
	for _, ia := range interactions {
		sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", ia.Element, ia.Trigger, ia.Behavior))
	}
	sb.WriteString("\n")

	// Implementation Notes
	sb.WriteString("## Implementation Notes\n\n")
	sb.WriteString(fmt.Sprintf("- Framework: %s\n", framework))
	sb.WriteString("- All components are presentational — add state management as needed\n")
	if len(fonts) > 0 {
		sb.WriteString(fmt.Sprintf("- Font loading: %s\n", strings.Join(fonts, ", ")))
	} else {
		sb.WriteString("- Font loading: (no custom fonts detected)\n")
	}
	if len(iconCDNs) > 0 {
		sb.WriteString(fmt.Sprintf("- Icon library: %s\n", strings.Join(iconCDNs, ", ")))
	} else {
		sb.WriteString("- Icon library: (no icon CDN detected)\n")
	}
	sb.WriteString("\n")

	return sb.String()
}
