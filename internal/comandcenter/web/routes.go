package web

import (
	"io/fs"
	"net/http"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/tasks"
)

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
	mux.Handle("GET /settings", ws.uiAuth(http.HandlerFunc(ws.handleSettings)))
	mux.Handle("GET /chat/{session_id}", ws.uiAuth(http.HandlerFunc(ws.handleChatView)))
	mux.Handle("GET /chat/{session_id}/info", ws.uiAuth(http.HandlerFunc(ws.handleSessionInfo)))
	mux.Handle("GET /chat/{session_id}/tasks", ws.uiAuth(http.HandlerFunc(ws.handleTaskList)))
	mux.Handle("GET /chat/{session_id}/tasks/{task_id}", ws.uiAuth(http.HandlerFunc(ws.handleTaskDetail)))
	mux.Handle("GET /partials/sessions", ws.uiAuth(http.HandlerFunc(ws.handlePartialSessions)))
	mux.Handle("GET /partials/messages/{session_id}", ws.uiAuth(http.HandlerFunc(ws.handlePartialMessages)))
	mux.Handle("POST /api/sessions/{session_id}/message", ws.uiAuth(http.HandlerFunc(ws.handleSendMessage)))
	mux.Handle("POST /api/sessions/by-name/{name}/message", ws.uiAuth(http.HandlerFunc(ws.handleSendMessageByName)))
	mux.Handle("GET /api/session-lookup/{name}", ws.uiAuth(http.HandlerFunc(ws.handleSessionLookupByName)))
	mux.Handle("GET /api/sessions/list", ws.uiAuth(http.HandlerFunc(ws.handleAPISessions)))
	mux.Handle("POST /api/sessions/{session_id}/upload", ws.uiAuth(http.HandlerFunc(ws.handleUpload)))
	mux.Handle("GET /uploads/{session_id}/{filename}", ws.uiAuth(http.HandlerFunc(ws.handleServeFile)))
	mux.Handle("GET /ws/ui", ws.uiAuth(http.HandlerFunc(ws.handleWSUI)))

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

	// Agent detail screen.
	mux.Handle("GET /chat/{session_id}/agent/{agent_id}", ws.uiAuth(http.HandlerFunc(ws.handleAgentDetail)))

	// Agent log streaming partial.
	mux.Handle("GET /chat/{session_id}/agents/{agent_name}/logs", ws.uiAuth(http.HandlerFunc(ws.handleAgentLogs)))

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

// SetPublicURL sets the externally accessible URL for bundle link construction.
func (ws *WebServer) SetPublicURL(url string) { ws.publicURL = url }

// SetVersion stores the build version shown on the settings page.
func (ws *WebServer) SetVersion(v string) { ws.version = v }
