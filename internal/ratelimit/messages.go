package ratelimit

import (
	"fmt"
	"math"
	"time"
)

// RateLimitDisplayNames maps limit types to human-readable names.
var RateLimitDisplayNames = map[RateLimitType]string{
	LimitFiveHour:     "session limit",
	LimitSevenDay:     "weekly limit",
	LimitSevenDayOpus: "Opus limit",
	LimitSevenDaySon:  "Sonnet limit",
	LimitOverage:      "extra usage limit",
}

// MessageSeverity indicates warning vs error.
type MessageSeverity string

const (
	SeverityWarning MessageSeverity = "warning"
	SeverityError   MessageSeverity = "error"
)

// Message is a rate-limit message with severity.
type Message struct {
	Text     string
	Severity MessageSeverity
}

// GetMessage returns the appropriate rate-limit message for the current state.
// Returns nil if no message should be shown.
func GetMessage(limits Limits) *Message {
	// Overage scenarios first
	if limits.IsUsingOverage {
		if limits.OverageStatus == StatusAllowedWarning {
			return &Message{
				Text:     "You're close to your extra usage spending limit",
				Severity: SeverityWarning,
			}
		}
		return nil
	}

	// Error: limits rejected
	if limits.Status == StatusRejected {
		return &Message{
			Text:     getLimitReachedText(limits),
			Severity: SeverityError,
		}
	}

	// Warning: approaching limits
	if limits.Status == StatusAllowedWarning {
		const warningThreshold = 0.7
		if limits.Utilization >= 0 && limits.Utilization < warningThreshold {
			return nil
		}
		if text := getEarlyWarningText(limits); text != "" {
			return &Message{Text: text, Severity: SeverityWarning}
		}
	}

	return nil
}

// GetWarning returns only the warning text (nil if error or no message).
func GetWarning(limits Limits) string {
	msg := GetMessage(limits)
	if msg != nil && msg.Severity == SeverityWarning {
		return msg.Text
	}
	return ""
}

// GetError returns only the error text (nil if warning or no message).
func GetError(limits Limits) string {
	msg := GetMessage(limits)
	if msg != nil && msg.Severity == SeverityError {
		return msg.Text
	}
	return ""
}

// GetUsingOverageText returns the notification for overage transitions.
func GetUsingOverageText(limits Limits) string {
	resetTime := ""
	if limits.ResetsAt > 0 {
		resetTime = FormatResetTime(limits.ResetsAt)
	}

	var limitName string
	switch limits.RateLimitType {
	case LimitFiveHour:
		limitName = "session limit"
	case LimitSevenDay:
		limitName = "weekly limit"
	case LimitSevenDayOpus:
		limitName = "Opus limit"
	case LimitSevenDaySon:
		limitName = "Sonnet limit"
	default:
		return "Now using extra usage"
	}

	if resetTime != "" {
		return fmt.Sprintf("You're now using extra usage · Your %s resets %s", limitName, resetTime)
	}
	return "You're now using extra usage"
}

func getLimitReachedText(limits Limits) string {
	resetTime := ""
	if limits.ResetsAt > 0 {
		resetTime = FormatResetTime(limits.ResetsAt)
	}
	overageResetTime := ""
	if limits.OverageResetsAt > 0 {
		overageResetTime = FormatResetTime(limits.OverageResetsAt)
	}
	resetMsg := ""
	if resetTime != "" {
		resetMsg = " · resets " + resetTime
	}

	// Both subscription and overage exhausted
	if limits.OverageStatus == StatusRejected {
		var msg string
		if limits.ResetsAt > 0 && limits.OverageResetsAt > 0 {
			if limits.ResetsAt < limits.OverageResetsAt {
				msg = " · resets " + resetTime
			} else {
				msg = " · resets " + overageResetTime
			}
		} else if resetTime != "" {
			msg = " · resets " + resetTime
		} else if overageResetTime != "" {
			msg = " · resets " + overageResetTime
		}

		if limits.OverageDisabledReason == OutOfCredits {
			return "You're out of extra usage" + msg
		}
		return fmt.Sprintf("You've hit your limit%s", msg)
	}

	switch limits.RateLimitType {
	case LimitSevenDaySon:
		return fmt.Sprintf("You've hit your Sonnet limit%s", resetMsg)
	case LimitSevenDayOpus:
		return fmt.Sprintf("You've hit your Opus limit%s", resetMsg)
	case LimitSevenDay:
		return fmt.Sprintf("You've hit your weekly limit%s", resetMsg)
	case LimitFiveHour:
		return fmt.Sprintf("You've hit your session limit%s", resetMsg)
	default:
		return fmt.Sprintf("You've hit your usage limit%s", resetMsg)
	}
}

func getEarlyWarningText(limits Limits) string {
	var limitName string
	switch limits.RateLimitType {
	case LimitSevenDay:
		limitName = "weekly limit"
	case LimitFiveHour:
		limitName = "session limit"
	case LimitSevenDayOpus:
		limitName = "Opus limit"
	case LimitSevenDaySon:
		limitName = "Sonnet limit"
	case LimitOverage:
		limitName = "extra usage"
	default:
		return ""
	}

	used := -1
	if limits.Utilization >= 0 {
		used = int(math.Floor(limits.Utilization * 100))
	}
	resetTime := ""
	if limits.ResetsAt > 0 {
		resetTime = FormatResetTime(limits.ResetsAt)
	}

	if used >= 0 && resetTime != "" {
		return fmt.Sprintf("You've used %d%% of your %s · resets %s", used, limitName, resetTime)
	}
	if used >= 0 {
		return fmt.Sprintf("You've used %d%% of your %s", used, limitName)
	}

	if limits.RateLimitType == LimitOverage {
		limitName += " limit"
	}

	if resetTime != "" {
		return fmt.Sprintf("Approaching %s · resets %s", limitName, resetTime)
	}
	return fmt.Sprintf("Approaching %s", limitName)
}

// FormatResetTime formats a unix epoch into a human-readable relative time.
func FormatResetTime(epochSeconds int64) string {
	now := time.Now().Unix()
	diff := epochSeconds - now
	if diff <= 0 {
		return "soon"
	}

	hours := diff / 3600
	minutes := (diff % 3600) / 60

	if hours > 24 {
		days := hours / 24
		h := hours % 24
		if h > 0 {
			return fmt.Sprintf("in %dd %dh", days, h)
		}
		return fmt.Sprintf("in %dd", days)
	}
	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("in %dh %dm", hours, minutes)
		}
		return fmt.Sprintf("in %dh", hours)
	}
	if minutes > 0 {
		return fmt.Sprintf("in %dm", minutes)
	}
	return "in <1m"
}
