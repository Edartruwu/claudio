package bus_test

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Abraxas-365/claudio/internal/bus"
)

func TestPubSub(t *testing.T) {
	b := bus.New()

	var received []bus.Event
	var mu sync.Mutex

	unsub := b.Subscribe("test.event", func(event bus.Event) {
		mu.Lock()
		received = append(received, event)
		mu.Unlock()
	})

	b.Publish(bus.Event{Type: "test.event", Payload: json.RawMessage(`{"key":"value"}`)})
	b.Publish(bus.Event{Type: "other.event", Payload: json.RawMessage(`{}`)})
	b.Publish(bus.Event{Type: "test.event", Payload: json.RawMessage(`{"key":"value2"}`)})

	time.Sleep(10 * time.Millisecond) // Let handlers run

	mu.Lock()
	count := len(received)
	mu.Unlock()

	if count != 2 {
		t.Errorf("expected 2 events, got %d", count)
	}

	unsub()

	b.Publish(bus.Event{Type: "test.event"})
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	count = len(received)
	mu.Unlock()

	if count != 2 {
		t.Errorf("after unsub: expected still 2, got %d", count)
	}
}

func TestSubscribeAll(t *testing.T) {
	b := bus.New()

	var count int
	var mu sync.Mutex

	unsub := b.SubscribeAll(func(event bus.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	b.Publish(bus.Event{Type: "a"})
	b.Publish(bus.Event{Type: "b"})
	b.Publish(bus.Event{Type: "c"})

	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	got := count
	mu.Unlock()

	if got != 3 {
		t.Errorf("SubscribeAll: expected 3, got %d", got)
	}

	unsub()
}

func TestChannel(t *testing.T) {
	b := bus.New()

	ch, cancel := b.Channel("test", 10)

	b.Publish(bus.Event{Type: "test", Payload: json.RawMessage(`"hello"`)})
	b.Publish(bus.Event{Type: "test", Payload: json.RawMessage(`"world"`)})

	select {
	case ev := <-ch:
		if ev.Type != "test" {
			t.Errorf("expected type 'test', got %q", ev.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}

	cancel()
}

func TestTimestamp(t *testing.T) {
	b := bus.New()

	var received bus.Event
	b.Subscribe("ts", func(e bus.Event) { received = e })

	b.Publish(bus.Event{Type: "ts"})
	time.Sleep(10 * time.Millisecond)

	if received.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}
