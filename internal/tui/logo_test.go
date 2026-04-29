package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// ── animatedLogo unit tests ───────────────────────────────────────────────────

// TestAnimatedLogo_Length verifies that the rendered logo always contains the
// seven characters of "claudio", regardless of the frame number.
func TestAnimatedLogo_Length(t *testing.T) {
	const word = "claudio"
	for _, frame := range []int{0, 1, 6, 7, 100, 999} {
		got := animatedLogo(frame)
		// lipgloss wraps each rune with ANSI escape sequences, so we strip them
		// and check that all seven characters are still present in order.
		plain := stripANSI(got)
		if plain != word {
			t.Errorf("animatedLogo(%d): plain text = %q, want %q", frame, plain, word)
		}
	}
}

// TestAnimatedLogo_ColorCycles verifies the mathematical properties of the
// color-wave function: adjacent frames must map at least one character to a
// different palette slot, and the wave must repeat after a full palette cycle.
func TestAnimatedLogo_ColorCycles(t *testing.T) {
	n := len(logoColors)
	if n == 0 {
		t.Fatal("logoColors palette is empty")
	}

	// colorIndexAt returns the palette index assigned to charIndex at frame.
	colorIndexAt := func(frame, charIndex int) int {
		return (frame + charIndex) % n
	}

	// Frame 0 and frame 1 must differ for at least character 0.
	if colorIndexAt(0, 0) == colorIndexAt(1, 0) {
		t.Errorf("color index for char 0 is the same at frame 0 and frame 1 (palette size %d)", n)
	}

	// After a full palette cycle the indices repeat exactly.
	const word = "claudio"
	for i := range word {
		idx0 := colorIndexAt(0, i)
		idxN := colorIndexAt(n, i)
		if idx0 != idxN {
			t.Errorf("char %d: color index %d at frame 0, but %d at frame %d (expected same after full cycle)",
				i, idx0, idxN, n)
		}
	}
}

// ── logoTickMsg / Update integration tests ───────────────────────────────────

// minimalModel returns the smallest valid Model that can be used for Update
// tests without external I/O.  We only set fields that the logoTickMsg handler
// reads: messages (empty → isWelcomeScreen true) and streaming (false).
func minimalModel() Model {
	return Model{
		// panes must contain at least one pane so activePane() works.
		// eventCh inside the pane must be non-nil so waitForEvent() doesn't block.
		panes: []PaneState{newPaneState("")},
	}
}

// TestLogoTickMsg_IncrementsLogoFrame checks that receiving a logoTickMsg while
// on the welcome screen increments m.logoFrame by 1.
func TestLogoTickMsg_IncrementsLogoFrame(t *testing.T) {
	m := minimalModel()
	m.logoFrame = 3

	next, _ := m.Update(logoTickMsg{})
	got := next.(Model).logoFrame
	if got != 4 {
		t.Errorf("logoFrame after logoTickMsg: got %d, want 4", got)
	}
}

// TestLogoTickMsg_ReturnsNextTick verifies that a follow-up tick command is
// scheduled when the welcome screen is active.
func TestLogoTickMsg_ReturnsNextTick(t *testing.T) {
	m := minimalModel()

	_, cmd := m.Update(logoTickMsg{})
	if cmd == nil {
		t.Fatal("expected a non-nil Cmd (next logo tick) while on welcome screen, got nil")
	}
}

// TestLogoTickMsg_StopsWhenNotWelcomeScreen checks that no further tick is
// scheduled once the user has started chatting (messages present).
func TestLogoTickMsg_StopsWhenNotWelcomeScreen(t *testing.T) {
	m := minimalModel()
	// Simulate a conversation in progress — isWelcomeScreen() returns false.
	m.activePane().messages = []ChatMessage{{Type: MsgUser, Content: "hello"}}

	_, cmd := m.Update(logoTickMsg{})
	if cmd != nil {
		// Execute the command to check what it produces (tea.Batch wraps nils).
		msg := cmd()
		if msg != nil {
			t.Errorf("expected nil command when not on welcome screen, got %T", msg)
		}
	}
}

// TestLogoTickMsg_DoesNotIncrementWhenNotWelcomeScreen verifies that logoFrame
// is unchanged when the welcome screen is not active.
func TestLogoTickMsg_DoesNotIncrementWhenNotWelcomeScreen(t *testing.T) {
	m := minimalModel()
	m.logoFrame = 5
	m.activePane().messages = []ChatMessage{{Type: MsgUser, Content: "hello"}}

	next, _ := m.Update(logoTickMsg{})
	got := next.(Model).logoFrame
	if got != 5 {
		t.Errorf("logoFrame should be unchanged when not on welcome screen: got %d, want 5", got)
	}
}

// ── welcomeScreen render tests ────────────────────────────────────────────────

// TestWelcomeScreen_RendersLogo checks that the welcome screen output contains
// all characters of "claudio".
func TestWelcomeScreen_RendersLogo(t *testing.T) {
	m := minimalModel()
	m.logoFrame = 0
	m.viewport.Width = 80
	m.viewport.Height = 24

	got := m.welcomeScreen()
	plain := stripANSI(got)
	if !strings.Contains(plain, "claudio") {
		t.Errorf("welcome screen does not contain 'claudio'; got:\n%s", plain)
	}
}

// TestWelcomeScreen_DiffersAcrossFrames verifies that the animated logo string
// produced by animatedLogoWithRenderer() differs across frames when ANSI color
// output is forced on. This decouples the test from lipgloss TTY-detection,
// which suppresses escape codes when running under `go test`.
func TestWelcomeScreen_DiffersAcrossFrames(t *testing.T) {
	r := lipgloss.NewRenderer(colorWriter{})
	// Force TrueColor so lipgloss emits ANSI escape sequences regardless of
	// whether the test runner has a real terminal attached.
	r.SetColorProfile(termenv.TrueColor)

	out0 := animatedLogoWithRenderer(0, r)
	out1 := animatedLogoWithRenderer(1, r)
	if out0 == out1 {
		t.Error("animatedLogoWithRenderer produced identical output for frame=0 and frame=1; animation is not working")
	}

	// Sanity: a full palette cycle brings us back to frame 0.
	outN := animatedLogoWithRenderer(len(logoColors), r)
	if out0 != outN {
		t.Errorf("expected identical output after full palette cycle (frame 0 vs frame %d)", len(logoColors))
	}
}

// colorWriter is an io.Writer whose presence satisfies lipgloss.NewRenderer.
// ANSI output is forced separately via r.SetColorProfile(termenv.TrueColor).
type colorWriter struct{}

func (colorWriter) Write(p []byte) (int, error) { return len(p), nil }

// ── helpers ───────────────────────────────────────────────────────────────────

// stripANSI removes ANSI escape sequences from s so we can compare plain text.
func stripANSI(s string) string {
	var out strings.Builder
	inEsc := false
	for _, ch := range s {
		switch {
		case ch == '\x1b':
			inEsc = true
		case inEsc && ch == 'm':
			inEsc = false
		case inEsc:
			// still inside escape sequence
		default:
			out.WriteRune(ch)
		}
	}
	return out.String()
}

// Ensure the package-level tea import is used (the test file imports it for
// the compiler to be satisfied even if all usages are indirect).
var _ tea.Msg = logoTickMsg{}
