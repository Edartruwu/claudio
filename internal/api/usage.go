package api

import (
	"fmt"
	"sync"
)

// ModelPricing holds cost per million tokens for a model.
type ModelPricing struct {
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheReadPerMillion  float64
	CacheWritePerMillion float64
}

// Known model pricing (USD per million tokens).
// Source: https://platform.claude.com/docs/en/about-claude/pricing
var modelPricing = map[string]ModelPricing{
	"claude-opus-4-6":   {InputPerMillion: 15.0, OutputPerMillion: 75.0, CacheReadPerMillion: 1.5, CacheWritePerMillion: 18.75},
	"claude-opus-4-5":   {InputPerMillion: 15.0, OutputPerMillion: 75.0, CacheReadPerMillion: 1.5, CacheWritePerMillion: 18.75},
	"claude-sonnet-4-6": {InputPerMillion: 3.0, OutputPerMillion: 15.0, CacheReadPerMillion: 0.3, CacheWritePerMillion: 3.75},
	"claude-sonnet-4-5": {InputPerMillion: 3.0, OutputPerMillion: 15.0, CacheReadPerMillion: 0.3, CacheWritePerMillion: 3.75},
	"claude-haiku-4-5":  {InputPerMillion: 0.25, OutputPerMillion: 1.25, CacheReadPerMillion: 0.025, CacheWritePerMillion: 0.3125},
}

// UsageTracker accumulates token usage and cost across a session.
type UsageTracker struct {
	mu               sync.Mutex
	model            string
	InputTokens      int
	OutputTokens     int
	CacheRead        int
	CacheCreate      int
	TurnCount        int
	TotalCost        float64
	Budget           float64 // 0 = unlimited
	LastContextTokens int    // input+cacheRead+cacheCreate from the most recent turn (current context window size)
}

// NewUsageTracker creates a tracker for the given model.
func NewUsageTracker(model string, budget float64) *UsageTracker {
	return &UsageTracker{
		model:  model,
		Budget: budget,
	}
}

// Add records token usage from a turn.
func (t *UsageTracker) Add(usage Usage) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.InputTokens += usage.InputTokens
	t.OutputTokens += usage.OutputTokens
	t.CacheRead += usage.CacheRead
	t.CacheCreate += usage.CacheCreate
	t.TurnCount++
	// Track current context window size: the API echoes back how much of the
	// context window this turn consumed. This is what the progress bar shows.
	// Must include all four fields to match claude-code's getTokenCountFromUsage.
	t.LastContextTokens = usage.InputTokens + usage.OutputTokens + usage.CacheRead + usage.CacheCreate

	t.TotalCost = t.calculateCost()
}

// Cost returns the current estimated cost.
func (t *UsageTracker) Cost() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.TotalCost
}

// IsOverBudget returns true if the budget has been exceeded.
func (t *UsageTracker) IsOverBudget() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Budget > 0 && t.TotalCost >= t.Budget
}

// Summary returns a human-readable summary of usage.
func (t *UsageTracker) Summary() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	return fmt.Sprintf(
		"Tokens: %dk in / %dk out | Cache: %dk read / %dk create | Cost: $%.4f | Turns: %d",
		t.InputTokens/1000, t.OutputTokens/1000,
		t.CacheRead/1000, t.CacheCreate/1000,
		t.TotalCost, t.TurnCount,
	)
}

// Snapshot returns current values for display.
// contextTokens is the last turn's input+cacheRead+cacheCreate — this is the
// actual current context window usage as reported by the API, not a cumulative sum.
// Using the cumulative sum would overcount by N× after N turns.
func (t *UsageTracker) Snapshot() (contextTokens int, cost float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.LastContextTokens, t.TotalCost
}

func (t *UsageTracker) calculateCost() float64 {
	pricing := lookupPricing(t.model)

	inputCost := float64(t.InputTokens) * pricing.InputPerMillion / 1_000_000
	outputCost := float64(t.OutputTokens) * pricing.OutputPerMillion / 1_000_000
	cacheReadCost := float64(t.CacheRead) * pricing.CacheReadPerMillion / 1_000_000
	cacheWriteCost := float64(t.CacheCreate) * pricing.CacheWritePerMillion / 1_000_000

	return inputCost + outputCost + cacheReadCost + cacheWriteCost
}

// lookupPricing finds pricing by exact match or prefix match.
// API model IDs often include date suffixes (e.g., claude-sonnet-4-6-20260301).
func lookupPricing(model string) ModelPricing {
	// Exact match
	if p, ok := modelPricing[model]; ok {
		return p
	}
	// Prefix match (longest prefix wins)
	var bestKey string
	for key := range modelPricing {
		if len(model) >= len(key) && model[:len(key)] == key {
			if len(key) > len(bestKey) {
				bestKey = key
			}
		}
	}
	if bestKey != "" {
		return modelPricing[bestKey]
	}
	// Keyword fallback
	switch {
	case contains(model, "opus"):
		return ModelPricing{InputPerMillion: 15.0, OutputPerMillion: 75.0, CacheReadPerMillion: 1.5, CacheWritePerMillion: 18.75}
	case contains(model, "haiku"):
		return ModelPricing{InputPerMillion: 0.25, OutputPerMillion: 1.25, CacheReadPerMillion: 0.025, CacheWritePerMillion: 0.3125}
	default:
		// Default to Sonnet pricing
		return ModelPricing{InputPerMillion: 3.0, OutputPerMillion: 15.0, CacheReadPerMillion: 0.3, CacheWritePerMillion: 3.75}
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
