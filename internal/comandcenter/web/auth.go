package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/a-h/templ"
)

// createSession generates a new session token + CSRF token, stores them, and returns both.
func (ws *WebServer) createSession() (token, csrfToken string, err error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	csrfBytes := make([]byte, 32)
	if _, err := rand.Read(csrfBytes); err != nil {
		return "", "", fmt.Errorf("generate csrf token: %w", err)
	}
	token = hex.EncodeToString(tokenBytes)
	csrfToken = hex.EncodeToString(csrfBytes)
	ws.sessionMu.Lock()
	ws.sessions[token] = sessionEntry{csrfToken: csrfToken, expiresAt: time.Now().Add(sessionTTL)}
	ws.sessionMu.Unlock()
	return token, csrfToken, nil
}

// validateSession checks if the token is valid and not expired. Returns the CSRF token if valid.
func (ws *WebServer) validateSession(token string) (csrfToken string, ok bool) {
	if token == "" {
		return "", false
	}
	ws.sessionMu.RLock()
	entry, exists := ws.sessions[token]
	ws.sessionMu.RUnlock()
	if !exists || time.Now().After(entry.expiresAt) {
		if exists {
			// Expired — clean up.
			ws.sessionMu.Lock()
			delete(ws.sessions, token)
			ws.sessionMu.Unlock()
		}
		return "", false
	}
	return entry.csrfToken, true
}

// csrfTokenFromSession extracts the CSRF token for the current request's session.
func (ws *WebServer) csrfTokenFromSession(r *http.Request) string {
	c, err := r.Cookie("auth")
	if err != nil {
		return ""
	}
	csrf, _ := ws.validateSession(c.Value)
	return csrf
}

// CSRFToken returns the CSRF token for the current request, for use in templates.
func (ws *WebServer) CSRFToken(r *http.Request) string {
	return ws.csrfTokenFromSession(r)
}

// ExpireSessionForTest sets a session's expiry to the given time. Test-only helper.
func (ws *WebServer) ExpireSessionForTest(token string, expiresAt time.Time) {
	ws.sessionMu.Lock()
	defer ws.sessionMu.Unlock()
	if entry, ok := ws.sessions[token]; ok {
		entry.expiresAt = expiresAt
		ws.sessions[token] = entry
	}
}

// uiAuth checks the "auth" HttpOnly cookie against the session store.
// Also applies CSP headers and CSRF validation for state-changing methods.
func (ws *WebServer) uiAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Minimal CSP — internal admin tool; only block framing.
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")

		// Trust same-machine requests (e.g. Playwright fidelity tool)
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if host == "127.0.0.1" || host == "::1" {
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie("auth")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		csrfToken, ok := ws.validateSession(c.Value)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		// CSRF validation for state-changing methods.
		if r.Method == http.MethodPost || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
			gotCSRF := r.Header.Get("X-CSRF-Token")
			if gotCSRF == "" {
				// Try form value (for non-htmx forms).
				// ParseForm is idempotent; safe to call even if body was read.
				_ = r.ParseForm()
				gotCSRF = r.FormValue("_csrf")
			}
			if subtle.ConstantTimeCompare([]byte(gotCSRF), []byte(csrfToken)) != 1 {
				http.Error(w, "forbidden: invalid CSRF token", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (ws *WebServer) validPassword(v string) bool {
	if v == "" || ws.password == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(v), []byte(ws.password)) == 1
}

func (ws *WebServer) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	templ.Handler(Login(LoginPageData{Error: ""})).ServeHTTP(w, r)
}

func (ws *WebServer) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	pass := r.FormValue("password")
	if !ws.validPassword(pass) {
		w.WriteHeader(http.StatusUnauthorized)
		templ.Handler(Login(LoginPageData{Error: "Invalid password"})).ServeHTTP(w, r)
		return
	}
	token, _, err := ws.createSession()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "auth",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
