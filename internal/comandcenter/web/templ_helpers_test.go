package web

import (
	"strings"
	"testing"
	"time"
)

func TestRelTime_RecentMessage(t *testing.T) {
	tests := []struct {
		name string
		dur  time.Duration
		want string
	}{
		{"just now", 30 * time.Second, "just now"},
		{"1 minute", 90 * time.Second, "1 min ago"},
		{"5 minutes", 5 * time.Minute, "5 mins ago"},
		{"1 hour", 90 * time.Minute, "1 hr ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RelTime(time.Now().Add(-tt.dur))
			if got != tt.want {
				t.Errorf("RelTime(%v ago) = %q, want %q", tt.dur, got, tt.want)
			}
		})
	}
}

func TestFirstChar_MultiByteRune(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Alice", "A"},
		{"bob", "B"},
		{"日本語", "日"},
		{"émile", "É"},
		{"", "?"},
		{" ", " "},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := FirstChar(tt.input)
			if got != tt.want {
				t.Errorf("FirstChar(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRenderMD_SanitizesScript(t *testing.T) {
	input := `Hello <script>alert("xss")</script> world`
	result := string(RenderMD(input))

	if strings.Contains(result, "<script>") {
		t.Errorf("RenderMD did not sanitize <script> tag:\n%s", result)
	}
	if !strings.Contains(result, "Hello") {
		t.Errorf("RenderMD stripped safe content:\n%s", result)
	}
}

func TestRenderMD_RendersMarkdown(t *testing.T) {
	input := "**bold** text"
	result := string(RenderMD(input))

	if !strings.Contains(result, "<strong>bold</strong>") {
		t.Errorf("RenderMD did not render markdown bold:\n%s", result)
	}
}
