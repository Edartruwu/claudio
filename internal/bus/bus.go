package bus

import (
	"encoding/json"
	"sync"
	"time"
)

// Event represents a system event flowing through the bus.
type Event struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
	SessionID string          `json:"session_id,omitempty"`
}

// Handler processes events from the bus.
type Handler func(event Event)

// Bus is a concurrent pub/sub event system.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[string][]subscription
	allSubs     []subscription
	nextID      int
}

type subscription struct {
	id      int
	handler Handler
}

// New creates a new event bus.
func New() *Bus {
	return &Bus{
		subscribers: make(map[string][]subscription),
	}
}

// Publish sends an event to all subscribers of the given event type.
func (b *Bus) Publish(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	// Type-specific subscribers
	for _, sub := range b.subscribers[event.Type] {
		sub.handler(event)
	}

	// Wildcard subscribers (subscribe to all events)
	for _, sub := range b.allSubs {
		sub.handler(event)
	}
}

// Subscribe registers a handler for a specific event type.
// Returns an unsubscribe function.
func (b *Bus) Subscribe(eventType string, handler Handler) func() {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	sub := subscription{id: id, handler: handler}
	b.subscribers[eventType] = append(b.subscribers[eventType], sub)
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs := b.subscribers[eventType]
		for i, s := range subs {
			if s.id == id {
				b.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
	}
}

// SubscribeAll registers a handler for all event types.
// Returns an unsubscribe function.
func (b *Bus) SubscribeAll(handler Handler) func() {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	sub := subscription{id: id, handler: handler}
	b.allSubs = append(b.allSubs, sub)
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		for i, s := range b.allSubs {
			if s.id == id {
				b.allSubs = append(b.allSubs[:i], b.allSubs[i+1:]...)
				break
			}
		}
	}
}

// Channel returns a channel that receives events of the given type.
// The returned cancel function stops the subscription and closes the channel.
func (b *Bus) Channel(eventType string, bufSize int) (<-chan Event, func()) {
	ch := make(chan Event, bufSize)
	unsub := b.Subscribe(eventType, func(event Event) {
		select {
		case ch <- event:
		default:
			// Drop event if channel is full (non-blocking)
		}
	})
	return ch, func() {
		unsub()
		close(ch)
	}
}
