package api

import (
	"fmt"
	"sync"
)

// ModelPricing holds cost per million tokens for a model.
type ModelPricing struct {
	InputPerMillion  float64
	OutputPerMillion float64
	CacheReadPerMillion float64
}

// Known model pricing (USD per million tokens).
var modelPricing = map[string]ModelPricing{
	"claude-opus-4-6":            {InputPerMillion: 5.0, OutputPerMillion: 25.0, CacheReadPerMillion: 0.5},
	"claude-sonnet-4-6":          {InputPerMillion: 3.0, OutputPerMillion: 15.0, CacheReadPerMillion: 0.3},
	"claude-haiku-4-5":           {InputPerMillion: 0.8, OutputPerMillion: 4.0, CacheReadPerMillion: 0.08},
	"claude-sonnet-4-5-20250514": {InputPerMillion: 3.0, OutputPerMillion: 15.0, CacheReadPerMillion: 0.3},
}

// UsageTracker accumulates token usage and cost across a session.
type UsageTracker struct {
	mu           sync.Mutex
	model        string
	InputTokens  int
	OutputTokens int
	CacheRead    int
	CacheCreate  int
	TurnCount    int
	TotalCost    float64
	Budget       float64 // 0 = unlimited
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
func (t *UsageTracker) Snapshot() (totalTokens int, cost float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.InputTokens + t.OutputTokens, t.TotalCost
}

func (t *UsageTracker) calculateCost() float64 {
	pricing, ok := modelPricing[t.model]
	if !ok {
		// Default to Sonnet pricing
		pricing = ModelPricing{InputPerMillion: 3.0, OutputPerMillion: 15.0, CacheReadPerMillion: 0.3}
	}

	inputCost := float64(t.InputTokens) * pricing.InputPerMillion / 1_000_000
	outputCost := float64(t.OutputTokens) * pricing.OutputPerMillion / 1_000_000
	cacheCost := float64(t.CacheRead) * pricing.CacheReadPerMillion / 1_000_000

	return inputCost + outputCost + cacheCost
}
