package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/services/compact"
	"github.com/Abraxas-365/claudio/internal/services/memory"
	"github.com/Abraxas-365/claudio/internal/teams"
	"github.com/Abraxas-365/claudio/internal/web/templates"
)

// handleStatic serves static files (CSS, JS).
// First checks embedded staticFiles, then falls back to filesystem in internal/web/static/
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/static/")
	
	// Try embedded files first
	content, ok := staticFiles[path]
	if ok {
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
		return
	}
	
	// Fallback to filesystem (for dynamically built files like Tailwind CSS)
	fsPath := filepath.Join("internal/web/static", path)
	data, err := os.ReadFile(fsPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	
	if strings.HasSuffix(path, ".css") {
		w.Header().Set("Content-Type", "text/css")
	} else if strings.HasSuffix(path, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
	}
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	w.Write(data)
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

// handleHome renders the sessions browser.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	// If session already created, redirect straight to chat
	if s.SessionID != "" {
		url := "/chat?project=" + s.ProjectPath + "&session=" + s.SessionID
		http.Redirect(w, r, url, http.StatusSeeOther)
		return
	}

	// Show agent/team picker page
	agentDefs := agents.AllAgents(agents.GetCustomDirs()...)
	agentOptions := make([]templates.AgentOption, len(agentDefs))
	for i, a := range agentDefs {
		agentOptions[i] = templates.AgentOption{
			ID:          a.Type,
			Name:        a.Type,
			Description: a.WhenToUse,
		}
	}

	teamTemplates := s.teams.ListTemplates()
	teamOptions := make([]templates.TeamOption, len(teamTemplates))
	for i, t := range teamTemplates {
		teamOptions[i] = templates.TeamOption{
			ID:          t.Name,
			Name:        t.Name,
			Description: t.Description,
		}
	}

	templates.PickerPage(agentOptions, teamOptions).Render(r.Context(), w)
}

// handleSessions shows all sessions (old home page behavior, now at /sessions)
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	// Collect all sessions across all projects
	var allSessions []templates.SessionInfo
	for _, projectPath := range s.sessions.ListProjects() {
		projectSessions := s.sessions.ListByProject(projectPath)
		for _, sess := range projectSessions {
			allSessions = append(allSessions, templates.SessionInfo{
				ID:       sess.ID,
				Title:    sess.Title,
				State:    string(sess.State),
				MsgCount: sess.MsgCount,
				Active:   false, // no session is "active" on home page
			})
		}
	}
	templates.HomePage(allSessions, s.config.Version).Render(r.Context(), w)
}

// handleChatPage renders the chat page for a project.
// Creates or resumes a session and renders the full multi-session layout.
func (s *Server) handleChatPage(w http.ResponseWriter, r *http.Request) {
	projectPath := r.URL.Query().Get("project")
	if projectPath == "" {
		projectPath = s.ProjectPath
	}
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
	if projectPath == "" {
		projectPath = s.ProjectPath
	}
	title := r.FormValue("title")
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

	// Parse and store optional agent_type and team_template from form
	agentType := r.FormValue("agent_type")
	if agentType != "" {
		sess.AgentType = agentType
	}

	teamTemplate := r.FormValue("team_template")
	if teamTemplate != "" {
		sess.TeamTemplate = teamTemplate
	}

	redirectURL := "/chat?project=" + sess.ProjectPath + "&session=" + sess.ID
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", redirectURL)
		w.WriteHeader(http.StatusOK)
	} else {
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
	}
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

// chatSendRequest is the JSON body for /api/chat/send.
type chatSendRequest struct {
	Session string `json:"session"`
	Message string `json:"message"`
	Images  []struct {
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	} `json:"images,omitempty"`
}

// handleChatSend handles a new user message for a specific session.
func (s *Server) handleChatSend(w http.ResponseWriter, r *http.Request) {
	var req chatSendRequest

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		// Backward compat: form-data
		req.Session = r.FormValue("session")
		req.Message = r.FormValue("message")
	}

	if req.Session == "" || (req.Message == "" && len(req.Images) == 0) {
		http.Error(w, "missing session or message", http.StatusBadRequest)
		return
	}

	sess := s.sessions.Get(req.Session)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	sess.AddMessage("user", req.Message)

	// Convert image attachments to API content blocks
	var imageBlocks []api.UserContentBlock
	for _, img := range req.Images {
		imageBlocks = append(imageBlocks, api.NewImageBlock(img.MediaType, img.Data))
	}

	handler, err := sess.SendMessage(context.Background(), req.Message, imageBlocks)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = handler

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "streaming",
		"session_id": req.Session,
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
		templates.PanelTabs("tasks").Render(r.Context(), w)
		templates.TasksPanel(data).Render(r.Context(), w)
	case "agents":
		templates.PanelTabs("agents").Render(r.Context(), w)
		// Get all available agents
		allAgents := agents.AllAgents()
		agentList := make([]templates.AgentInfo, len(allAgents))
		for i, a := range allAgents {
			// Mock statuses for now
			status := "idle"
			if i%3 == 0 && len(allAgents) > 1 {
				status = "running"
			}
			agentList[i] = templates.AgentInfo{
				ID:     a.Type,
				Name:   a.Type,
				Model:  a.Model,
				Status: status,
			}
		}
		templates.AgentsPanelContent(agentList).Render(r.Context(), w)
	case "tools":
		data.SessionID = sessionID
		data.Tools = collectToolInfos(sess)
		templates.ToolsPanel(data).Render(r.Context(), w)
	default:
		fmt.Fprintf(w, `<div style="color:var(--dim);padding:8px">Unknown panel: %s</div>`, panelName)
	}
}



// collectToolInfos builds the ToolInfo list for the tools panel.
func collectToolInfos(sess *ProjectSession) []templates.ToolInfo {
	if sess == nil || sess.Registry == nil {
		return nil
	}
	reg := sess.Registry
	hints := reg.ToolSearchHints()
	names := reg.Names()
	infos := make([]templates.ToolInfo, 0, len(names))
	for _, name := range names {
		infos = append(infos, templates.ToolInfo{
			Name:       name,
			Hint:       hints[name],
			Deferred:   reg.IsDeferred(name),
			Deferrable: reg.IsDeferrable(name),
			Overridden: reg.HasDeferOverride(name),
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})
	return infos
}

// handleToolDeferToggle toggles or resets a tool's deferral override and
// returns the refreshed tools panel HTML.
func (s *Server) handleToolDeferToggle(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	name := r.URL.Query().Get("name")
	action := r.URL.Query().Get("action")

	sess := s.sessions.Get(sessionID)
	if sess == nil || sess.Registry == nil {
		http.Error(w, "no session", http.StatusBadRequest)
		return
	}
	if name == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}

	reg := sess.Registry
	switch action {
	case "reset":
		reg.ClearDeferOverride(name)
	case "toggle", "":
		if !reg.IsDeferrable(name) {
			http.Error(w, "tool is not deferrable", http.StatusBadRequest)
			return
		}
		// Flip current effective state.
		reg.SetDeferOverride(name, !reg.IsDeferred(name))
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}

	data := templates.PanelData{
		SessionID: sessionID,
		Tools:     collectToolInfos(sess),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	templates.ToolsPanel(data).Render(r.Context(), w)
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

// webCommands is the master list of slash commands available in the web UI.
// It is used by both the autocomplete endpoint and the /discover command handler.
var webCommands = []webCommand{
	{Name: "help", Description: "Show available commands", ClientOnly: true},
	{Name: "clear", Description: "Clear conversation history"},
	{Name: "model", Description: "Show or change the AI model"},
	{Name: "compact", Description: "Compact conversation history (optional: focus prompt)"},
	{Name: "cost", Description: "Show session cost and token usage", ClientOnly: true},
	{Name: "new", Description: "Start a new session"},
	{Name: "rename", Description: "Rename the current session"},
	{Name: "diff", Description: "Show git diff"},
	{Name: "status", Description: "Show git status"},
	{Name: "commit", Description: "Create a git commit with AI message"},
	{Name: "export", Description: "Export conversation as markdown"},
	{Name: "undo", Description: "Undo the last exchange"},
	{Name: "tasks", Description: "Show background tasks", ClientOnly: true},
	{Name: "agent", Description: "Chat with a running agent"},
	{Name: "team", Description: "Spawn or switch to a team"},
	{Name: "analytics", Description: "Show analytics panel", ClientOnly: true},
	{Name: "gain", Description: "Show session token usage stats"},
	{Name: "discover", Description: "Show all available commands"},
}

// handleAutocompleteCommands returns the list of available slash commands.
func (s *Server) handleAutocompleteCommands(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(webCommands)
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

	case "agent":
		if args == "" {
			// No args → open picker modal
			result = cmdResult{
				Status: "ok",
				Action: "open_modal",
				Data:   "/api/picker/agents?session=" + sessionID,
			}
		} else {
			// With args (agent name) → directly address that agent
			result = cmdResult{
				Status:  "ok",
				Action:  "agent_selected",
				Data:    args,
				Message: "Now chatting with " + args,
			}
		}

	case "team":
		result = cmdResult{
			Status: "ok",
			Action: "open_modal",
			Data:   "/api/picker/teams?session=" + sessionID,
		}

	case "compact":
		if sess.engine == nil {
			result = cmdResult{Status: "error", Message: "No active conversation to compact."}
			break
		}
		sess.mu.Lock()
		msgs := sess.engine.Messages()
		if len(msgs) == 0 {
			sess.mu.Unlock()
			result = cmdResult{Status: "ok", Message: "Nothing to compact (no messages)."}
			break
		}
		sess.mu.Unlock()

		compacted, summary, err := compact.Compact(context.Background(), sess.Client, msgs, 10, args)
		if err != nil {
			result = cmdResult{Status: "error", Message: "Usage: /compact [focus prompt]\nCompact failed: " + err.Error()}
			break
		}

		// Convert compacted api.Messages → display ChatMessages for the UI
		chatMsgs := make([]templates.ChatMessage, 0, len(compacted))
		for _, m := range compacted {
			text := apiMessageText(m.Content)
			if text == "" {
				continue
			}
			chatMsgs = append(chatMsgs, templates.ChatMessage{Role: m.Role, Content: text})
		}

		sess.mu.Lock()
		sess.engine.SetMessages(compacted)
		sess.Messages = chatMsgs
		sess.mu.Unlock()

		result = cmdResult{Status: "ok", Action: "clear_messages", Message: summary}

	case "diff":
		gitCmd := exec.Command("git", "diff")
		gitCmd.Dir = sess.ProjectPath
		out, err := gitCmd.Output()
		if err != nil {
			result = cmdResult{Status: "ok", Message: "git diff failed: " + err.Error()}
			break
		}
		if len(strings.TrimSpace(string(out))) == 0 {
			result = cmdResult{Status: "ok", Message: "No changes (working tree is clean)."}
			break
		}
		result = cmdResult{Status: "ok", Message: "```diff\n" + string(out) + "```"}

	case "status":
		gitCmd := exec.Command("git", "status")
		gitCmd.Dir = sess.ProjectPath
		out, err := gitCmd.Output()
		if err != nil {
			result = cmdResult{Status: "ok", Message: "git status failed: " + err.Error()}
			break
		}
		result = cmdResult{Status: "ok", Message: "```\n" + string(out) + "```"}

	case "commit":
		// Check for staged changes first
		stagedCmd := exec.Command("git", "diff", "--staged")
		stagedCmd.Dir = sess.ProjectPath
		stagedOut, err := stagedCmd.Output()
		if err != nil {
			result = cmdResult{Status: "ok", Message: "git diff --staged failed: " + err.Error()}
			break
		}
		if len(strings.TrimSpace(string(stagedOut))) == 0 {
			result = cmdResult{Status: "ok", Message: "Nothing staged. Run `git add` first."}
			break
		}
		// Ask AI for a commit message
		sess.mu.Lock()
		client := sess.Client
		sess.mu.Unlock()
		prompt := "Generate a concise git commit message (conventional commits style) for the following diff. Return only the commit message text, nothing else:\n\n```diff\n" + string(stagedOut) + "\n```"
		userContent, _ := json.Marshal([]api.UserContentBlock{api.NewTextBlock(prompt)})
		aiReq := &api.MessagesRequest{
			MaxTokens: 256,
			Messages: []api.Message{
				{Role: "user", Content: userContent},
			},
		}
		aiResp, err := client.SendMessage(context.Background(), aiReq)
		if err != nil {
			result = cmdResult{Status: "ok", Message: "Failed to generate commit message: " + err.Error()}
			break
		}
		commitMsg := ""
		for _, block := range aiResp.Content {
			if block.Type == "text" {
				commitMsg = strings.TrimSpace(block.Text)
				break
			}
		}
		if commitMsg == "" {
			result = cmdResult{Status: "ok", Message: "AI returned an empty commit message. Aborting."}
			break
		}
		// Run git commit
		commitCmd := exec.Command("git", "commit", "-m", commitMsg)
		commitCmd.Dir = sess.ProjectPath
		commitOut, err := commitCmd.CombinedOutput()
		if err != nil {
			result = cmdResult{Status: "ok", Message: fmt.Sprintf("git commit failed: %v\n```\n%s\n```", err, string(commitOut))}
			break
		}
		result = cmdResult{Status: "ok", Message: fmt.Sprintf("Committed: **%s**\n\n```\n%s\n```", commitMsg, strings.TrimSpace(string(commitOut)))}

	case "gain":
		sess.mu.Lock()
		in := sess.TotalInputTokens
		out := sess.TotalOutputTokens
		n := len(sess.Messages)
		sess.mu.Unlock()
		msg := fmt.Sprintf("Session Token Usage\n%s\nMessages:       %d\nInput tokens:   %d\nOutput tokens:  %d\nTotal tokens:   %d",
			strings.Repeat("─", 38), n, in, out, in+out)
		result = cmdResult{Status: "ok", Message: msg}

	case "discover":
		var sb strings.Builder
		sb.WriteString("## Available Commands\n\n")
		for _, cmd := range webCommands {
			marker := ""
			if cmd.ClientOnly {
				marker = " *(client-side)*"
			}
			sb.WriteString(fmt.Sprintf("- **/%s** — %s%s\n", cmd.Name, cmd.Description, marker))
		}
		result = cmdResult{Status: "ok", Message: sb.String()}

	default:
		// Check if this is a skill command
		if s.skills != nil {
			if skill, ok := s.skills.Get(command); ok {
				result = cmdResult{Status: "ok", Action: "send_as_message", Data: skill.Content}
				break
			}
		}
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

// ── Nav Sidebar API ──

// NavAgentItem describes an agent for the nav sidebar.
type NavAgentItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	TaskCount int    `json:"task_count"`
}

// handleNavAgents returns the list of available agents.
func (s *Server) handleNavAgents(w http.ResponseWriter, r *http.Request) {
	projectPath := r.URL.Query().Get("project")
	if projectPath == "" {
		http.Error(w, "missing project", http.StatusBadRequest)
		return
	}

	allAgents := agents.AllAgents()
	items := make([]NavAgentItem, 0, len(allAgents))
	for _, a := range allAgents {
		items = append(items, NavAgentItem{
			ID:        a.Type,
			Name:      a.Type,
			Status:    "idle",
			TaskCount: 0,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// NavTeamItem describes a team for the nav sidebar.
type NavTeamItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
	Status      string `json:"status"`
}

// handleNavTeams returns the list of active teams.
func (s *Server) handleNavTeams(w http.ResponseWriter, r *http.Request) {
	projectPath := r.URL.Query().Get("project")
	if projectPath == "" {
		http.Error(w, "missing project", http.StatusBadRequest)
		return
	}

	allTeams := s.teams.ListTeams()
	items := make([]NavTeamItem, 0, len(allTeams))
	for _, t := range allTeams {
		status := "idle"
		// Check if any team member is working
		for _, member := range t.Members {
			if member.Status == teams.StatusWorking {
				status = "active"
				break
			}
		}

		items = append(items, NavTeamItem{
			ID:          t.Name,
			Name:        t.Name,
			MemberCount: len(t.Members),
			Status:      status,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// ── Picker API (for /agent and /team commands) ──

// handlePickerAgents renders an HTML partial with a list of active agents.
func (s *Server) handlePickerAgents(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(w, "missing session", http.StatusBadRequest)
		return
	}

	// Get all available agent definitions
	agentDefs := agents.AllAgents(agents.GetCustomDirs()...)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<div class="picker-list">`)
	fmt.Fprintf(w, `<div class="picker-header">Available Agents</div>`)

	for _, agent := range agentDefs {
		// Escape agent name for htmx values
		fmt.Fprintf(w, `<button class="picker-item" hx-post="/api/picker/select-agent" hx-vals='{"name":"%s","session":"%s"}' hx-target="#picker-modal-content" hx-swap="innerHTML" onclick="void(0)">`, 
			escapeHTMLAttr(agent.Type), escapeHTMLAttr(sessionID))
		fmt.Fprintf(w, `<span class="picker-name">%s</span>`, escapeHTML(agent.Type))
		fmt.Fprintf(w, `<span class="picker-desc">%s</span>`, escapeHTML(agent.WhenToUse))
		fmt.Fprintf(w, `<span class="picker-status-dot picker-status-idle"></span>`)
		fmt.Fprintf(w, `</button>`)
	}

	fmt.Fprintf(w, `</div>`)
}

// handlePickerTeams renders an HTML partial with a list of team templates.
func (s *Server) handlePickerTeams(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(w, "missing session", http.StatusBadRequest)
		return
	}

	// Get all team templates
	templates := s.teams.ListTemplates()

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<div class="picker-list">`)
	fmt.Fprintf(w, `<div class="picker-header">Team Templates</div>`)

	for _, tmpl := range templates {
		// Escape template name for htmx values
		fmt.Fprintf(w, `<button class="picker-item" hx-post="/api/picker/spawn-team" hx-vals='{"template":"%s","session":"%s"}' hx-target="#picker-modal-content" hx-swap="innerHTML">`,
			escapeHTMLAttr(tmpl.Name), escapeHTMLAttr(sessionID))
		fmt.Fprintf(w, `<span class="picker-name">%s</span>`, escapeHTML(tmpl.Name))
		if tmpl.Description != "" {
			fmt.Fprintf(w, `<span class="picker-desc">%s</span>`, escapeHTML(tmpl.Description))
		}
		fmt.Fprintf(w, `<span class="picker-count">%d members</span>`, len(tmpl.Members))
		fmt.Fprintf(w, `</button>`)
	}

	fmt.Fprintf(w, `</div>`)
}

// handlePickerSelectAgent handles selecting an agent from the picker.
func (s *Server) handlePickerSelectAgent(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	agentName := r.FormValue("name")
	sessionID := r.FormValue("session")

	if agentName == "" || sessionID == "" {
		http.Error(w, "missing name or session", http.StatusBadRequest)
		return
	}

	// Validate agent exists
	agent := agents.GetAgent(agentName)
	if agent.Type != agentName {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	type pickerResult struct {
		Status  string `json:"status"`
		Action  string `json:"action"`
		Data    string `json:"data"`
		Message string `json:"message"`
	}

	result := pickerResult{
		Status:  "ok",
		Action:  "agent_selected",
		Data:    agentName,
		Message: "Now chatting with " + agentName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handlePickerSpawnTeam handles spawning a team from the picker.
func (s *Server) handlePickerSpawnTeam(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	templateName := r.FormValue("template")
	sessionID := r.FormValue("session")

	if templateName == "" || sessionID == "" {
		http.Error(w, "missing template or session", http.StatusBadRequest)
		return
	}

	// Get the template to validate it exists
	tmpl, err := s.teams.GetTemplate(templateName)
	if err != nil {
		http.Error(w, "template not found", http.StatusNotFound)
		return
	}

	// Generate unique team name from template name and session ID
	teamName := templateName
	if len(sessionID) >= 8 {
		teamName = templateName + "-" + sessionID[:8]
	}

	// Create the team and pre-register members
	if _, err := s.teams.CreateTeam(teamName, tmpl.Description, sessionID, tmpl.Model); err != nil {
		// Team may already exist, proceed anyway
		_ = err
	}

	if tmpl.AutoCompactThreshold > 0 {
		s.teams.SetAutoCompactThreshold(teamName, tmpl.AutoCompactThreshold)
	}

	// Pre-register members from template
	for _, m := range tmpl.Members {
		model := m.Model
		if model == "" {
			model = tmpl.Model
		}
		_, _ = s.teams.AddMember(teamName, m.Name, model, "", m.SubagentType, m.AutoCompactThreshold)
		if m.Advisor != nil {
			s.teams.SetMemberAdvisorConfig(teamName, m.Name, m.Advisor)
		}
	}

	type pickerResult struct {
		Status  string `json:"status"`
		Action  string `json:"action"`
		Message string `json:"message"`
	}

	result := pickerResult{
		Status:  "ok",
		Action:  "team_spawned",
		Message: "Team '" + teamName + "' spawned",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleAgentsList renders the agents panel with live agent list.
func (s *Server) handleAgentsList(w http.ResponseWriter, r *http.Request) {
	// Get all available agents
	allAgents := agents.AllAgents()
	
	// Build agent list with mock status data
	// In production, this would read from s.teams.Runner() or similar
	agentList := make([]templates.AgentInfo, len(allAgents))
	for i, a := range allAgents {
		// Mock statuses for now - in a real implementation,
		// these would come from TeammateRunner state tracking
		status := "idle"
		if i%3 == 0 && len(allAgents) > 1 {
			status = "running"
		}
		
		agentList[i] = templates.AgentInfo{
			ID:     a.Type,
			Name:   a.Type,
			Model:  a.Model,
			Status: status,
		}
	}
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Render just the inner HTML (without outer panel-section wrapper)
	// to work with hx-swap="innerHTML"
	if len(agentList) == 0 {
		fmt.Fprintf(w, `<div style="color:var(--fg4);padding:8px 0;font-size:var(--font-size-xs);">No agents running</div>`)
	} else {
		for _, a := range agentList {
			fmt.Fprintf(w, `<div class="agent-card">`)
			fmt.Fprintf(w, `<div class="agent-card-header">`)
			fmt.Fprintf(w, `<span class="agent-name">%s</span>`, escapeHTML(a.Name))
			fmt.Fprintf(w, `<span class="agent-badge agent-badge-%s">%s</span>`, escapeHTML(a.Status), escapeHTML(a.Status))
			fmt.Fprintf(w, `</div>`)
			fmt.Fprintf(w, `<div class="agent-model">%s</div>`, escapeHTML(a.Model))
			fmt.Fprintf(w, `</div>`)
		}
	}
}

// handleAgentsStream is the SSE endpoint for real-time agent status updates.
func (s *Server) handleAgentsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	ctx := r.Context()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Get all available agents
			allAgents := agents.AllAgents()

			// Build agent list with mock status data
			agentList := make([]templates.AgentInfo, len(allAgents))
			for i, a := range allAgents {
				// Mock statuses for now - in a real implementation,
				// these would come from TeammateRunner state tracking
				status := "idle"
				if i%3 == 0 && len(allAgents) > 1 {
					status = "running"
				}

				agentList[i] = templates.AgentInfo{
					ID:     a.Type,
					Name:   a.Type,
					Model:  a.Model,
					Status: status,
				}
			}

			// Render the agents list HTML and send as SSE event
			var html strings.Builder
			if len(agentList) == 0 {
				html.WriteString(`<div style="color:var(--fg4);padding:8px 0;font-size:var(--font-size-xs);">No agents running</div>`)
			} else {
				for _, a := range agentList {
					html.WriteString(`<div class="agent-card">`)
					html.WriteString(`<div class="agent-card-header">`)
					html.WriteString(fmt.Sprintf(`<span class="agent-name">%s</span>`, escapeHTML(a.Name)))
					html.WriteString(fmt.Sprintf(`<span class="agent-badge agent-badge-%s">%s</span>`, escapeHTML(a.Status), escapeHTML(a.Status)))
					html.WriteString(`</div>`)
					html.WriteString(fmt.Sprintf(`<div class="agent-model">%s</div>`, escapeHTML(a.Model)))
					html.WriteString(`</div>`)
				}
			}

			// Send SSE event with HTML content
			data := html.String()
			data = strings.ReplaceAll(data, "\n", "\ndata: ")
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// escapeHTML escapes HTML special characters.
func escapeHTML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	).Replace(s)
}

// escapeHTMLAttr escapes HTML attribute values (also escapes single quotes for use in single-quoted attributes).
func escapeHTMLAttr(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	).Replace(s)
}

// ── Info Pages ──

// handleTools renders the tools catalog page.
func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	
	// Build tool catalog from all sessions' registries (use first available)
	var toolCatalog []templates.ToolCatalogInfo
	for _, projectPath := range s.sessions.ListProjects() {
		sessions := s.sessions.ListByProject(projectPath)
		if len(sessions) > 0 {
			sess := s.sessions.Get(sessions[0].ID)
			if sess != nil && sess.Registry != nil {
				for _, toolName := range sess.Registry.Names() {
					t, err := sess.Registry.Get(toolName)
					if err != nil {
						continue
					}
					// Determine category: deferred or core
					category := "core"
					if sess.Registry.IsDeferred(toolName) {
						category = "deferred"
					}
					toolCatalog = append(toolCatalog, templates.ToolCatalogInfo{
						Name:        toolName,
						Description: t.Description(),
						Category:    category,
					})
				}
				break
			}
		}
	}
	
	// Filter by search query
	if query != "" {
		var filtered []templates.ToolCatalogInfo
		lowerQ := strings.ToLower(query)
		for _, tool := range toolCatalog {
			if strings.Contains(strings.ToLower(tool.Name), lowerQ) ||
				strings.Contains(strings.ToLower(tool.Description), lowerQ) {
				filtered = append(filtered, tool)
			}
		}
		toolCatalog = filtered
	}
	
	// If HTMX request, render just the list partial
	if r.Header.Get("HX-Request") == "true" {
		templates.ToolsList(toolCatalog).Render(r.Context(), w)
		return
	}
	
	// Full page render
	templates.ToolsPage(toolCatalog, query).Render(r.Context(), w)
}

// handleMemory renders the memory browser page.
func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	var entries []templates.MemoryEntryInfo
	
	// Try to load memories from the default location
	paths := config.GetPaths()
	memStore := memory.NewStore(paths.Memory)
	if memStore != nil {
		memories := memStore.LoadAll()
		for _, m := range memories {
			// Truncate value to 120 chars for preview
			value := m.Content
			if len(value) > 120 {
				value = value[:120] + "..."
			}
			// Determine scope (default to global, could parse from Type field)
			scope := "global"
			if m.Type == "user" {
				scope = "session"
			}
			entries = append(entries, templates.MemoryEntryInfo{
				Key:   m.Name,
				Value: value,
				Scope: scope,
			})
		}
	}
	
	templates.MemoryPage(entries).Render(r.Context(), w)
}

// handleConfig renders the config display page.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	var sections []templates.ConfigDisplaySection
	
	// Load settings from a project, or use defaults
	var globalSettings *config.Settings
	projects := s.sessions.ListProjects()
	if len(projects) > 0 {
		loaded, err := config.Load(projects[0])
		if err == nil {
			globalSettings = loaded
		}
	}
	if globalSettings == nil {
		globalSettings = config.DefaultSettings()
	}
	
	// Model section
	modelItems := []templates.ConfigItem{
		{Key: "model", Value: globalSettings.Model},
		{Key: "smallModel", Value: globalSettings.SmallModel},
		{Key: "thinkingMode", Value: globalSettings.ThinkingMode},
	}
	sections = append(sections, templates.ConfigDisplaySection{
		Name:  "Model",
		Items: modelItems,
	})
	
	// Permissions section
	permItems := []templates.ConfigItem{
		{Key: "permissionMode", Value: globalSettings.PermissionMode},
		{Key: "autoCompact", Value: fmt.Sprintf("%v", globalSettings.AutoCompact)},
		{Key: "autoMemoryExtract", Value: fmt.Sprintf("%v", globalSettings.AutoMemoryExtract)},
	}
	sections = append(sections, templates.ConfigDisplaySection{
		Name:  "Permissions",
		Items: permItems,
	})
	
	// Storage section (display paths safely, no secrets)
	paths := config.GetPaths()
	storageItems := []templates.ConfigItem{
		{Key: "home", Value: paths.Home},
		{Key: "settings", Value: paths.Settings},
		{Key: "db", Value: paths.DB},
	}
	sections = append(sections, templates.ConfigDisplaySection{
		Name:  "Storage",
		Items: storageItems,
	})
	
	templates.ConfigPage(sections).Render(r.Context(), w)
}

// apiMessageText extracts plain text from an api.Message Content field.
// Content is a json.RawMessage that is either a JSON string or an array of
// content blocks (e.g. [{"type":"text","text":"..."}]).
func apiMessageText(content json.RawMessage) string {
	// Try plain string first
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return s
	}
	// Try array of content blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &blocks); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}
