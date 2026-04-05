package compact

import (
	"encoding/json"

	"github.com/Abraxas-365/claudio/internal/api"
)

// EnsureToolResultPairing repairs broken tool_use/tool_result pairs in the
// message history. After compaction or truncation, an assistant message may
// contain tool_use blocks whose matching tool_result blocks were dropped, or
// a user message may have tool_result blocks with no matching tool_use.
//
// Orphaned tool_results are removed. Missing tool_results for existing
// tool_use blocks get a synthetic error placeholder so the API doesn't reject
// the conversation.
func EnsureToolResultPairing(messages []api.Message) []api.Message {
	if len(messages) == 0 {
		return messages
	}

	result := make([]api.Message, len(messages))
	copy(result, messages)

	for i := 0; i < len(result); i++ {
		msg := result[i]
		if msg.Role != "assistant" {
			continue
		}

		// Collect tool_use IDs from this assistant message.
		toolUseIDs := extractToolUseIDs(msg)
		if len(toolUseIDs) == 0 {
			continue
		}

		// Check the next message for matching tool_results.
		if i+1 >= len(result) || result[i+1].Role != "user" {
			// No user message follows — create one with synthetic results.
			result = insertSyntheticResults(result, i+1, toolUseIDs)
			continue
		}

		// Find which tool_use IDs are present vs missing in the next user message.
		nextMsg := result[i+1]
		existingIDs := extractToolResultIDs(nextMsg)

		// Remove orphaned tool_results (no matching tool_use).
		nextMsg = removeOrphanedToolResults(nextMsg, toolUseIDs)

		// Add synthetic results for missing tool_use IDs.
		var missing []string
		for _, id := range toolUseIDs {
			if !existingIDs[id] {
				missing = append(missing, id)
			}
		}
		if len(missing) > 0 {
			nextMsg = addSyntheticToolResults(nextMsg, missing)
		}

		result[i+1] = nextMsg
	}

	return result
}

// extractToolUseIDs returns all tool_use IDs from an assistant message.
func extractToolUseIDs(msg api.Message) []string {
	var blocks []json.RawMessage
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return nil
	}
	var ids []string
	for _, b := range blocks {
		var tu struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if json.Unmarshal(b, &tu) == nil && tu.Type == "tool_use" && tu.ID != "" {
			ids = append(ids, tu.ID)
		}
	}
	return ids
}

// extractToolResultIDs returns a set of tool_use_ids present in tool_result blocks.
func extractToolResultIDs(msg api.Message) map[string]bool {
	var blocks []json.RawMessage
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return nil
	}
	ids := make(map[string]bool)
	for _, b := range blocks {
		var tr struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
		}
		if json.Unmarshal(b, &tr) == nil && tr.Type == "tool_result" {
			ids[tr.ToolUseID] = true
		}
	}
	return ids
}

// removeOrphanedToolResults strips tool_result blocks that have no matching
// tool_use in the given validIDs set.
func removeOrphanedToolResults(msg api.Message, validToolUseIDs []string) api.Message {
	valid := make(map[string]bool, len(validToolUseIDs))
	for _, id := range validToolUseIDs {
		valid[id] = true
	}

	var blocks []json.RawMessage
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return msg
	}

	var kept []json.RawMessage
	for _, b := range blocks {
		var tr struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
		}
		if json.Unmarshal(b, &tr) == nil && tr.Type == "tool_result" {
			if !valid[tr.ToolUseID] {
				continue // orphaned — drop it
			}
		}
		kept = append(kept, b)
	}

	if len(kept) == len(blocks) {
		return msg // nothing removed
	}
	if len(kept) == 0 {
		// Don't create an empty content array — keep original
		return msg
	}
	msg.Content, _ = json.Marshal(kept)
	return msg
}

// addSyntheticToolResults appends synthetic error tool_result blocks for the
// given missing tool_use IDs.
func addSyntheticToolResults(msg api.Message, missingIDs []string) api.Message {
	var blocks []json.RawMessage
	if json.Unmarshal(msg.Content, &blocks) != nil {
		blocks = []json.RawMessage{}
	}

	for _, id := range missingIDs {
		synthetic := struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
			Content   string `json:"content"`
			IsError   bool   `json:"is_error"`
		}{
			Type:      "tool_result",
			ToolUseID: id,
			Content:   "[Tool result missing due to conversation compaction]",
			IsError:   true,
		}
		b, _ := json.Marshal(synthetic)
		blocks = append(blocks, b)
	}

	msg.Content, _ = json.Marshal(blocks)
	return msg
}

// insertSyntheticResults inserts a new user message at position idx with
// synthetic error tool_results for all given tool_use IDs.
func insertSyntheticResults(messages []api.Message, idx int, toolUseIDs []string) []api.Message {
	var blocks []json.RawMessage
	for _, id := range toolUseIDs {
		synthetic := struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
			Content   string `json:"content"`
			IsError   bool   `json:"is_error"`
		}{
			Type:      "tool_result",
			ToolUseID: id,
			Content:   "[Tool result missing due to conversation compaction]",
			IsError:   true,
		}
		b, _ := json.Marshal(synthetic)
		blocks = append(blocks, b)
	}

	content, _ := json.Marshal(blocks)
	newMsg := api.Message{Role: "user", Content: content}

	// Insert at position idx
	result := make([]api.Message, 0, len(messages)+1)
	result = append(result, messages[:idx]...)
	result = append(result, newMsg)
	if idx < len(messages) {
		result = append(result, messages[idx:]...)
	}
	return result
}
