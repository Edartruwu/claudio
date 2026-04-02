// Package analytics provides token usage tracking, cost calculation, and budget enforcement.
package analytics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ModelPricing defines per-million-token pricing for a model.
type ModelPricing struct {
	InputPerMTok  float64
	OutputPerMTok float64
}

// KnownPricing contains pricing for known Claude models (per million tokens).
var KnownPricing = map[string]ModelPricing{
	"claude-opus-4-6":          {InputPerMTok: 15.0, OutputPerMTok: 75.0},
	"claude-opus-4-5":          {InputPerMTok: 15.0, OutputPerMTok: 75.0},
	"claude-sonnet-4-6":        {InputPerMTok: 3.0, OutputPerMTok: 15.0},
	"claude-sonnet-4-5":        {InputPerMTok: 3.0, OutputPerMTok: 15.0},
	"claude-haiku-4-5-20251001": {InputPerMTok: 0.25, OutputPerMTok: 1.25},
}

// Tracker accumulates token usage and cost for a session.
type Tracker struct {
	mu           sync.Mutex
	model        string
	inputTokens  int
	outputTokens int
	toolCalls    int
	apiCalls     int
	startTime    time.Time
	maxBudget    float64 // 0 = unlimited
	saveDir      string
}

// NewTracker creates a new analytics tracker.
func NewTracker(model string, maxBudget float64, saveDir string) *Tracker {
	return &Tracker{
		model:     model,
		maxBudget: maxBudget,
		startTime: time.Now(),
		saveDir:   saveDir,
	}
}

// RecordUsage adds token usage from an API call.
func (t *Tracker) RecordUsage(inputTokens, outputTokens int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.inputTokens += inputTokens
	t.outputTokens += outputTokens
	t.apiCalls++
}

// RecordToolCall increments the tool call counter.
func (t *Tracker) RecordToolCall() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.toolCalls++
}

// TotalTokens returns total tokens used.
func (t *Tracker) TotalTokens() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.inputTokens + t.outputTokens
}

// InputTokens returns input tokens used.
func (t *Tracker) InputTokens() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.inputTokens
}

// OutputTokens returns output tokens used.
func (t *Tracker) OutputTokens() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.outputTokens
}

// Cost returns the estimated cost in USD.
func (t *Tracker) Cost() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.computeCost()
}

func (t *Tracker) computeCost() float64 {
	pricing := getPricing(t.model)
	inputCost := float64(t.inputTokens) / 1_000_000 * pricing.InputPerMTok
	outputCost := float64(t.outputTokens) / 1_000_000 * pricing.OutputPerMTok
	return inputCost + outputCost
}

// CheckBudget returns an error if the budget has been exceeded.
// Returns a warning string if approaching the budget (>80%).
func (t *Tracker) CheckBudget() (warning string, exceeded bool) {
	if t.maxBudget <= 0 {
		return "", false
	}

	t.mu.Lock()
	cost := t.computeCost()
	t.mu.Unlock()

	if cost >= t.maxBudget {
		return fmt.Sprintf("Budget exceeded: $%.4f / $%.2f", cost, t.maxBudget), true
	}
	if cost >= t.maxBudget*0.8 {
		return fmt.Sprintf("Budget warning: $%.4f / $%.2f (%.0f%%)", cost, t.maxBudget, cost/t.maxBudget*100), false
	}
	return "", false
}

// Report returns a formatted session report.
func (t *Tracker) Report() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	cost := t.computeCost()
	duration := time.Since(t.startTime).Round(time.Second)

	var sb strings.Builder
	sb.WriteString("Session Analytics\n")
	sb.WriteString("=================\n")
	sb.WriteString(fmt.Sprintf("Model:         %s\n", t.model))
	sb.WriteString(fmt.Sprintf("Duration:      %s\n", duration))
	sb.WriteString(fmt.Sprintf("API Calls:     %d\n", t.apiCalls))
	sb.WriteString(fmt.Sprintf("Tool Calls:    %d\n", t.toolCalls))
	sb.WriteString(fmt.Sprintf("Input Tokens:  %d\n", t.inputTokens))
	sb.WriteString(fmt.Sprintf("Output Tokens: %d\n", t.outputTokens))
	sb.WriteString(fmt.Sprintf("Total Tokens:  %d\n", t.inputTokens+t.outputTokens))
	sb.WriteString(fmt.Sprintf("Estimated Cost: $%.4f\n", cost))

	if t.maxBudget > 0 {
		sb.WriteString(fmt.Sprintf("Budget:        $%.2f (%.0f%% used)\n", t.maxBudget, cost/t.maxBudget*100))
	}

	return sb.String()
}

// SaveReport persists the session analytics to disk.
func (t *Tracker) SaveReport(sessionID string) error {
	if t.saveDir == "" {
		return nil
	}

	os.MkdirAll(t.saveDir, 0755)

	t.mu.Lock()
	report := map[string]interface{}{
		"session_id":    sessionID,
		"model":         t.model,
		"start_time":    t.startTime.Format(time.RFC3339),
		"end_time":      time.Now().Format(time.RFC3339),
		"duration_s":    time.Since(t.startTime).Seconds(),
		"input_tokens":  t.inputTokens,
		"output_tokens": t.outputTokens,
		"total_tokens":  t.inputTokens + t.outputTokens,
		"api_calls":     t.apiCalls,
		"tool_calls":    t.toolCalls,
		"cost_usd":      t.computeCost(),
		"max_budget":    t.maxBudget,
	}
	t.mu.Unlock()

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("%s-%s.json", time.Now().Format("2006-01-02"), sessionID[:8])
	return os.WriteFile(filepath.Join(t.saveDir, filename), data, 0644)
}

func getPricing(model string) ModelPricing {
	// Try exact match first
	if p, ok := KnownPricing[model]; ok {
		return p
	}
	// Try prefix match
	for key, p := range KnownPricing {
		if strings.HasPrefix(model, key) {
			return p
		}
	}
	// Try keyword match
	lower := strings.ToLower(model)
	if strings.Contains(lower, "opus") {
		return ModelPricing{InputPerMTok: 15.0, OutputPerMTok: 75.0}
	}
	if strings.Contains(lower, "haiku") {
		return ModelPricing{InputPerMTok: 0.25, OutputPerMTok: 1.25}
	}
	// Default to Sonnet pricing
	return ModelPricing{InputPerMTok: 3.0, OutputPerMTok: 15.0}
}
