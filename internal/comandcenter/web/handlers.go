package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	cc "github.com/Abraxas-365/claudio/internal/comandcenter"
	agentspkg "github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/services/compact"
	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/Abraxas-365/claudio/internal/teams"
	"github.com/a-h/templ"
)

// ChatViewData holds data for the chat view page.
type ChatViewData struct {
	Session   cc.Session
	Messages  []MessageView
	SessionID string
}

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
	CsrfToken string
}

// SettingsData holds data for the /settings page.
type SettingsData struct {
	SessionID    string
	CsrfToken    string
	Version      string
	SessionCount int
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

func (ws *WebServer) handleChatList(w http.ResponseWriter, r *http.Request) {
	sessions, err := ws.storage.ListSessions("")
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	rows := ws.buildSessionRows(sessions)
	templ.Handler(ChatList(ChatListData{Rows: rows, SessionID: "", CsrfToken: ws.CSRFToken(r)})).ServeHTTP(w, r)
}

func (ws *WebServer) handleSettings(w http.ResponseWriter, r *http.Request) {
	ws.sessionMu.RLock()
	sessionCount := len(ws.sessions)
	ws.sessionMu.RUnlock()

	data := SettingsData{
		SessionID:    "",
		CsrfToken:    ws.CSRFToken(r),
		Version:      ws.version,
		SessionCount: sessionCount,
	}
	templ.Handler(SettingsPage(data)).ServeHTTP(w, r)
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
	msgs, hasMore, err := ws.storage.ListMessagesPaginated(id, 50, "")
	if err != nil {
		msgs = nil
	}
	// ListMessagesPaginated returns newest first; reverse for display.
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
	pag := MessagePagination{HasMore: hasMore, SessionID: id}
	if len(views) > 0 {
		pag.FirstMessageID = views[0].ID
	}

	if r.Header.Get("HX-Request") == "true" {
		ChatView(sess, views, id, pag).Render(r.Context(), w)
		return
	}
	// Full-page render (hard refresh / direct URL): render full shell with sidebar.
	sessions, _ := ws.storage.ListSessions("")
	rows := ws.buildSessionRows(sessions)
	listData := ChatListData{Rows: rows, SessionID: id, CsrfToken: ws.CSRFToken(r)}
	templ.Handler(ChatPage(listData, sess, views, id, pag)).ServeHTTP(w, r)
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
	sessionID := r.PathValue("session_id")
	taskID := r.PathValue("task_id")
	task, err := ws.storage.GetTask(taskID, sessionID)
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
	TeamMembers(agents, id).Render(r.Context(), w)
}

// handleAgentDetail renders the full agent detail screen (screen 16).
// GET /chat/{session_id}/agent/{agent_id}
func (ws *WebServer) handleAgentDetail(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	agentID := r.PathValue("agent_id")

	agents, err := ws.storage.ListAgents(sessionID)
	if err != nil {
		agents = nil
	}

	var agent cc.Agent
	for _, a := range agents {
		if a.ID == agentID {
			agent = a
			break
		}
	}
	// If agent not found, render with a stub so the page still loads.
	if agent.ID == "" {
		agent = cc.Agent{ID: agentID, SessionID: sessionID, Name: agentID, Status: "done"}
	}

	data := AgentDetailData{
		SessionID: sessionID,
		Agent:     agent,
		Events:    nil, // pre-loaded events; real-time updates arrive via WS OOB swap
		CsrfToken: ws.CSRFToken(r),
	}

	templ.Handler(AgentDetailPage(data)).ServeHTTP(w, r)
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

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	beforeID := r.URL.Query().Get("before")

	msgs, hasMore, err := ws.storage.ListMessagesPaginated(id, limit, beforeID)
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

	pag := MessagePagination{HasMore: hasMore, SessionID: id}
	if len(views) > 0 {
		pag.FirstMessageID = views[0].ID
	}

	// When loading older messages (before= set), render just the messages + sentinel.
	// When no more messages, signal the client to stop polling.
	if beforeID != "" {
		if !hasMore && len(views) == 0 {
			w.Header().Set("HX-Reswap", "none")
			w.WriteHeader(http.StatusOK)
			return
		}
		MessagesOlderPage(views, pag).Render(r.Context(), w)
		return
	}
	MessagesPartial(views).Render(r.Context(), w)
}

func (ws *WebServer) handleAgentLogs(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	agentName := r.PathValue("agent_name")
	msgs, err := ws.storage.ListMessagesByAgent(sessionID, agentName, 200)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	AgentLogs(msgs).Render(r.Context(), w)
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

	// Normal (non-@mention) path — store and push user bubble first, then forward to agent.
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
		if wsPayload, err := json.Marshal(map[string]string{
			"type": "message.user",
			"html": buf.String(),
		}); err == nil {
			ws.pushToSessionClients(sessionID, wsPayload)
		}
	}

	payload, _ := json.Marshal(attach.UserMsgPayload{Content: content, ModelOverride: modelOverride})
	env := attach.Envelope{Type: attach.EventMsgUser, Payload: payload}
	if err := ws.hub.Send(sessionID, env); err != nil {
		http.Error(w, "session not connected", http.StatusServiceUnavailable)
		return
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

// handleSessionLookupByName handles GET /api/sessions/by-name/{name}.
// Returns {"id":"..."} for the most recent session with the given name, or 404.
func (ws *WebServer) handleSessionLookupByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	sess, found, err := ws.storage.GetSessionByName(name)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": sess.ID})
}

// handleCompact runs the compact service on the session's message history,
// replaces DB messages with compacted ones, and pushes a confirmation bubble.
func (ws *WebServer) handleCompact(w http.ResponseWriter, sessionID, instruction string) {
	msgs, err := ws.storage.ListMessages(sessionID, 1000)
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

		// Notify attached engine of compacted messages so in-memory state stays in sync.
		if env, err := attach.NewEnvelope(attach.EventSetMessages, attach.SetMessagesPayload{Messages: compacted}); err == nil {
			_ = ws.hub.Send(sessionID, env)
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var buf bytes.Buffer
	if err := MessageBubble(MessageView{Message: msg}).Render(ctx, &buf); err == nil {
		if p, err := json.Marshal(map[string]string{
			"type": "message.user",
			"html": buf.String(),
		}); err == nil {
			ws.pushToSessionClients(sessionID, p)
		}
	}
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

// handleDeleteAllSessions is a stub endpoint for the settings page "Clear all sessions" button.
// Full implementation is deferred; returns 501 Not Implemented.
func (ws *WebServer) handleDeleteAllSessions(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
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
		Type      string `json:"type"`
		WhenToUse string `json:"when_to_use"`
		Model     string `json:"model"`
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
		CronNotConfigured().Render(r.Context(), w)
		return
	}

	var entries []tasks.CronEntry
	for _, e := range ws.cronStore.All() {
		if e.SessionID == sessionID {
			entries = append(entries, e)
		}
	}

	if len(entries) == 0 {
		CronEmpty().Render(r.Context(), w)
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
			prompt = string([]rune(prompt)[:60]) + "…"
		}
		CronRow(CronRowData{
			Entry:  e,
			Type:   cronType,
			Prompt: prompt,
			Badge:  badgeColor,
		}).Render(r.Context(), w)
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
// Multipart form: "file" (one or more, required), "content" (optional caption).
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

	fileHeaders := r.MultipartForm.File["file"]
	if len(fileHeaders) == 0 {
		http.Error(w, "file field missing", http.StatusBadRequest)
		return
	}

	caption := strings.TrimSpace(r.FormValue("content"))
	now := time.Now()

	// One message for all files.
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

	// Ensure per-session upload directory exists.
	dir := filepath.Join(ws.uploadsDir, sessionID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	var atts []cc.Attachment
	var attachPayloads []attach.Attachment

	for _, fh := range fileHeaders {
		f, err := fh.Open()
		if err != nil {
			continue
		}

		// Detect MIME from first 512 bytes then reset reader.
		sniff := make([]byte, 512)
		n, _ := f.Read(sniff)
		mimeType := http.DetectContentType(sniff[:n])
		if ct := fh.Header.Get("Content-Type"); ct != "" && ct != "application/octet-stream" {
			mimeType = ct
		}
		if idx := strings.Index(mimeType, ";"); idx != -1 {
			mimeType = strings.TrimSpace(mimeType[:idx])
		}
		if seeker, ok := f.(io.Seeker); ok {
			seeker.Seek(0, io.SeekStart)
		}

		storedName := cc.NewID() + filepath.Ext(fh.Filename)
		dst, err := os.Create(filepath.Join(dir, storedName))
		if err != nil {
			f.Close()
			continue
		}
		size, _ := io.Copy(dst, f)
		dst.Close()
		f.Close()

		att := cc.Attachment{
			ID:           cc.NewID(),
			SessionID:    sessionID,
			MessageID:    msg.ID,
			Filename:     storedName,
			OriginalName: fh.Filename,
			MimeType:     mimeType,
			Size:         size,
			CreatedAt:    now,
		}
		if err := ws.storage.InsertAttachment(att); err != nil {
			continue
		}
		atts = append(atts, att)
		attachPayloads = append(attachPayloads, attach.Attachment{
			FilePath: filepath.Join(ws.uploadsDir, sessionID, storedName),
			MimeType: mimeType,
		})
	}

	if len(atts) == 0 {
		http.Error(w, "no files saved", http.StatusInternalServerError)
		return
	}

	// Push single bubble with all attachments.
	var buf bytes.Buffer
	view := MessageView{Message: msg, Attachments: atts}
	if err := MessageBubble(view).Render(r.Context(), &buf); err == nil {
		payload, _ := json.Marshal(map[string]string{"type": "message.user", "html": buf.String()})
		ws.pushToSessionClients(sessionID, payload)
	}

	// Forward one UserMsgPayload with all attachments.
	fwdEnv, fwdErr := attach.NewEnvelope(attach.EventMsgUser, attach.UserMsgPayload{
		Content:     caption,
		Attachments: attachPayloads,
	})
	if fwdErr == nil {
		_ = ws.hub.Send(sessionID, fwdEnv)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"count": len(atts)})
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

	templ.Handler(Designs(DesignGalleryData{Sessions: sessions, PublicURL: ws.publicURL, CsrfToken: ws.CSRFToken(r)})).ServeHTTP(w, r)
}
