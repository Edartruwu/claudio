package session

import (
	"encoding/json"
	"fmt"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/storage"
)

// ReconstructEngineMessages rebuilds a []api.Message slice from stored DB records so
// the engine has full conversation history when a session is resumed.
// Groups: "assistant" + following "tool_use" rows -> one assistant message;
// consecutive "tool_result" rows -> one user message, IDs paired by position.
func ReconstructEngineMessages(storedMsgs []storage.MessageRecord) []api.Message {
	type trBlock struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
		Content   string `json:"content"`
	}

	var result []api.Message
	var pendingIDs []string
	tuCounter := 0

	i := 0
	for i < len(storedMsgs) {
		msg := storedMsgs[i]
		switch msg.Type {
		case "user":
			content, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(msg.Content)})
			result = append(result, api.Message{Role: "user", Content: content})
			pendingIDs = nil // tool_results cannot bridge past a plain user message
			i++

		case "assistant":
			var blocks []api.ContentBlock
			if msg.Content != "" {
				blocks = append(blocks, api.ContentBlock{Type: "text", Text: msg.Content})
			}
			i++
			pendingIDs = nil
			for i < len(storedMsgs) && storedMsgs[i].Type == "tool_use" {
				// Prefer the stored ID; fall back to synthetic only for old rows
				id := storedMsgs[i].ToolUseID
				if id == "" {
					tuCounter++
					id = fmt.Sprintf("toolu_%04d", tuCounter)
				}
				pendingIDs = append(pendingIDs, id)
				input := json.RawMessage(storedMsgs[i].Content)
				if len(input) == 0 || !json.Valid(input) {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, api.ContentBlock{
					Type:  "tool_use",
					ID:    id,
					Name:  storedMsgs[i].ToolName,
					Input: input,
				})
				i++
			}
			if len(blocks) > 0 {
				content, _ := json.Marshal(blocks)
				result = append(result, api.Message{Role: "assistant", Content: content})
			}

		case "tool_result":
			// Skip orphaned tool_results with no preceding tool_use.
			if len(pendingIDs) == 0 {
				for i < len(storedMsgs) && storedMsgs[i].Type == "tool_result" {
					i++
				}
				continue
			}
			var trs []trBlock
			j := 0
			for i < len(storedMsgs) && storedMsgs[i].Type == "tool_result" {
				// Prefer the stored ID; fall back to positional pendingIDs
				id := storedMsgs[i].ToolUseID
				if id == "" {
					if j < len(pendingIDs) {
						id = pendingIDs[j]
					} else {
						tuCounter++
						id = fmt.Sprintf("toolu_%04d", tuCounter)
					}
				}
				trs = append(trs, trBlock{
					Type:      "tool_result",
					ToolUseID: id,
					Content:   storedMsgs[i].Content,
				})
				i++
				j++
			}
			if len(trs) > 0 {
				content, _ := json.Marshal(trs)
				result = append(result, api.Message{Role: "user", Content: content})
			}
			pendingIDs = nil

		default:
			i++
		}
	}
	return SanitizeToolPairs(result)
}

// SanitizeToolPairs removes unmatched tool_use/tool_result pairs from a
// reconstructed message list using ID-level matching. The Anthropic API requires
// that every tool_result in a user message has a matching tool_use (by ID) in
// the immediately preceding assistant message, and vice-versa.
func SanitizeToolPairs(msgs []api.Message) []api.Message {
	type toolUseHeader struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	type toolResultHeader struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
	}

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

	result := make([]api.Message, 0, len(msgs))
	for i := 0; i < len(msgs); i++ {
		msg := msgs[i]

		useIDs := extractToolUseIDs(msg.Content)
		if msg.Role == "assistant" && len(useIDs) > 0 {
			var resultIDs map[string]bool
			if i+1 < len(msgs) && msgs[i+1].Role == "user" {
				resultIDs = extractToolResultIDs(msgs[i+1].Content)
			}
			orphaned := map[string]bool{}
			for id := range useIDs {
				if !resultIDs[id] {
					orphaned[id] = true
				}
			}
			if len(orphaned) == len(useIDs) {
				stripped := stripToolUseByID(msg.Content, useIDs)
				if stripped != nil {
					result = append(result, api.Message{Role: "assistant", Content: stripped})
				}
			} else if len(orphaned) > 0 {
				stripped := stripToolUseByID(msg.Content, orphaned)
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
			var prevUseIDs map[string]bool
			if len(result) > 0 && result[len(result)-1].Role == "assistant" {
				prevUseIDs = extractToolUseIDs(result[len(result)-1].Content)
			}
			orphaned := map[string]bool{}
			for id := range resultIDs {
				if !prevUseIDs[id] {
					orphaned[id] = true
				}
			}
			if len(orphaned) == len(resultIDs) {
				stripped := stripToolResultByID(msg.Content, resultIDs)
				if stripped != nil {
					result = append(result, api.Message{Role: "user", Content: stripped})
				}
			} else if len(orphaned) > 0 {
				stripped := stripToolResultByID(msg.Content, orphaned)
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
