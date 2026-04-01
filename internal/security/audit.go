package security

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/storage"
)

// Auditor logs all tool executions for security review.
type Auditor struct {
	db  *storage.DB
	bus *bus.Bus
}

// NewAuditor creates an auditor that logs to SQLite and publishes events.
func NewAuditor(db *storage.DB, eventBus *bus.Bus) *Auditor {
	return &Auditor{db: db, bus: eventBus}
}

// LogToolCall records a tool invocation.
func (a *Auditor) LogToolCall(sessionID, tool, inputSummary, outputSummary, approval string, tokensUsed int, duration time.Duration) {
	entry := storage.AuditEntry{
		SessionID:     sessionID,
		Tool:          tool,
		InputSummary:  truncateForAudit(inputSummary, 500),
		OutputSummary: truncateForAudit(outputSummary, 500),
		Approval:      approval,
		TokensUsed:    tokensUsed,
		DurationMs:    duration.Milliseconds(),
	}

	// Write to DB
	if a.db != nil {
		a.db.LogAudit(entry)
	}

	// Publish event
	if a.bus != nil {
		payload, _ := json.Marshal(map[string]any{
			"tool":       tool,
			"approval":   approval,
			"duration_ms": duration.Milliseconds(),
		})
		a.bus.Publish(bus.Event{
			Type:      bus.EventAuditEntry,
			Payload:   payload,
			SessionID: sessionID,
		})
	}
}

// CheckAndWarn scans output for secrets and logs warnings.
func (a *Auditor) CheckAndWarn(tool, output string) []string {
	secrets := ScanForSecrets(output)
	if len(secrets) > 0 {
		warning := fmt.Sprintf("Potential secret detected in %s output: %v", tool, secrets)
		if a.bus != nil {
			payload, _ := json.Marshal(map[string]any{
				"warning": warning,
				"tool":    tool,
				"count":   len(secrets),
			})
			a.bus.Publish(bus.Event{
				Type:    "security.warning",
				Payload: payload,
			})
		}
	}
	return secrets
}

func truncateForAudit(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
