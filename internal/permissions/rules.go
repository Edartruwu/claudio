// Package permissions provides content-pattern permission rules for tools.
// Rules allow fine-grained control like "allow Bash for git commands" or
// "deny Write to *.env files".
package permissions

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/Abraxas-365/claudio/internal/config"
)

// Match checks permission rules against a tool invocation.
// Returns the behavior string and true if a rule matched, or ("", false) if none matched.
// Rules are evaluated in order; first match wins.
func Match(toolName string, input json.RawMessage, rules []config.PermissionRule) (string, bool) {
	for _, rule := range rules {
		if !matchToolName(rule.Tool, toolName) {
			continue
		}

		content := extractMatchContent(toolName, input)
		if content == "" {
			continue
		}

		if matchPattern(rule.Pattern, content) {
			return rule.Behavior, true
		}
	}
	return "", false
}

// matchToolName checks if a rule's tool field matches the given tool name.
func matchToolName(ruleToolName, toolName string) bool {
	if ruleToolName == "*" {
		return true
	}
	return strings.EqualFold(ruleToolName, toolName)
}

// extractMatchContent pulls the relevant content string from tool input for pattern matching.
func extractMatchContent(toolName string, input json.RawMessage) string {
	switch toolName {
	case "Bash":
		var in struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(input, &in) == nil {
			return in.Command
		}
	case "Read":
		var in struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal(input, &in) == nil {
			return in.FilePath
		}
	case "Write":
		var in struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal(input, &in) == nil {
			return in.FilePath
		}
	case "Edit":
		var in struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal(input, &in) == nil {
			return in.FilePath
		}
	case "Glob":
		var in struct {
			Pattern string `json:"pattern"`
		}
		if json.Unmarshal(input, &in) == nil {
			return in.Pattern
		}
	case "Grep":
		var in struct {
			Pattern string `json:"pattern"`
		}
		if json.Unmarshal(input, &in) == nil {
			return in.Pattern
		}
	default:
		// For unknown tools, match against the raw JSON input
		return string(input)
	}
	return ""
}

// matchPattern checks if content matches a glob-like pattern.
// Supports:
//   - "*" matches everything
//   - "git *" matches any string starting with "git "
//   - "*.env" matches any string ending with ".env"
//   - Standard glob patterns via filepath.Match for path-like content
func matchPattern(pattern, content string) bool {
	if pattern == "*" {
		return true
	}

	// Try glob match (works for file paths and simple patterns)
	if matched, _ := filepath.Match(pattern, content); matched {
		return true
	}

	// For command-like patterns (e.g., "git *"), try prefix matching
	// Convert "git *" to prefix check "git "
	if strings.HasSuffix(pattern, " *") {
		prefix := strings.TrimSuffix(pattern, "*")
		if strings.HasPrefix(content, prefix) {
			return true
		}
	}

	// Try matching against just the first N words of content
	// e.g., pattern "git commit" matches "git commit -m 'msg'"
	if !strings.Contains(pattern, "*") {
		if strings.HasPrefix(content, pattern+" ") || content == pattern {
			return true
		}
	}

	// For path patterns like "*.test.*", try matching against basename
	if strings.Contains(content, "/") {
		base := filepath.Base(content)
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
	}

	return false
}
