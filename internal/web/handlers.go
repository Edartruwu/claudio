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

	"github.com/Abraxas-365/claudio/internal/agents"
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
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	w.Header().Set("ETag", fmt.Sprintf(`"%x"`, len(content)))
	if match := r.Header.Get("If-None-Match"); match == fmt.Sprintf(`"%x"`, len(content)) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
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
		SameSite: http.SameSiteLaxMode,
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
// Creates or resumes a session and renders the full multi-session layout.
func (s *Server) handleChatPage(w http.ResponseWriter, r *http.Request) {
	projectPath := r.URL.Query().Get("project")
	if projectPath == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Determine which session to show
	sessionID := r.URL.Query().Get("session")
	var sess *ProjectSession
	var err error

	if sessionID != "" {
		sess = s.sessions.Get(sessionID)
	}
	if sess == nil {
		sess, err = s.sessions.GetOrCreateDefault(projectPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Build session list for sidebar
	sessionInfos := s.sessions.ListByProject(projectPath)
	tplSessions := make([]templates.SessionInfo, len(sessionInfos))
	for i, si := range sessionInfos {
		tplSessions[i] = templates.SessionInfo{
			ID:       si.ID,
			Title:    si.Title,
			State:    string(si.State),
			MsgCount: si.MsgCount,
			Active:   si.ID == sess.ID,
		}
	}

	sess.mu.Lock()
	messages := make([]templates.ChatMessage, len(sess.Messages))
	copy(messages, sess.Messages)
	sess.mu.Unlock()

	templates.ChatPage(projectPath, sess.ID, tplSessions, messages).Render(r.Context(), w)
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

// ── Session API ──

// handleSessionCreate creates a new session for a project.
func (s *Server) handleSessionCreate(w http.ResponseWriter, r *http.Request) {
	projectPath := r.FormValue("project")
	title := r.FormValue("title")
	if projectPath == "" {
		http.Error(w, "missing project", http.StatusBadRequest)
		return
	}
	if title == "" {
		// Auto-generate title
		existing := s.sessions.ListByProject(projectPath)
		title = fmt.Sprintf("Session %d", len(existing)+1)
	}

	sess, err := s.sessions.Create(projectPath, title)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sess.Info())
}

// handleSessionList returns all sessions for a project.
func (s *Server) handleSessionList(w http.ResponseWriter, r *http.Request) {
	projectPath := r.URL.Query().Get("project")
	if projectPath == "" {
		http.Error(w, "missing project", http.StatusBadRequest)
		return
	}
	infos := s.sessions.ListByProject(projectPath)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(infos)
}

// handleSessionDelete deletes a session.
func (s *Server) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	sessionID := r.FormValue("session")
	if sessionID == "" {
		http.Error(w, "missing session", http.StatusBadRequest)
		return
	}
	s.sessions.Delete(sessionID)
	w.WriteHeader(http.StatusOK)
}

// handleSessionRename renames a session.
func (s *Server) handleSessionRename(w http.ResponseWriter, r *http.Request) {
	sessionID := r.FormValue("session")
	title := r.FormValue("title")
	if sessionID == "" || title == "" {
		http.Error(w, "missing session or title", http.StatusBadRequest)
		return
	}
	sess := s.sessions.Get(sessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	sess.Rename(title)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sess.Info())
}

// handleSessionHistory returns the full message history for a session.
func (s *Server) handleSessionHistory(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(w, "missing session", http.StatusBadRequest)
		return
	}
	sess := s.sessions.Get(sessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	sess.mu.Lock()
	messages := make([]templates.ChatMessage, len(sess.Messages))
	copy(messages, sess.Messages)
	sess.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// ── Chat API (session-aware) ──

// handleChatSend handles a new user message for a specific session.
func (s *Server) handleChatSend(w http.ResponseWriter, r *http.Request) {
	sessionID := r.FormValue("session")
	message := r.FormValue("message")
	if sessionID == "" || message == "" {
		http.Error(w, "missing session or message", http.StatusBadRequest)
		return
	}

	sess := s.sessions.Get(sessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	sess.AddMessage("user", message)

	handler, err := sess.SendMessage(context.Background(), message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = handler

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "streaming",
		"session_id": sessionID,
	})
}

// handleChatStream is the SSE endpoint — streams events from the current handler for a session.
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(w, "missing session", http.StatusBadRequest)
		return
	}

	sess := s.sessions.Get(sessionID)
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

	// Replay missed events from the buffer
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
			data := strings.ReplaceAll(evt.Data, "\n", "\ndata: ")
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", evt.Seq, evt.Event, data)
			flusher.Flush()

			if evt.Event == "text" {
				assistantText.WriteString(evt.Data)
			}

			if evt.Event == "done" {
				if text := assistantText.String(); text != "" {
					sess.AddMessage("assistant", text)
				}
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

			if evt.Event == "error" {
				if text := assistantText.String(); text != "" {
					sess.AddMessage("assistant", text)
				}
				return
			}
		}
	}
}

// handleChatStatus returns the current streaming status for a session.
func (s *Server) handleChatStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(w, "missing session", http.StatusBadRequest)
		return
	}

	sess := s.sessions.Get(sessionID)
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

// handleChatReplay returns buffered SSE events since a given sequence number for a session.
func (s *Server) handleChatReplay(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(w, "missing session", http.StatusBadRequest)
		return
	}

	sinceStr := r.URL.Query().Get("since")
	sinceSeq := 0
	if sinceStr != "" {
		fmt.Sscanf(sinceStr, "%d", &sinceSeq)
	}

	sess := s.sessions.Get(sessionID)
	if sess == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"events": []interface{}{}})
		return
	}

	handler := sess.CurrentHandler()
	if handler == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"events": []interface{}{}})
		return
	}

	events := handler.EventsSince(sinceSeq)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"events": events})
}

// handleToolApprove approves a pending tool execution for a session.
func (s *Server) handleToolApprove(w http.ResponseWriter, r *http.Request) {
	s.handleApproval(w, r, true)
}

// handleToolDeny denies a pending tool execution for a session.
func (s *Server) handleToolDeny(w http.ResponseWriter, r *http.Request) {
	s.handleApproval(w, r, false)
}

func (s *Server) handleApproval(w http.ResponseWriter, r *http.Request, approved bool) {
	sessionID := r.FormValue("session")
	if sessionID == "" {
		http.Error(w, "missing session", http.StatusBadRequest)
		return
	}

	sess := s.sessions.Get(sessionID)
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
	sessionID := r.URL.Query().Get("session")

	sess := s.sessions.Get(sessionID)

	data := templates.PanelData{
		Model:          "claude-sonnet-4-6",
		PermissionMode: "headless",
	}

	if sess != nil {
		data.ProjectPath = sess.ProjectPath
		data.Model = sess.Client.GetModel()
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
	case "agents":
		templates.AgentsPanelContent().Render(r.Context(), w)
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

// ── Autocomplete API ──

// skipDirs are directories to skip during file scanning.
var autocompleteSkipDirs = map[string]bool{
	"node_modules": true, "vendor": true, "__pycache__": true,
	"dist": true, "build": true, ".git": true, ".claudio": true,
}

// handleAutocompleteFiles returns file/dir suggestions for the @ file picker.
func (s *Server) handleAutocompleteFiles(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	projectPath := r.URL.Query().Get("project")
	if projectPath == "" {
		http.Error(w, "missing project", http.StatusBadRequest)
		return
	}

	// Split query into directory prefix and fuzzy filter
	scanDir, fuzzy := splitAutocompletePath(query)

	// Resolve scan directory relative to project
	var scanAbs string
	var displayPrefix string
	if strings.HasPrefix(scanDir, "~") {
		home, _ := os.UserHomeDir()
		scanAbs = filepath.Clean(strings.Replace(scanDir, "~", home, 1))
		displayPrefix = scanDir
		if !strings.HasSuffix(displayPrefix, "/") {
			displayPrefix += "/"
		}
	} else {
		scanAbs = filepath.Clean(filepath.Join(projectPath, scanDir))
		if scanDir != "." {
			displayPrefix = filepath.Clean(scanDir) + "/"
		}
	}

	info, err := os.Stat(scanAbs)
	if err != nil || !info.IsDir() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	showHidden := strings.HasPrefix(fuzzy, ".")
	fq := strings.ToLower(fuzzy)

	entries, err := os.ReadDir(scanAbs)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	type fileItem struct {
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
	}
	var items []fileItem

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && !showHidden {
			continue
		}
		if entry.IsDir() && autocompleteSkipDirs[name] {
			continue
		}
		if fq != "" && !fuzzyMatch(strings.ToLower(name), fq) {
			continue
		}
		items = append(items, fileItem{
			Path:  displayPrefix + name,
			IsDir: entry.IsDir(),
		})
	}

	// Dirs first, then alphabetical
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return items[i].Path < items[j].Path
	})

	// Limit results
	if len(items) > 50 {
		items = items[:50]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func splitAutocompletePath(query string) (dir, fuzzy string) {
	if query == "" {
		return ".", ""
	}
	if strings.HasSuffix(query, "/") {
		return filepath.Clean(query), ""
	}
	lastSep := strings.LastIndexByte(query, '/')
	if lastSep < 0 {
		return ".", query
	}
	return filepath.Clean(query[:lastSep+1]), query[lastSep+1:]
}

func fuzzyMatch(str, pattern string) bool {
	pi := 0
	for si := 0; si < len(str) && pi < len(pattern); si++ {
		if str[si] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

// webCommand describes a slash command available in the web UI.
type webCommand struct {
	Name        string `json:"name"`
	Description string `json:"desc"`
	ClientOnly  bool   `json:"client_only,omitempty"` // handled purely in JS
}

// handleAutocompleteCommands returns the list of available slash commands.
func (s *Server) handleAutocompleteCommands(w http.ResponseWriter, _ *http.Request) {
	cmds := []webCommand{
		{Name: "help", Description: "Show available commands", ClientOnly: true},
		{Name: "clear", Description: "Clear conversation history"},
		{Name: "model", Description: "Show or change the AI model"},
		{Name: "compact", Description: "Compact conversation to save context"},
		{Name: "cost", Description: "Show session cost and token usage", ClientOnly: true},
		{Name: "new", Description: "Start a new session"},
		{Name: "rename", Description: "Rename the current session"},
		{Name: "diff", Description: "Show git diff"},
		{Name: "status", Description: "Show git status"},
		{Name: "commit", Description: "Create a git commit with AI message"},
		{Name: "export", Description: "Export conversation as markdown"},
		{Name: "undo", Description: "Undo the last exchange"},
		{Name: "tasks", Description: "Show background tasks", ClientOnly: true},
		{Name: "agents", Description: "Show agents panel", ClientOnly: true},
		{Name: "analytics", Description: "Show analytics panel", ClientOnly: true},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cmds)
}

// handleAutocompleteAgents returns the list of available agents for >> communication.
func (s *Server) handleAutocompleteAgents(w http.ResponseWriter, _ *http.Request) {
	type agentInfo struct {
		Name string `json:"name"`
		Desc string `json:"desc"`
	}

	allAgents := agents.AllAgents()
	items := make([]agentInfo, 0, len(allAgents))
	for _, a := range allAgents {
		items = append(items, agentInfo{
			Name: a.Type,
			Desc: a.WhenToUse,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// handleCommandExecute executes a slash command server-side and returns the result.
func (s *Server) handleCommandExecute(w http.ResponseWriter, r *http.Request) {
	sessionID := r.FormValue("session")
	command := r.FormValue("command")
	args := r.FormValue("args")

	sess := s.sessions.Get(sessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	type cmdResult struct {
		Status  string `json:"status"` // "ok", "redirect", "error"
		Message string `json:"message,omitempty"`
		Action  string `json:"action,omitempty"`  // client-side action hint
		Data    string `json:"data,omitempty"`
	}

	var result cmdResult

	switch command {
	case "clear":
		sess.mu.Lock()
		sess.Messages = sess.Messages[:0]
		if sess.engine != nil {
			sess.engine = nil // reset engine to clear conversation context
		}
		sess.mu.Unlock()
		result = cmdResult{Status: "ok", Action: "clear_messages", Message: "Conversation cleared."}

	case "new":
		newSess, err := s.sessions.Create(sess.ProjectPath, "")
		if err != nil {
			result = cmdResult{Status: "error", Message: err.Error()}
		} else {
			result = cmdResult{Status: "redirect", Data: "/chat?project=" + sess.ProjectPath + "&session=" + newSess.ID}
		}

	case "rename":
		if args == "" {
			result = cmdResult{Status: "error", Message: "Usage: /rename <new title>"}
		} else {
			sess.Rename(args)
			result = cmdResult{Status: "ok", Action: "rename", Data: args, Message: "Session renamed to: " + args}
		}

	case "model":
		if args == "" {
			sess.mu.Lock()
			model := "unknown"
			if sess.Client != nil {
				model = sess.Client.GetModel()
			}
			sess.mu.Unlock()
			result = cmdResult{Status: "ok", Message: "Current model: " + model}
		} else {
			sess.mu.Lock()
			if sess.Client != nil {
				sess.Client.SetModel(args)
			}
			sess.mu.Unlock()
			result = cmdResult{Status: "ok", Message: "Model set to: " + args}
		}

	case "cost":
		sess.mu.Lock()
		in := sess.TotalInputTokens
		out := sess.TotalOutputTokens
		sess.mu.Unlock()
		msg := fmt.Sprintf("Session usage:\n  Input tokens:  %d\n  Output tokens: %d\n  Total:         %d", in, out, in+out)
		result = cmdResult{Status: "ok", Message: msg}

	case "undo":
		sess.mu.Lock()
		n := len(sess.Messages)
		if n >= 2 && sess.Messages[n-1].Role == "assistant" && sess.Messages[n-2].Role == "user" {
			sess.Messages = sess.Messages[:n-2]
			result = cmdResult{Status: "ok", Action: "undo", Message: "Last exchange removed."}
		} else if n >= 1 {
			sess.Messages = sess.Messages[:n-1]
			result = cmdResult{Status: "ok", Action: "undo", Message: "Last message removed."}
		} else {
			result = cmdResult{Status: "ok", Message: "Nothing to undo."}
		}
		sess.mu.Unlock()

	case "export":
		sess.mu.Lock()
		var sb strings.Builder
		for _, msg := range sess.Messages {
			if msg.Role == "user" {
				sb.WriteString("## User\n\n")
			} else {
				sb.WriteString("## Assistant\n\n")
			}
			sb.WriteString(msg.Content)
			sb.WriteString("\n\n---\n\n")
		}
		sess.mu.Unlock()
		result = cmdResult{Status: "ok", Action: "export", Data: sb.String(), Message: "Conversation exported."}

	default:
		// For commands we don't handle server-side, send as a regular message to the AI
		// prefixed with the command so the AI can interpret it
		result = cmdResult{Status: "ok", Action: "send_as_message", Data: "/" + command + " " + args}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ── Model API ──

// handleGetModel returns the current model for a session.
func (s *Server) handleGetModel(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	sess := s.sessions.Get(sessionID)
	model := "claude-sonnet-4-6"
	if sess != nil {
		model = sess.Client.GetModel()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"model": model})
}

// handleSetModel changes the model for a session.
func (s *Server) handleSetModel(w http.ResponseWriter, r *http.Request) {
	sessionID := r.FormValue("session")
	model := r.FormValue("model")
	if sessionID == "" || model == "" {
		http.Error(w, "missing session or model", http.StatusBadRequest)
		return
	}
	sess := s.sessions.Get(sessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	sess.mu.Lock()
	sess.Client.SetModel(model)
	sess.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"model": model, "status": "ok"})
}

// handleListModels returns the list of available models.
func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	models := []map[string]string{
		{"id": "claude-opus-4-6", "label": "Claude Opus 4.6", "description": "Most capable, best for complex tasks"},
		{"id": "claude-sonnet-4-6", "label": "Claude Sonnet 4.6", "description": "Fast and intelligent, great balance"},
		{"id": "claude-haiku-4-5-20251001", "label": "Claude Haiku 4.5", "description": "Fastest, most compact"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models)
}

// ── Config update API ──

// handleConfigUpdate updates a setting for a session.
func (s *Server) handleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	sessionID := r.FormValue("session")
	key := r.FormValue("key")
	value := r.FormValue("value")
	if sessionID == "" || key == "" {
		http.Error(w, "missing session or key", http.StatusBadRequest)
		return
	}
	sess := s.sessions.Get(sessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	switch key {
	case "model":
		sess.mu.Lock()
		sess.Client.SetModel(value)
		sess.mu.Unlock()
	default:
		http.Error(w, "unknown key: "+key, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
