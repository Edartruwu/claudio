// budget.go implements a per-message tool result budget system modeled on
// claude-code's ContentReplacementState. Before each API call the engine calls
// EnforceToolResultBudget which:
//   1. Re-applies cached replacements for previously-replaced results (stable bytes → prompt cache)
//   2. Leaves previously-seen-but-unreplaced results frozen (cannot change without busting cache)
//   3. Replaces the largest fresh results until the total is under the per-message budget
//
// Full results are persisted to disk via the toolcache.Store so the model can
// read them back with the Read tool if needed.
package compact

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/services/toolcache"
)

const (
	// PerMessageBudget is the maximum aggregate size (bytes) of all tool_result
	// content in one API-level message group before the budget system kicks in.
	PerMessageBudget = 200_000

	// PreviewSize is the number of bytes kept from a replaced result's content.
	PreviewSize = 2000
)

// ReplacementState tracks tool result replacements across turns.
// It must be created once per session and carried through all engine iterations.
type ReplacementState struct {
	// SeenIDs contains every tool_use_id that has passed through the budget check.
	// Once an ID is in SeenIDs its fate (replaced or frozen) is permanent.
	SeenIDs map[string]bool

	// Replacements maps tool_use_id → the exact replacement string the model saw.
	// Re-applied byte-identically on every turn to preserve prompt cache.
	Replacements map[string]string
}

// NewReplacementState creates an empty replacement state for a new session.
func NewReplacementState() *ReplacementState {
	return &ReplacementState{
		SeenIDs:      make(map[string]bool),
		Replacements: make(map[string]string),
	}
}

// candidate is a tool_result block eligible for budget consideration.
type candidate struct {
	msgIdx    int    // index in the messages slice
	blockIdx  int    // index in the content blocks array
	toolUseID string // the tool_use_id for this result
	content   string // raw content string
	size      int    // len(content)
}

// EnforceToolResultBudget scans messages for tool_result blocks and ensures
// that no API-level message exceeds PerMessageBudget bytes of tool result
// content. Results that were previously replaced get their cached replacement
// re-applied. New results that push the total over budget are persisted to
// disk (via store) and replaced with a 2KB preview.
//
// The function modifies messages in-place and returns them.
// Pass store=nil to skip disk persistence (replacements still happen in-memory).
func EnforceToolResultBudget(messages []api.Message, state *ReplacementState, store *toolcache.Store) []api.Message {
	if state == nil {
		return messages
	}

	// Collect all tool_result candidates across messages.
	// Skip Read tool results — they have their own dedup via ReadCache and
	// self-bound via maxTokens. Persisting them would be circular (model
	// would need to Read the persisted file). This matches claude-code's
	// behavior (maxResultSizeChars: Infinity → skipped by budget).
	skipToolIDs := findToolUseIDsByName(messages, "Read")

	var allCandidates []candidate
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
				Type      string `json:"type"`
				ToolUseID string `json:"tool_use_id"`
				Content   string `json:"content"`
			}
			if json.Unmarshal(b, &tr) != nil || tr.Type != "tool_result" {
				continue
			}
			if skipToolIDs[tr.ToolUseID] {
				// Mark as seen but don't count toward budget.
				state.SeenIDs[tr.ToolUseID] = true
				continue
			}
			allCandidates = append(allCandidates, candidate{
				msgIdx:    i,
				blockIdx:  j,
				toolUseID: tr.ToolUseID,
				content:   tr.Content,
				size:      len(tr.Content),
			})
		}
	}

	if len(allCandidates) == 0 {
		return messages
	}

	// Partition into three groups.
	var mustReapply []candidate // previously replaced → reapply cached replacement
	var frozen []candidate      // previously seen, unreplaced → leave as-is
	var fresh []candidate       // never seen → eligible for new replacement

	for _, c := range allCandidates {
		if _, replaced := state.Replacements[c.toolUseID]; replaced {
			mustReapply = append(mustReapply, c)
		} else if state.SeenIDs[c.toolUseID] {
			frozen = append(frozen, c)
		} else {
			fresh = append(fresh, c)
		}
	}

	// Re-apply cached replacements (byte-identical for prompt cache stability).
	for _, c := range mustReapply {
		applyReplacement(messages, c, state.Replacements[c.toolUseID])
	}

	// Calculate total fresh + frozen size.
	frozenSize := 0
	for _, c := range frozen {
		frozenSize += c.size
	}
	freshSize := 0
	for _, c := range fresh {
		freshSize += c.size
	}

	// Select fresh candidates to replace if over budget.
	var toReplace []candidate
	if frozenSize+freshSize > PerMessageBudget {
		toReplace = selectFreshToReplace(fresh, frozenSize, PerMessageBudget)
	}

	// Mark non-replaced fresh candidates as seen (frozen for future turns).
	replaceSet := make(map[string]bool, len(toReplace))
	for _, c := range toReplace {
		replaceSet[c.toolUseID] = true
	}
	for _, c := range fresh {
		if !replaceSet[c.toolUseID] {
			state.SeenIDs[c.toolUseID] = true
		}
	}

	// Persist and replace selected candidates.
	for _, c := range toReplace {
		replacement := buildPreview(c.content, c.size, store, c.toolUseID)
		applyReplacement(messages, c, replacement)
		state.SeenIDs[c.toolUseID] = true
		state.Replacements[c.toolUseID] = replacement
	}

	return messages
}

// selectFreshToReplace picks the largest fresh candidates to replace until the
// remaining total is at or under the budget. Greedy: largest first.
func selectFreshToReplace(fresh []candidate, frozenSize, budget int) []candidate {
	sorted := make([]candidate, len(fresh))
	copy(sorted, fresh)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].size > sorted[j].size })

	remaining := frozenSize
	for _, c := range fresh {
		remaining += c.size
	}

	var selected []candidate
	for _, c := range sorted {
		if remaining <= budget {
			break
		}
		selected = append(selected, c)
		remaining -= c.size // replaced with ~2KB preview, but close enough
	}
	return selected
}

// buildPreview generates the replacement text for a large tool result.
// If store is non-nil the full content is persisted to disk.
func buildPreview(content string, originalSize int, store *toolcache.Store, toolUseID string) string {
	// Persist full content to disk if store is available.
	if store != nil {
		store.MaybePersist(toolUseID, content)
	}

	preview := content
	if len(preview) > PreviewSize {
		// Truncate at last newline within the preview window if possible.
		preview = preview[:PreviewSize]
		if idx := strings.LastIndex(preview, "\n"); idx > PreviewSize/2 {
			preview = preview[:idx]
		}
	}

	hasMore := originalSize > PreviewSize
	moreIndicator := ""
	if hasMore {
		moreIndicator = "\n...\n"
	}

	return fmt.Sprintf(
		"[Tool output too large (%d bytes). Full output saved to disk — use Read tool to retrieve if needed.]\nPreview (first %d bytes):\n%s%s",
		originalSize, len(preview), preview, moreIndicator,
	)
}

// findToolUseIDsByName scans assistant messages for tool_use blocks with the
// given tool name and returns a set of their IDs.
func findToolUseIDsByName(messages []api.Message, toolName string) map[string]bool {
	ids := make(map[string]bool)
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
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			if json.Unmarshal(b, &tu) == nil && tu.Type == "tool_use" && tu.Name == toolName {
				ids[tu.ID] = true
			}
		}
	}
	return ids
}

// applyReplacement modifies a single tool_result block in-place.
func applyReplacement(messages []api.Message, c candidate, replacement string) {
	var blocks []json.RawMessage
	if json.Unmarshal(messages[c.msgIdx].Content, &blocks) != nil {
		return
	}

	var tr struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
		Content   string `json:"content"`
		IsError   bool   `json:"is_error,omitempty"`
	}
	if json.Unmarshal(blocks[c.blockIdx], &tr) != nil {
		return
	}
	tr.Content = replacement
	blocks[c.blockIdx], _ = json.Marshal(tr)
	messages[c.msgIdx].Content, _ = json.Marshal(blocks)
}
