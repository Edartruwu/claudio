package web

import (
	"context"
	"fmt"
	"os"
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

// ProjectSession holds a live Claudio session for a project.
type ProjectSession struct {
	ProjectPath string
	Client      *api.Client
	Registry    *tools.Registry
	System      string
	Messages    []templates.ChatMessage
	Active      bool
	LastUsed    time.Time

	// Analytics (accumulated across messages)
	TotalInputTokens  int
	TotalOutputTokens int
	CacheReadTokens   int
	CacheCreateTokens int

	// currentHandler is set when a query is running; nil otherwise.
	currentHandler *WebHandler
	// engine is reused across messages to keep conversation context.
	engine *query.Engine

	mu     sync.Mutex
	cancel context.CancelFunc
}

// SessionManager manages multiple project sessions.
type SessionManager struct {
	sessions map[string]*ProjectSession // key = project path
	mu       sync.RWMutex
}

// NewSessionManager creates a new session manager.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*ProjectSession),
	}
}

// GetOrCreate returns an existing session or creates a new one for the project path.
func (sm *SessionManager) GetOrCreate(projectPath string) (*ProjectSession, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sess, ok := sm.sessions[projectPath]; ok {
		sess.LastUsed = time.Now()
		return sess, nil
	}

	sess, err := sm.createSession(projectPath)
	if err != nil {
		return nil, err
	}
	sm.sessions[projectPath] = sess
	return sess, nil
}

// Get returns an existing session or nil.
func (sm *SessionManager) Get(projectPath string) *ProjectSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sess, ok := sm.sessions[projectPath]; ok {
		sess.LastUsed = time.Now()
		return sess
	}
	return nil
}

// ListProjects returns all active project paths.
func (sm *SessionManager) ListProjects() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var paths []string
	for p := range sm.sessions {
		paths = append(paths, p)
	}
	return paths
}

func (sm *SessionManager) createSession(projectPath string) (*ProjectSession, error) {
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

	// Create auth resolver + API client (same pattern as app.New)
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

	return &ProjectSession{
		ProjectPath: projectPath,
		Client:      client,
		Registry:    registry,
		System:      systemPrompt,
		Messages:    []templates.ChatMessage{},
		Active:      true,
		LastUsed:    time.Now(),
		cancel:      cancel,
	}, nil
}

// SendMessage creates a new handler, sets up the engine, runs the query in background,
// and returns the handler so the SSE endpoint can stream events.
func (ps *ProjectSession) SendMessage(ctx context.Context, userMessage string) (*WebHandler, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	handler := NewWebHandler()
	ps.currentHandler = handler

	// Create engine on first message, or swap handler on subsequent ones
	if ps.engine == nil {
		ps.engine = query.NewEngineWithConfig(ps.Client, ps.Registry, handler, query.EngineConfig{
			PermissionMode: "headless",
		})
		ps.engine.SetSystem(ps.System)
	} else {
		ps.engine.SetHandler(handler)
	}

	// Run in background — events stream to the handler's channel
	go func() {
		if err := ps.engine.Run(ctx, userMessage); err != nil {
			handler.OnError(err)
		}
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
