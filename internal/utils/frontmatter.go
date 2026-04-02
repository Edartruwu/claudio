// Package utils provides common utilities for Claudio.
package utils

import (
	"strings"
)

// Frontmatter holds parsed YAML frontmatter key-value pairs.
type Frontmatter map[string]string

// ParseFrontmatter extracts YAML frontmatter and body from markdown content.
// Returns the frontmatter map and the body text after the closing ---.
func ParseFrontmatter(content string) (Frontmatter, string) {
	lines := strings.Split(content, "\n")
	fm := make(Frontmatter)

	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return fm, content
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}

	if endIdx < 0 {
		return fm, content
	}

	// Parse frontmatter key-value pairs
	for _, line := range lines[1:endIdx] {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		// Remove quotes
		value = strings.Trim(value, `"'`)
		fm[key] = value
	}

	body := strings.Join(lines[endIdx+1:], "\n")
	return fm, strings.TrimSpace(body)
}

// Get returns a frontmatter value by key, or empty string if not found.
func (fm Frontmatter) Get(key string) string {
	return fm[key]
}

// GetOr returns a frontmatter value by key, or the default if not found.
func (fm Frontmatter) GetOr(key, defaultVal string) string {
	if v, ok := fm[key]; ok && v != "" {
		return v
	}
	return defaultVal
}

// GetList returns a comma-separated frontmatter value as a string slice.
func (fm Frontmatter) GetList(key string) []string {
	val := fm[key]
	if val == "" {
		return nil
	}

	// Handle YAML array syntax: [item1, item2]
	val = strings.TrimPrefix(val, "[")
	val = strings.TrimSuffix(val, "]")

	parts := strings.Split(val, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
