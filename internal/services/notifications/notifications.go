// Package notifications provides desktop and terminal notification support.
package notifications

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Notifier sends notifications to the user.
type Notifier interface {
	Notify(title, body string) error
}

// OSNotifier sends native OS notifications.
type OSNotifier struct{}

// NewOSNotifier creates a platform-appropriate notifier.
func NewOSNotifier() Notifier {
	return &OSNotifier{}
}

// Notify sends a desktop notification.
func (n *OSNotifier) Notify(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		return notifyMacOS(title, body)
	case "linux":
		return notifyLinux(title, body)
	default:
		return fmt.Errorf("notifications not supported on %s", runtime.GOOS)
	}
}

func notifyMacOS(title, body string) error {
	script := fmt.Sprintf(`display notification %q with title %q`, body, title)
	return exec.Command("osascript", "-e", script).Run()
}

func notifyLinux(title, body string) error {
	// Try notify-send first
	if _, err := exec.LookPath("notify-send"); err == nil {
		return exec.Command("notify-send", title, body).Run()
	}
	return fmt.Errorf("notify-send not found")
}

// TerminalNotifier sends terminal bell/OSC notifications.
type TerminalNotifier struct{}

// NewTerminalNotifier creates a terminal-based notifier.
func NewTerminalNotifier() Notifier {
	return &TerminalNotifier{}
}

// Notify sends a terminal notification using OSC escape sequences.
func (t *TerminalNotifier) Notify(title, body string) error {
	// OSC 9 for iTerm2 notifications
	fmt.Printf("\033]9;%s: %s\007", title, body)
	// Terminal bell as fallback
	fmt.Print("\a")
	return nil
}

// MultiNotifier sends to multiple notifiers.
type MultiNotifier struct {
	notifiers []Notifier
}

// NewMultiNotifier creates a notifier that sends to all backends.
func NewMultiNotifier(notifiers ...Notifier) Notifier {
	return &MultiNotifier{notifiers: notifiers}
}

// Notify sends to all backends. Returns first error encountered.
func (m *MultiNotifier) Notify(title, body string) error {
	var firstErr error
	for _, n := range m.notifiers {
		if err := n.Notify(title, body); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// NoopNotifier does nothing (for when notifications are disabled).
type NoopNotifier struct{}

func (n *NoopNotifier) Notify(title, body string) error { return nil }
