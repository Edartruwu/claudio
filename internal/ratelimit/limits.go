package ratelimit

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// QuotaStatus represents the current rate limit status.
type QuotaStatus string

const (
	StatusAllowed        QuotaStatus = "allowed"
	StatusAllowedWarning QuotaStatus = "allowed_warning"
	StatusRejected       QuotaStatus = "rejected"
)

// RateLimitType identifies which quota window was hit.
type RateLimitType string

const (
	LimitFiveHour     RateLimitType = "five_hour"
	LimitSevenDay     RateLimitType = "seven_day"
	LimitSevenDayOpus RateLimitType = "seven_day_opus"
	LimitSevenDaySon  RateLimitType = "seven_day_sonnet"
	LimitOverage      RateLimitType = "overage"
)

// OverageDisabledReason indicates why overage is unavailable.
type OverageDisabledReason string

const (
	OverageNotProvisioned      OverageDisabledReason = "overage_not_provisioned"
	OrgLevelDisabled           OverageDisabledReason = "org_level_disabled"
	OrgLevelDisabledUntil      OverageDisabledReason = "org_level_disabled_until"
	OutOfCredits               OverageDisabledReason = "out_of_credits"
	SeatTierLevelDisabled      OverageDisabledReason = "seat_tier_level_disabled"
	MemberLevelDisabled        OverageDisabledReason = "member_level_disabled"
	SeatTierZeroCreditLimit    OverageDisabledReason = "seat_tier_zero_credit_limit"
	GroupZeroCreditLimit       OverageDisabledReason = "group_zero_credit_limit"
	MemberZeroCreditLimit      OverageDisabledReason = "member_zero_credit_limit"
	OrgServiceLevelDisabled    OverageDisabledReason = "org_service_level_disabled"
	OrgServiceZeroCreditLimit  OverageDisabledReason = "org_service_zero_credit_limit"
	NoLimitsConfigured         OverageDisabledReason = "no_limits_configured"
	OverageReasonUnknown       OverageDisabledReason = "unknown"
)

// Limits holds the current rate-limit state parsed from API response headers.
type Limits struct {
	Status                QuotaStatus
	FallbackAvailable     bool
	ResetsAt              int64 // unix epoch seconds, 0 = unknown
	RateLimitType         RateLimitType
	Utilization           float64 // 0-1, -1 = unknown
	OverageStatus         QuotaStatus
	OverageResetsAt       int64
	OverageDisabledReason OverageDisabledReason
	IsUsingOverage        bool
	SurpassedThreshold    float64 // 0-1, -1 = not set
}

// RawWindowUtilization tracks per-window usage from headers.
type RawWindowUtilization struct {
	Utilization float64
	ResetsAt    int64
}

// RawUtilization holds per-window utilization data.
type RawUtilization struct {
	FiveHour *RawWindowUtilization
	SevenDay *RawWindowUtilization
}

// StatusChangeListener is called when limits change.
type StatusChangeListener func(Limits)

// earlyWarningThreshold defines when to trigger a client-side early warning.
type earlyWarningThreshold struct {
	utilization float64 // trigger when usage >= this
	timePct     float64 // trigger when time elapsed <= this
}

type earlyWarningConfig struct {
	rateLimitType RateLimitType
	claimAbbrev   string
	windowSeconds int64
	thresholds    []earlyWarningThreshold
}

var earlyWarningConfigs = []earlyWarningConfig{
	{
		rateLimitType: LimitFiveHour,
		claimAbbrev:   "5h",
		windowSeconds: 5 * 60 * 60,
		thresholds:    []earlyWarningThreshold{{utilization: 0.9, timePct: 0.72}},
	},
	{
		rateLimitType: LimitSevenDay,
		claimAbbrev:   "7d",
		windowSeconds: 7 * 24 * 60 * 60,
		thresholds: []earlyWarningThreshold{
			{utilization: 0.75, timePct: 0.6},
			{utilization: 0.5, timePct: 0.35},
			{utilization: 0.25, timePct: 0.15},
		},
	},
}

// claimAbbrev -> RateLimitType for header-based early warning
var earlyWarningClaimMap = map[string]RateLimitType{
	"5h":      LimitFiveHour,
	"7d":      LimitSevenDay,
	"overage": LimitOverage,
}

var (
	mu             sync.RWMutex
	current        = Limits{Status: StatusAllowed}
	rawUtil        = RawUtilization{}
	listeners      []StatusChangeListener
)

// Current returns the current rate-limit state (thread-safe copy).
func Current() Limits {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// RawUtil returns the current raw per-window utilization.
func RawUtil() RawUtilization {
	mu.RLock()
	defer mu.RUnlock()
	return rawUtil
}

// OnStatusChange registers a listener that fires when limits change.
func OnStatusChange(fn StatusChangeListener) {
	mu.Lock()
	defer mu.Unlock()
	listeners = append(listeners, fn)
}

func emitStatusChange(l Limits) {
	current = l
	for _, fn := range listeners {
		fn(l)
	}
}

// ExtractFromHeaders updates global limits from a successful API response.
func ExtractFromHeaders(h http.Header) {
	mu.Lock()
	defer mu.Unlock()

	rawUtil = extractRawUtilization(h)
	newLimits := computeFromHeaders(h)

	if !limitsEqual(current, newLimits) {
		emitStatusChange(newLimits)
	}
}

// ExtractFromError updates global limits from a 429 error response.
func ExtractFromError(statusCode int, h http.Header) {
	if statusCode != http.StatusTooManyRequests {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	rawUtil = extractRawUtilization(h)
	newLimits := computeFromHeaders(h)
	newLimits.Status = StatusRejected

	if !limitsEqual(current, newLimits) {
		emitStatusChange(newLimits)
	}
}

// Reset clears all state back to defaults.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	current = Limits{Status: StatusAllowed}
	rawUtil = RawUtilization{}
}

// --- header parsing ---

func hdr(h http.Header, name string) string {
	return h.Get(name)
}

func hdrFloat(h http.Header, name string) (float64, bool) {
	v := h.Get(name)
	if v == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func hdrInt64(h http.Header, name string) (int64, bool) {
	v := h.Get(name)
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		// Try float (some headers send epoch as float)
		f, err2 := strconv.ParseFloat(v, 64)
		if err2 != nil {
			return 0, false
		}
		return int64(f), true
	}
	return n, true
}

func extractRawUtilization(h http.Header) RawUtilization {
	var ru RawUtilization
	for _, pair := range []struct {
		key    string
		abbrev string
	}{
		{"FiveHour", "5h"},
		{"SevenDay", "7d"},
	} {
		util, uOK := hdrFloat(h, "anthropic-ratelimit-unified-"+pair.abbrev+"-utilization")
		reset, rOK := hdrInt64(h, "anthropic-ratelimit-unified-"+pair.abbrev+"-reset")
		if uOK && rOK {
			w := &RawWindowUtilization{Utilization: util, ResetsAt: reset}
			if pair.key == "FiveHour" {
				ru.FiveHour = w
			} else {
				ru.SevenDay = w
			}
		}
	}
	return ru
}

func computeFromHeaders(h http.Header) Limits {
	status := QuotaStatus(hdr(h, "anthropic-ratelimit-unified-status"))
	if status == "" {
		status = StatusAllowed
	}

	resetsAt, _ := hdrInt64(h, "anthropic-ratelimit-unified-reset")
	fallback := hdr(h, "anthropic-ratelimit-unified-fallback") == "available"
	rateLimitType := RateLimitType(hdr(h, "anthropic-ratelimit-unified-representative-claim"))

	overageStatus := QuotaStatus(hdr(h, "anthropic-ratelimit-unified-overage-status"))
	overageResetsAt, _ := hdrInt64(h, "anthropic-ratelimit-unified-overage-reset")
	overageDisabledReason := OverageDisabledReason(hdr(h, "anthropic-ratelimit-unified-overage-disabled-reason"))

	isUsingOverage := status == StatusRejected &&
		(overageStatus == StatusAllowed || overageStatus == StatusAllowedWarning)

	// Check early warning if allowed
	if status == StatusAllowed || status == StatusAllowedWarning {
		if ew := getEarlyWarning(h, fallback); ew != nil {
			return *ew
		}
		status = StatusAllowed
	}

	return Limits{
		Status:                status,
		FallbackAvailable:     fallback,
		ResetsAt:              resetsAt,
		RateLimitType:         rateLimitType,
		Utilization:           -1,
		OverageStatus:         overageStatus,
		OverageResetsAt:       overageResetsAt,
		OverageDisabledReason: overageDisabledReason,
		IsUsingOverage:        isUsingOverage,
		SurpassedThreshold:    -1,
	}
}

// --- early warning ---

func getEarlyWarning(h http.Header, fallback bool) *Limits {
	// 1. Header-based detection (server sends surpassed-threshold)
	for abbrev, rlt := range earlyWarningClaimMap {
		st, ok := hdrFloat(h, "anthropic-ratelimit-unified-"+abbrev+"-surpassed-threshold")
		if !ok {
			continue
		}
		util, _ := hdrFloat(h, "anthropic-ratelimit-unified-"+abbrev+"-utilization")
		reset, _ := hdrInt64(h, "anthropic-ratelimit-unified-"+abbrev+"-reset")
		return &Limits{
			Status:             StatusAllowedWarning,
			ResetsAt:           reset,
			RateLimitType:      rlt,
			Utilization:        util,
			FallbackAvailable:  fallback,
			IsUsingOverage:     false,
			SurpassedThreshold: st,
		}
	}

	// 2. Client-side time-relative fallback
	for _, cfg := range earlyWarningConfigs {
		util, uOK := hdrFloat(h, "anthropic-ratelimit-unified-"+cfg.claimAbbrev+"-utilization")
		reset, rOK := hdrInt64(h, "anthropic-ratelimit-unified-"+cfg.claimAbbrev+"-reset")
		if !uOK || !rOK {
			continue
		}
		tp := computeTimeProgress(reset, cfg.windowSeconds)
		for _, t := range cfg.thresholds {
			if util >= t.utilization && tp <= t.timePct {
				return &Limits{
					Status:            StatusAllowedWarning,
					ResetsAt:          reset,
					RateLimitType:     cfg.rateLimitType,
					Utilization:       util,
					FallbackAvailable: fallback,
					IsUsingOverage:    false,
				}
			}
		}
	}

	return nil
}

func computeTimeProgress(resetsAt int64, windowSeconds int64) float64 {
	now := time.Now().Unix()
	windowStart := resetsAt - windowSeconds
	elapsed := float64(now - windowStart)
	return math.Max(0, math.Min(1, elapsed/float64(windowSeconds)))
}

// limitsEqual is a simple structural equality check.
func limitsEqual(a, b Limits) bool {
	return a.Status == b.Status &&
		a.FallbackAvailable == b.FallbackAvailable &&
		a.ResetsAt == b.ResetsAt &&
		a.RateLimitType == b.RateLimitType &&
		a.Utilization == b.Utilization &&
		a.OverageStatus == b.OverageStatus &&
		a.OverageResetsAt == b.OverageResetsAt &&
		a.OverageDisabledReason == b.OverageDisabledReason &&
		a.IsUsingOverage == b.IsUsingOverage &&
		a.SurpassedThreshold == b.SurpassedThreshold
}
