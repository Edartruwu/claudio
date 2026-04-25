// Package web — gap-fill tests for Batch B UX changes.
// Covers: lightbox a11y, cron row, focus-visible CSS, login focus, ARIA tabs.
// Internal package test so unexported templ components are accessible.
package web

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	cc "github.com/Abraxas-365/claudio/internal/comandcenter"
	"github.com/Abraxas-365/claudio/internal/tasks"
)

// renderComp renders a templ component to string; fatals on render error.
func renderComp(t *testing.T, name string, comp interface {
	Render(context.Context, io.Writer) error
}) string {
	t.Helper()
	var buf bytes.Buffer
	if err := comp.Render(context.Background(), &buf); err != nil {
		t.Fatalf("%s: Render failed: %v", name, err)
	}
	return buf.String()
}

// clamp returns s[:n] when len(s)>n, else s — avoids slice OOB in error msgs.
func clamp(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ─── Lightbox dialog attributes (designs.templ) ───────────────────────────────

func TestDesignsContent_LightboxHasRoleDialog(t *testing.T) {
	html := renderComp(t, "DesignsContent", DesignsContent(DesignGalleryData{}))
	if !strings.Contains(html, `role="dialog"`) {
		t.Errorf("DesignsContent lightbox: want role=\"dialog\":\n%s", clamp(html, 3000))
	}
}

func TestDesignsContent_LightboxHasAriaModal(t *testing.T) {
	html := renderComp(t, "DesignsContent", DesignsContent(DesignGalleryData{}))
	if !strings.Contains(html, `aria-modal="true"`) {
		t.Errorf("DesignsContent lightbox: want aria-modal=\"true\":\n%s", clamp(html, 3000))
	}
}

func TestDesignsContent_LightboxHasAriaLabelDesignPreview(t *testing.T) {
	html := renderComp(t, "DesignsContent", DesignsContent(DesignGalleryData{}))
	if !strings.Contains(html, `aria-label="Design preview"`) {
		t.Errorf("DesignsContent lightbox: want aria-label=\"Design preview\":\n%s", clamp(html, 3000))
	}
}

func TestDesignsContent_CloseButtonHasAriaLabelClose(t *testing.T) {
	html := renderComp(t, "DesignsContent", DesignsContent(DesignGalleryData{}))
	if !strings.Contains(html, `aria-label="Close"`) {
		t.Errorf("DesignsContent lightbox: want aria-label=\"Close\" on close button:\n%s", clamp(html, 3000))
	}
}

// ─── CronRow (cron_list.templ) ────────────────────────────────────────────────

func newTestCronRowData() CronRowData {
	return CronRowData{
		Entry: tasks.CronEntry{
			ID:       "cron-test-1",
			Schedule: "@daily",
			Prompt:   "Run backup",
			Enabled:  true,
		},
		Type:   "inline",
		Prompt: "Run backup",
	}
}

func TestCronRow_NoRawEmojiTrash(t *testing.T) {
	html := renderComp(t, "CronRow", CronRow(newTestCronRowData()))
	if strings.Contains(html, "🗑") {
		t.Errorf("CronRow: raw trash emoji 🗑 found — expected SVG DeleteIcon:\n%s", html)
	}
}

func TestCronRow_HasCardContainerClass(t *testing.T) {
	html := renderComp(t, "CronRow", CronRow(newTestCronRowData()))
	if !strings.Contains(html, "card-container") {
		t.Errorf("CronRow: want card-container class (from CardContainer wrapper):\n%s", html)
	}
}

// ─── Layout focus-visible CSS (layout.templ) ─────────────────────────────────

func TestLayout_HasFocusVisibleRule(t *testing.T) {
	html := renderComp(t, "Layout", Layout("", ""))
	if !strings.Contains(html, ":focus-visible") {
		t.Errorf("Layout: want :focus-visible rule in <style>:\n%s", clamp(html, 4000))
	}
}

func TestLayout_FocusVisibleUsesBrandColor(t *testing.T) {
	html := renderComp(t, "Layout", Layout("", ""))
	// Locate :focus-visible rule and verify it uses var(--color-brand).
	idx := strings.Index(html, ":focus-visible")
	if idx < 0 {
		t.Fatal("Layout: :focus-visible rule not found in output")
	}
	// Check within next 200 chars of the rule.
	window := html[idx : idx+min200(len(html)-idx)]
	if !strings.Contains(window, "var(--color-brand)") {
		t.Errorf("Layout: :focus-visible rule does not use var(--color-brand):\n%s", window)
	}
}

// ─── Login focus state (login.templ) ─────────────────────────────────────────

func TestLoginContent_FocusUsesBrandColor(t *testing.T) {
	html := renderComp(t, "LoginContent", LoginContent(LoginPageData{}))
	idx := strings.Index(html, ".login-input:focus")
	if idx < 0 {
		t.Fatal("LoginContent: .login-input:focus rule not found")
	}
	rule := html[idx : idx+min200(len(html)-idx)]
	if !strings.Contains(rule, "var(--color-brand)") {
		t.Errorf("LoginContent: .login-input:focus does not use var(--color-brand):\n%s", rule)
	}
}

func TestLoginContent_FocusDoesNotUseBorderColor(t *testing.T) {
	html := renderComp(t, "LoginContent", LoginContent(LoginPageData{}))
	idx := strings.Index(html, ".login-input:focus")
	if idx < 0 {
		t.Fatal("LoginContent: .login-input:focus rule not found")
	}
	// Only the first ~200 chars of the rule block.
	rule := html[idx : idx+min200(len(html)-idx)]
	if strings.Contains(rule, "border-color: var(--color-border)") {
		t.Errorf("LoginContent: .login-input:focus still uses var(--color-border) — update not applied:\n%s", rule)
	}
}

// ─── ARIA tabs (info_panel.templ) ─────────────────────────────────────────────

func newTestInfoPageData() InfoPageData {
	return InfoPageData{
		Session: cc.Session{
			ID:           "info-test-sess",
			Name:         "TestAgent",
			Path:         "/tmp",
			Status:       "active",
			CreatedAt:    time.Now(),
			LastActiveAt: time.Now(),
		},
		ActiveTab: "tasks",
	}
}

func TestInfoPanel_HasRoleTablist(t *testing.T) {
	html := renderComp(t, "InfoPanel", InfoPanel(newTestInfoPageData()))
	if !strings.Contains(html, `role="tablist"`) {
		t.Errorf("InfoPanel: want role=\"tablist\" on tab bar:\n%s", clamp(html, 3000))
	}
}

func TestInfoPanel_HasRoleTab(t *testing.T) {
	html := renderComp(t, "InfoPanel", InfoPanel(newTestInfoPageData()))
	if !strings.Contains(html, `role="tab"`) {
		t.Errorf("InfoPanel: want role=\"tab\" on tab buttons:\n%s", clamp(html, 3000))
	}
}

func TestInfoPanel_HasRoleTabpanel(t *testing.T) {
	html := renderComp(t, "InfoPanel", InfoPanel(newTestInfoPageData()))
	if !strings.Contains(html, `role="tabpanel"`) {
		t.Errorf("InfoPanel: want role=\"tabpanel\" on tab content container:\n%s", clamp(html, 3000))
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// min200 returns n if n<200, else 200 — avoids slice OOB when extracting rule windows.
func min200(n int) int {
	if n < 200 {
		return n
	}
	return 200
}
