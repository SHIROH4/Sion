package cognition

import (
	"math"
	"time"
)

// DynamicInterval computes the optimal decision tick interval based on quantified
// features. Three tiers:
//
//	Tier 1 — Active (1–3 min): user is chatting, high intimacy, high drives
//	Tier 2 — Normal (5 min): baseline, no strong signal either way
//	Tier 3 — Dormant (15–60 min): night, rejections, deep work, quota exhausted
//
// Returns the recommended interval. Callers should clamp to their own min/max.
func DynamicInterval(
	timeSinceChatMin float64, // U14
	isWorking bool,           // U3
	continuousWorkMin float64, // U4
	isNight bool,             // U12
	rejectionSeverity float64, // R4
	dailyQuotaRemaining float64, // E4
	socialDrive float64,       // from ComputeDrives
	careDrive float64,         // from ComputeDrives
	curiousDrive float64,      // from ComputeDrives
) time.Duration {
	// ---- Tier 3: Dormant (long interval) ----

	// Quota exhausted → maximum interval (effectively suspended).
	if dailyQuotaRemaining <= 0 {
		return 60 * time.Minute
	}

	// Night time (22:00–08:00) → 30 min.
	if isNight {
		return 30 * time.Minute
	}

	// Heavy rejections → suppress.
	if rejectionSeverity > 0.5 {
		return 30 * time.Minute
	}

	// Deep work > 2h + high focus → leave them alone.
	if continuousWorkMin > 120 && isWorking {
		return 15 * time.Minute
	}

	// ---- Tier 1: Active (short interval) ----

	// User is chatting + high intimacy → stay close.
	hasHighDrive := socialDrive > 0.7 || careDrive > 0.7 || curiousDrive > 0.7
	if timeSinceChatMin < 10 && hasHighDrive {
		return 1 * time.Minute
	}

	// User was recently chatting (within 10 min) → stay responsive.
	if timeSinceChatMin < 10 {
		return 3 * time.Minute
	}

	// High drive even without recent chat (e.g. loneliness spike).
	if hasHighDrive {
		return 3 * time.Minute
	}

	// ---- Tier 2: Normal (baseline) ----
	return 5 * time.Minute
}

// AdaptiveScreenInterval returns the screen observation interval based on
// the decision interval. Faster decisions need faster observations.
func AdaptiveScreenInterval(decisionInterval time.Duration) time.Duration {
	// Screen observation at 1/3 of decision interval, bounded to [30s, 120s].
	raw := decisionInterval / 3
	if raw < 30*time.Second {
		return 30 * time.Second
	}
	if raw > 120*time.Second {
		return 120 * time.Second
	}
	return raw
}

// IntervalChanged returns true if the new interval differs from the old one
// by more than 10%, avoiding unnecessary ticker resets.
func IntervalChanged(old, new time.Duration) bool {
	if old == 0 {
		return true
	}
	ratio := float64(new) / float64(old)
	return math.Abs(ratio-1.0) > 0.1
}
