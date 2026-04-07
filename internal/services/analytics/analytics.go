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
	InputPerMTok      float64
	OutputPerMTok     float64
	CacheReadPerMTok  float64
	CacheWritePerMTok float64
}

// KnownPricing contains pricing for known Claude models (per million tokens).
var KnownPricing = map[string]ModelPricing{
	"claude-opus-4-6":           {InputPerMTok: 15.0, OutputPerMTok: 75.0, CacheReadPerMTok: 1.5, CacheWritePerMTok: 18.75},
	"claude-opus-4-5":           {InputPerMTok: 15.0, OutputPerMTok: 75.0, CacheReadPerMTok: 1.5, CacheWritePerMTok: 18.75},
	"claude-sonnet-4-6":         {InputPerMTok: 3.0, OutputPerMTok: 15.0, CacheReadPerMTok: 0.3, CacheWritePerMTok: 3.75},
	"claude-sonnet-4-5":         {InputPerMTok: 3.0, OutputPerMTok: 15.0, CacheReadPerMTok: 0.3, CacheWritePerMTok: 3.75},
	"claude-haiku-4-5-20251001": {InputPerMTok: 0.25, OutputPerMTok: 1.25, CacheReadPerMTok: 0.025, CacheWritePerMTok: 0.3125},
}

// Tracker accumulates token usage and cost for a session.
type Tracker struct {
	mu              sync.Mutex
	model           string
	inputTokens     int
	outputTokens    int
	cacheReadTokens int
	cacheCreateTokens int
	toolCalls       int
	apiCalls        int
	startTime       time.Time
	maxBudget       float64 // 0 = unlimited
	saveDir         string
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
func (t *Tracker) RecordUsage(inputTokens, outputTokens, cacheReadTokens, cacheCreateTokens int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.inputTokens += inputTokens
	t.outputTokens += outputTokens
	t.cacheReadTokens += cacheReadTokens
	t.cacheCreateTokens += cacheCreateTokens
	t.apiCalls++
}

// RecordToolCall increments the tool call counter.
func (t *Tracker) RecordToolCall() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.toolCalls++
}

// TotalTokens returns total tokens used, including cache read/write tokens.
// Anthropic splits prompt content across input_tokens, cache_read_input_tokens,
// and cache_creation_input_tokens — summing only input+output severely
// underreports the real context size once prompt caching is active.
func (t *Tracker) TotalTokens() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.inputTokens + t.outputTokens + t.cacheReadTokens + t.cacheCreateTokens
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

// CacheReadTokens returns cache read tokens used.
func (t *Tracker) CacheReadTokens() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cacheReadTokens
}

// CacheCreateTokens returns cache creation tokens used.
func (t *Tracker) CacheCreateTokens() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cacheCreateTokens
}

// CacheHitRate returns the percentage of input tokens served from cache (0-100).
func (t *Tracker) CacheHitRate() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	total := t.inputTokens + t.cacheReadTokens
	if total == 0 {
		return 0
	}
	return float64(t.cacheReadTokens) / float64(total) * 100
}

// MaxBudget returns the configured budget limit (0 = unlimited).
func (t *Tracker) MaxBudget() float64 {
	return t.maxBudget
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
	cacheReadCost := float64(t.cacheReadTokens) / 1_000_000 * pricing.CacheReadPerMTok
	cacheWriteCost := float64(t.cacheCreateTokens) / 1_000_000 * pricing.CacheWritePerMTok
	return inputCost + outputCost + cacheReadCost + cacheWriteCost
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
	sb.WriteString(fmt.Sprintf("Cache Read:    %d\n", t.cacheReadTokens))
	sb.WriteString(fmt.Sprintf("Cache Create:  %d\n", t.cacheCreateTokens))
	sb.WriteString(fmt.Sprintf("Total Tokens:  %d\n", t.inputTokens+t.outputTokens+t.cacheReadTokens+t.cacheCreateTokens))
	sb.WriteString(fmt.Sprintf("Estimated Cost: $%.4f\n", cost))
	totalInput := t.inputTokens + t.cacheReadTokens
	if totalInput > 0 {
		sb.WriteString(fmt.Sprintf("Cache Hit Rate: %.1f%%\n", float64(t.cacheReadTokens)/float64(totalInput)*100))
	}

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
	totalInput := t.inputTokens + t.cacheReadTokens
	var cacheHitRate float64
	if totalInput > 0 {
		cacheHitRate = float64(t.cacheReadTokens) / float64(totalInput) * 100
	}
	report := map[string]interface{}{
		"session_id":          sessionID,
		"model":               t.model,
		"start_time":          t.startTime.Format(time.RFC3339),
		"end_time":            time.Now().Format(time.RFC3339),
		"duration_s":          time.Since(t.startTime).Seconds(),
		"input_tokens":        t.inputTokens,
		"output_tokens":       t.outputTokens,
		"cache_read_tokens":   t.cacheReadTokens,
		"cache_create_tokens": t.cacheCreateTokens,
		"total_tokens":        t.inputTokens + t.outputTokens + t.cacheReadTokens + t.cacheCreateTokens,
		"api_calls":           t.apiCalls,
		"tool_calls":          t.toolCalls,
		"cost_usd":            t.computeCost(),
		"max_budget":          t.maxBudget,
		"cache_hit_rate":      cacheHitRate,
	}
	t.mu.Unlock()

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	shortID := sessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	filename := fmt.Sprintf("%s-%s.json", time.Now().Format("2006-01-02"), shortID)
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
		return ModelPricing{InputPerMTok: 15.0, OutputPerMTok: 75.0, CacheReadPerMTok: 1.5, CacheWritePerMTok: 18.75}
	}
	if strings.Contains(lower, "haiku") {
		return ModelPricing{InputPerMTok: 0.25, OutputPerMTok: 1.25, CacheReadPerMTok: 0.025, CacheWritePerMTok: 0.3125}
	}
	// Default to Sonnet pricing
	return ModelPricing{InputPerMTok: 3.0, OutputPerMTok: 15.0, CacheReadPerMTok: 0.3, CacheWritePerMTok: 3.75}
}
