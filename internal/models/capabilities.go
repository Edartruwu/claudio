// Package models provides model capabilities caching and lookup.
package models

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

// ModelCapability describes a model's capabilities and limits.
type ModelCapability struct {
	ID              string `json:"id"`
	MaxInputTokens  int    `json:"max_input_tokens"`
	MaxOutputTokens int    `json:"max_output_tokens"`
	SupportsThinking bool  `json:"supports_thinking"`
	SupportsEffort   bool  `json:"supports_effort"`
}

// Cache holds cached model capabilities loaded from disk.
type Cache struct {
	mu     sync.RWMutex
	models []ModelCapability
}

// global cache instance
var (
	globalCache     *Cache
	globalCacheOnce sync.Once
)

// SetGlobalCache sets the global model capabilities cache.
func SetGlobalCache(c *Cache) {
	globalCache = c
}

// GetGlobalCache returns the global cache, or nil if not set.
func GetGlobalCache() *Cache {
	return globalCache
}

// LoadCache reads model capabilities from a JSON file.
// Returns an empty cache if the file doesn't exist or is invalid.
func LoadCache(path string) *Cache {
	c := &Cache{}

	data, err := os.ReadFile(path)
	if err != nil {
		return c
	}

	var models []ModelCapability
	if err := json.Unmarshal(data, &models); err != nil {
		return c
	}

	c.models = models
	return c
}

// SaveCache writes the cache to a JSON file.
func (c *Cache) SaveCache(path string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, err := json.MarshalIndent(c.models, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// MaxContext returns the maximum input tokens for the given model.
// Uses longest-ID-match strategy. Returns 0 if not found.
func (c *Cache) MaxContext(model string) int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	cap := c.findBestMatch(model)
	if cap != nil {
		return cap.MaxInputTokens
	}
	return 0
}

// MaxOutput returns the maximum output tokens for the given model.
// Returns 0 if not found.
func (c *Cache) MaxOutput(model string) int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	cap := c.findBestMatch(model)
	if cap != nil {
		return cap.MaxOutputTokens
	}
	return 0
}

// SupportsThinking returns whether the model supports extended thinking.
func (c *Cache) SupportsThinking(model string) bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	cap := c.findBestMatch(model)
	if cap != nil {
		return cap.SupportsThinking
	}
	return false
}

// findBestMatch finds the cached model with the longest matching ID.
// Uses substring matching: model "claude-opus-4-6" matches cache entry "opus-4-6".
func (c *Cache) findBestMatch(model string) *ModelCapability {
	modelLower := strings.ToLower(model)

	var best *ModelCapability
	bestLen := 0

	for i := range c.models {
		idLower := strings.ToLower(c.models[i].ID)
		if strings.Contains(modelLower, idLower) || strings.Contains(idLower, modelLower) {
			if len(c.models[i].ID) > bestLen {
				best = &c.models[i]
				bestLen = len(c.models[i].ID)
			}
		}
	}

	return best
}

// DefaultCapabilities returns hardcoded defaults for known models.
func DefaultCapabilities() []ModelCapability {
	return []ModelCapability{
		{ID: "claude-opus-4-6", MaxInputTokens: 200_000, MaxOutputTokens: 32_000, SupportsThinking: true, SupportsEffort: true},
		{ID: "claude-opus-4-5", MaxInputTokens: 200_000, MaxOutputTokens: 32_000, SupportsThinking: true, SupportsEffort: true},
		{ID: "claude-sonnet-4-6", MaxInputTokens: 200_000, MaxOutputTokens: 16_000, SupportsThinking: true, SupportsEffort: true},
		{ID: "claude-sonnet-4-5", MaxInputTokens: 200_000, MaxOutputTokens: 16_000, SupportsThinking: true, SupportsEffort: true},
		{ID: "claude-haiku-4-5", MaxInputTokens: 200_000, MaxOutputTokens: 8_192, SupportsThinking: false, SupportsEffort: false},
	}
}

// NewDefaultCache creates a cache pre-populated with hardcoded defaults.
func NewDefaultCache() *Cache {
	return &Cache{models: DefaultCapabilities()}
}
