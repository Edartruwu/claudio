package oauth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// AuthCodeListener is a temporary HTTP server that captures the OAuth callback.
type AuthCodeListener struct {
	server   *http.Server
	port     int
	resultCh chan authResult
	state    string
	mu       sync.Mutex
}

type authResult struct {
	code string
	err  error
}

// NewAuthCodeListener creates a listener for OAuth redirects.
func NewAuthCodeListener() *AuthCodeListener {
	return &AuthCodeListener{
		resultCh: make(chan authResult, 1),
	}
}

// Start begins listening on a random available port.
// Returns the port number.
func (l *AuthCodeListener) Start() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("failed to start callback listener: %w", err)
	}

	l.port = listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", l.handleCallback)

	l.server = &http.Server{Handler: mux}

	go func() {
		if err := l.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			l.resultCh <- authResult{err: err}
		}
	}()

	return l.port, nil
}

// WaitForCode blocks until the OAuth redirect is received or context is cancelled.
func (l *AuthCodeListener) WaitForCode(ctx context.Context, state string) (string, error) {
	l.mu.Lock()
	l.state = state
	l.mu.Unlock()

	defer l.Shutdown()

	select {
	case result := <-l.resultCh:
		return result.code, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Port returns the port the listener is running on.
func (l *AuthCodeListener) Port() int {
	return l.port
}

// RedirectURI returns the callback URL.
func (l *AuthCodeListener) RedirectURI() string {
	return fmt.Sprintf("http://localhost:%d/callback", l.port)
}

// Shutdown stops the listener.
func (l *AuthCodeListener) Shutdown() {
	if l.server != nil {
		l.server.Close()
	}
}

func (l *AuthCodeListener) handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errParam := r.URL.Query().Get("error")

	if errParam != "" {
		desc := r.URL.Query().Get("error_description")
		l.resultCh <- authResult{err: fmt.Errorf("OAuth error: %s - %s", errParam, desc)}
		http.Error(w, "Authentication failed", http.StatusBadRequest)
		return
	}

	l.mu.Lock()
	expectedState := l.state
	l.mu.Unlock()

	if state != expectedState {
		l.resultCh <- authResult{err: fmt.Errorf("state mismatch: expected %q, got %q", expectedState, state)}
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	if code == "" {
		l.resultCh <- authResult{err: fmt.Errorf("no authorization code received")}
		http.Error(w, "No authorization code", http.StatusBadRequest)
		return
	}

	// Send success response to browser
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html><html><body>
		<h2>Authentication successful!</h2>
		<p>You can close this tab and return to the terminal.</p>
		<script>window.close()</script>
	</body></html>`)

	l.resultCh <- authResult{code: code}
}
