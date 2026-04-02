package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Abraxas-365/claudio/internal/api"
)

// ExtractorConfig configures the background memory extraction agent.
type ExtractorConfig struct {
	Client   *api.Client
	Store    *ScopedStore
	MinTurns int // minimum conversation turns before extraction runs (default: 4)
}

// BuildExtractorCallback returns a function suitable for Engine.SetOnTurnEnd.
// It reviews the conversation and extracts memories worth persisting.
func BuildExtractorCallback(cfg ExtractorConfig) func(messages []api.Message) {
	minTurns := cfg.MinTurns
	if minTurns <= 0 {
		minTurns = 4
	}

	return func(messages []api.Message) {
		// Only extract if there's enough conversation
		turnCount := 0
		for _, m := range messages {
			if m.Role == "user" {
				turnCount++
			}
		}
		if turnCount < minTurns {
			return
		}

		ExtractFromMessages(cfg.Client, cfg.Store, messages)
	}
}

// ExtractFromMessages runs memory extraction on the given messages and returns
// the number of memories saved. Uses Haiku for cost efficiency.
func ExtractFromMessages(client *api.Client, store *ScopedStore, messages []api.Message) int {
	// Summarize the last few messages for the extraction prompt
	summary := summarizeRecentMessages(messages, 10)
	if summary == "" {
		return 0
	}

	// Get existing memories to avoid duplicates
	existing := store.LoadAll()
	var existingNames []string
	for _, e := range existing {
		existingNames = append(existingNames, e.Name)
	}

	prompt := buildExtractionPrompt(summary, existingNames)

	// Use Haiku for extraction — cheap and fast enough for this task
	contentJSON, _ := json.Marshal(prompt)
	req := &api.MessagesRequest{
		Model: "claude-haiku-4-5-20251001",
		Messages: []api.Message{
			{Role: "user", Content: contentJSON},
		},
		MaxTokens: 2048,
	}

	ctx := context.Background()
	resp, err := client.SendMessage(ctx, req)
	if err != nil {
		return 0
	}

	// Parse the response for memory entries
	var responseText string
	for _, block := range resp.Content {
		if block.Type == "text" {
			responseText += block.Text
		}
	}

	// Extract structured memories from the response
	memories := parseExtractionResponse(responseText)
	for _, m := range memories {
		store.Save(m)
	}
	return len(memories)
}

func summarizeRecentMessages(messages []api.Message, maxMessages int) string {
	start := 0
	if len(messages) > maxMessages {
		start = len(messages) - maxMessages
	}

	var parts []string
	for _, msg := range messages[start:] {
		var content string
		json.Unmarshal(msg.Content, &content)
		if content == "" {
			content = string(msg.Content)
		}
		preview := content
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		parts = append(parts, fmt.Sprintf("[%s]: %s", msg.Role, preview))
	}
	return strings.Join(parts, "\n")
}

func buildExtractionPrompt(conversationSummary string, existingMemories []string) string {
	existing := "None"
	if len(existingMemories) > 0 {
		existing = strings.Join(existingMemories, ", ")
	}

	return fmt.Sprintf(`Review this conversation and decide if any information is worth saving as a persistent memory.

## Conversation
%s

## Existing Memories
%s

## Instructions
If you find something worth remembering, respond with one or more memory entries in this exact format:

---MEMORY---
name: <short descriptive name>
description: <one-line description>
type: <user|feedback|project|reference>
content: <the memory content, can be multiple lines>
---END---

Only extract NON-OBVIOUS patterns:
- User preferences or corrections
- Important project decisions
- Feedback on approaches (what worked, what didn't)
- References to external systems

Do NOT extract:
- Code patterns visible in the codebase
- Standard best practices
- Ephemeral task details
- Things already in existing memories

If there's nothing worth saving, respond with just: NOTHING_TO_EXTRACT`, conversationSummary, existing)
}

func parseExtractionResponse(response string) []*Entry {
	if strings.Contains(response, "NOTHING_TO_EXTRACT") {
		return nil
	}

	var entries []*Entry
	parts := strings.Split(response, "---MEMORY---")

	for _, part := range parts[1:] { // skip everything before first marker
		endIdx := strings.Index(part, "---END---")
		if endIdx < 0 {
			continue
		}
		block := strings.TrimSpace(part[:endIdx])

		entry := &Entry{}
		lines := strings.Split(block, "\n")
		var contentLines []string
		inContent := false

		for _, line := range lines {
			if inContent {
				contentLines = append(contentLines, line)
				continue
			}

			if strings.HasPrefix(line, "name:") {
				entry.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			} else if strings.HasPrefix(line, "description:") {
				entry.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			} else if strings.HasPrefix(line, "type:") {
				entry.Type = strings.TrimSpace(strings.TrimPrefix(line, "type:"))
			} else if strings.HasPrefix(line, "content:") {
				inContent = true
				firstLine := strings.TrimSpace(strings.TrimPrefix(line, "content:"))
				if firstLine != "" {
					contentLines = append(contentLines, firstLine)
				}
			}
		}

		if entry.Name != "" && len(contentLines) > 0 {
			entry.Content = strings.TrimSpace(strings.Join(contentLines, "\n"))
			entries = append(entries, entry)
		}
	}

	return entries
}
