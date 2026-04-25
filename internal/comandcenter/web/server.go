// Package web provides the browser UI for ComandCenter.
package web

import (
	"bytes"
	"embed"
	"html"
	"regexp"
	"strings"
	"sync"
	"time"

	cc "github.com/Abraxas-365/claudio/internal/comandcenter"
	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/tasks"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
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
	result := string(mdPolicy.SanitizeBytes(buf.Bytes()))
	result = strings.ReplaceAll(result, "<table>", `<div class="md-table-wrap"><table>`)
	result = strings.ReplaceAll(result, "</table>", `</table></div>`)
	return result
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

// MessagePagination carries pagination state for message lists.
type MessagePagination struct {
	HasMore        bool   // true when older messages exist
	FirstMessageID string // ID of the oldest message in the current page
	SessionID      string // session ID for building load-more URL
}

// sessionEntry holds a session token's metadata.
type sessionEntry struct {
	csrfToken string
	expiresAt time.Time
}

// sessionTTL is how long a session token remains valid.
const sessionTTL = 24 * time.Hour

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
	version          string

	mu      sync.RWMutex
	clients map[*uiClient]struct{}
	done    chan struct{} // closed on Close() to stop fanout goroutine

	sessionMu sync.RWMutex
	sessions  map[string]sessionEntry // token → entry
}

// NewWebServer creates a WebServer. uploadsDir is the base directory for uploaded files.
func NewWebServer(storage *cc.Storage, hub *cc.Hub, password, uploadsDir string) *WebServer {
	ws := &WebServer{
		storage:  storage,
		hub:      hub,
		sessions: make(map[string]sessionEntry),
		password: password,
		clients:    make(map[*uiClient]struct{}),
		uploadsDir: uploadsDir,
		done:       make(chan struct{}),
	}
	go ws.fanout()
	return ws
}

// Close shuts down the fanout goroutine and releases resources.
func (ws *WebServer) Close() {
	close(ws.done)
}

