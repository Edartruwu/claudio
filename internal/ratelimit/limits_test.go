package ratelimit

import (
	"net/http"
	"testing"
)

func TestExtractRawUtilization(t *testing.T) {
	h := http.Header{}
	h.Set("anthropic-ratelimit-unified-5h-utilization", "0.45")
	h.Set("anthropic-ratelimit-unified-5h-reset", "1700000000")
	h.Set("anthropic-ratelimit-unified-7d-utilization", "0.20")
	h.Set("anthropic-ratelimit-unified-7d-reset", "1700500000")

	ru := extractRawUtilization(h)

	if ru.FiveHour == nil {
		t.Fatal("expected FiveHour to be set")
	}
	if ru.FiveHour.Utilization != 0.45 {
		t.Errorf("FiveHour utilization = %f, want 0.45", ru.FiveHour.Utilization)
	}
	if ru.FiveHour.ResetsAt != 1700000000 {
		t.Errorf("FiveHour ResetsAt = %d, want 1700000000", ru.FiveHour.ResetsAt)
	}
	if ru.SevenDay == nil {
		t.Fatal("expected SevenDay to be set")
	}
	if ru.SevenDay.Utilization != 0.20 {
		t.Errorf("SevenDay utilization = %f, want 0.20", ru.SevenDay.Utilization)
	}
}

func TestComputeFromHeaders_Allowed(t *testing.T) {
	h := http.Header{}
	h.Set("anthropic-ratelimit-unified-status", "allowed")
	h.Set("anthropic-ratelimit-unified-reset", "1700000000")

	l := computeFromHeaders(h)

	if l.Status != StatusAllowed {
		t.Errorf("status = %q, want allowed", l.Status)
	}
	if l.ResetsAt != 1700000000 {
		t.Errorf("ResetsAt = %d, want 1700000000", l.ResetsAt)
	}
}

func TestComputeFromHeaders_Rejected(t *testing.T) {
	h := http.Header{}
	h.Set("anthropic-ratelimit-unified-status", "rejected")
	h.Set("anthropic-ratelimit-unified-representative-claim", "five_hour")
	h.Set("anthropic-ratelimit-unified-reset", "1700000000")

	l := computeFromHeaders(h)

	if l.Status != StatusRejected {
		t.Errorf("status = %q, want rejected", l.Status)
	}
	if l.RateLimitType != LimitFiveHour {
		t.Errorf("RateLimitType = %q, want five_hour", l.RateLimitType)
	}
}

func TestComputeFromHeaders_Overage(t *testing.T) {
	h := http.Header{}
	h.Set("anthropic-ratelimit-unified-status", "rejected")
	h.Set("anthropic-ratelimit-unified-overage-status", "allowed")
	h.Set("anthropic-ratelimit-unified-representative-claim", "five_hour")
	h.Set("anthropic-ratelimit-unified-reset", "1700000000")

	l := computeFromHeaders(h)

	if !l.IsUsingOverage {
		t.Error("expected IsUsingOverage = true")
	}
	if l.OverageStatus != StatusAllowed {
		t.Errorf("OverageStatus = %q, want allowed", l.OverageStatus)
	}
}

func TestComputeFromHeaders_HeaderBasedEarlyWarning(t *testing.T) {
	h := http.Header{}
	h.Set("anthropic-ratelimit-unified-status", "allowed")
	h.Set("anthropic-ratelimit-unified-5h-surpassed-threshold", "0.8")
	h.Set("anthropic-ratelimit-unified-5h-utilization", "0.85")
	h.Set("anthropic-ratelimit-unified-5h-reset", "1700000000")

	l := computeFromHeaders(h)

	if l.Status != StatusAllowedWarning {
		t.Errorf("status = %q, want allowed_warning", l.Status)
	}
	if l.RateLimitType != LimitFiveHour {
		t.Errorf("RateLimitType = %q, want five_hour", l.RateLimitType)
	}
	if l.Utilization != 0.85 {
		t.Errorf("Utilization = %f, want 0.85", l.Utilization)
	}
	if l.SurpassedThreshold != 0.8 {
		t.Errorf("SurpassedThreshold = %f, want 0.8", l.SurpassedThreshold)
	}
}

func TestComputeFromHeaders_OverageDisabledReason(t *testing.T) {
	h := http.Header{}
	h.Set("anthropic-ratelimit-unified-status", "rejected")
	h.Set("anthropic-ratelimit-unified-overage-status", "rejected")
	h.Set("anthropic-ratelimit-unified-overage-disabled-reason", "out_of_credits")

	l := computeFromHeaders(h)

	if l.OverageDisabledReason != OutOfCredits {
		t.Errorf("OverageDisabledReason = %q, want out_of_credits", l.OverageDisabledReason)
	}
}

func TestLimitsEqual(t *testing.T) {
	a := Limits{Status: StatusAllowed, Utilization: -1, SurpassedThreshold: -1}
	b := Limits{Status: StatusAllowed, Utilization: -1, SurpassedThreshold: -1}
	if !limitsEqual(a, b) {
		t.Error("identical limits should be equal")
	}

	b.Status = StatusRejected
	if limitsEqual(a, b) {
		t.Error("different limits should not be equal")
	}
}

func TestEmptyHeaders(t *testing.T) {
	h := http.Header{}
	l := computeFromHeaders(h)
	if l.Status != StatusAllowed {
		t.Errorf("empty headers should give StatusAllowed, got %q", l.Status)
	}
}
