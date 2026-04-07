package naming

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Abraxas-365/claudio/internal/api"
)

const namePrompt = `Generate a concise 2-5 word title for this conversation.
Reply with ONLY the title — no punctuation, no quotes, no explanation.`

// GenerateSessionName calls the given model to produce a short session title
// from the provided conversation messages. It returns the trimmed title.
func GenerateSessionName(ctx context.Context, client *api.Client, model string, msgs []api.Message) (string, error) {
	// Build a short preview of the conversation (first 10 messages, truncated)
	var parts []string
	limit := len(msgs)
	if limit > 10 {
		limit = 10
	}
	for _, msg := range msgs[:limit] {
		var text string
		// Try to extract plain text from message content
		var blocks []api.UserContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err == nil {
			for _, b := range blocks {
				if b.Type == "text" {
					text += b.Text
				}
			}
		} else {
			// Fallback: raw string content
			_ = json.Unmarshal(msg.Content, &text)
		}
		if text == "" {
			continue
		}
		if len(text) > 300 {
			text = text[:300] + "..."
		}
		parts = append(parts, "["+msg.Role+"]: "+text)
	}
	if len(parts) == 0 {
		return "", nil
	}

	conversation := strings.Join(parts, "\n")
	contentJSON, _ := json.Marshal(namePrompt + "\n\nConversation:\n" + conversation)

	req := &api.MessagesRequest{
		Model: model,
		Messages: []api.Message{
			{Role: "user", Content: contentJSON},
		},
		MaxTokens: 20,
	}

	resp, err := client.SendMessage(ctx, req)
	if err != nil {
		return "", err
	}

	var name string
	for _, block := range resp.Content {
		if block.Type == "text" {
			name += block.Text
		}
	}
	name = strings.TrimSpace(name)
	name = strings.Trim(name, `"'`)
	return name, nil
}
