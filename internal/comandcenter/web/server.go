// Package web provides the browser UI for ComandCenter.
package web

import (
	"bytes"
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	cc "github.com/Abraxas-365/claudio/internal/comandcenter"
	agentspkg "github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/services/compact"
	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/Abraxas-365/claudio/internal/teams"
	"github.com/a-h/templ"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"golang.org/x/net/websocket"
)

// mentionRe matches "@Name message" at the start of content.
// Group 1 = session name, Group 2 = message body.
var mentionRe = regexp.MustCompile(`^@(\w[\w\s]*?)\s+(.+)$`)

var modelAliases = map[string]string{
	"haiku":         "claude-haiku-4-5-20251001",
	"sonnet":        "claude-sonnet-4-6",
	"opus":          "claude-opus-4-6",
	"claude-haiku":  "claude-haiku-4-5-20251001",
	"claude-sonnet": "claude-sonnet-4-6",
	"claude-opus":   "claude-opus-4-6",
}

//go:embed static
var staticFS embed.FS

// renderMarkdown converts markdown to sanitized HTML safe for template.HTML use.
var mdParser = goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.TaskList))

var mdPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("target").Matching(regexp.MustCompile(`^_blank$`)).OnElements("a")
	return p
}()

// renderMarkdown converts markdown to sanitized HTML. Returns a raw HTML string;
// templ callers should wrap with templ.Raw() to emit without escaping.
func renderMarkdown(s string) string {
	var buf bytes.Buffer
	if err := mdParser.Convert([]byte(s), &buf); err != nil {
		return html.EscapeString(s)
	}
	return string(mdPolicy.SanitizeBytes(buf.Bytes()))
}

// uiClient is a browser WebSocket connection watching a session.
type uiClient struct {
	sessionID string
	send      chan []byte
}

// MessageView wraps a cc.Message with its associated attachments for template rendering.
type MessageView struct {
	cc.Message
	Attachments []cc.Attachment
}

// WebServer serves the browser UI for ComandCenter.
type WebServer struct {
	storage          *cc.Storage
	hub              *cc.Hub
	password         string
	uploadsDir       string
	vapidPublicKey   string
	cronStore        *tasks.CronStore
	apiClient        *api.Client
	teamTemplatesDir string
	publicURL        string

	mu      sync.RWMutex
	clients map[*uiClient]struct{}
}

// NewWebServer creates a WebServer. uploadsDir is the base directory for uploaded files.
func NewWebServer(storage *cc.Storage, hub *cc.Hub, password, uploadsDir string) *WebServer {
	ws := &WebServer{
		storage:  storage,
		hub:      hub,
		password: password,
		clients:    make(map[*uiClient]struct{}),
		uploadsDir: uploadsDir,
	}
	go ws.fanout()
	return ws
}

// RegisterRoutes mounts all UI routes on mux.
func (ws *WebServer) RegisterRoutes(mux *http.ServeMux) {
	// Static files — sub-FS so URL /static/foo maps to embed path static/foo.
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("web: static sub-FS: " + err.Error())
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	// Service worker — must be served at root scope, no auth.
	mux.HandleFunc("GET /sw.js", ws.handleServiceWorker)

	// No-auth routes.
	mux.HandleFunc("GET /login", ws.handleLoginGet)
	mux.HandleFunc("POST /login", ws.handleLoginPost)

	// Auth-gated routes.
	mux.Handle("GET /", ws.uiAuth(http.HandlerFunc(ws.handleChatList)))
	mux.Handle("GET /chat/{session_id}", ws.uiAuth(http.HandlerFunc(ws.handleChatView)))
	mux.Handle("GET /chat/{session_id}/info", ws.uiAuth(http.HandlerFunc(ws.handleSessionInfo)))
	mux.Handle("GET /chat/{session_id}/tasks", ws.uiAuth(http.HandlerFunc(ws.handleTaskList)))
	mux.Handle("GET /chat/{session_id}/tasks/{task_id}", ws.uiAuth(http.HandlerFunc(ws.handleTaskDetail)))
	mux.Handle("GET /partials/sessions", ws.uiAuth(http.HandlerFunc(ws.handlePartialSessions)))
	mux.Handle("GET /partials/messages/{session_id}", ws.uiAuth(http.HandlerFunc(ws.handlePartialMessages)))
	mux.Handle("POST /api/sessions/{session_id}/message", ws.uiAuth(http.HandlerFunc(ws.handleSendMessage)))
	mux.Handle("POST /api/sessions/by-name/{name}/message", ws.uiAuth(http.HandlerFunc(ws.handleSendMessageByName)))
	mux.Handle("GET /api/sessions/list", ws.uiAuth(http.HandlerFunc(ws.handleAPISessions)))
	mux.Handle("POST /api/sessions/{session_id}/upload", ws.uiAuth(http.HandlerFunc(ws.handleUpload)))
	mux.Handle("GET /uploads/{session_id}/{filename}", ws.uiAuth(http.HandlerFunc(ws.handleServeFile)))
	mux.Handle("GET /ws/ui", ws.uiAuth(http.HandlerFunc(ws.handleWSUI)))

	// Sessions JSON API — used by @mention autocomplete.


	// Session management API.
	mux.Handle("PATCH /api/sessions/{id}/archive", ws.uiAuth(http.HandlerFunc(ws.handleArchiveSession)))
	mux.Handle("DELETE /api/sessions/{id}", ws.uiAuth(http.HandlerFunc(ws.handleDeleteSession)))
	mux.Handle("POST /api/sessions/{id}/interrupt", ws.uiAuth(http.HandlerFunc(ws.handleInterruptSession)))
	mux.Handle("GET /api/sessions/{session_id}/browse", ws.uiAuth(http.HandlerFunc(ws.handleBrowseSession)))
	mux.Handle("GET /api/push/vapid-public-key", ws.uiAuth(http.HandlerFunc(ws.handleVAPIDPublicKey)))
	mux.Handle("POST /api/push/subscribe", ws.uiAuth(http.HandlerFunc(ws.handlePushSubscribe)))
	mux.Handle("DELETE /api/push/subscribe", ws.uiAuth(http.HandlerFunc(ws.handlePushUnsubscribe)))

	// Agent/team discovery + live session config.
	mux.Handle("GET /api/projects", ws.uiAuth(http.HandlerFunc(ws.handleAPIProjects)))
	mux.Handle("GET /api/agents", ws.uiAuth(http.HandlerFunc(ws.handleAPIAgents)))
	mux.Handle("GET /api/teams", ws.uiAuth(http.HandlerFunc(ws.handleAPITeams)))
	mux.Handle("POST /api/sessions/{id}/set-agent", ws.uiAuth(http.HandlerFunc(ws.handleSetAgent)))
	mux.Handle("POST /api/sessions/{id}/set-team", ws.uiAuth(http.HandlerFunc(ws.handleSetTeam)))

	// Team panel partial — HTMX polling endpoint.
	mux.Handle("GET /api/sessions/{id}/team", ws.uiAuth(http.HandlerFunc(ws.handleAPISessionTeam)))

	// Cron endpoints.
	mux.Handle("GET /chat/{session_id}/crons", ws.uiAuth(http.HandlerFunc(ws.handleCronList)))
	mux.Handle("DELETE /api/crons/{id}", ws.uiAuth(http.HandlerFunc(ws.handleCronDelete)))

	// Designs gallery.
	mux.Handle("GET /designs", ws.uiAuth(http.HandlerFunc(ws.handleDesignGallery)))
	mux.Handle("GET /designs/static/{id}/{rest...}", ws.uiAuth(http.HandlerFunc(ws.handleDesignStatic)))
	// Project-scoped design assets: ~/.claudio/projects/{slug}/designs/{id}/{rest...}
	mux.Handle("GET /designs/project/{slug}/{id}/{rest...}", ws.uiAuth(http.HandlerFunc(ws.handleDesignProject)))
}

// SetVAPIDPublicKey stores the VAPID public key for the browser subscription flow.
func (ws *WebServer) SetVAPIDPublicKey(key string) { ws.vapidPublicKey = key }

// SetCronStore attaches a CronStore so cron API endpoints are available.
func (ws *WebServer) SetCronStore(cs *tasks.CronStore) { ws.cronStore = cs }

// SetAPIClient attaches an API client used for /compact command execution.
func (ws *WebServer) SetAPIClient(c *api.Client) { ws.apiClient = c }

// SetTeamTemplatesDir sets the directory where team template JSON files are stored.
func (ws *WebServer) SetTeamTemplatesDir(dir string) { ws.teamTemplatesDir = dir }
func (ws *WebServer) SetPublicURL(url string)         { ws.publicURL = url }

// handleVAPIDPublicKey returns the VAPID public key for browser push subscription.
func (ws *WebServer) handleVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"publicKey": ws.vapidPublicKey})
}

// handlePushSubscribe saves a browser push subscription (cookie-auth version).
func (ws *WebServer) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := ws.storage.SavePushSubscription(cc.PushSubscription{Endpoint: body.Endpoint, P256dh: body.Keys.P256dh, Auth: body.Keys.Auth}); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePushUnsubscribe removes a browser push subscription (cookie-auth version).
func (ws *WebServer) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Endpoint == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := ws.storage.DeletePushSubscription(body.Endpoint); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCronList returns a partial HTML list of cron entries for the given session.
func (ws *WebServer) handleCronList(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	if ws.cronStore == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<div class="text-gray-500 text-sm px-4 py-2">Cron store not configured.</div>`)
		return
	}

	var entries []tasks.CronEntry
	for _, e := range ws.cronStore.All() {
		if e.SessionID == sessionID {
			entries = append(entries, e)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if len(entries) == 0 {
		fmt.Fprint(w, `<div class="text-gray-500 text-sm px-4 py-2">No scheduled tasks.</div>`)
		return
	}

	for _, e := range entries {
		cronType := e.Type
		if cronType == "" {
			cronType = "inline"
		}
		badgeColor := "background:#3B82F6" // blue for inline
		if cronType == "background" {
			badgeColor = "background:#8B5CF6" // violet for background
		}
		prompt := e.Prompt
		if len([]rune(prompt)) > 60 {
			runes := []rune(prompt)
			prompt = string(runes[:60]) + "…"
		}
		agent := ""
		if e.Agent != "" {
			agent = fmt.Sprintf(`<span class="text-gray-400 text-xs ml-2">agent: %s</span>`, html.EscapeString(e.Agent))
		}
		fmt.Fprintf(w, `
<div class="cron-row" style="background:#1C1C1E;border-radius:12px;padding:12px 16px;margin-bottom:8px;display:flex;align-items:center;gap:12px;">
  <div style="flex:1;min-width:0;">
    <div style="display:flex;align-items:center;gap:8px;margin-bottom:4px;">
      <span style="%s;color:#fff;font-size:11px;font-weight:600;padding:2px 8px;border-radius:999px;">%s</span>
      <span class="text-gray-300 text-xs font-mono">%s</span>
      %s
    </div>
    <div class="text-gray-400 text-xs truncate">%s</div>
  </div>
  <button
    hx-delete="/api/crons/%s"
    hx-confirm="Delete this cron?"
    hx-target="closest .cron-row"
    hx-swap="outerHTML swap:300ms"
    style="background:none;border:none;cursor:pointer;color:#EF4444;padding:4px 8px;border-radius:6px;font-size:18px;"
    title="Delete cron">🗑</button>
</div>`,
			badgeColor,
			html.EscapeString(cronType),
			html.EscapeString(e.Schedule),
			agent,
			html.EscapeString(prompt),
			html.EscapeString(e.ID),
		)
	}
}

// handleCronDelete removes a cron entry by ID and returns 204.
func (ws *WebServer) handleCronDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if ws.cronStore == nil {
		http.Error(w, "cron store not configured", http.StatusServiceUnavailable)
		return
	}
	if err := ws.cronStore.Remove(id); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// uiAuth checks the "auth" HttpOnly cookie.
func (ws *WebServer) uiAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Trust same-machine requests (e.g. Playwright fidelity tool)
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if host == "127.0.0.1" || host == "::1" {
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie("auth")
		if err != nil || !ws.validPassword(c.Value) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (ws *WebServer) validPassword(token string) bool {
	if token == "" || ws.password == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(ws.password)) == 1
}

// --- Handlers ---

func (ws *WebServer) handleLoginGet(w http.ResponseWriter, r *http.Request) {
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
	http.SetCookie(w, &http.Cookie{
		Name:     "auth",
		Value:    pass,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleServiceWorker serves /sw.js at root scope so the SW controls all pages.
func (ws *WebServer) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	content, err := staticFS.ReadFile("static/sw.js")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(content)
}

// LoginPageData and ChatListData are defined in their respective templ files.

// ChatViewData holds data for the chat view page.
type ChatViewData struct {
	Session   cc.Session
	Messages  []MessageView
	SessionID string
}

// SessionsData, MessagesData, TeamMembersData removed — templ fns accept plain slices.

// InfoPageData holds data for the session info panel.
type InfoPageData struct {
	Session         cc.Session
	Tasks           []cc.Task
	Agents          []cc.Agent
	SessionID       string
	ActiveTab       string            // which tab is active (tasks/team/media/crons/config)
	Images          []cc.Attachment   // image attachments for media grid
	Docs            []cc.Attachment   // non-image attachments for document list
	Crons           []tasks.CronEntry // scheduled tasks for this session
	AvailableAgents []agentspkg.AgentDefinition
	AvailableTeams  []string
}

// TaskDetailData holds data for the task detail partial.
// DescHTML is a sanitized HTML string; use templ.Raw(data.DescHTML) in templates.
type TaskDetailData struct {
	Task     cc.Task
	DescHTML string // markdown description pre-rendered to sanitized HTML
}

// renderMarkdown converts a markdown string to safe HTML.
func (ws *WebServer) handleChatList(w http.ResponseWriter, r *http.Request) {
	sessions, err := ws.storage.ListSessions("")
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	rows := ws.buildSessionRows(sessions)
	templ.Handler(ChatList(ChatListData{Rows: rows, SessionID: ""})).ServeHTTP(w, r)
}

func (ws *WebServer) handleChatView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("session_id")
	sess, err := ws.storage.GetSession(id)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	// Mark as read when chat view opens.
	_ = ws.storage.MarkRead(id)
	// cc_messages is the authoritative store for ComandCenter sessions.
	// Hub writes all incoming events to cc_messages; the native messages table
	// is only populated by headless TUI sessions and is empty for CC sessions.
	msgs, err := ws.storage.ListMessages(id, 100)
	if err != nil {
		msgs = nil
	}
	// ListMessages returns newest first; reverse for display.
	reversed := reverseMessages(msgs)

	// Load all session attachments and group by message_id for O(1) lookup.
	allAtts, _ := ws.storage.ListAttachments(id)
	attsByMsg := make(map[string][]cc.Attachment, len(allAtts))
	for _, att := range allAtts {
		if att.MessageID != "" {
			attsByMsg[att.MessageID] = append(attsByMsg[att.MessageID], att)
		}
	}
	views := make([]MessageView, len(reversed))
	for i, m := range reversed {
		views[i] = MessageView{Message: m, Attachments: attsByMsg[m.ID]}
	}

	if r.Header.Get("HX-Request") == "true" {
		ChatView(sess, views, id).Render(r.Context(), w)
		return
	}
	// Full-page render (hard refresh / direct URL): render full shell with sidebar.
	sessions, _ := ws.storage.ListSessions("")
	rows := ws.buildSessionRows(sessions)
	listData := ChatListData{Rows: rows, SessionID: id}
	templ.Handler(ChatPage(listData, sess, views, id)).ServeHTTP(w, r)
}

func (ws *WebServer) handleSessionInfo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("session_id")
	sess, err := ws.storage.GetSession(id)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	sessionTasks, err := ws.storage.ListTasks(id)
	if err != nil {
		sessionTasks = nil
	}
	agents, err := ws.storage.ListAgents(id)
	if err != nil {
		agents = nil
	}
	allAtts, _ := ws.storage.ListAttachments(id)
	var images, docs []cc.Attachment
	for _, att := range allAtts {
		if strings.HasPrefix(att.MimeType, "image/") {
			images = append(images, att)
		} else {
			docs = append(docs, att)
		}
	}
	var crons []tasks.CronEntry
	if ws.cronStore != nil {
		for _, e := range ws.cronStore.All() {
			if e.SessionID == id {
				crons = append(crons, e)
			}
		}
	}
	allAgentDefs := agentspkg.AllAgents(agentspkg.GetCustomDirs()...)
	allTeamTpls := teams.LoadTemplates(ws.teamTemplatesDir)
	teamNames := make([]string, 0, len(allTeamTpls))
	for _, t := range allTeamTpls {
		teamNames = append(teamNames, t.Name)
	}
	activeTab := r.URL.Query().Get("tab")
	if activeTab == "" {
		activeTab = "tasks"
	}

	data := InfoPageData{
		Session:         sess,
		Tasks:           sessionTasks,
		Agents:          agents,
		SessionID:       id,
		ActiveTab:       activeTab,
		Images:          images,
		Docs:            docs,
		Crons:           crons,
		AvailableAgents: allAgentDefs,
		AvailableTeams:  teamNames,
	}

	if r.Header.Get("HX-Request") == "true" {
		// Tab switch — render only the tab content partial, not the full panel
		if r.URL.Query().Get("tab") != "" {
			switch activeTab {
			case "team":
				TabTeam(data).Render(r.Context(), w)
			case "media":
				TabMedia(data).Render(r.Context(), w)
			case "crons":
				TabCrons(data).Render(r.Context(), w)
			case "config":
				TabConfig(data).Render(r.Context(), w)
			default:
				TabTasks(data).Render(r.Context(), w)
			}
			return
		}
		InfoPanel(data).Render(r.Context(), w)
		return
	}
	InfoPanel(data).Render(r.Context(), w)
}

func (ws *WebServer) handleTaskList(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	tasks, err := ws.storage.ListTasks(sessionID)
	if err != nil {
		tasks = nil
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	TaskRows(tasks, sessionID).Render(r.Context(), w)
}

func (ws *WebServer) handleTaskDetail(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	task, err := ws.storage.GetTask(taskID)
	if err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	TaskDetail(TaskDetailData{
		Task:     task,
		DescHTML: renderMarkdown(task.Description),
	}).Render(r.Context(), w)
}

// handleAPISessionTeam returns an HTML fragment of agent rows for the given session.
// Called by HTMX polling every 3s and on WS-triggered refresh events.
func (ws *WebServer) handleAPISessionTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agents, err := ws.storage.ListAgents(id)
	if err != nil {
		agents = nil
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	TeamMembers(agents).Render(r.Context(), w)
}

func (ws *WebServer) handlePartialSessions(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	sessions, err := ws.storage.ListSessions(filter)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	rows := ws.buildSessionRows(sessions)
	SessionsPartial(rows).Render(r.Context(), w)
}

func (ws *WebServer) handlePartialMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("session_id")
	msgs, err := ws.storage.GetNativeMessages(id, 100)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	reversed := reverseMessages(msgs)

	allAtts, _ := ws.storage.ListAttachments(id)
	attsByMsg := make(map[string][]cc.Attachment, len(allAtts))
	for _, att := range allAtts {
		if att.MessageID != "" {
			attsByMsg[att.MessageID] = append(attsByMsg[att.MessageID], att)
		}
	}
	views := make([]MessageView, len(reversed))
	for i, m := range reversed {
		views[i] = MessageView{Message: m, Attachments: attsByMsg[m.ID]}
	}
	MessagesPartial(views).Render(r.Context(), w)
}

func (ws *WebServer) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	content := r.FormValue("content")
	if content == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Parse /alias query — model override for this turn.
	var modelOverride string
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "/") {
		rest := trimmed[1:]
		var alias, query string
		if idx := strings.IndexAny(rest, " \n\t"); idx != -1 {
			alias = strings.ToLower(rest[:idx])
			query = strings.TrimSpace(rest[idx+1:])
		} else {
			alias = strings.ToLower(rest)
			query = ""
		}
		if fullModel, ok := modelAliases[alias]; ok {
			modelOverride = fullModel
			content = query
			if content == "" {
				// No query — just confirm the model switch without sending a message.
				confirm := cc.Message{
					ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
					SessionID: sessionID,
					Role:      "assistant",
					AgentName: "system",
					Content:   "Model set to " + fullModel + " for next turn ✓",
					CreatedAt: time.Now(),
				}
				_ = ws.storage.InsertMessage(confirm)
				ws.pushMsgBubble(sessionID, confirm)
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
	}

	// Intercept /clear — wipe history, confirm, return early.
	if strings.TrimSpace(content) == "/clear" {
		if err := ws.storage.DeleteMessages(sessionID); err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		_ = ws.storage.DeleteNativeMessages(sessionID)
		confirm := cc.Message{
			ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
			SessionID: sessionID,
			Role:      "assistant",
			AgentName: "system",
			Content:   "Conversation cleared. ✓",
			CreatedAt: time.Now(),
		}
		_ = ws.storage.InsertMessage(confirm)
		// Forward EventClearHistory to the attached claudio engine.
		clearEnv := attach.Envelope{Type: attach.EventClearHistory}
		_ = ws.hub.Send(sessionID, clearEnv)
		// Tell all connected clients to clear their message list, then show confirm bubble.
		if p, err := json.Marshal(map[string]string{"type": "messages.cleared"}); err == nil {
			ws.pushToSessionClients(sessionID, p)
		}
		ws.pushMsgBubble(sessionID, confirm)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Intercept /compact [instruction] — summarize+replace history via compact service.
	if strings.HasPrefix(strings.TrimSpace(content), "/compact") {
		instruction := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(content), "/compact"))
		ws.handleCompact(w, sessionID, instruction)
		return
	}

	// Detect @mention: "@Name message body"
	if m := mentionRe.FindStringSubmatch(content); m != nil {
		targetName := m[1]
		msgBody := m[2]
		ws.handleMentionRoute(w, sessionID, targetName, msgBody, content)
		return
	}

	// Normal (non-@mention) path — existing behavior.
	payload, _ := json.Marshal(attach.UserMsgPayload{Content: content, ModelOverride: modelOverride})
	env := attach.Envelope{Type: attach.EventMsgUser, Payload: payload}
	if err := ws.hub.Send(sessionID, env); err != nil {
		http.Error(w, "session not connected", http.StatusServiceUnavailable)
		return
	}

	msg := cc.Message{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		SessionID: sessionID,
		Role:      "user",
		Content:   content,
		CreatedAt: time.Now(),
	}
	_ = ws.storage.InsertMessage(msg)

	// Push user bubble to UI WS clients immediately (no refresh needed).
	var buf bytes.Buffer
	if err := MessageBubble(MessageView{Message: msg}).Render(r.Context(), &buf); err == nil {
		if payload, err := json.Marshal(map[string]string{
			"type": "message.user",
			"html": buf.String(),
		}); err == nil {
			ws.pushToSessionClients(sessionID, payload)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleMentionRoute handles @Name routing: stores in origin session with reply metadata,
// routes a copy to the target session, and pushes UI events to both sessions' WS clients.
func (ws *WebServer) handleMentionRoute(w http.ResponseWriter, originID, targetName, msgBody, fullContent string) {
	// Resolve target session by name.
	targetSess, found, err := ws.storage.GetSessionByName(targetName)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Route message body to target session via hub.
	payload, _ := json.Marshal(attach.UserMsgPayload{Content: msgBody})
	env := attach.Envelope{Type: attach.EventMsgUser, Payload: payload}
	if err := ws.hub.Send(targetSess.ID, env); err != nil {
		http.Error(w, "session not connected", http.StatusServiceUnavailable)
		return
	}

	now := time.Now()

	// Quoted content: first 80 runes of full message.
	quoted := fullContent
	if r := []rune(fullContent); len(r) > 80 {
		quoted = string(r[:80])
	}

	// Store in originating session with reply metadata.
	originMsg := cc.Message{
		ID:             fmt.Sprintf("%d", now.UnixNano()),
		SessionID:      originID,
		Role:           "user",
		Content:        fullContent,
		CreatedAt:      now,
		ReplyToSession: targetName,
		QuotedContent:  quoted,
	}
	_ = ws.storage.InsertMessage(originMsg)

	// Store copy in target session (plain user message, no reply fields).
	targetMsg := cc.Message{
		ID:        fmt.Sprintf("%da", now.UnixNano()),
		SessionID: targetSess.ID,
		Role:      "user",
		Content:   msgBody,
		CreatedAt: now,
	}
	_ = ws.storage.InsertMessage(targetMsg)

	// Push bubbles to both sessions' WS clients.
	ws.pushMsgBubble(originID, originMsg)
	ws.pushMsgBubble(targetSess.ID, targetMsg)

	w.WriteHeader(http.StatusNoContent)
}

// handleSendMessageByName handles POST /api/sessions/by-name/{name}/message.
// Looks up the session by name, then sends the message via hub.
func (ws *WebServer) handleSendMessageByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	content := r.FormValue("content")
	if content == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	sess, found, err := ws.storage.GetSessionByName(name)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	payload, _ := json.Marshal(attach.UserMsgPayload{Content: content})
	env := attach.Envelope{Type: attach.EventMsgUser, Payload: payload}
	if err := ws.hub.Send(sess.ID, env); err != nil {
		http.Error(w, "session not connected", http.StatusServiceUnavailable)
		return
	}

	msg := cc.Message{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		SessionID: sess.ID,
		Role:      "user",
		Content:   content,
		CreatedAt: time.Now(),
	}
	_ = ws.storage.InsertMessage(msg)

	var buf bytes.Buffer
	if err := MessageBubble(MessageView{Message: msg}).Render(r.Context(), &buf); err == nil {
		if p, err := json.Marshal(map[string]string{
			"type": "message.user",
			"html": buf.String(),
		}); err == nil {
			ws.pushToSessionClients(sess.ID, p)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleCompact runs the compact service on the session's message history,
// replaces DB messages with compacted ones, and pushes a confirmation bubble.
func (ws *WebServer) handleCompact(w http.ResponseWriter, sessionID, instruction string) {
	msgs, err := ws.storage.GetNativeMessages(sessionID, 1000)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if len(msgs) == 0 {
		confirm := cc.Message{
			ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
			SessionID: sessionID,
			Role:      "assistant",
			AgentName: "system",
			Content:   "Nothing to compact.",
			CreatedAt: time.Now(),
		}
		_ = ws.storage.InsertMessage(confirm)
		ws.pushMsgBubble(sessionID, confirm)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if ws.apiClient == nil {
		http.Error(w, "compact unavailable: no API client configured", http.StatusServiceUnavailable)
		return
	}

	// Push an immediate "in progress" bubble so the user knows it started.
	pending := cc.Message{
		ID:        fmt.Sprintf("%dp", time.Now().UnixNano()),
		SessionID: sessionID,
		Role:      "assistant",
		AgentName: "system",
		Content:   "Compacting conversation… ⏳",
		CreatedAt: time.Now(),
	}
	_ = ws.storage.InsertMessage(pending)
	ws.pushMsgBubble(sessionID, pending)

	// Return 202 immediately — compact can take 30-120s.
	w.WriteHeader(http.StatusAccepted)

	// Run compact in background; push result via WS when done.
	apiMsgs := ccMessagesToAPI(reverseMessages(msgs))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		compacted, summary, err := compact.Compact(ctx, ws.apiClient, apiMsgs, 10, instruction)

		// Remove the pending bubble.
		_ = ws.storage.DeleteMessageByID(pending.ID)

		if err != nil {
			errMsg := cc.Message{
				ID:        fmt.Sprintf("%de", time.Now().UnixNano()),
				SessionID: sessionID,
				Role:      "assistant",
				AgentName: "system",
				Content:   "Compact failed: " + err.Error(),
				CreatedAt: time.Now(),
			}
			_ = ws.storage.InsertMessage(errMsg)
			ws.pushMsgBubble(sessionID, errMsg)
			return
		}

		// Replace DB messages with compacted set.
		_ = ws.storage.DeleteMessages(sessionID)
		_ = ws.storage.DeleteNativeMessages(sessionID)
		now := time.Now()
		for i, am := range compacted {
			cm := cc.Message{
				ID:        fmt.Sprintf("%d-%d", now.UnixNano(), i),
				SessionID: sessionID,
				Role:      apiRoleToCC(am.Role),
				Content:   apiMessageText(am),
				AgentName: "system",
				CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
			}
			_ = ws.storage.InsertMessage(cm)
			_ = ws.storage.InsertNativeMessage(sessionID, cm.Role, cm.Content, cm.CreatedAt)
		}

		confirmText := "Conversation compacted. ✓"
		if summary != "" {
			runes := []rune(summary)
			if len(runes) > 200 {
				runes = runes[:200]
			}
			confirmText = "Conversation compacted. ✓\n\n" + string(runes) + "…"
		}
		confirm := cc.Message{
			ID:        fmt.Sprintf("%dc", now.UnixNano()),
			SessionID: sessionID,
			Role:      "assistant",
			AgentName: "system",
			Content:   confirmText,
			CreatedAt: now.Add(time.Duration(len(compacted)) * time.Millisecond),
		}
		_ = ws.storage.InsertMessage(confirm)
		ws.pushMsgBubble(sessionID, confirm)
		if p, err := json.Marshal(map[string]string{"type": "messages.compacted"}); err == nil {
			ws.pushToSessionClients(sessionID, p)
		}
	}()
}

// ccMessagesToAPI converts cc.Message records to api.Message format for the compact service.
func ccMessagesToAPI(msgs []cc.Message) []api.Message {
	out := make([]api.Message, 0, len(msgs))
	for _, m := range msgs {
		role := m.Role
		if role == "tool_use" {
			role = "assistant"
		}
		if role != "user" && role != "assistant" {
			continue
		}
		content, _ := json.Marshal([]map[string]string{{"type": "text", "text": m.Content}})
		out = append(out, api.Message{Role: role, Content: json.RawMessage(content)})
	}
	return out
}

// apiRoleToCC converts an API message role to a cc.Message role.
func apiRoleToCC(role string) string {
	if role == "assistant" {
		return "assistant"
	}
	return "user"
}

// apiMessageText extracts plain text from an api.Message content.
func apiMessageText(m api.Message) string {
	// Try array of blocks first.
	var blocks []json.RawMessage
	if json.Unmarshal(m.Content, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			var block struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if json.Unmarshal(b, &block) == nil && block.Type == "text" && block.Text != "" {
				parts = append(parts, block.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	// Fallback: try plain string.
	var s string
	if json.Unmarshal(m.Content, &s) == nil {
		return s
	}
	return string(m.Content)
}

// pushMsgBubble renders and pushes a user message bubble to a session's WS clients.
func (ws *WebServer) pushMsgBubble(sessionID string, msg cc.Message) {
	var buf bytes.Buffer
	if err := MessageBubble(MessageView{Message: msg}).Render(context.Background(), &buf); err == nil {
		if p, err := json.Marshal(map[string]string{
			"type": "message.user",
			"html": buf.String(),
		}); err == nil {
			ws.pushToSessionClients(sessionID, p)
		}
	}
}

// handleWSUI upgrades to WebSocket and streams new messages to the browser.
func (ws *WebServer) handleWSUI(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	wsServer := websocket.Server{
		Handshake: func(cfg *websocket.Config, req *http.Request) error {
			return nil
		},
		Handler: websocket.Handler(func(conn *websocket.Conn) {
			client := &uiClient{
				sessionID: sessionID,
				send:      make(chan []byte, 64),
			}
			ws.addClient(client)
			defer ws.removeClient(client)

			// Write loop: send messages until connection closes or send closes.
			done := make(chan struct{})
			go func() {
				defer close(done)
				// Read loop: detect client disconnect.
				buf := make([]byte, 1)
				for {
					if _, err := conn.Read(buf); err != nil {
						return
					}
				}
			}()

			for {
				select {
				case msg, ok := <-client.send:
					if !ok {
						return
					}
					if _, err := conn.Write(msg); err != nil {
						return
					}
				case <-done:
					return
				}
			}
		}),
	}
	wsServer.ServeHTTP(w, r)
}

// pushToSessionClients delivers payload bytes to all UI WS clients watching sessionID.
func (ws *WebServer) pushToSessionClients(sessionID string, payload []byte) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	for client := range ws.clients {
		if client.sessionID == sessionID {
			select {
			case client.send <- payload:
			default: // drop if full
			}
		}
	}
}

// fanout reads UIBroadcast events and forwards rendered HTML to interested clients.
func (ws *WebServer) fanout() {
	ch := ws.hub.UIBroadcast()
	for ev := range ch {
		switch ev.Envelope.Type {
		case attach.EventMsgAssistant:
			msg := envelopeToMessage(ev)
			if msg == nil {
				continue
			}
			var buf bytes.Buffer
			if err := MessageBubble(MessageView{Message: *msg}).Render(context.Background(), &buf); err != nil {
				continue
			}
			payload, err := json.Marshal(map[string]string{
				"type": "message.assistant",
				"html": buf.String(),
			})
			if err != nil {
				continue
			}
			ws.pushToSessionClients(ev.SessionID, payload)

		case attach.EventMsgToolUse:
			// 1. Permanent tool-use bubble in chat history.
			msg := envelopeToMessage(ev)
			if msg == nil {
				continue
			}
			var buf bytes.Buffer
			if err := MessageBubble(MessageView{Message: *msg}).Render(context.Background(), &buf); err != nil {
				continue
			}
			bubblePayload, err := json.Marshal(map[string]string{
				"type": "message.tool_use",
				"html": buf.String(),
			})
			if err != nil {
				continue
			}
			ws.pushToSessionClients(ev.SessionID, bubblePayload)

			// 2. Transient typing indicator with tool + agent name.
			var p attach.ToolUsePayload
			if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
				continue
			}
			typingPayload, err := json.Marshal(map[string]string{
				"type":      "typing",
				"tool":      p.Tool,
				"agentName": p.AgentName,
			})
			if err != nil {
				continue
			}
			ws.pushToSessionClients(ev.SessionID, typingPayload)

		case attach.EventMsgToolResult:
			var p attach.ToolResultPayload
			if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
				continue
			}
			// Push updated bubble HTML with output filled in.
			resultPayload, err := json.Marshal(map[string]string{
				"type":       "message.tool_result",
				"toolUseID":  p.ToolUseID,
				"output":     p.Output,
			})
			if err != nil {
				continue
			}
			ws.pushToSessionClients(ev.SessionID, resultPayload)

		case attach.EventMsgStreamDelta:
			var p attach.StreamDeltaPayload
			if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
				continue
			}
			pushMsg, err := json.Marshal(map[string]interface{}{
				"type":        "message.stream_delta",
				"delta":       p.Delta,
				"accumulated": p.Accumulated,
			})
			if err != nil {
				continue
			}
			ws.pushToSessionClients(ev.SessionID, pushMsg)

		case attach.EventDesignScreenshot:
			var p attach.DesignScreenshotPayload
			if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
				continue
			}
			ws.handleScreenshotPush(ev.SessionID, p)

		case attach.EventDesignBundleReady:
			var p attach.DesignBundlePayload
			if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
				continue
			}
			ws.handleBundleLinkPush(ev.SessionID, p)

		case attach.EventAgentStatus:
			var p attach.AgentStatusPayload
			if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
				continue
			}
			payload, err := json.Marshal(map[string]string{
				"type":   "agent_status",
				"name":   p.Name,
				"status": p.Status,
			})
			if err != nil {
				continue
			}
			ws.pushToSessionClients(ev.SessionID, payload)

		case attach.EventClearHistory:
			payload, err := json.Marshal(map[string]string{
				"type": "messages.cleared",
			})
			if err != nil {
				continue
			}
			ws.pushToSessionClients(ev.SessionID, payload)

		case attach.EventTaskCreated:
			payload, err := json.Marshal(map[string]string{
				"type": "task.created",
			})
			if err != nil {
				continue
			}
			ws.pushToSessionClients(ev.SessionID, payload)

		case attach.EventTaskUpdated:
			payload, err := json.Marshal(map[string]string{
				"type": "task.updated",
			})
			if err != nil {
				continue
			}
			ws.pushToSessionClients(ev.SessionID, payload)

		case attach.EventConfigChanged:
			var p attach.ConfigChangedPayload
			if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
				continue
			}
			payload, err := json.Marshal(map[string]string{
				"type":            "config.changed",
				"model":           p.Model,
				"permission_mode": p.PermissionMode,
				"output_style":    p.OutputStyle,
			})
			if err != nil {
				continue
			}
			ws.pushToSessionClients(ev.SessionID, payload)

		case attach.EventAgentChanged:
			var p attach.AgentChangedPayload
			if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
				continue
			}
			payload, err := json.Marshal(map[string]string{
				"type":       "agent.changed",
				"agent_type": p.AgentType,
			})
			if err != nil {
				continue
			}
			ws.pushToSessionClients(ev.SessionID, payload)

		case attach.EventTeamChanged:
			var p attach.TeamChangedPayload
			if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
				continue
			}
			payload, err := json.Marshal(map[string]string{
				"type":          "team.changed",
				"team_template": p.TeamTemplate,
			})
			if err != nil {
				continue
			}
			ws.pushToSessionClients(ev.SessionID, payload)
		}
	}
}

// handleScreenshotPush copies a screenshot saved by the CLI into the uploads
// directory and pushes an image bubble to all browser clients watching the session.
func (ws *WebServer) handleScreenshotPush(sessionID string, p attach.DesignScreenshotPayload) {
	// 1. Copy screenshot into uploads dir so it's served at /uploads/{sessionID}/{filename}.
	destDir := filepath.Join(ws.uploadsDir, sessionID)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return
	}
	destPath := filepath.Join(destDir, p.Filename)
	if err := copyFile(p.FilePath, destPath); err != nil {
		return
	}

	fi, _ := os.Stat(destPath)
	var size int64
	if fi != nil {
		size = fi.Size()
	}

	// 2. Insert assistant message row (content empty; image attachment carries the payload).
	now := time.Now()
	msg := cc.Message{
		ID:        cc.NewID(),
		SessionID: sessionID,
		Role:      "assistant",
		Content:   "",
		CreatedAt: now,
	}
	if err := ws.storage.InsertMessage(msg); err != nil {
		return
	}

	// 3. Record attachment.
	att := cc.Attachment{
		ID:           cc.NewID(),
		SessionID:    sessionID,
		MessageID:    msg.ID,
		Filename:     p.Filename,
		OriginalName: p.Filename,
		MimeType:     "image/png",
		Size:         size,
		CreatedAt:    now,
	}
	if err := ws.storage.InsertAttachment(att); err != nil {
		return
	}

	// 4. Render message-bubble template and push to browser clients.
	var buf bytes.Buffer
	view := MessageView{Message: msg, Attachments: []cc.Attachment{att}}
	if err := MessageBubble(view).Render(context.Background(), &buf); err != nil {
		return
	}
	wsPayload, _ := json.Marshal(map[string]string{"type": "message.assistant", "html": buf.String()})
	ws.pushToSessionClients(sessionID, wsPayload)
}

// handleBundleLinkPush inserts an assistant message with a clickable link to
// the bundle HTML and pushes it to all browser clients watching the session.
func (ws *WebServer) handleBundleLinkPush(sessionID string, p attach.DesignBundlePayload) {
	now := time.Now()
	bundleURL := p.BundleURL
	if ws.publicURL != "" && strings.HasPrefix(bundleURL, "/") {
		bundleURL = strings.TrimRight(ws.publicURL, "/") + bundleURL
	}
	msg := cc.Message{
		ID:        cc.NewID(),
		SessionID: sessionID,
		Role:      "bundle",
		Content:   bundleURL,
		AgentName: p.SessionName,
		CreatedAt: now,
	}
	if err := ws.storage.InsertMessage(msg); err != nil {
		return
	}

	var buf bytes.Buffer
	if err := MessageBubble(MessageView{Message: msg}).Render(context.Background(), &buf); err != nil {
		return
	}
	wsPayload, _ := json.Marshal(map[string]string{"type": "message.assistant", "html": buf.String()})
	ws.pushToSessionClients(sessionID, wsPayload)
}

// copyFile copies src to dst, creating dst if it does not exist.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func (ws *WebServer) addClient(c *uiClient) {
	ws.mu.Lock()
	ws.clients[c] = struct{}{}
	ws.mu.Unlock()
}

func (ws *WebServer) removeClient(c *uiClient) {
	ws.mu.Lock()
	delete(ws.clients, c)
	ws.mu.Unlock()
}

// handleArchiveSession sets a session's status to 'archived' and returns 200 with empty body.
// htmx swaps the row with the empty response, removing it from the DOM.
func (ws *WebServer) handleArchiveSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := ws.storage.ArchiveSession(id); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleDeleteSession permanently deletes a session + all its messages/tasks/agents.
// Returns 200 with empty body so htmx removes the row via outerHTML swap.
func (ws *WebServer) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := ws.storage.DeleteSession(id); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleInterruptSession sends an interrupt signal to the active engine turn for a session.
// Returns 200 on success, 404 if the session is unknown, 503 if no active turn is registered.
func (ws *WebServer) handleInterruptSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := ws.storage.GetSession(id); err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if !ws.hub.Interrupt(id) {
		http.Error(w, "no active turn", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleAPISessions returns all non-archived sessions as JSON.
// Used by the @mention autocomplete in the chat UI.
func (ws *WebServer) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := ws.storage.ListSessions("")
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if sessions == nil {
		sessions = []cc.Session{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// handleAPIAgents returns all available agent definitions (built-in + custom) as JSON.
// Response: [{"type":"...","description":"...","when_to_use":"...","model":"..."}]
func (ws *WebServer) handleAPIProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := ws.storage.ListProjects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type projectJSON struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		Count int    `json:"count"`
	}
	out := make([]projectJSON, 0, len(projects))
	for _, p := range projects {
		out = append(out, projectJSON{Name: p.Name, Path: p.Path, Count: p.Count})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (ws *WebServer) handleAPIAgents(w http.ResponseWriter, r *http.Request) {
	all := agentspkg.AllAgents(agentspkg.GetCustomDirs()...)
	type agentJSON struct {
		Type       string `json:"type"`
		WhenToUse  string `json:"when_to_use"`
		Model      string `json:"model"`
	}
	out := make([]agentJSON, 0, len(all))
	for _, a := range all {
		out = append(out, agentJSON{
			Type:      a.Type,
			WhenToUse: a.WhenToUse,
			Model:     a.Model,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// handleAPITeams returns all available team templates as JSON.
// Response: [{"name":"...","description":"..."}]
func (ws *WebServer) handleAPITeams(w http.ResponseWriter, r *http.Request) {
	all := teams.LoadTemplates(ws.teamTemplatesDir)
	type teamJSON struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	out := make([]teamJSON, 0, len(all))
	for _, t := range all {
		out = append(out, teamJSON{Name: t.Name, Description: t.Description})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// handleSetAgent switches the active agent for a running session.
// Body: {"agent_type": "string"} (empty = clear/default)
func (ws *WebServer) handleSetAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		AgentType string `json:"agent_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := ws.hub.SetAgent(id, body.AgentType); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	// Persist: read current team template then update both config fields.
	sess, err := ws.storage.GetSession(id)
	if err != nil {
		// Session not in DB yet; best-effort persist using empty team.
		_ = ws.storage.UpdateSessionConfig(id, body.AgentType, "")
	} else {
		_ = ws.storage.UpdateSessionConfig(id, body.AgentType, sess.TeamTemplate)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// handleSetTeam switches the active team for a running session.
// Body: {"team_name": "string"} (empty = clear/default)
func (ws *WebServer) handleSetTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		TeamName string `json:"team_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := ws.hub.SetTeam(id, body.TeamName); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	// Persist: read current agent type then update both config fields.
	sess, err := ws.storage.GetSession(id)
	if err != nil {
		// Session not in DB yet; best-effort persist using empty agent.
		_ = ws.storage.UpdateSessionConfig(id, "", body.TeamName)
	} else {
		_ = ws.storage.UpdateSessionConfig(id, sess.AgentType, body.TeamName)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// buildSessionRows fetches the last message for each session and unread count.
func (ws *WebServer) buildSessionRows(sessions []cc.Session) []sessionRow {
	rows := make([]sessionRow, 0, len(sessions))
	for _, sess := range sessions {
		row := sessionRow{Session: sess}
		msgs, err := ws.storage.ListMessages(sess.ID, 1)
		if err == nil && len(msgs) > 0 {
			content := msgs[0].Content
			r := []rune(content)
			if len(r) > 60 {
				content = string(r[:60]) + "…"
			}
			row.LastMessage = content
		}
		// Populate unread count.
		count, err := ws.storage.UnreadCount(sess.ID)
		if err == nil {
			row.UnreadCount = count
		}
		rows = append(rows, row)
	}
	return rows
}

// reverseMessages reverses a slice (DB returns newest first; UI needs oldest first).
func reverseMessages(msgs []cc.Message) []cc.Message {
	out := make([]cc.Message, len(msgs))
	for i, m := range msgs {
		out[len(msgs)-1-i] = m
	}
	return out
}

// POST /api/sessions/{session_id}/upload
// Multipart form: "file" (required), "content" (optional caption).
func (ws *WebServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")

	// Validate session.
	if _, err := ws.storage.GetSession(sessionID); err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Parse multipart — 32 MB limit.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "parse multipart failed", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field missing", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Detect MIME from first 512 bytes then reset reader.
	sniff := make([]byte, 512)
	n, _ := file.Read(sniff)
	mimeType := http.DetectContentType(sniff[:n])
	// Also honour the content-type from the part header if available.
	if ct := header.Header.Get("Content-Type"); ct != "" && ct != "application/octet-stream" {
		mimeType = ct
	}

	// Strip MIME parameters (e.g. "image/jpeg; name=...").
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}

	// Seek back to start by seeking on the underlying file.
	if seeker, ok := file.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
	}

	ext := filepath.Ext(header.Filename)
	storedName := cc.NewID() + ext

	// Ensure per-session upload directory exists.
	dir := filepath.Join(ws.uploadsDir, sessionID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	// Write file to disk.
	dst, err := os.Create(filepath.Join(dir, storedName))
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	size, err := io.Copy(dst, file)
	dst.Close()
	if err != nil {
		http.Error(w, "write error", http.StatusInternalServerError)
		return
	}

	caption := strings.TrimSpace(r.FormValue("content"))
	if caption == "" {
		caption = header.Filename
	}

	now := time.Now()

	// Create a user message for this upload.
	msg := cc.Message{
		ID:        cc.NewID(),
		SessionID: sessionID,
		Role:      "user",
		Content:   caption,
		CreatedAt: now,
	}
	if err := ws.storage.InsertMessage(msg); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Record attachment linked to the message.
	att := cc.Attachment{
		ID:           cc.NewID(),
		SessionID:    sessionID,
		MessageID:    msg.ID,
		Filename:     storedName,
		OriginalName: header.Filename,
		MimeType:     mimeType,
		Size:         size,
		CreatedAt:    now,
	}
	if err := ws.storage.InsertAttachment(att); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Push bubble to WS clients.
	var buf bytes.Buffer
	view := MessageView{Message: msg, Attachments: []cc.Attachment{att}}
	if err := MessageBubble(view).Render(r.Context(), &buf); err == nil {
		payload, _ := json.Marshal(map[string]string{"type": "message.user", "html": buf.String()})
		ws.pushToSessionClients(sessionID, payload)
	}

	// Forward to headless Claudio session.
	diskPath := filepath.Join(ws.uploadsDir, sessionID, storedName)
	fwdEnv, fwdErr := attach.NewEnvelope(attach.EventMsgUser, attach.UserMsgPayload{
		Content:     caption,
		Attachments: []attach.Attachment{{FilePath: diskPath, MimeType: mimeType}},
	})
	if fwdErr == nil {
		_ = ws.hub.Send(sessionID, fwdEnv) // ignore error — session may not be connected
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"url":          "/uploads/" + sessionID + "/" + storedName,
		"filename":     storedName,
		"originalName": header.Filename,
		"mimeType":     mimeType,
		"size":         size,
	})
}

// GET /uploads/{session_id}/{filename}
func (ws *WebServer) handleServeFile(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	filename := r.PathValue("filename")

	// Sanitize: prevent path traversal.
	if strings.Contains(filename, "..") || strings.ContainsAny(filename, "/\\") {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}

	path := filepath.Join(ws.uploadsDir, sessionID, filename)
	http.ServeFile(w, r, path)
}

// browseItem is a single directory entry for the file browser JSON response.
type browseItem struct {
	Name     string    `json:"name"`
	IsDir    bool      `json:"is_dir"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// browseResponse is the JSON body for GET /api/sessions/{session_id}/browse.
type browseResponse struct {
	Current string       `json:"current"`
	Root    string       `json:"root"`
	Items   []browseItem `json:"items"`
}

// handleBrowseSession lists files/directories inside the session's working directory.
// GET /api/sessions/{session_id}/browse?path=<relative-path>
func (ws *WebServer) handleBrowseSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	sess, err := ws.storage.GetSession(sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if sess.Path == "" {
		http.Error(w, "session has no path set", http.StatusBadRequest)
		return
	}

	root, err := filepath.Abs(sess.Path)
	if err != nil {
		http.Error(w, "invalid session path", http.StatusInternalServerError)
		return
	}

	// Resolve the requested subpath.
	subPath := r.URL.Query().Get("path")
	var target string
	if subPath == "" || subPath == "/" {
		target = root
	} else {
		// Join and clean; then verify it doesn't escape root.
		target = filepath.Join(root, filepath.FromSlash(subPath))
		rel, err := filepath.Rel(root, target)
		if err != nil || strings.HasPrefix(rel, "..") {
			http.Error(w, "path traversal not allowed", http.StatusForbidden)
			return
		}
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		http.Error(w, "cannot read directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	items := make([]browseItem, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, browseItem{
			Name:     e.Name(),
			IsDir:    e.IsDir(),
			Size:     info.Size(),
			Modified: info.ModTime(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(browseResponse{
		Current: target,
		Root:    root,
		Items:   items,
	})
}

// ── Designs gallery ──────────────────────────────────────────────────────────

// DesignSession holds metadata for one design output directory.
type DesignSession struct {
	ID          string   // directory name (timestamp used as identifier)
	HasBundle   bool     // bundle/mockup.html exists
	HasHandoff  bool     // handoff/spec.md exists
	Screenshots []string // filenames inside screenshots/
}

// DesignGalleryData is the template data for the designs gallery page.
type DesignGalleryData struct {
	Sessions  []DesignSession
	SessionID string
	PublicURL string
}

// handleDesignGallery lists all design sessions from project-scoped dirs.
// Scans ~/.claudio/projects/*/designs/ for all projects.
func (ws *WebServer) handleDesignGallery(w http.ResponseWriter, r *http.Request) {
	projectsDir := config.GetPaths().Projects

	var sessions []DesignSession

	// Walk all project dirs, collect design sessions from each.
	projectEntries, _ := os.ReadDir(projectsDir)
	for _, proj := range projectEntries {
		if !proj.IsDir() {
			continue
		}
		designsDir := filepath.Join(projectsDir, proj.Name(), "designs")
		entries, err := os.ReadDir(designsDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			id := e.Name()
			sessionDir := filepath.Join(designsDir, id)

			ds := DesignSession{ID: proj.Name() + "/" + id}

			if _, err := os.Stat(filepath.Join(sessionDir, "bundle", "mockup.html")); err == nil {
				ds.HasBundle = true
			}
			if _, err := os.Stat(filepath.Join(sessionDir, "handoff", "spec.md")); err == nil {
				ds.HasHandoff = true
			}
			if ssEntries, err := os.ReadDir(filepath.Join(sessionDir, "screenshots")); err == nil {
				for _, se := range ssEntries {
					if !se.IsDir() && strings.HasSuffix(strings.ToLower(se.Name()), ".png") {
						ds.Screenshots = append(ds.Screenshots, se.Name())
					}
				}
			}

			sessions = append(sessions, ds)
		}
	}

	// Newest first (session IDs are timestamps).
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ID > sessions[j].ID
	})

	templ.Handler(Designs(DesignGalleryData{Sessions: sessions, PublicURL: ws.publicURL})).ServeHTTP(w, r)
}

// handleDesignStatic serves static assets (screenshots, etc.) from the designs dir.
// Path traversal is prevented by verifying the resolved path stays under designsDir.
func (ws *WebServer) handleDesignStatic(w http.ResponseWriter, r *http.Request) {
	designsDir := config.GetPaths().Designs
	id := r.PathValue("id")
	rest := r.PathValue("rest")

	// Reject any path component that looks like a traversal attempt early.
	if strings.Contains(id, "..") || strings.Contains(rest, "..") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	fp := filepath.Join(designsDir, id, rest)
	cleaned := filepath.Clean(designsDir) + string(os.PathSeparator)
	if !strings.HasPrefix(fp, cleaned) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	http.ServeFile(w, r, fp)
}

// handleDesignProject serves static assets from project-scoped design dirs.
// Route: GET /designs/project/{slug}/{id}/{rest...}
// Serves from: ~/.claudio/projects/{slug}/designs/{id}/{rest}
// Path traversal is prevented identically to handleDesignStatic.
func (ws *WebServer) handleDesignProject(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	id := r.PathValue("id")
	rest := r.PathValue("rest")

	if strings.Contains(slug, "..") || strings.Contains(id, "..") || strings.Contains(rest, "..") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	projectsDir := config.GetPaths().Projects
	designsDir := filepath.Join(projectsDir, slug, "designs")
	fp := filepath.Join(designsDir, id, rest)
	cleaned := filepath.Clean(designsDir) + string(os.PathSeparator)
	if !strings.HasPrefix(fp, cleaned) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	http.ServeFile(w, r, fp)
}

// envelopeToMessage converts a UIEvent envelope to a displayable Message-like struct.
func envelopeToMessage(ev cc.UIEvent) *cc.Message {
	now := time.Now()
	switch ev.Envelope.Type {
	case attach.EventMsgAssistant:
		var p attach.AssistantMsgPayload
		if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
			return nil
		}
		return &cc.Message{
			SessionID: ev.SessionID,
			Role:      "assistant",
			Content:   p.Content,
			AgentName: p.AgentName,
			CreatedAt: now,
		}
	case attach.EventMsgToolUse:
		var p attach.ToolUsePayload
		if err := ev.Envelope.UnmarshalPayload(&p); err != nil {
			return nil
		}
		content := p.Tool
		if len(p.Input) > 0 && string(p.Input) != "null" {
			content = p.Tool + ": " + string(p.Input)
		}
		return &cc.Message{
			SessionID: ev.SessionID,
			Role:      "tool_use",
			Content:   content,
			AgentName: p.AgentName,
			ToolUseID: p.ID,
			CreatedAt: now,
		}
	}
	return nil
}
