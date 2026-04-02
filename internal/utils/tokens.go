package utils

import (
	"strings"
	"unicode/utf8"
)

// EstimateTokens provides a rough token count estimation.
// Claude uses ~4 characters per token on average for English text.
// This is a heuristic — actual tokenization depends on the model's tokenizer.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}

	// Rough heuristic: ~4 chars per token for English
	// Adjust for code which tends to have more tokens per character
	charCount := utf8.RuneCountInString(text)
	wordCount := len(strings.Fields(text))

	// Use a blend of character-based and word-based estimation
	charEstimate := charCount / 4
	wordEstimate := int(float64(wordCount) * 1.3) // ~1.3 tokens per word

	// Average the two estimates
	estimate := (charEstimate + wordEstimate) / 2
	if estimate < 1 && charCount > 0 {
		estimate = 1
	}

	return estimate
}

// EstimateTokensJSON estimates tokens for JSON content.
// JSON tends to be more token-dense due to structural characters.
func EstimateTokensJSON(text string) int {
	if text == "" {
		return 0
	}
	// JSON is roughly 3 chars per token due to brackets, quotes, etc.
	return utf8.RuneCountInString(text) / 3
}

// TokenBudget helps manage context window usage.
type TokenBudget struct {
	MaxTokens     int // Total context window
	SystemTokens  int // Tokens used by system prompt
	ToolTokens    int // Tokens used by tool definitions
	MessageTokens int // Tokens used by conversation messages
}

// Available returns the number of tokens available for new content.
func (tb *TokenBudget) Available() int {
	used := tb.SystemTokens + tb.ToolTokens + tb.MessageTokens
	avail := tb.MaxTokens - used
	if avail < 0 {
		return 0
	}
	return avail
}

// UsagePercent returns the percentage of context window used.
func (tb *TokenBudget) UsagePercent() float64 {
	if tb.MaxTokens == 0 {
		return 0
	}
	used := tb.SystemTokens + tb.ToolTokens + tb.MessageTokens
	return float64(used) / float64(tb.MaxTokens) * 100
}

// ShouldCompact returns true if usage exceeds the given threshold (0-100).
func (tb *TokenBudget) ShouldCompact(threshold float64) bool {
	return tb.UsagePercent() >= threshold
}

// MaxContextForModel returns the maximum context window size for a model.
func MaxContextForModel(model string) int {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "opus"):
		return 200_000
	case strings.Contains(lower, "sonnet"):
		return 200_000
	case strings.Contains(lower, "haiku"):
		return 200_000
	default:
		return 200_000 // default to 200K
	}
}

// MaxOutputForModel returns the maximum output tokens for a model.
func MaxOutputForModel(model string) int {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "opus"):
		return 32_000
	case strings.Contains(lower, "sonnet"):
		return 16_000
	case strings.Contains(lower, "haiku"):
		return 8_192
	default:
		return 16_000
	}
}
