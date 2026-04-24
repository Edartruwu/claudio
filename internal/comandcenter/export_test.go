package comandcenter

import "time"

// WaitSessionWorker blocks until the worker goroutine for sessionID exits or
// the timeout elapses. Returns true if the worker exited, false on timeout.
// If the session is already gone from the map (already cleaned up) it returns
// true immediately.
func (h *Hub) WaitSessionWorker(sessionID string, timeout time.Duration) bool {
	h.mu.RLock()
	ch, ok := h.workerDone[sessionID]
	h.mu.RUnlock()
	if !ok {
		return true // already cleaned up
	}
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}
