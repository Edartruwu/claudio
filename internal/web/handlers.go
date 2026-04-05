package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/web/templates"
)

// handleStatic serves static files (CSS, JS).
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/static/")
	content, ok := staticFiles[path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(path, ".css") {
		w.Header().Set("Content-Type", "text/css")
	} else if strings.HasSuffix(path, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	io.WriteString(w, content)
}

// handleLoginPage renders the login form.
func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	errMsg := r.URL.Query().Get("error")
	templates.LoginPage(errMsg).Render(r.Context(), w)
}

// handleLogin validates password and sets auth cookie.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	password := r.FormValue("password")
	if !s.validatePassword(password) {
		templates.LoginPage("Invalid password").Render(r.Context(), w)
		return
	}

	token := generateToken()
	s.mu.Lock()
	s.tokens[token] = time.Now().Add(24 * time.Hour)
	s.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "claudio_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLogout clears the auth cookie.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("claudio_token")
	if err == nil {
		s.mu.Lock()
		delete(s.tokens, cookie.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "claudio_token",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// handleHome renders the project browser.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	projects := s.listKnownProjects()
	templates.HomePage(projects, s.config.Version).Render(r.Context(), w)
}

// handleChatPage renders the chat page for a project.
func (s *Server) handleChatPage(w http.ResponseWriter, r *http.Request) {
	projectPath := r.URL.Query().Get("project")
	if projectPath == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Ensure session exists
	sess, err := s.sessions.GetOrCreate(projectPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sess.mu.Lock()
	messages := make([]templates.ChatMessage, len(sess.Messages))
	copy(messages, sess.Messages)
	sess.mu.Unlock()

	templates.ChatPage(projectPath, messages).Render(r.Context(), w)
}

// handleProjectInit initializes Claudio in a project directory.
func (s *Server) handleProjectInit(w http.ResponseWriter, r *http.Request) {
	projectPath := r.FormValue("path")
	if projectPath == "" {
		w.WriteHeader(http.StatusBadRequest)
		templates.ErrorMessage("Project path is required").Render(r.Context(), w)
		return
	}

	// Expand ~ to home dir
	if strings.HasPrefix(projectPath, "~/") {
		home, _ := os.UserHomeDir()
		projectPath = filepath.Join(home, projectPath[2:])
	}

	// Make absolute
	projectPath, _ = filepath.Abs(projectPath)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templates.ErrorMessage(fmt.Sprintf("Failed to create directory: %v", err)).Render(r.Context(), w)
		return
	}

	// Create .claudio directory
	claudioDir := filepath.Join(projectPath, ".claudio")
	if err := os.MkdirAll(claudioDir, 0755); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templates.ErrorMessage(fmt.Sprintf("Failed to init: %v", err)).Render(r.Context(), w)
		return
	}

	// Create default settings.json if not exists
	settingsPath := filepath.Join(claudioDir, "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		defaultSettings := map[string]interface{}{
			"model": "claude-sonnet-4-6",
		}
		data, _ := json.MarshalIndent(defaultSettings, "", "  ")
		os.WriteFile(settingsPath, data, 0644)
	}

	// Return updated project list via HTMX
	projects := s.listKnownProjects()
	// Ensure the new project is in the list
	found := false
	for _, p := range projects {
		if p.Path == projectPath {
			found = true
			break
		}
	}
	if !found {
		projects = append([]templates.ProjectInfo{{
			Path:        projectPath,
			Name:        filepath.Base(projectPath),
			Initialized: true,
		}}, projects...)
	}
	templates.ProjectList(projects).Render(r.Context(), w)
}

// handleChatSend handles a new user message.
// Flow: creates a new WebHandler, starts Engine.Run in background, returns HTML
// that includes an SSE connection div which connects to /api/chat/stream.
func (s *Server) handleChatSend(w http.ResponseWriter, r *http.Request) {
	projectPath := r.FormValue("project")
	message := r.FormValue("message")
	if projectPath == "" || message == "" {
		http.Error(w, "missing project or message", http.StatusBadRequest)
		return
	}

	sess, err := s.sessions.GetOrCreate(projectPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sess.AddMessage("user", message)

	// Create a fresh handler and start the query
	handler, err := sess.SendMessage(context.Background(), message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = handler // handler is stored in session, SSE endpoint reads it

	// Return user message bubble + an SSE-connected div for the response stream
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	templates.UserMessage(message).Render(r.Context(), w)
	templates.StreamingResponse(projectPath).Render(r.Context(), w)
}

// handleChatStream is the SSE endpoint — streams events from the current handler.
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	projectPath := r.URL.Query().Get("project")
	if projectPath == "" {
		http.Error(w, "missing project", http.StatusBadRequest)
		return
	}

	sess := s.sessions.Get(projectPath)
	if sess == nil {
		http.Error(w, "no active session", http.StatusBadRequest)
		return
	}

	handler := sess.CurrentHandler()
	if handler == nil {
		http.Error(w, "no active query", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Replay any buffered events that the client missed (reconnect scenario)
	sinceStr := r.URL.Query().Get("since")
	sinceSeq := 0
	if sinceStr != "" {
		fmt.Sscanf(sinceStr, "%d", &sinceSeq)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	// First, replay missed events from the buffer
	if sinceSeq > 0 {
		missed := handler.EventsSince(sinceSeq)
		for _, evt := range missed {
			data := strings.ReplaceAll(evt.Data, "\n", "\ndata: ")
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", evt.Seq, evt.Event, data)
		}
		flusher.Flush()
	}

	ctx := r.Context()
	assistantText := &strings.Builder{}

	// Rebuild assistant text from buffered events for persistence
	if sinceSeq > 0 {
		allEvents := handler.EventsSince(0)
		for _, evt := range allEvents {
			if evt.Event == "text" {
				assistantText.WriteString(evt.Data)
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-handler.Events():
			if !ok {
				return
			}
			// Format the SSE data — escape newlines per SSE protocol
			data := strings.ReplaceAll(evt.Data, "\n", "\ndata: ")
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", evt.Seq, evt.Event, data)
			flusher.Flush()

			// Track assistant text
			if evt.Event == "text" {
				assistantText.WriteString(evt.Data)
			}

			// On done, save message + update analytics, then exit
			if evt.Event == "done" {
				if text := assistantText.String(); text != "" {
					sess.AddMessage("assistant", text)
				}
				// Parse usage for analytics
				var usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				}
				if json.Unmarshal([]byte(evt.Data), &usage) == nil {
					sess.mu.Lock()
					sess.TotalInputTokens += usage.InputTokens
					sess.TotalOutputTokens += usage.OutputTokens
					sess.mu.Unlock()
				}
				return
			}

			// On error, also exit
			if evt.Event == "error" {
				if text := assistantText.String(); text != "" {
					sess.AddMessage("assistant", text)
				}
				return
			}
		}
	}
}

// handleChatStatus returns the current streaming status for a project session.
func (s *Server) handleChatStatus(w http.ResponseWriter, r *http.Request) {
	projectPath := r.URL.Query().Get("project")
	if projectPath == "" {
		http.Error(w, "missing project", http.StatusBadRequest)
		return
	}

	sess := s.sessions.Get(projectPath)
	running := false
	eventCount := 0

	if sess != nil {
		handler := sess.CurrentHandler()
		if handler != nil {
			running = handler.IsRunning()
			eventCount = handler.EventCount()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"running":     running,
		"event_count": eventCount,
	})
}

// handleChatReplay returns buffered SSE events since a given sequence number.
func (s *Server) handleChatReplay(w http.ResponseWriter, r *http.Request) {
	projectPath := r.URL.Query().Get("project")
	if projectPath == "" {
		http.Error(w, "missing project", http.StatusBadRequest)
		return
	}

	sinceStr := r.URL.Query().Get("since")
	sinceSeq := 0
	if sinceStr != "" {
		fmt.Sscanf(sinceStr, "%d", &sinceSeq)
	}

	sess := s.sessions.Get(projectPath)
	if sess == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"events": []interface{}{},
		})
		return
	}

	handler := sess.CurrentHandler()
	if handler == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"events": []interface{}{},
		})
		return
	}

	events := handler.EventsSince(sinceSeq)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"events": events,
	})
}

// handleToolApprove approves a pending tool execution.
func (s *Server) handleToolApprove(w http.ResponseWriter, r *http.Request) {
	s.handleApproval(w, r, true)
}

// handleToolDeny denies a pending tool execution.
func (s *Server) handleToolDeny(w http.ResponseWriter, r *http.Request) {
	s.handleApproval(w, r, false)
}

func (s *Server) handleApproval(w http.ResponseWriter, r *http.Request, approved bool) {
	projectPath := r.FormValue("project")
	if projectPath == "" {
		http.Error(w, "missing project", http.StatusBadRequest)
		return
	}

	sess := s.sessions.Get(projectPath)
	if sess == nil {
		http.Error(w, "no active session", http.StatusBadRequest)
		return
	}

	handler := sess.CurrentHandler()
	if handler != nil {
		handler.Approve(approved)
	}
	w.WriteHeader(http.StatusOK)
}

// handlePanel serves panel HTML fragments via HTMX.
func (s *Server) handlePanel(w http.ResponseWriter, r *http.Request) {
	panelName := strings.TrimPrefix(r.URL.Path, "/api/panel/")
	projectPath := r.URL.Query().Get("project")

	sess := s.sessions.Get(projectPath)

	data := templates.PanelData{
		Model:          "claude-sonnet-4-6",
		PermissionMode: "headless",
		ProjectPath:    projectPath,
	}

	// Enrich with session analytics if available
	if sess != nil {
		sess.mu.Lock()
		data.InputTokens = sess.TotalInputTokens
		data.OutputTokens = sess.TotalOutputTokens
		data.TotalTokens = sess.TotalInputTokens + sess.TotalOutputTokens
		data.CacheRead = sess.CacheReadTokens
		data.CacheCreate = sess.CacheCreateTokens
		sess.mu.Unlock()
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	switch panelName {
	case "analytics":
		templates.AnalyticsPanel(data).Render(r.Context(), w)
	case "config":
		templates.ConfigPanel(data).Render(r.Context(), w)
	case "tasks":
		templates.TasksPanel(data).Render(r.Context(), w)
	default:
		fmt.Fprintf(w, `<div style="color:var(--dim);padding:8px">Unknown panel: %s</div>`, panelName)
	}
}

// listKnownProjects finds projects with .claudio directories.
func (s *Server) listKnownProjects() []templates.ProjectInfo {
	var projects []templates.ProjectInfo

	// Include active sessions
	for _, path := range s.sessions.ListProjects() {
		_, hasInit := os.Stat(filepath.Join(path, ".claudio"))
		projects = append(projects, templates.ProjectInfo{
			Path:        path,
			Name:        filepath.Base(path),
			Initialized: hasInit == nil,
		})
	}

	// Scan home directory for .claudio projects (common locations)
	home, _ := os.UserHomeDir()
	scanDirs := []string{
		home,
		filepath.Join(home, "Projects"),
		filepath.Join(home, "projects"),
		filepath.Join(home, "Personal"),
		filepath.Join(home, "Work"),
		filepath.Join(home, "work"),
		filepath.Join(home, "dev"),
		filepath.Join(home, "src"),
		filepath.Join(home, "code"),
		filepath.Join(home, "Code"),
	}

	seen := make(map[string]bool)
	for _, p := range projects {
		seen[p.Path] = true
	}

	for _, dir := range scanDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			full := filepath.Join(dir, entry.Name())
			if seen[full] {
				continue
			}
			claudioDir := filepath.Join(full, ".claudio")
			if _, err := os.Stat(claudioDir); err == nil {
				seen[full] = true
				projects = append(projects, templates.ProjectInfo{
					Path:        full,
					Name:        entry.Name(),
					Initialized: true,
				})
			}
		}
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})

	return projects
}
