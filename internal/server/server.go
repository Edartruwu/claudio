// Package server provides a headless HTTP API for Claudio,
// enabling IDE integration and remote access via the --headless flag.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/query"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// Config holds server configuration.
type Config struct {
	Port     int
	Host     string
	APIClient *api.Client
	Registry  *tools.Registry
	System    string // system prompt
}

// Server is the headless HTTP API server.
type Server struct {
	config Config
	mux    *http.ServeMux
	server *http.Server
	mu     sync.Mutex
	engine *query.Engine
}

// New creates a new headless server.
func New(cfg Config) *Server {
	s := &Server{config: cfg}
	s.mux = http.NewServeMux()
	s.registerRoutes()

	handler := &query.StdoutHandler{Verbose: false}
	s.engine = query.NewEngine(cfg.APIClient, cfg.Registry, handler)
	if cfg.System != "" {
		s.engine.SetSystem(cfg.System)
	}

	return s
}

// Start begins serving HTTP requests.
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second, // Long for streaming
		IdleTimeout:  60 * time.Second,
	}

	fmt.Printf("Claudio headless server listening on %s\n", addr)
	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("POST /v1/messages", s.handleMessages)
	s.mux.HandleFunc("GET /v1/tools", s.handleListTools)
	s.mux.HandleFunc("POST /v1/tools/{name}", s.handleInvokeTool)
	s.mux.HandleFunc("GET /v1/health", s.handleHealth)
	s.mux.HandleFunc("GET /v1/status", s.handleStatus)
}

// --- Handlers ---

type messageRequest struct {
	Message string `json:"message"`
	Stream  bool   `json:"stream,omitempty"`
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	var req messageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, `{"error":"message required"}`, http.StatusBadRequest)
		return
	}

	if req.Stream {
		s.handleStreamingMessage(w, r, req.Message)
		return
	}

	// Non-streaming: collect full response
	s.mu.Lock()
	collector := &responseCollector{}
	engine := query.NewEngine(s.config.APIClient, s.config.Registry, collector)
	engine.SetSystem(s.config.System)
	engine.SetMessages(s.engine.Messages()) // share context
	s.mu.Unlock()

	ctx := r.Context()
	if err := engine.Run(ctx, req.Message); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Update shared engine state
	s.mu.Lock()
	s.engine.SetMessages(engine.Messages())
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response": collector.text.String(),
		"usage": map[string]int{
			"input_tokens":  collector.usage.InputTokens,
			"output_tokens": collector.usage.OutputTokens,
		},
	})
}

func (s *Server) handleStreamingMessage(w http.ResponseWriter, r *http.Request, message string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	streamer := &sseHandler{w: w, flusher: flusher}

	s.mu.Lock()
	engine := query.NewEngine(s.config.APIClient, s.config.Registry, streamer)
	engine.SetSystem(s.config.System)
	engine.SetMessages(s.engine.Messages())
	s.mu.Unlock()

	ctx := r.Context()
	if err := engine.Run(ctx, message); err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
	}

	s.mu.Lock()
	s.engine.SetMessages(engine.Messages())
	s.mu.Unlock()

	fmt.Fprintf(w, "event: done\ndata: {}\n\n")
	flusher.Flush()
}

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	toolList := s.config.Registry.All()
	var result []map[string]string
	for _, t := range toolList {
		result = append(result, map[string]string{
			"name":        t.Name(),
			"description": truncate(t.Description(), 200),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleInvokeTool(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	tool, err := s.config.Registry.Get(name)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"tool not found: %s"}`, name), http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}

	result, err := tool.Execute(r.Context(), body)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	msgCount := len(s.engine.Messages())
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "running",
		"messages": msgCount,
		"tools":    len(s.config.Registry.All()),
	})
}

// --- Response Collectors ---

type responseCollector struct {
	text  strings.Builder
	usage api.Usage
}

func (c *responseCollector) OnTextDelta(text string)                              { c.text.WriteString(text) }
func (c *responseCollector) OnThinkingDelta(text string)                          {}
func (c *responseCollector) OnToolUseStart(tu tools.ToolUse)                      {}
func (c *responseCollector) OnToolUseEnd(tu tools.ToolUse, result *tools.Result)  {}
func (c *responseCollector) OnTurnComplete(usage api.Usage)                       { c.usage = usage }
func (c *responseCollector) OnToolApprovalNeeded(tu tools.ToolUse) bool           { return true }
func (c *responseCollector) OnCostConfirmNeeded(currentCost, threshold float64) bool { return true }
func (c *responseCollector) OnError(err error)                                    {}
func (c *responseCollector) OnRetry(_ []tools.ToolUse)                            {}

type sseHandler struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func (h *sseHandler) OnTextDelta(text string) {
	data, _ := json.Marshal(map[string]string{"text": text})
	fmt.Fprintf(h.w, "event: text\ndata: %s\n\n", data)
	h.flusher.Flush()
}

func (h *sseHandler) OnThinkingDelta(text string) {
	data, _ := json.Marshal(map[string]string{"thinking": text})
	fmt.Fprintf(h.w, "event: thinking\ndata: %s\n\n", data)
	h.flusher.Flush()
}

func (h *sseHandler) OnToolUseStart(tu tools.ToolUse) {
	data, _ := json.Marshal(map[string]string{"tool": tu.Name, "id": tu.ID})
	fmt.Fprintf(h.w, "event: tool_start\ndata: %s\n\n", data)
	h.flusher.Flush()
}

func (h *sseHandler) OnToolUseEnd(tu tools.ToolUse, result *tools.Result) {
	data, _ := json.Marshal(map[string]interface{}{"tool": tu.Name, "content": truncate(result.Content, 500), "error": result.IsError})
	fmt.Fprintf(h.w, "event: tool_end\ndata: %s\n\n", data)
	h.flusher.Flush()
}

func (h *sseHandler) OnTurnComplete(usage api.Usage) {
	data, _ := json.Marshal(map[string]int{"input_tokens": usage.InputTokens, "output_tokens": usage.OutputTokens})
	fmt.Fprintf(h.w, "event: usage\ndata: %s\n\n", data)
	h.flusher.Flush()
}

func (h *sseHandler) OnToolApprovalNeeded(tu tools.ToolUse) bool { return true }
func (h *sseHandler) OnCostConfirmNeeded(currentCost, threshold float64) bool { return true }

func (h *sseHandler) OnError(err error) {
	fmt.Fprintf(h.w, "event: error\ndata: %s\n\n", err.Error())
	h.flusher.Flush()
}

func (h *sseHandler) OnRetry(toolUses []tools.ToolUse) {
	ids := make([]string, len(toolUses))
	for i, tu := range toolUses {
		ids[i] = tu.ID
	}
	data, _ := json.Marshal(map[string]interface{}{"tool_ids": ids})
	fmt.Fprintf(h.w, "event: retry\ndata: %s\n\n", data)
	h.flusher.Flush()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
