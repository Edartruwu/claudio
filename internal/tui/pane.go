package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/query"
)

// PaneState holds all per-agent-session state for one split pane.
// root.go Model keeps a slice of these; the active pane is addressed
// via activePaneIdx. This extraction is the foundation for multi-pane support.
type PaneState struct {
	SessionID string

	// Title is a human-readable label for tab/split headers.
	// Set to agent name, session title, or "New Pane" as fallback.
	Title string

	// Engine + channels
	engine     *query.Engine
	engineRef  **query.Engine // optional external pointer updated whenever engine is set
	eventCh    chan tuiEvent
	approvalCh chan bool
	cancelFunc context.CancelFunc

	// Message history + streaming
	messages            []ChatMessage
	messageIDs          []int64 // storage IDs parallel to messages; 0 means no stored ID
	streaming           bool
	streamText          *strings.Builder
	streamDirty         bool
	pendingToolCount    int
	pendingPostToolText *strings.Builder // text_delta buffered while tools are in-flight
	pendingEngineMessages []api.Message

	// Stats
	totalTokens int
	totalCost   float64
	turns       int

	// UI state
	expandedGroups  map[int]bool         // tool group msg indices that are expanded
	thinkingExpanded map[int]bool        // message index → thinking block expanded state
	lastToolGroup   int                  // msg index of the last tool group start (-1 = none)
	toolStartTimes  map[string]time.Time // ToolUseID → execution start time
	spinText        string               // current spinner status text
	toolSpinFrame   int                  // braille spinner frame counter
	undoStash       []ChatMessage        // last exchange popped by /undo, restored by /redo
	messageQueue    []string             // messages queued while streaming

	// Viewport state
	viewportOffset  int       // scroll position saved when this pane is not active
	vpCursor        int       // viewport section cursor (-1 = none)
	vpSections      []Section // cached section metadata from last render

	// Viewport search
	vpSearchActive  bool
	vpSearchQuery   string
	vpSearchMatches []int
	vpSearchIdx     int

	// Message pinning
	pinnedMsgIndices map[int]bool

	// Session context
	systemPrompt  string
	userContext   string // CLAUDE.md injected as first user message
	systemContext string // git status appended to system prompt

	// Plan mode
	planModeActive     bool
	planFilePath       string
	planApprovalCursor int
	planContentCache   string

	// Model restore
	pendingModelRestore string
	resumeSummarySet    bool

	// Rate limit state
	rateLimitWarning string
	rateLimitError   string
	isUsingOverage   bool

	// AskUser dialog
	askUserDialog *askUserDialogState

	// Branch display
	branchParentTitle string
}

// newPaneState creates a PaneState with sane defaults for a new session.
func newPaneState(sessionID string) PaneState {
	return PaneState{
		SessionID:        sessionID,
		eventCh:          make(chan tuiEvent, 64),
		streamText:       &strings.Builder{},
		pendingPostToolText: &strings.Builder{},
		expandedGroups:   make(map[int]bool),
		thinkingExpanded: make(map[int]bool),
		lastToolGroup:    -1,
		toolStartTimes:   make(map[string]time.Time),
		pinnedMsgIndices: make(map[int]bool),
		vpCursor:         -1,
	}
}

// paneEventMsg wraps a tuiEvent with the session ID of the pane it originated
// from. Using session ID instead of index makes routing immune to index shifts
// that occur when a pane is closed from a multi-pane set.
type paneEventMsg struct {
	sessionID string
	event     tuiEvent
}

// waitForPaneEvent returns a tea.Cmd that blocks on a pane's event channel
// and wraps the result with the pane's session ID.
func waitForPaneEvent(sessionID string, ch chan tuiEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return nil
		}
		return paneEventMsg{sessionID: sessionID, event: event}
	}
}
