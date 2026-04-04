package cachetracker

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// BreakReason constants
// ---------------------------------------------------------------------------

func TestBreakReasonConstants(t *testing.T) {
	tests := []struct {
		name  string
		value BreakReason
		want  string
	}{
		{"NewUserMessage", BreakReasonNewUserMessage, "new_user_message"},
		{"SystemChanged", BreakReasonSystemChanged, "system_changed"},
		{"Unknown", BreakReasonUnknown, "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.value) != tc.want {
				t.Errorf("got %q, want %q", tc.value, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tracker.Record
// ---------------------------------------------------------------------------

func TestTracker_Record_NoCacheMiss(t *testing.T) {
	var tr Tracker
	reason := tr.Record(0, "system", 2)
	if reason != "" {
		t.Errorf("expected empty reason for zero cacheCreate, got %q", reason)
	}
	events := tr.Events()
	if len(events) != 0 {
		t.Errorf("expected no events, got %d", len(events))
	}
}

func TestTracker_Record_SystemChanged(t *testing.T) {
	var tr Tracker
	// Seed internal state without a miss
	tr.Record(0, "old system", 1)
	// Now change the system prompt with cache tokens > 0
	reason := tr.Record(100, "new system", 1)
	if reason != BreakReasonSystemChanged {
		t.Errorf("expected BreakReasonSystemChanged, got %q", reason)
	}
}

func TestTracker_Record_NewUserMessage(t *testing.T) {
	var tr Tracker
	tr.Record(0, "same system", 1)
	reason := tr.Record(50, "same system", 2)
	if reason != BreakReasonNewUserMessage {
		t.Errorf("expected BreakReasonNewUserMessage, got %q", reason)
	}
}

func TestTracker_Record_Unknown(t *testing.T) {
	var tr Tracker
	// cacheCreate > 0 but neither system changed nor msg count increased
	tr.Record(0, "same system", 3)
	reason := tr.Record(10, "same system", 3)
	if reason != BreakReasonUnknown {
		t.Errorf("expected BreakReasonUnknown, got %q", reason)
	}
}

func TestTracker_Record_SystemChangedTakesPrecedence(t *testing.T) {
	var tr Tracker
	tr.Record(0, "old system", 1)
	// Both system changed AND msg count increased
	reason := tr.Record(200, "new system", 5)
	if reason != BreakReasonSystemChanged {
		t.Errorf("expected BreakReasonSystemChanged (takes precedence), got %q", reason)
	}
}

func TestTracker_Record_EventFields(t *testing.T) {
	var tr Tracker
	before := time.Now()
	tr.Record(0, "sys", 0)
	tr.Record(77, "sys", 1) // turn=2, NewUserMessage
	after := time.Now()

	events := tr.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Turn != 2 {
		t.Errorf("Turn: got %d, want 2", e.Turn)
	}
	if e.Reason != BreakReasonNewUserMessage {
		t.Errorf("Reason: got %q, want %q", e.Reason, BreakReasonNewUserMessage)
	}
	if e.CacheCreateTokens != 77 {
		t.Errorf("CacheCreateTokens: got %d, want 77", e.CacheCreateTokens)
	}
	if e.At.Before(before) || e.At.After(after) {
		t.Errorf("At timestamp %v not in expected range [%v, %v]", e.At, before, after)
	}
}

func TestTracker_Record_MultipleEvents(t *testing.T) {
	var tr Tracker
	tr.Record(0, "sys", 1)
	tr.Record(10, "sys", 2)  // NewUserMessage
	tr.Record(20, "sys2", 2) // SystemChanged
	tr.Record(30, "sys2", 2) // Unknown

	events := tr.Events()
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Reason != BreakReasonNewUserMessage {
		t.Errorf("event[0] reason: got %q", events[0].Reason)
	}
	if events[1].Reason != BreakReasonSystemChanged {
		t.Errorf("event[1] reason: got %q", events[1].Reason)
	}
	if events[2].Reason != BreakReasonUnknown {
		t.Errorf("event[2] reason: got %q", events[2].Reason)
	}
}

func TestTracker_Record_IncrementsTurnOnEveryCall(t *testing.T) {
	var tr Tracker
	for i := 0; i < 5; i++ {
		tr.Record(0, "sys", i)
	}
	// Trigger a miss on turn 6
	tr.Record(1, "sys", 10)
	events := tr.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Turn != 6 {
		t.Errorf("Turn: got %d, want 6", events[0].Turn)
	}
}

func TestTracker_Record_ZeroCacheCreateNoEvent(t *testing.T) {
	var tr Tracker
	// Even when system changes, if cacheCreate==0 no event is stored
	tr.Record(0, "old", 1)
	tr.Record(0, "new", 5)
	if len(tr.Events()) != 0 {
		t.Errorf("expected no events when cacheCreate is 0")
	}
}

// ---------------------------------------------------------------------------
// Tracker.Events – returns a copy
// ---------------------------------------------------------------------------

func TestTracker_Events_ReturnsCopy(t *testing.T) {
	var tr Tracker
	tr.Record(0, "sys", 0)
	tr.Record(5, "sys", 1)

	e1 := tr.Events()
	// Mutate the returned slice
	e1[0].CacheCreateTokens = 9999

	e2 := tr.Events()
	if e2[0].CacheCreateTokens == 9999 {
		t.Error("Events() should return a copy; mutation affected internal state")
	}
}

func TestTracker_Events_EmptyWhenNoMisses(t *testing.T) {
	var tr Tracker
	tr.Record(0, "sys", 1)
	tr.Record(0, "sys", 2)
	events := tr.Events()
	if events == nil {
		t.Error("Events() should return non-nil slice")
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestTracker_Events_EmptyOnFreshTracker(t *testing.T) {
	var tr Tracker
	events := tr.Events()
	if len(events) != 0 {
		t.Errorf("fresh tracker should have 0 events, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// Tracker.Summary
// ---------------------------------------------------------------------------

func TestTracker_Summary_NoTurns(t *testing.T) {
	var tr Tracker
	s := tr.Summary()
	if s != "no turns recorded" {
		t.Errorf("got %q, want %q", s, "no turns recorded")
	}
}

func TestTracker_Summary_Format(t *testing.T) {
	var tr Tracker
	tr.Record(0, "sys", 0)  // turn 1, no miss
	tr.Record(10, "sys", 1) // turn 2, miss – NewUserMessage, 10 tokens
	tr.Record(20, "sys2", 1) // turn 3, miss – SystemChanged, 20 tokens

	s := tr.Summary()
	// Expected: "turns=3 misses=2 cache_create_tokens=30"
	if !strings.Contains(s, "turns=3") {
		t.Errorf("summary missing turns=3: %q", s)
	}
	if !strings.Contains(s, "misses=2") {
		t.Errorf("summary missing misses=2: %q", s)
	}
	if !strings.Contains(s, "cache_create_tokens=30") {
		t.Errorf("summary missing cache_create_tokens=30: %q", s)
	}
}

func TestTracker_Summary_ZeroMisses(t *testing.T) {
	var tr Tracker
	tr.Record(0, "sys", 0)
	tr.Record(0, "sys", 1)
	s := tr.Summary()
	if !strings.Contains(s, "turns=2") {
		t.Errorf("expected turns=2: %q", s)
	}
	if !strings.Contains(s, "misses=0") {
		t.Errorf("expected misses=0: %q", s)
	}
	if !strings.Contains(s, "cache_create_tokens=0") {
		t.Errorf("expected cache_create_tokens=0: %q", s)
	}
}

func TestTracker_Summary_SingleMiss(t *testing.T) {
	var tr Tracker
	tr.Record(42, "sys", 0) // turn 1, unknown (first call, nothing changed)
	s := tr.Summary()
	if !strings.Contains(s, "turns=1") {
		t.Errorf("expected turns=1: %q", s)
	}
	if !strings.Contains(s, "misses=1") {
		t.Errorf("expected misses=1: %q", s)
	}
	if !strings.Contains(s, "cache_create_tokens=42") {
		t.Errorf("expected cache_create_tokens=42: %q", s)
	}
}

// ---------------------------------------------------------------------------
// Tracker – concurrency safety
// ---------------------------------------------------------------------------

func TestTracker_ConcurrentRecord(t *testing.T) {
	var tr Tracker
	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			tr.Record(n, "sys", n)
		}(i)
	}
	wg.Wait()
	// Just ensure no race / panic; turn count should equal goroutines
	s := tr.Summary()
	if !strings.Contains(s, "turns=50") {
		t.Errorf("expected turns=50 in summary: %q", s)
	}
}

func TestTracker_ConcurrentEventsAndRecord(t *testing.T) {
	var tr Tracker
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			tr.Record(n, "sys", n)
		}(i)
		go func() {
			defer wg.Done()
			_ = tr.Events()
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// NewExpiryWatcher
// ---------------------------------------------------------------------------

func TestNewExpiryWatcher(t *testing.T) {
	ttl := 5 * time.Minute
	w := NewExpiryWatcher(ttl)
	if w == nil {
		t.Fatal("NewExpiryWatcher returned nil")
	}
}

// ---------------------------------------------------------------------------
// ExpiryWatcher.IsExpired
// ---------------------------------------------------------------------------

func TestExpiryWatcher_IsExpired_NoCallRecorded(t *testing.T) {
	w := NewExpiryWatcher(5 * time.Minute)
	if w.IsExpired() {
		t.Error("IsExpired should return false when no call has been recorded")
	}
}

func TestExpiryWatcher_IsExpired_JustRecorded(t *testing.T) {
	w := NewExpiryWatcher(5 * time.Minute)
	w.RecordCall()
	if w.IsExpired() {
		t.Error("IsExpired should return false immediately after RecordCall")
	}
}

func TestExpiryWatcher_IsExpired_AfterTTL(t *testing.T) {
	w := NewExpiryWatcher(10 * time.Millisecond)
	w.RecordCall()
	time.Sleep(20 * time.Millisecond)
	if !w.IsExpired() {
		t.Error("IsExpired should return true after TTL has elapsed")
	}
}

func TestExpiryWatcher_IsExpired_ResetAfterNewCall(t *testing.T) {
	w := NewExpiryWatcher(10 * time.Millisecond)
	w.RecordCall()
	time.Sleep(20 * time.Millisecond)
	if !w.IsExpired() {
		t.Skip("first expiry check failed – skip reset test")
	}
	// Record a new call; should no longer be expired
	w.RecordCall()
	if w.IsExpired() {
		t.Error("IsExpired should return false right after a fresh RecordCall")
	}
}

func TestExpiryWatcher_IsExpired_ExactBoundary(t *testing.T) {
	// TTL of 0 means every call is immediately expired.
	w := NewExpiryWatcher(0)
	w.RecordCall()
	// time.Since > 0 (even nanoseconds) should be > 0 TTL
	time.Sleep(1 * time.Millisecond)
	if !w.IsExpired() {
		t.Error("IsExpired should be true when TTL=0 and some time has passed")
	}
}

// ---------------------------------------------------------------------------
// ExpiryWatcher.TimeSinceLastCall
// ---------------------------------------------------------------------------

func TestExpiryWatcher_TimeSinceLastCall_NoCallRecorded(t *testing.T) {
	w := NewExpiryWatcher(5 * time.Minute)
	d := w.TimeSinceLastCall()
	if d != 0 {
		t.Errorf("expected 0 when no call recorded, got %v", d)
	}
}

func TestExpiryWatcher_TimeSinceLastCall_AfterRecord(t *testing.T) {
	w := NewExpiryWatcher(5 * time.Minute)
	w.RecordCall()
	time.Sleep(5 * time.Millisecond)
	d := w.TimeSinceLastCall()
	if d <= 0 {
		t.Errorf("expected positive duration after RecordCall, got %v", d)
	}
}

func TestExpiryWatcher_TimeSinceLastCall_IncreasesOverTime(t *testing.T) {
	w := NewExpiryWatcher(5 * time.Minute)
	w.RecordCall()
	d1 := w.TimeSinceLastCall()
	time.Sleep(10 * time.Millisecond)
	d2 := w.TimeSinceLastCall()
	if d2 <= d1 {
		t.Errorf("expected d2 (%v) > d1 (%v)", d2, d1)
	}
}

func TestExpiryWatcher_TimeSinceLastCall_ResetsOnNewRecord(t *testing.T) {
	w := NewExpiryWatcher(5 * time.Minute)
	w.RecordCall()
	time.Sleep(10 * time.Millisecond)
	d1 := w.TimeSinceLastCall()
	w.RecordCall() // reset
	d2 := w.TimeSinceLastCall()
	if d2 >= d1 {
		t.Errorf("expected d2 (%v) < d1 (%v) after reset", d2, d1)
	}
}

// ---------------------------------------------------------------------------
// ExpiryWatcher – concurrency safety
// ---------------------------------------------------------------------------

func TestExpiryWatcher_ConcurrentRecordAndIsExpired(t *testing.T) {
	w := NewExpiryWatcher(5 * time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			w.RecordCall()
		}()
		go func() {
			defer wg.Done()
			_ = w.IsExpired()
		}()
	}
	wg.Wait()
}

func TestExpiryWatcher_ConcurrentTimeSinceLastCall(t *testing.T) {
	w := NewExpiryWatcher(time.Second)
	w.RecordCall()
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = w.TimeSinceLastCall()
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// Event struct – zero value
// ---------------------------------------------------------------------------

func TestEvent_ZeroValue(t *testing.T) {
	var e Event
	if e.Turn != 0 {
		t.Errorf("zero Turn: got %d", e.Turn)
	}
	if e.Reason != "" {
		t.Errorf("zero Reason: got %q", e.Reason)
	}
	if e.CacheCreateTokens != 0 {
		t.Errorf("zero CacheCreateTokens: got %d", e.CacheCreateTokens)
	}
	if !e.At.IsZero() {
		t.Errorf("zero At should be zero time")
	}
}
