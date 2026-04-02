package cachetracker

import (
	"fmt"
	"sync"
	"time"
)

// BreakReason describes why the prompt cache was invalidated.
type BreakReason string

const (
	BreakReasonNewUserMessage BreakReason = "new_user_message"
	BreakReasonSystemChanged  BreakReason = "system_changed"
	BreakReasonUnknown        BreakReason = "unknown"
)

// Event records a single cache miss.
type Event struct {
	Turn              int
	Reason            BreakReason
	CacheCreateTokens int
	At                time.Time
}

// Tracker records prompt cache misses and infers their cause.
type Tracker struct {
	mu           sync.Mutex
	events       []Event
	lastSystem   string
	lastMsgCount int
	turn         int
}

// Record is called after each API response. If cache creation tokens are > 0,
// a cache miss occurred and the reason is inferred from what changed.
func (t *Tracker) Record(cacheCreate int, systemPrompt string, msgCount int) BreakReason {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.turn++
	if cacheCreate == 0 {
		t.lastSystem = systemPrompt
		t.lastMsgCount = msgCount
		return ""
	}

	var reason BreakReason
	switch {
	case systemPrompt != t.lastSystem:
		reason = BreakReasonSystemChanged
	case msgCount > t.lastMsgCount:
		reason = BreakReasonNewUserMessage
	default:
		reason = BreakReasonUnknown
	}

	t.events = append(t.events, Event{
		Turn:              t.turn,
		Reason:            reason,
		CacheCreateTokens: cacheCreate,
		At:                time.Now(),
	})
	t.lastSystem = systemPrompt
	t.lastMsgCount = msgCount
	return reason
}

// Events returns all recorded cache miss events.
func (t *Tracker) Events() []Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Event, len(t.events))
	copy(out, t.events)
	return out
}

// Summary returns a human-readable summary of cache efficiency.
func (t *Tracker) Summary() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.turn == 0 {
		return "no turns recorded"
	}
	misses := len(t.events)
	totalTokens := 0
	for _, e := range t.events {
		totalTokens += e.CacheCreateTokens
	}
	return fmt.Sprintf("turns=%d misses=%d cache_create_tokens=%d", t.turn, misses, totalTokens)
}

// ExpiryWatcher tracks whether the prompt cache TTL (5 min) has likely expired.
type ExpiryWatcher struct {
	mu          sync.Mutex
	lastAPICall time.Time
	ttl         time.Duration
}

// NewExpiryWatcher creates a watcher with the given TTL (typically 5 minutes).
func NewExpiryWatcher(ttl time.Duration) *ExpiryWatcher {
	return &ExpiryWatcher{ttl: ttl}
}

// RecordCall marks the time of the most recent API call.
func (w *ExpiryWatcher) RecordCall() {
	w.mu.Lock()
	w.lastAPICall = time.Now()
	w.mu.Unlock()
}

// IsExpired returns true if more time than TTL has passed since the last call.
// Returns false if no call has been recorded yet.
func (w *ExpiryWatcher) IsExpired() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lastAPICall.IsZero() {
		return false
	}
	return time.Since(w.lastAPICall) > w.ttl
}

// TimeSinceLastCall returns the duration since the last recorded API call.
func (w *ExpiryWatcher) TimeSinceLastCall() time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lastAPICall.IsZero() {
		return 0
	}
	return time.Since(w.lastAPICall)
}
