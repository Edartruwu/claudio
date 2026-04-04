package ratelimit

import (
	"strings"
	"testing"
	"time"
)

func TestGetMessage_OverageWarning(t *testing.T) {
	limits := Limits{
		IsUsingOverage: true,
		OverageStatus:  StatusAllowedWarning,
	}
	msg := GetMessage(limits)
	if msg == nil {
		t.Fatal("expected message, got nil")
	}
	if msg.Severity != SeverityWarning {
		t.Errorf("severity = %q, want warning", msg.Severity)
	}
	if !strings.Contains(msg.Text, "extra usage") {
		t.Errorf("text = %q, expected mention of extra usage", msg.Text)
	}
}

func TestGetMessage_OverageAllowed_NoMessage(t *testing.T) {
	limits := Limits{
		IsUsingOverage: true,
		OverageStatus:  StatusAllowed,
	}
	msg := GetMessage(limits)
	if msg != nil {
		t.Errorf("expected nil message for overage allowed, got %+v", msg)
	}
}

func TestGetMessage_Rejected(t *testing.T) {
	limits := Limits{
		Status:        StatusRejected,
		RateLimitType: LimitFiveHour,
	}
	msg := GetMessage(limits)
	if msg == nil {
		t.Fatal("expected message, got nil")
	}
	if msg.Severity != SeverityError {
		t.Errorf("severity = %q, want error", msg.Severity)
	}
	if !strings.Contains(msg.Text, "session limit") {
		t.Errorf("text = %q, expected mention of session limit", msg.Text)
	}
}

func TestGetMessage_Rejected_SevenDay(t *testing.T) {
	limits := Limits{
		Status:        StatusRejected,
		RateLimitType: LimitSevenDay,
	}
	msg := GetMessage(limits)
	if msg == nil {
		t.Fatal("expected message")
	}
	if !strings.Contains(msg.Text, "weekly limit") {
		t.Errorf("text = %q, expected weekly limit", msg.Text)
	}
}

func TestGetMessage_Rejected_Opus(t *testing.T) {
	limits := Limits{
		Status:        StatusRejected,
		RateLimitType: LimitSevenDayOpus,
	}
	msg := GetMessage(limits)
	if msg == nil {
		t.Fatal("expected message")
	}
	if !strings.Contains(msg.Text, "Opus") {
		t.Errorf("text = %q, expected Opus", msg.Text)
	}
}

func TestGetMessage_Rejected_Sonnet(t *testing.T) {
	limits := Limits{
		Status:        StatusRejected,
		RateLimitType: LimitSevenDaySon,
	}
	msg := GetMessage(limits)
	if msg == nil {
		t.Fatal("expected message")
	}
	if !strings.Contains(msg.Text, "Sonnet") {
		t.Errorf("text = %q, expected Sonnet", msg.Text)
	}
}

func TestGetMessage_Rejected_WithResetTime(t *testing.T) {
	limits := Limits{
		Status:        StatusRejected,
		RateLimitType: LimitFiveHour,
		ResetsAt:      time.Now().Unix() + 3600, // 1 hour from now
	}
	msg := GetMessage(limits)
	if msg == nil {
		t.Fatal("expected message")
	}
	if !strings.Contains(msg.Text, "resets") {
		t.Errorf("text = %q, expected reset time info", msg.Text)
	}
}

func TestGetMessage_Rejected_BothExhausted(t *testing.T) {
	limits := Limits{
		Status:        StatusRejected,
		OverageStatus: StatusRejected,
		ResetsAt:      time.Now().Unix() + 3600,
	}
	msg := GetMessage(limits)
	if msg == nil {
		t.Fatal("expected message")
	}
	if msg.Severity != SeverityError {
		t.Errorf("severity = %q, want error", msg.Severity)
	}
}

func TestGetMessage_Rejected_OutOfCredits(t *testing.T) {
	limits := Limits{
		Status:                StatusRejected,
		OverageStatus:         StatusRejected,
		OverageDisabledReason: OutOfCredits,
	}
	msg := GetMessage(limits)
	if msg == nil {
		t.Fatal("expected message")
	}
	if !strings.Contains(msg.Text, "out of extra usage") {
		t.Errorf("text = %q, expected out of extra usage", msg.Text)
	}
}

func TestGetMessage_AllowedWarning_HighUtilization(t *testing.T) {
	limits := Limits{
		Status:        StatusAllowedWarning,
		RateLimitType: LimitFiveHour,
		Utilization:   0.85,
		ResetsAt:      time.Now().Unix() + 3600,
	}
	msg := GetMessage(limits)
	if msg == nil {
		t.Fatal("expected warning message")
	}
	if msg.Severity != SeverityWarning {
		t.Errorf("severity = %q, want warning", msg.Severity)
	}
	if !strings.Contains(msg.Text, "85%") {
		t.Errorf("text = %q, expected 85%% utilization", msg.Text)
	}
}

func TestGetMessage_AllowedWarning_LowUtilization_NoMessage(t *testing.T) {
	limits := Limits{
		Status:        StatusAllowedWarning,
		RateLimitType: LimitFiveHour,
		Utilization:   0.3,
	}
	msg := GetMessage(limits)
	if msg != nil {
		t.Errorf("expected nil for low utilization warning, got %+v", msg)
	}
}

func TestGetMessage_Allowed_NoMessage(t *testing.T) {
	limits := Limits{Status: StatusAllowed}
	msg := GetMessage(limits)
	if msg != nil {
		t.Errorf("expected nil for allowed status, got %+v", msg)
	}
}

func TestGetWarning(t *testing.T) {
	// Warning case
	limits := Limits{
		IsUsingOverage: true,
		OverageStatus:  StatusAllowedWarning,
	}
	w := GetWarning(limits)
	if w == "" {
		t.Error("expected warning text, got empty")
	}

	// Error case should return empty
	limits = Limits{
		Status:        StatusRejected,
		RateLimitType: LimitFiveHour,
	}
	w = GetWarning(limits)
	if w != "" {
		t.Errorf("expected empty for error case, got %q", w)
	}
}

func TestGetError(t *testing.T) {
	// Error case
	limits := Limits{
		Status:        StatusRejected,
		RateLimitType: LimitFiveHour,
	}
	e := GetError(limits)
	if e == "" {
		t.Error("expected error text, got empty")
	}

	// Warning case should return empty
	limits = Limits{
		IsUsingOverage: true,
		OverageStatus:  StatusAllowedWarning,
	}
	e = GetError(limits)
	if e != "" {
		t.Errorf("expected empty for warning case, got %q", e)
	}
}

func TestGetUsingOverageText(t *testing.T) {
	tests := []struct {
		name     string
		limits   Limits
		contains string
	}{
		{
			name:     "five_hour with reset",
			limits:   Limits{RateLimitType: LimitFiveHour, ResetsAt: time.Now().Unix() + 7200},
			contains: "session limit",
		},
		{
			name:     "seven_day with reset",
			limits:   Limits{RateLimitType: LimitSevenDay, ResetsAt: time.Now().Unix() + 7200},
			contains: "weekly limit",
		},
		{
			name:     "opus",
			limits:   Limits{RateLimitType: LimitSevenDayOpus, ResetsAt: time.Now().Unix() + 7200},
			contains: "Opus limit",
		},
		{
			name:     "sonnet",
			limits:   Limits{RateLimitType: LimitSevenDaySon, ResetsAt: time.Now().Unix() + 7200},
			contains: "Sonnet limit",
		},
		{
			name:     "unknown type",
			limits:   Limits{RateLimitType: "unknown"},
			contains: "Now using extra usage",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := GetUsingOverageText(tt.limits)
			if !strings.Contains(text, tt.contains) {
				t.Errorf("text = %q, expected to contain %q", text, tt.contains)
			}
		})
	}
}

func TestFormatResetTime(t *testing.T) {
	now := time.Now().Unix()

	tests := []struct {
		name     string
		epoch    int64
		contains string
	}{
		{"past", now - 100, "soon"},
		{"minutes", now + 300, "5m"},
		{"hours", now + 7200, "2h"},
		{"days", now + 90000, "1d"},
		{"less than 1m", now + 30, "<1m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatResetTime(tt.epoch)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("FormatResetTime(%d) = %q, want containing %q", tt.epoch, result, tt.contains)
			}
		})
	}
}

func TestGetMessage_AllowedWarning_NoUtilization(t *testing.T) {
	limits := Limits{
		Status:        StatusAllowedWarning,
		RateLimitType: LimitFiveHour,
		Utilization:   -1,
		ResetsAt:      time.Now().Unix() + 3600,
	}
	msg := GetMessage(limits)
	if msg == nil {
		t.Fatal("expected message for warning without utilization")
	}
	if !strings.Contains(msg.Text, "Approaching") {
		t.Errorf("text = %q, expected 'Approaching'", msg.Text)
	}
}

func TestGetMessage_AllowedWarning_Overage(t *testing.T) {
	limits := Limits{
		Status:        StatusAllowedWarning,
		RateLimitType: LimitOverage,
		Utilization:   0.9,
	}
	msg := GetMessage(limits)
	if msg == nil {
		t.Fatal("expected message")
	}
	if !strings.Contains(msg.Text, "extra usage") {
		t.Errorf("text = %q, expected extra usage", msg.Text)
	}
}
