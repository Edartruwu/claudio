package utils

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// GracefulShutdown manages cleanup on process termination.
type GracefulShutdown struct {
	mu       sync.Mutex
	handlers []func()
	done     chan struct{}
}

// NewGracefulShutdown creates a shutdown manager that listens for SIGINT/SIGTERM.
func NewGracefulShutdown() *GracefulShutdown {
	gs := &GracefulShutdown{
		done: make(chan struct{}),
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		gs.execute()
		close(gs.done)
	}()

	return gs
}

// OnShutdown registers a cleanup function.
// Functions are called in reverse order (LIFO).
func (gs *GracefulShutdown) OnShutdown(fn func()) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.handlers = append(gs.handlers, fn)
}

// Done returns a channel that's closed when shutdown is triggered.
func (gs *GracefulShutdown) Done() <-chan struct{} {
	return gs.done
}

// Shutdown manually triggers the shutdown sequence.
func (gs *GracefulShutdown) Shutdown() {
	gs.execute()
}

func (gs *GracefulShutdown) execute() {
	gs.mu.Lock()
	handlers := make([]func(), len(gs.handlers))
	copy(handlers, gs.handlers)
	gs.mu.Unlock()

	// Execute in reverse order (LIFO)
	for i := len(handlers) - 1; i >= 0; i-- {
		handlers[i]()
	}
}

// CleanupRegistry tracks resources that need cleanup.
type CleanupRegistry struct {
	mu       sync.Mutex
	cleanups map[string]func()
}

// NewCleanupRegistry creates a new cleanup registry.
func NewCleanupRegistry() *CleanupRegistry {
	return &CleanupRegistry{
		cleanups: make(map[string]func()),
	}
}

// Register adds a named cleanup function.
func (cr *CleanupRegistry) Register(name string, fn func()) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.cleanups[name] = fn
}

// Deregister removes a cleanup function.
func (cr *CleanupRegistry) Deregister(name string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	delete(cr.cleanups, name)
}

// RunAll executes all cleanup functions.
func (cr *CleanupRegistry) RunAll() {
	cr.mu.Lock()
	fns := make([]func(), 0, len(cr.cleanups))
	for _, fn := range cr.cleanups {
		fns = append(fns, fn)
	}
	cr.mu.Unlock()

	for _, fn := range fns {
		fn()
	}
}

// ContextWithCancel creates a context that's cancelled on signal.
func ContextWithCancel() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}
