package utils

import "sync"

// CircularBuffer is a thread-safe fixed-size ring buffer for strings.
// Used for capping output lines in background tasks.
type CircularBuffer struct {
	mu    sync.Mutex
	items []string
	cap   int
	head  int
	count int
}

// NewCircularBuffer creates a buffer with the given capacity.
func NewCircularBuffer(capacity int) *CircularBuffer {
	return &CircularBuffer{
		items: make([]string, capacity),
		cap:   capacity,
	}
}

// Push adds an item. If full, the oldest item is overwritten.
func (b *CircularBuffer) Push(item string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	idx := (b.head + b.count) % b.cap
	b.items[idx] = item

	if b.count < b.cap {
		b.count++
	} else {
		b.head = (b.head + 1) % b.cap
	}
}

// All returns all items in order (oldest to newest).
func (b *CircularBuffer) All() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := make([]string, b.count)
	for i := 0; i < b.count; i++ {
		result[i] = b.items[(b.head+i)%b.cap]
	}
	return result
}

// Last returns the most recent n items.
func (b *CircularBuffer) Last(n int) []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if n > b.count {
		n = b.count
	}

	result := make([]string, n)
	start := b.count - n
	for i := 0; i < n; i++ {
		result[i] = b.items[(b.head+start+i)%b.cap]
	}
	return result
}

// Len returns the number of items in the buffer.
func (b *CircularBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count
}

// Clear empties the buffer.
func (b *CircularBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.head = 0
	b.count = 0
}
