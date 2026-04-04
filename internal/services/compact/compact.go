package compact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/tools/readcache"
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

// ShouldPartialCompact returns true if partial compaction (clearing old tool results) is warranted.
func (s *State) ShouldPartialCompact() bool {
	return s.TotalTokens > s.MaxTokens*70/100
}

// ShouldFullCompact returns true if a full compaction (API summarization) should be suggested.
func (s *State) ShouldFullCompact() bool {
	return s.TotalTokens > s.MaxTokens*90/100
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
// pinnedIndices is an optional set of message indices that should be preserved
// verbatim through compaction (not summarized). Pass nil to compact everything.
func Compact(ctx context.Context, client *api.Client, messages []api.Message, keepLast int, pinnedIndices ...map[int]bool) ([]api.Message, string, error) {
	if len(messages) <= keepLast {
		return messages, "", nil
	}

	pinned := map[int]bool{}
	if len(pinnedIndices) > 0 && pinnedIndices[0] != nil {
		pinned = pinnedIndices[0]
	}

	// Split into old (to summarize) and recent (to keep).
	// Adjust cutoff so we never split a tool_use/tool_result pair —
	// if the first "recent" message has tool_result blocks, pull the
	// cutoff back to include the preceding assistant message (with the
	// matching tool_use blocks).
	cutoff := len(messages) - keepLast
	for cutoff > 0 && messageHasToolResults(messages[cutoff]) {
		cutoff--
	}
	if cutoff <= 0 {
		return messages, "", nil
	}
	recentMessages := messages[cutoff:]

	// Separate pinned messages from old messages
	var oldMessages []api.Message
	var pinnedMessages []api.Message
	for i := 0; i < cutoff; i++ {
		if pinned[i] {
			pinnedMessages = append(pinnedMessages, messages[i])
		} else {
			oldMessages = append(oldMessages, messages[i])
		}
	}

	// Build summary prompt from non-pinned old messages
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

	// Build new message list: [system summary] + pinned messages + recent messages
	summaryContent, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(fmt.Sprintf("[Conversation Summary]\n%s", summary))})
	compacted := []api.Message{
		{Role: "user", Content: summaryContent},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"Understood. I have the context from the summary. Let's continue."}]`)},
	}

	// Insert pinned messages (they need to maintain valid user/assistant alternation)
	if len(pinnedMessages) > 0 {
		pinnedContent, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock("[Pinned context — preserved through compaction]")})
		compacted = append(compacted, api.Message{Role: "user", Content: pinnedContent})
		for _, pm := range pinnedMessages {
			compacted = append(compacted, pm)
		}
		// Ensure valid alternation — if last pinned was user, add assistant ack
		if len(pinnedMessages) > 0 && pinnedMessages[len(pinnedMessages)-1].Role == "user" {
			compacted = append(compacted, api.Message{
				Role:    "assistant",
				Content: json.RawMessage(`[{"type":"text","text":"Noted the pinned context."}]`),
			})
		}
	}

	compacted = append(compacted, recentMessages...)

	// Sanitize: drop orphaned tool_result messages that no longer have a
	// matching assistant tool_use after compaction reshuffled messages.
	compacted = sanitizeToolPairs(compacted)

	return compacted, summary, nil
}

// messageHasToolResults returns true if the message contains any tool_result content blocks.
func messageHasToolResults(msg api.Message) bool {
	var blocks []json.RawMessage
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return false
	}
	for _, b := range blocks {
		var block struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(b, &block) == nil && block.Type == "tool_result" {
			return true
		}
	}
	return false
}

// sanitizeToolPairs removes orphaned tool_use/tool_result blocks that lost
// their counterpart during compaction. The Anthropic API requires that every
// tool_result in a user message has a matching tool_use (by ID) in the
// immediately preceding assistant message, and vice-versa. This function
// enforces that constraint by ID-matching, not just by block type.
func sanitizeToolPairs(msgs []api.Message) []api.Message {
	type toolUseHeader struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	type toolResultHeader struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
	}

	// Extract tool_use IDs from an assistant message's content blocks.
	extractToolUseIDs := func(content json.RawMessage) map[string]bool {
		var blocks []json.RawMessage
		if json.Unmarshal(content, &blocks) != nil {
			return nil
		}
		ids := map[string]bool{}
		for _, b := range blocks {
			var h toolUseHeader
			if json.Unmarshal(b, &h) == nil && h.Type == "tool_use" && h.ID != "" {
				ids[h.ID] = true
			}
		}
		return ids
	}

	// Extract tool_result IDs from a user message's content blocks.
	extractToolResultIDs := func(content json.RawMessage) map[string]bool {
		var blocks []json.RawMessage
		if json.Unmarshal(content, &blocks) != nil {
			return nil
		}
		ids := map[string]bool{}
		for _, b := range blocks {
			var h toolResultHeader
			if json.Unmarshal(b, &h) == nil && h.Type == "tool_result" && h.ToolUseID != "" {
				ids[h.ToolUseID] = true
			}
		}
		return ids
	}

	// Strip specific tool_use blocks by ID from content; returns nil if nothing remains.
	stripToolUseByID := func(content json.RawMessage, removeIDs map[string]bool) json.RawMessage {
		var blocks []json.RawMessage
		if json.Unmarshal(content, &blocks) != nil {
			return content
		}
		var kept []json.RawMessage
		for _, b := range blocks {
			var h toolUseHeader
			if json.Unmarshal(b, &h) == nil && h.Type == "tool_use" && removeIDs[h.ID] {
				continue
			}
			kept = append(kept, b)
		}
		if len(kept) == 0 {
			return nil
		}
		out, _ := json.Marshal(kept)
		return out
	}

	// Strip specific tool_result blocks by ID from content; returns nil if nothing remains.
	stripToolResultByID := func(content json.RawMessage, removeIDs map[string]bool) json.RawMessage {
		var blocks []json.RawMessage
		if json.Unmarshal(content, &blocks) != nil {
			return content
		}
		var kept []json.RawMessage
		for _, b := range blocks {
			var h toolResultHeader
			if json.Unmarshal(b, &h) == nil && h.Type == "tool_result" && removeIDs[h.ToolUseID] {
				continue
			}
			kept = append(kept, b)
		}
		if len(kept) == 0 {
			return nil
		}
		out, _ := json.Marshal(kept)
		return out
	}

	// First pass: pair adjacent assistant(tool_use) → user(tool_result) by ID.
	// Build the result list, stripping unmatched IDs from both sides.
	result := make([]api.Message, 0, len(msgs))
	for i := 0; i < len(msgs); i++ {
		msg := msgs[i]

		useIDs := extractToolUseIDs(msg.Content)
		if msg.Role == "assistant" && len(useIDs) > 0 {
			// Look ahead: does the next message have matching tool_results?
			var resultIDs map[string]bool
			if i+1 < len(msgs) && msgs[i+1].Role == "user" {
				resultIDs = extractToolResultIDs(msgs[i+1].Content)
			}

			// Find tool_use IDs that have no matching tool_result in the next message.
			orphanedUseIDs := map[string]bool{}
			for id := range useIDs {
				if !resultIDs[id] {
					orphanedUseIDs[id] = true
				}
			}

			if len(orphanedUseIDs) == len(useIDs) {
				// ALL tool_use blocks are orphaned — strip them all
				stripped := stripToolUseByID(msg.Content, useIDs)
				if stripped != nil {
					result = append(result, api.Message{Role: "assistant", Content: stripped})
				}
			} else if len(orphanedUseIDs) > 0 {
				// Some are orphaned — strip only those
				stripped := stripToolUseByID(msg.Content, orphanedUseIDs)
				if stripped != nil {
					result = append(result, api.Message{Role: "assistant", Content: stripped})
				}
			} else {
				result = append(result, msg)
			}
			continue
		}

		resultIDs := extractToolResultIDs(msg.Content)
		if msg.Role == "user" && len(resultIDs) > 0 {
			// Look back: does the previous result message have matching tool_uses?
			var prevUseIDs map[string]bool
			if len(result) > 0 && result[len(result)-1].Role == "assistant" {
				prevUseIDs = extractToolUseIDs(result[len(result)-1].Content)
			}

			// Find tool_result IDs that have no matching tool_use in the previous message.
			orphanedResultIDs := map[string]bool{}
			for id := range resultIDs {
				if !prevUseIDs[id] {
					orphanedResultIDs[id] = true
				}
			}

			if len(orphanedResultIDs) == len(resultIDs) {
				// ALL tool_result blocks are orphaned — strip them, keep any text blocks
				stripped := stripToolResultByID(msg.Content, resultIDs)
				if stripped != nil {
					result = append(result, api.Message{Role: "user", Content: stripped})
				}
			} else if len(orphanedResultIDs) > 0 {
				// Some are orphaned — strip only those
				stripped := stripToolResultByID(msg.Content, orphanedResultIDs)
				if stripped != nil {
					result = append(result, api.Message{Role: "user", Content: stripped})
				}
			} else {
				result = append(result, msg)
			}
			continue
		}

		result = append(result, msg)
	}
	return result
}

// readHeavyTools are tools whose output can be safely cleared (read-only, reproducible).
var readHeavyTools = map[string]bool{
	"Bash": true, "Read": true, "Glob": true, "Grep": true,
	"WebFetch": true, "WebSearch": true, "LSP": true, "ToolSearch": true,
}

// filePathForToolUseID scans messages for a tool_use block with the given ID and
// returns its file_path input if the tool is "Read".
func filePathForToolUseID(messages []api.Message, toolUseID string) string {
	for _, m := range messages {
		if m.Role != "assistant" {
			continue
		}
		var blocks []json.RawMessage
		if json.Unmarshal(m.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			var tu struct {
				Type  string          `json:"type"`
				ID    string          `json:"id"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			}
			if json.Unmarshal(b, &tu) != nil || tu.Type != "tool_use" || tu.ID != toolUseID {
				continue
			}
			if tu.Name == "Read" {
				var inp struct {
					FilePath string `json:"file_path"`
				}
				if json.Unmarshal(tu.Input, &inp) == nil {
					return inp.FilePath
				}
			}
			return ""
		}
	}
	return ""
}

// MicroCompact proactively clears large read-heavy tool results from old messages
// on every tool turn, without waiting for a token threshold.
// It preserves the last keepLastResults tool results intact.
// This runs continuously to keep the message history lean.
// Pass rc to invalidate ReadCache entries for any Read results that get cleared,
// so the model can re-read those files instead of receiving a stale stub.
func MicroCompact(messages []api.Message, keepLastResults int, minSizeBytes int, rc ...*readcache.Cache) []api.Message {
	if len(messages) == 0 {
		return messages
	}

	// Count total tool_result blocks to find cutoff
	type resultPos struct{ msgIdx, blockIdx int }
	var positions []resultPos
	for i, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		var blocks []json.RawMessage
		if json.Unmarshal(msg.Content, &blocks) != nil {
			continue
		}
		for j, b := range blocks {
			var tr struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(b, &tr) == nil && tr.Type == "tool_result" {
				positions = append(positions, resultPos{i, j})
			}
		}
	}

	if len(positions) <= keepLastResults {
		return messages
	}

	// Mark positions that should be cleared (all but the last keepLastResults)
	type clearKey struct{ msgIdx, blockIdx int }
	toClear := make(map[clearKey]bool)
	cutoff := len(positions) - keepLastResults
	for _, pos := range positions[:cutoff] {
		toClear[clearKey{pos.msgIdx, pos.blockIdx}] = true
	}

	result := make([]api.Message, len(messages))
	copy(result, messages)

	for i, msg := range result {
		if msg.Role != "user" {
			continue
		}
		var blocks []json.RawMessage
		if json.Unmarshal(msg.Content, &blocks) != nil {
			continue
		}
		modified := false
		for j, b := range blocks {
			if !toClear[clearKey{i, j}] {
				continue
			}
			var tr struct {
				Type      string `json:"type"`
				ToolUseID string `json:"tool_use_id"`
				Content   string `json:"content"`
				IsError   bool   `json:"is_error,omitempty"`
			}
			if json.Unmarshal(b, &tr) != nil || tr.Type != "tool_result" {
				continue
			}
			if len(tr.Content) < minSizeBytes || tr.IsError {
				continue
			}
			// Invalidate the ReadCache entry for this file so the model can
			// re-read it fresh instead of receiving a stale "refer to earlier
			// result" stub that points to content we just cleared.
			if len(rc) > 0 && rc[0] != nil {
				if fp := filePathForToolUseID(messages, tr.ToolUseID); fp != "" {
					rc[0].Invalidate(fp)
				}
			}
			tr.Content = fmt.Sprintf("[result cleared — %d bytes]", len(tr.Content))
			blocks[j], _ = json.Marshal(tr)
			modified = true
		}
		if modified {
			result[i].Content, _ = json.Marshal(blocks)
		}
	}

	return result
}

// ContentClearCompact replaces large tool results in old messages with a placeholder.
// Messages in the last keepLast are preserved. Only tool_result blocks larger than
// minSize bytes are cleared. Returns the modified message slice (in-place modification
// of copies).
func ContentClearCompact(messages []api.Message, keepLast int, minSize int) []api.Message {
	if len(messages) <= keepLast {
		return messages
	}

	cutoff := len(messages) - keepLast
	result := make([]api.Message, len(messages))
	copy(result, messages)

	for i := 0; i < cutoff; i++ {
		msg := result[i]
		if msg.Role != "user" {
			continue
		}

		// Try to parse as array of tool_result blocks
		var blocks []json.RawMessage
		if json.Unmarshal(msg.Content, &blocks) != nil {
			continue
		}

		modified := false
		for j, block := range blocks {
			var tr struct {
				Type      string `json:"type"`
				ToolUseID string `json:"tool_use_id"`
				Content   string `json:"content"`
				IsError   bool   `json:"is_error,omitempty"`
			}
			if json.Unmarshal(block, &tr) != nil || tr.Type != "tool_result" {
				continue
			}
			if len(tr.Content) < minSize {
				continue
			}

			// Replace with placeholder
			tr.Content = fmt.Sprintf("[content cleared — %d bytes]", len(tr.Content))
			blocks[j], _ = json.Marshal(tr)
			modified = true
		}

		if modified {
			result[i].Content, _ = json.Marshal(blocks)
		}
	}

	return result
}

// PartialCompact strips content from read-heavy tool results in old messages.
// Write tool results (Write, Edit) are preserved intact.
func PartialCompact(messages []api.Message, keepLast int) []api.Message {
	if len(messages) <= keepLast {
		return messages
	}

	cutoff := len(messages) - keepLast
	result := make([]api.Message, len(messages))
	copy(result, messages)

	for i := 0; i < cutoff; i++ {
		msg := result[i]

		// Check assistant messages for tool_use blocks to identify tool names
		if msg.Role == "assistant" {
			continue
		}

		// For user messages, clear large tool_result blocks
		var blocks []json.RawMessage
		if json.Unmarshal(msg.Content, &blocks) != nil {
			continue
		}

		modified := false
		for j, block := range blocks {
			var tr struct {
				Type      string `json:"type"`
				ToolUseID string `json:"tool_use_id"`
				Content   string `json:"content"`
				IsError   bool   `json:"is_error,omitempty"`
			}
			if json.Unmarshal(block, &tr) != nil || tr.Type != "tool_result" {
				continue
			}
			if len(tr.Content) < 1024 { // only clear results > 1KB
				continue
			}

			tr.Content = fmt.Sprintf("[result cleared — %d bytes]", len(tr.Content))
			blocks[j], _ = json.Marshal(tr)
			modified = true
		}

		if modified {
			result[i].Content, _ = json.Marshal(blocks)
		}
	}

	return result
}
