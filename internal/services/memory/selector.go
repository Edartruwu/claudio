package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Abraxas-365/claudio/internal/api"
)

const (
	maxSelectedMemories = 5
	maxMemoryBytes      = 4096 // 4KB per memory file
)

// MemorySelector selects relevant memories for the current context.
type MemorySelector interface {
	Select(ctx context.Context, candidates []*Entry, query string) ([]*Entry, error)
}

// KeywordSelector selects memories using simple keyword matching.
// Used as a fallback when AI selection isn't available.
type KeywordSelector struct{}

func (s *KeywordSelector) Select(_ context.Context, candidates []*Entry, query string) ([]*Entry, error) {
	lower := strings.ToLower(query)
	var relevant []*Entry

	for _, entry := range candidates {
		desc := strings.ToLower(entry.Description + " " + entry.Name)
		if strings.Contains(lower, strings.ToLower(entry.Name)) ||
			containsAnyWord(lower, desc) {
			relevant = append(relevant, entry)
			if len(relevant) >= maxSelectedMemories {
				break
			}
		}
	}

	return relevant, nil
}

// AISelector uses an AI model to pick the most relevant memories.
type AISelector struct {
	Client *api.Client
}

func (s *AISelector) Select(ctx context.Context, candidates []*Entry, query string) ([]*Entry, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	// If few enough candidates, just return them all
	if len(candidates) <= maxSelectedMemories {
		return candidates, nil
	}

	// Build manifest of candidates
	var manifest strings.Builder
	nameMap := make(map[string]*Entry)
	for i, entry := range candidates {
		manifest.WriteString(fmt.Sprintf("%d. [%s] (%s): %s\n", i+1, entry.Name, entry.Type, entry.Description))
		nameMap[entry.Name] = entry
	}

	prompt := fmt.Sprintf(`Given the current context, select the %d most relevant memories from this list.

## Current Context
%s

## Available Memories
%s

Respond with ONLY the names of the selected memories, one per line. Nothing else.`, maxSelectedMemories, query, manifest.String())

	contentJSON, _ := json.Marshal(prompt)
	req := &api.MessagesRequest{
		Model: "claude-haiku-4-5-20251001",
		Messages: []api.Message{
			{Role: "user", Content: contentJSON},
		},
		MaxTokens: 512,
	}

	resp, err := s.Client.SendMessage(ctx, req)
	if err != nil {
		// Fallback to keyword selector on API error
		fallback := &KeywordSelector{}
		return fallback.Select(ctx, candidates, query)
	}

	var responseText string
	for _, block := range resp.Content {
		if block.Type == "text" {
			responseText += block.Text
		}
	}

	// Parse selected names
	var selected []*Entry
	lines := strings.Split(responseText, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Try exact match first
		if entry, ok := nameMap[line]; ok {
			selected = append(selected, entry)
			continue
		}
		// Try partial match (model might include numbering or brackets)
		for name, entry := range nameMap {
			if strings.Contains(line, name) {
				selected = append(selected, entry)
				delete(nameMap, name) // prevent duplicates
				break
			}
		}
		if len(selected) >= maxSelectedMemories {
			break
		}
	}

	// If AI returned nothing useful, fallback
	if len(selected) == 0 {
		fallback := &KeywordSelector{}
		return fallback.Select(ctx, candidates, query)
	}

	return selected, nil
}

// SelectRelevant picks the most relevant memories for the given context using the provided selector.
func (s *ScopedStore) SelectRelevant(ctx context.Context, query string, selector MemorySelector) ([]*Entry, error) {
	candidates := s.LoadAll()
	if len(candidates) == 0 {
		return nil, nil
	}
	return selector.Select(ctx, candidates, query)
}

// ForSystemPromptWithSelection returns selected memories formatted for injection.
// Unlike ForSystemPrompt() which dumps everything, this uses intelligent selection.
func (s *ScopedStore) ForSystemPromptWithSelection(ctx context.Context, query string, selector MemorySelector) string {
	selected, err := s.SelectRelevant(ctx, query, selector)
	if err != nil {
		// On selector error, fall back to all memories (capped at 25KB)
		return s.ForSystemPrompt()
	}
	if len(selected) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Memories\n\n")
	sb.WriteString("The following relevant memories from previous sessions:\n\n")

	totalLen := 0
	for _, m := range selected {
		content := m.Content
		if len(content) > maxMemoryBytes {
			content = content[:maxMemoryBytes] + "\n... (truncated, see full file)"
		}

		entry := fmt.Sprintf("## %s (%s)\n%s\n\n", m.Name, m.Type, content)
		if totalLen+len(entry) > maxIndexBytes {
			break
		}
		sb.WriteString(entry)
		totalLen += len(entry)
	}

	return sb.String()
}
