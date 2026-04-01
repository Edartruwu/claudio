package compact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Abraxas-365/claudio/internal/api"
)

// Strategy defines when compaction should happen.
type Strategy string

const (
	StrategyAuto      Strategy = "auto"      // Compact at token thresholds
	StrategyManual    Strategy = "manual"     // Only compact on user request
	StrategyStrategic Strategy = "strategic"  // Suggest at phase boundaries
)

// State tracks compaction metrics for the current session.
type State struct {
	TotalTokens   int
	MaxTokens     int
	ToolCallCount int
	PhaseChanges  int
	LastPhase     string // "exploring", "planning", "implementing", "testing"
}

// ShouldSuggest returns true if compaction should be suggested.
func (s *State) ShouldSuggest(strategy Strategy) bool {
	switch strategy {
	case StrategyAuto:
		return s.TotalTokens > s.MaxTokens*80/100 // 80% of context window
	case StrategyStrategic:
		return s.ToolCallCount > 50 || s.TotalTokens > s.MaxTokens*70/100
	case StrategyManual:
		return false
	}
	return false
}

// ShouldForce returns true if compaction is mandatory (about to overflow).
func (s *State) ShouldForce() bool {
	return s.TotalTokens > s.MaxTokens*95/100
}

// DetectPhase infers the current work phase from recent tool usage.
func (s *State) DetectPhase(recentTools []string) string {
	readCount, writeCount, bashCount := 0, 0, 0
	for _, t := range recentTools {
		switch t {
		case "Read", "Glob", "Grep", "LSP":
			readCount++
		case "Write", "Edit":
			writeCount++
		case "Bash":
			bashCount++
		}
	}

	if readCount > writeCount*2 {
		return "exploring"
	}
	if writeCount > readCount {
		return "implementing"
	}
	if bashCount > readCount+writeCount {
		return "testing"
	}
	return "mixed"
}

// Compact summarizes old messages using the API.
func Compact(ctx context.Context, client *api.Client, messages []api.Message, keepLast int) ([]api.Message, string, error) {
	if len(messages) <= keepLast {
		return messages, "", nil
	}

	// Split into old (to summarize) and recent (to keep)
	oldMessages := messages[:len(messages)-keepLast]
	recentMessages := messages[len(messages)-keepLast:]

	// Build summary prompt
	var summaryParts []string
	for _, msg := range oldMessages {
		var content string
		json.Unmarshal(msg.Content, &content)
		if content == "" {
			content = string(msg.Content)
		}
		preview := content
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		summaryParts = append(summaryParts, fmt.Sprintf("[%s]: %s", msg.Role, preview))
	}

	summaryPrompt := fmt.Sprintf(
		"Summarize this conversation history in 2-3 paragraphs. Focus on: what was accomplished, key decisions made, current state of work, and any important context for continuing.\n\n%s",
		strings.Join(summaryParts, "\n"),
	)

	contentJSON, _ := json.Marshal(summaryPrompt)
	summaryReq := &api.MessagesRequest{
		Messages: []api.Message{
			{Role: "user", Content: contentJSON},
		},
		MaxTokens: 1024,
	}

	resp, err := client.SendMessage(ctx, summaryReq)
	if err != nil {
		return messages, "", fmt.Errorf("compaction summary failed: %w", err)
	}

	var summary string
	for _, block := range resp.Content {
		if block.Type == "text" {
			summary += block.Text
		}
	}

	// Build new message list: [system summary] + recent messages
	summaryContent, _ := json.Marshal(fmt.Sprintf("[Conversation Summary]\n%s", summary))
	compacted := []api.Message{
		{Role: "user", Content: summaryContent},
		{Role: "assistant", Content: json.RawMessage(`"Understood. I have the context from the summary. Let's continue."`)},
	}
	compacted = append(compacted, recentMessages...)

	return compacted, summary, nil
}
