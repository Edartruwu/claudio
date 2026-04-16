package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
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
	"github.com/Abraxas-365/claudio/internal/session"
	"github.com/Abraxas-365/claudio/internal/storage"
	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/web/templates"
)

// SessionState represents the lifecycle state of a session.
type SessionState string

const (
	StateIdle     SessionState = "idle"
	StateStreaming SessionState = "streaming"
	StateApproval SessionState = "approval"
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
	AgentType   string // e.g. "general-purpose", "Explore"
	TeamTemplate string // e.g. "backend-team", optional template name

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

	// db is the shared SQLite database for message persistence.
	db *storage.DB

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
	db       *storage.DB
	mu       sync.RWMutex
}

// NewSessionManager creates a new session manager backed by a shared DB.
// db may be nil, in which case sessions are in-memory only.
func NewSessionManager(db *storage.DB) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*ProjectSession),
		db:       db,
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

// Get returns a session by ID, or nil. If the session is not in memory but
// exists in the DB, it will be loaded.
func (sm *SessionManager) Get(sessionID string) *ProjectSession {
	sm.mu.RLock()
	sess := sm.sessions[sessionID]
	sm.mu.RUnlock()
	if sess != nil {
		return sess
	}

	// Try loading from DB
	if sm.db == nil {
		return nil
	}
	dbSess, err := sm.db.GetSession(sessionID)
	if err != nil || dbSess == nil {
		return nil
	}
	loaded, err := sm.loadDBSession(dbSess)
	if err != nil {
		return nil
	}
	sm.mu.Lock()
	sm.sessions[loaded.ID] = loaded
	sm.mu.Unlock()
	return loaded
}

// GetOrCreateDefault returns the most recent session for a project, or creates one.
// It first checks in-memory sessions, then falls back to DB-persisted sessions.
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

	// Try loading from DB
	if sm.db != nil {
		dbSessions, err := sm.db.ListSessionsByProject(projectPath, 1)
		if err == nil && len(dbSessions) > 0 {
			loaded, err := sm.loadDBSession(&dbSessions[0])
			if err == nil {
				sm.mu.Lock()
				sm.sessions[loaded.ID] = loaded
				sm.mu.Unlock()
				return loaded, nil
			}
		}
	}

	return sm.Create(projectPath, "Session 1")
}

// ListByProject returns all sessions for a project, sorted by last used (most recent first).
// It merges in-memory sessions with lightweight DB metadata (no heavy engine setup).
func (sm *SessionManager) ListByProject(projectPath string) []SessionInfo {
	sm.mu.RLock()
	// Collect in-memory sessions
	inMemory := make(map[string]bool)
	var infos []SessionInfo
	for _, s := range sm.sessions {
		if s.ProjectPath == projectPath {
			inMemory[s.ID] = true
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
	sm.mu.RUnlock()

	// Merge DB sessions that aren't already in memory (lightweight metadata only)
	if sm.db != nil {
		dbSessions, err := sm.db.ListSessionsByProject(projectPath, 50)
		if err == nil {
			for _, ds := range dbSessions {
				if inMemory[ds.ID] {
					continue
				}
				title := ds.Title
				if title == "" {
					title = ds.ID
				}
				// Count messages from DB
				msgCount := 0
				msgs, err := sm.db.GetMessages(ds.ID)
				if err == nil {
					for _, m := range msgs {
						if m.Type == "user" || m.Type == "assistant" {
							msgCount++
						}
					}
				}
				infos = append(infos, SessionInfo{
					ID:        ds.ID,
					Title:     title,
					Project:   ds.ProjectDir,
					State:     StateIdle, // DB sessions are always idle until resumed
					CreatedAt: ds.CreatedAt,
					LastUsed:  ds.UpdatedAt,
					MsgCount:  msgCount,
				})
			}
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

	// Also delete from DB
	if sm.db != nil {
		sm.db.DeleteSession(sessionID)
	}
}

// loadDBSession converts a storage.Session from the DB into a live ProjectSession
// with its message history hydrated for display.
func (sm *SessionManager) loadDBSession(dbSess *storage.Session) (*ProjectSession, error) {
	projectPath := dbSess.ProjectDir

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

	model := dbSess.Model
	if model == "" && settings.Model != "" {
		model = settings.Model
	}

	var apiOpts []api.ClientOption
	apiOpts = append(apiOpts, api.WithStorage(store))
	if model != "" {
		apiOpts = append(apiOpts, api.WithModel(model))
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
	systemPrompt := prompts.BuildSystemPrompt(model, "", settings.CavemanEnabled())
	os.Chdir(oldDir)

	_, cancel := context.WithCancel(context.Background())

	title := dbSess.Title
	if title == "" {
		title = dbSess.ID
	}

	ps := &ProjectSession{
		ID:          dbSess.ID,
		Title:       title,
		ProjectPath: projectPath,
		Client:      client,
		Registry:    registry,
		System:      systemPrompt,
		Messages:    []templates.ChatMessage{},
		Active:      true,
		CreatedAt:   dbSess.CreatedAt,
		LastUsed:    dbSess.UpdatedAt,
		State:       StateIdle,
		subscribers: make(map[chan SessionEvent]bool),
		db:          sm.db,
		cancel:      cancel,
	}

	// Load message history from DB for display
	if sm.db != nil {
		storedMsgs, err := sm.db.GetMessages(dbSess.ID)
		if err == nil && len(storedMsgs) > 0 {
			// Pair tool_use with its subsequent tool_result for display
			type pendingTool struct {
				name  string
				input string
			}
			var pending *pendingTool
			for _, msg := range storedMsgs {
				switch msg.Type {
				case "user":
					pending = nil
					if msg.Content != "" {
						ps.Messages = append(ps.Messages, templates.ChatMessage{
							Role:    "user",
							Content: msg.Content,
						})
					}
				case "assistant":
					pending = nil
					if msg.Content != "" {
						ps.Messages = append(ps.Messages, templates.ChatMessage{
							Role:    "assistant",
							Content: msg.Content,
						})
					}
				case "tool_use":
					pending = &pendingTool{name: msg.ToolName, input: msg.Content}
				case "tool_result":
					name := ""
					input := ""
					if pending != nil {
						name = pending.name
						input = pending.input
						pending = nil
					}
					ps.Messages = append(ps.Messages, templates.ChatMessage{
						Role:     "tool",
						ToolName: name,
						Content:  input,
						ToolOut:  msg.Content,
					})
				}
			}

			// Restore engine conversation history so the model has full context
			engineMsgs := session.ReconstructEngineMessages(storedMsgs)
			handler := NewWebHandler()
			ps.engine = query.NewEngineWithConfig(client, registry, handler, query.EngineConfig{
				SessionID:      dbSess.ID,
				PermissionMode: "headless",
			})
			ps.engine.SetSystem(systemPrompt)
			ps.engine.SetMessages(engineMsgs)
		}
	}

	return ps, nil
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
	apiOpts = append(apiOpts, api.WithStorage(store))
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
	systemPrompt := prompts.BuildSystemPrompt(settings.Model, "", settings.CavemanEnabled())
	os.Chdir(oldDir)

	_, cancel := context.WithCancel(context.Background())

	// Persist to DB if available
	var id string
	if sm.db != nil {
		model := settings.Model
		if model == "" {
			model = "claude-sonnet-4-6"
		}
		dbSess, err := sm.db.CreateSession(projectPath, model)
		if err != nil {
			log.Printf("Warning: failed to persist session to DB: %v", err)
			id = generateSessionID()
		} else {
			id = dbSess.ID
			if title != "" {
				sm.db.UpdateSessionTitle(id, title)
			}
		}
	} else {
		id = generateSessionID()
	}

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
		db:          sm.db,
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
func (ps *ProjectSession) SendMessage(ctx context.Context, userMessage string, images []api.UserContentBlock) (*WebHandler, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	handler := NewWebHandler()
	ps.currentHandler = handler
	ps.State = StateStreaming
	ps.LastUsed = time.Now()

	// Create engine on first message, or swap handler on subsequent ones
	if ps.engine == nil {
		ps.engine = query.NewEngineWithConfig(ps.Client, ps.Registry, handler, query.EngineConfig{
			SessionID:      ps.ID,
			PermissionMode: "headless",
		})
		ps.engine.SetSystem(ps.System)
	} else {
		ps.engine.SetHandler(handler)
	}

	// Persist user message to DB
	if ps.db != nil {
		ps.db.AddMessage(ps.ID, "user", userMessage, "user", "", "")
	}

	ps.broadcast(SessionEvent{Type: "session_state", SessionID: ps.ID, Data: "streaming"})

	// Run in background — events stream to the handler's channel
	sessionID := ps.ID
	db := ps.db
	go func() {
		var err error
		if len(images) > 0 {
			err = ps.engine.RunWithImages(ctx, userMessage, images)
		} else {
			err = ps.engine.Run(ctx, userMessage)
		}
		if err != nil {
			handler.OnError(err)
		}

		// Persist assistant response to DB
		if db != nil {
			ps.persistNewMessages(db, sessionID)
		}

		ps.mu.Lock()
		ps.State = StateIdle
		ps.mu.Unlock()
		ps.broadcast(SessionEvent{Type: "session_state", SessionID: sessionID, Data: "idle"})
	}()

	return handler, nil
}

// persistNewMessages saves new engine messages to DB that weren't already persisted.
// It looks at the engine's message history and persists the last assistant turn
// (assistant text + tool_use + tool_result messages).
func (ps *ProjectSession) persistNewMessages(db *storage.DB, sessionID string) {
	if ps.engine == nil {
		return
	}
	msgs := ps.engine.Messages()
	if len(msgs) == 0 {
		return
	}

	// Walk backwards from the end to find the last assistant message and any
	// following tool_result user message. The user message was already persisted
	// in SendMessage, so we only need to persist assistant content.
	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]
		if msg.Role == "assistant" {
			// Parse content blocks and persist each
			var blocks []struct {
				Type  string          `json:"type"`
				Text  string          `json:"text,omitempty"`
				ID    string          `json:"id,omitempty"`
				Name  string          `json:"name,omitempty"`
				Input json.RawMessage `json:"input,omitempty"`
			}
			if err := json.Unmarshal(msg.Content, &blocks); err != nil {
				// Single text response
				db.AddMessage(sessionID, "assistant", string(msg.Content), "assistant", "", "")
				break
			}
			for _, block := range blocks {
				switch block.Type {
				case "text":
					db.AddMessage(sessionID, "assistant", block.Text, "assistant", "", "")
				case "tool_use":
					inputStr := "{}"
					if len(block.Input) > 0 {
						inputStr = string(block.Input)
					}
					db.AddMessage(sessionID, "assistant", inputStr, "tool_use", block.ID, block.Name)
				}
			}
			// Also persist any tool_result messages that follow
			if i+1 < len(msgs) && msgs[i+1].Role == "user" {
				var resultBlocks []struct {
					Type      string `json:"type"`
					ToolUseID string `json:"tool_use_id"`
					Content   string `json:"content"`
				}
				if err := json.Unmarshal(msgs[i+1].Content, &resultBlocks); err == nil {
					for _, rb := range resultBlocks {
						if rb.Type == "tool_result" {
							db.AddMessage(sessionID, "user", rb.Content, "tool_result", rb.ToolUseID, "")
						}
					}
				}
			}
			break
		}
	}
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
	// Persist to DB
	if ps.db != nil {
		ps.db.UpdateSessionTitle(ps.ID, title)
	}
}
