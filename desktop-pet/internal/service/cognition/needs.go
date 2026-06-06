package cognition

import (
	"encoding/json"
	"log/slog"
	"math"
	"sync"
	"time"

	"desktop-pet/internal/domain"
)

// NeedModel manages the agent's 6 intrinsic needs — purely in-memory.
// Needs grow via passive decay + active growth each tick, and are satisfied
// by actions. On restart, they start fresh from baseline.
type NeedModel struct {
	mu    sync.RWMutex
	needs domain.IntrinsicNeeds
}

// NewNeedModel creates a NeedModel with baseline neutral needs.
func NewNeedModel() *NeedModel {
	return &NeedModel{
		needs: domain.IntrinsicNeeds{
			Companionship: 0.3,
			Rest:          0.2,
			Play:          0.3,
			Curiosity:     0.4,
			Care:          0.3,
			Autonomy:      0.3,
			LastUpdated:   time.Now().Unix(),
		},
	}
}

// Snapshot returns a copy of the current needs.
func (m *NeedModel) Snapshot() domain.IntrinsicNeeds {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.needs
}

// Modulation returns how needs should modulate emotion rates.
func (m *NeedModel) Modulation() domain.NeedModulation {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.needs.NeedModulation()
}

// Grow advances needs based on elapsed time and context.
func (m *NeedModel) Grow(now time.Time, isWorking bool, hour int, timeSinceChat time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	elapsedHrs := float64(now.Unix()-m.needs.LastUpdated) / 3600.0
	if elapsedHrs <= 0 {
		elapsedHrs = 5.0 / 60.0
	}
	if elapsedHrs > 8 {
		elapsedHrs = 8
	}
	m.needs.LastUpdated = now.Unix()
	idleHrs := timeSinceChat.Hours()

	const decayRate = 0.03
	decay := func(v float64) float64 {
		if v > 0.3 {
			return v - decayRate*elapsedHrs
		}
		return v + decayRate*elapsedHrs
	}
	m.needs.Companionship = decay(m.needs.Companionship)
	m.needs.Rest = decay(m.needs.Rest)
	m.needs.Play = decay(m.needs.Play)
	m.needs.Curiosity = decay(m.needs.Curiosity)
	m.needs.Care = decay(m.needs.Care)
	m.needs.Autonomy = decay(m.needs.Autonomy)

	m.needs.Companionship = clampNeed(m.needs.Companionship + 0.03*elapsedHrs)
	if idleHrs > 1 {
		m.needs.Companionship = clampNeed(m.needs.Companionship + 0.02*elapsedHrs)
	}

	restRate := 0.04
	if hour >= 23 || hour <= 2 {
		restRate = 0.08
	}
	m.needs.Rest = clampNeed(m.needs.Rest + restRate*elapsedHrs)

	m.needs.Play = clampNeed(m.needs.Play + 0.03*elapsedHrs)
	if hour >= 1 && hour <= 5 {
		m.needs.Play = clampNeed(m.needs.Play - 0.04*elapsedHrs)
	}

	curRate := 0.03
	m.needs.Curiosity = clampNeed(m.needs.Curiosity + curRate*elapsedHrs)

	careRate := 0.03
	if isWorking {
		careRate = 0.05
	}
	m.needs.Care = clampNeed(m.needs.Care + careRate*elapsedHrs)

	autoRate := 0.02
	if idleHrs > 2 {
		autoRate = 0.04
	}
	m.needs.Autonomy = clampNeed(m.needs.Autonomy + autoRate*elapsedHrs)
}

// Satisfy applies need satisfaction from an action + outcome.
func (m *NeedModel) Satisfy(action string, outcome domain.OutcomeResult) {
	s := domain.NeedSatisfactionForAction(action, outcome)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.needs.Companionship = clampNeed(m.needs.Companionship + s.Companionship)
	m.needs.Rest = clampNeed(m.needs.Rest + s.Rest)
	m.needs.Play = clampNeed(m.needs.Play + s.Play)
	m.needs.Curiosity = clampNeed(m.needs.Curiosity + s.Curiosity)
	m.needs.Care = clampNeed(m.needs.Care + s.Care)
	m.needs.Autonomy = clampNeed(m.needs.Autonomy + s.Autonomy)
}

func clampNeed(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// LoadFrom loads persisted needs from a key-value store. Returns baseline defaults if missing.
func (m *NeedModel) LoadFrom(load func(string) string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data := load("intrinsic_needs")
	if data == "" {
		return
	}
	var saved domain.IntrinsicNeeds
	if json.Unmarshal([]byte(data), &saved) == nil && saved.LastUpdated > 0 {
		m.needs = saved
	}
}

// SaveTo persists the current needs via a key-value store.
func (m *NeedModel) SaveTo(save func(string, string)) {
	m.mu.RLock()
	data, err := json.Marshal(m.needs)
	m.mu.RUnlock()
	if err != nil {
		slog.Warn("needs: failed to marshal", "err", err)
		return
	}
	save("intrinsic_needs", string(data))
}
