package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
	authpkg "github.com/Abraxas-365/claudio/internal/auth"
	authstorage "github.com/Abraxas-365/claudio/internal/auth/storage"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/query"
	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/web/templates"
)

// SessionState represents the lifecycle state of a session.
type SessionState string

const (
	StateIdle      SessionState = "idle"
	StateStreaming  SessionState = "streaming"
	StateApproval  SessionState = "approval"
)

// ProjectSession holds a live Claudio session for a project.
type ProjectSession struct {
	ID          string
	Title       string
	ProjectPath string
	Client      *api.Client
	Registry    *tools.Registry
	System      string
	Messages    []templates.ChatMessage
	Active      bool
	CreatedAt   time.Time
	LastUsed    time.Time
	State       SessionState

	// Analytics (accumulated across messages)
	TotalInputTokens  int
	TotalOutputTokens int
	CacheReadTokens   int
	CacheCreateTokens int

	// currentHandler is set when a query is running; nil otherwise.
	currentHandler *WebHandler
	// engine is reused across messages to keep conversation context.
	engine *query.Engine

	// subscribers are notified of session state changes.
	subscribers map[chan SessionEvent]bool

	mu     sync.Mutex
	cancel context.CancelFunc
}

// SessionEvent is pushed to subscribers when session state changes.
type SessionEvent struct {
	Type      string      `json:"type"`
	SessionID string      `json:"session_id"`
	Data      interface{} `json:"data,omitempty"`
}

// SessionInfo is a lightweight summary for listing sessions.
type SessionInfo struct {
	ID        string       `json:"id"`
	Title     string       `json:"title"`
	Project   string       `json:"project"`
	State     SessionState `json:"state"`
	CreatedAt time.Time    `json:"created_at"`
	LastUsed  time.Time    `json:"last_used"`
	MsgCount  int          `json:"msg_count"`
}

// SessionManager manages multiple project sessions.
type SessionManager struct {
	sessions map[string]*ProjectSession // key = session ID
	mu       sync.RWMutex
}

// NewSessionManager creates a new session manager.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*ProjectSession),
	}
}

func generateSessionID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Create creates a new session for the given project path.
func (sm *SessionManager) Create(projectPath, title string) (*ProjectSession, error) {
	sess, err := sm.newSession(projectPath, title)
	if err != nil {
		return nil, err
	}
	sm.mu.Lock()
	sm.sessions[sess.ID] = sess
	sm.mu.Unlock()
	return sess, nil
}

// Get returns a session by ID, or nil.
func (sm *SessionManager) Get(sessionID string) *ProjectSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[sessionID]
}

// GetOrCreateDefault returns the most recent session for a project, or creates one.
func (sm *SessionManager) GetOrCreateDefault(projectPath string) (*ProjectSession, error) {
	sm.mu.RLock()
	var latest *ProjectSession
	for _, s := range sm.sessions {
		if s.ProjectPath == projectPath {
			if latest == nil || s.LastUsed.After(latest.LastUsed) {
				latest = s
			}
		}
	}
	sm.mu.RUnlock()

	if latest != nil {
		return latest, nil
	}
	return sm.Create(projectPath, "Session 1")
}

// ListByProject returns all sessions for a project, sorted by last used (most recent first).
func (sm *SessionManager) ListByProject(projectPath string) []SessionInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var infos []SessionInfo
	for _, s := range sm.sessions {
		if s.ProjectPath == projectPath {
			s.mu.Lock()
			infos = append(infos, SessionInfo{
				ID:        s.ID,
				Title:     s.Title,
				Project:   s.ProjectPath,
				State:     s.State,
				CreatedAt: s.CreatedAt,
				LastUsed:  s.LastUsed,
				MsgCount:  len(s.Messages),
			})
			s.mu.Unlock()
		}
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].LastUsed.After(infos[j].LastUsed)
	})
	return infos
}

// ListProjects returns all unique project paths that have sessions.
func (sm *SessionManager) ListProjects() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	seen := make(map[string]bool)
	var paths []string
	for _, s := range sm.sessions {
		if !seen[s.ProjectPath] {
			seen[s.ProjectPath] = true
			paths = append(paths, s.ProjectPath)
		}
	}
	return paths
}

// Delete removes a session by ID.
func (sm *SessionManager) Delete(sessionID string) {
	sm.mu.Lock()
	if sess, ok := sm.sessions[sessionID]; ok {
		sess.mu.Lock()
		if sess.cancel != nil {
			sess.cancel()
		}
		// Close all subscriber channels.
		for ch := range sess.subscribers {
			close(ch)
		}
		sess.subscribers = nil
		sess.mu.Unlock()
		delete(sm.sessions, sessionID)
	}
	sm.mu.Unlock()
}

func (sm *SessionManager) newSession(projectPath, title string) (*ProjectSession, error) {
	// Validate path exists
	info, err := os.Stat(projectPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("invalid project path: %s", projectPath)
	}

	// Load config for project
	settings, err := config.Load(projectPath)
	if err != nil {
		settings = config.DefaultSettings()
	}

	// Create auth resolver + API client
	store := authstorage.NewDefaultStorage()
	resolver := authpkg.NewResolver(store)

	var apiOpts []api.ClientOption
	if settings.Model != "" {
		apiOpts = append(apiOpts, api.WithModel(settings.Model))
	}
	if settings.APIBaseURL != "" {
		apiOpts = append(apiOpts, api.WithBaseURL(settings.APIBaseURL))
	}
	client := api.NewClient(resolver, apiOpts...)

	// Create tool registry with all core tools
	registry := tools.DefaultRegistry()

	// Build system prompt scoped to the project directory
	oldDir, _ := os.Getwd()
	os.Chdir(projectPath)
	systemPrompt := prompts.BuildSystemPrompt(settings.Model, "")
	os.Chdir(oldDir)

	_, cancel := context.WithCancel(context.Background())

	id := generateSessionID()
	if title == "" {
		title = "New Session"
	}

	return &ProjectSession{
		ID:          id,
		Title:       title,
		ProjectPath: projectPath,
		Client:      client,
		Registry:    registry,
		System:      systemPrompt,
		Messages:    []templates.ChatMessage{},
		Active:      true,
		CreatedAt:   time.Now(),
		LastUsed:    time.Now(),
		State:       StateIdle,
		subscribers: make(map[chan SessionEvent]bool),
		cancel:      cancel,
	}, nil
}

// Subscribe returns a channel that receives session state events.
func (ps *ProjectSession) Subscribe() chan SessionEvent {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ch := make(chan SessionEvent, 64)
	if ps.subscribers == nil {
		ps.subscribers = make(map[chan SessionEvent]bool)
	}
	ps.subscribers[ch] = true
	return ch
}

// Unsubscribe removes a subscriber channel.
func (ps *ProjectSession) Unsubscribe(ch chan SessionEvent) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.subscribers, ch)
}

func (ps *ProjectSession) broadcast(evt SessionEvent) {
	for ch := range ps.subscribers {
		select {
		case ch <- evt:
		default:
		}
	}
}

// SendMessage creates a new handler, sets up the engine, runs the query in background,
// and returns the handler so the SSE endpoint can stream events.
func (ps *ProjectSession) SendMessage(ctx context.Context, userMessage string) (*WebHandler, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	handler := NewWebHandler()
	ps.currentHandler = handler
	ps.State = StateStreaming
	ps.LastUsed = time.Now()

	// Create engine on first message, or swap handler on subsequent ones
	if ps.engine == nil {
		ps.engine = query.NewEngineWithConfig(ps.Client, ps.Registry, handler, query.EngineConfig{
			PermissionMode: "headless",
		})
		ps.engine.SetSystem(ps.System)
	} else {
		ps.engine.SetHandler(handler)
	}

	ps.broadcast(SessionEvent{Type: "session_state", SessionID: ps.ID, Data: "streaming"})

	// Run in background — events stream to the handler's channel
	go func() {
		if err := ps.engine.Run(ctx, userMessage); err != nil {
			handler.OnError(err)
		}
		ps.mu.Lock()
		ps.State = StateIdle
		ps.mu.Unlock()
		ps.broadcast(SessionEvent{Type: "session_state", SessionID: ps.ID, Data: "idle"})
	}()

	return handler, nil
}

// CurrentHandler returns the current active handler, or nil.
func (ps *ProjectSession) CurrentHandler() *WebHandler {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.currentHandler
}

// AddMessage adds a chat message to the session history.
func (ps *ProjectSession) AddMessage(role, content string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.Messages = append(ps.Messages, templates.ChatMessage{
		Role:    role,
		Content: content,
	})
}

// Info returns a lightweight session summary.
func (ps *ProjectSession) Info() SessionInfo {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return SessionInfo{
		ID:        ps.ID,
		Title:     ps.Title,
		Project:   ps.ProjectPath,
		State:     ps.State,
		CreatedAt: ps.CreatedAt,
		LastUsed:  ps.LastUsed,
		MsgCount:  len(ps.Messages),
	}
}

// Rename updates the session title.
func (ps *ProjectSession) Rename(title string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.Title = title
}
