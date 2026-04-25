package web

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestStatusBadge_AllStatuses(t *testing.T) {
	statuses := []struct {
		status    string
		wantClass string
		wantText  string
	}{
		{"done", "color:var(--color-brand)", "done"},
		{"in_progress", "color:var(--color-ai)", "in_progress"},
		{"running", "color:var(--color-ai)", "running"},
		{"blocked", "color:var(--color-error)", "blocked"},
		{"failed", "color:var(--color-error)", "failed"},
		{"cancelled", "color:var(--color-textSecondary)", "cancelled"},
		{"pending", "color:var(--color-textMuted)", "pending"},
		{"unknown_status", "color:var(--color-textMuted)", "unknown_status"},
	}

	for _, tt := range statuses {
		t.Run(tt.status, func(t *testing.T) {
			var buf bytes.Buffer
			err := StatusBadge(tt.status).Render(context.Background(), &buf)
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}
			html := buf.String()
			if !strings.Contains(html, tt.wantClass) {
				t.Errorf("StatusBadge(%q): want class %q in:\n%s", tt.status, tt.wantClass, html)
			}
			if !strings.Contains(html, tt.wantText) {
				t.Errorf("StatusBadge(%q): want text %q in:\n%s", tt.status, tt.wantText, html)
			}
		})
	}
}

func TestAvatarCircle_ActiveInactive(t *testing.T) {
	tests := []struct {
		name   string
		active bool
		want   string // expected presence in HTML
		absent string // expected absence in HTML
	}{
		{"Alice", true, "bg-[var(--color-brand)]", ""},
		{"Bob", false, "A", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := AvatarCircle(tt.name, tt.active, "w-9 h-9").Render(context.Background(), &buf)
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}
			html := buf.String()
			// Active avatars should have the status dot
			if tt.active {
				if !strings.Contains(html, `aria-label="Online"`) {
					t.Errorf("AvatarCircle(%q, active=true): want Online dot in:\n%s", tt.name, html)
				}
			} else {
				if strings.Contains(html, `aria-label="Online"`) {
					t.Errorf("AvatarCircle(%q, active=false): want no Online dot in:\n%s", tt.name, html)
				}
			}
			// Should always contain the first character
			fc := FirstChar(tt.name)
			if !strings.Contains(html, fc) {
				t.Errorf("AvatarCircle(%q): want first char %q in:\n%s", tt.name, fc, html)
			}
		})
	}
}

func TestErrorToast_Renders(t *testing.T) {
	var buf bytes.Buffer
	err := ErrorToast("Something went wrong").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	html := buf.String()

	// Must have role="alert" for a11y
	if !strings.Contains(html, `role="alert"`) {
		t.Errorf("ErrorToast: want role=alert in:\n%s", html)
	}
	// Must have aria-live="assertive"
	if !strings.Contains(html, `aria-live="assertive"`) {
		t.Errorf("ErrorToast: want aria-live=assertive in:\n%s", html)
	}
	// Must contain the message
	if !strings.Contains(html, "Something went wrong") {
		t.Errorf("ErrorToast: want message text in:\n%s", html)
	}
}
