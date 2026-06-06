package scheduler

import (
	"math"
	"sync"
	"time"

	"desktop-pet/internal/domain"
)

// AdaptiveParams holds learnable decision thresholds.
type AdaptiveParams struct {
	AnnoyanceThreshold     float64 // annoyance above which we go silent
	FocusSuppressThreshold float64 // focus level above which we don't interrupt
}

// DefaultParams returns sensible defaults.
func DefaultParams() AdaptiveParams {
	return AdaptiveParams{
		AnnoyanceThreshold:     0.5,
		FocusSuppressThreshold: 0.85,
	}
}

// Scheduler is the System 1 safety gate. System 2 (LLM) makes autonomous
// decisions; the Scheduler validates them against hard constraints.
type Scheduler struct {
	mu              sync.Mutex
	lastActionAt    time.Time
	lastBySource    map[domain.ProactiveSource]time.Time
	escalationBySrc map[domain.ProactiveSource]int
	cooldown        time.Duration
	dailyCount      int
	maxDaily        int
	lastResetDay    int64

	outcomeRepo   domain.ActionOutcomeRepository
	curiosityRepo domain.CuriosityRepository
	minSamples    int
	params        AdaptiveParams
}

// NewScheduler creates a Scheduler with sensible defaults.
func NewScheduler() *Scheduler {
	return &Scheduler{
		cooldown:        5 * time.Minute,
		maxDaily:        50,
		minSamples:      5,
		params:          DefaultParams(),
		lastBySource:    make(map[domain.ProactiveSource]time.Time),
		escalationBySrc: make(map[domain.ProactiveSource]int),
	}
}

func (s *Scheduler) SetOutcomeRepo(repo domain.ActionOutcomeRepository) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomeRepo = repo
}

func (s *Scheduler) SetCuriosityRepo(repo domain.CuriosityRepository) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.curiosityRepo = repo
}

// ValidateDecision is the System 1 safety gate for System 2 LLM decisions.
func (s *Scheduler) ValidateDecision(in domain.SchedulerInput, dec *domain.DecisionOutput) (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Only hard gate: daily cap (queried from DB, survives restart).
	todayCount := s.DailyCount()
	if todayCount >= s.maxDaily {
		return false, "daily cap"
	}

	return true, ""
}
// MarkReplied resets the escalation counter when the user replies.
func (s *Scheduler) MarkReplied(source domain.ProactiveSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.escalationBySrc[source] = 0
}

// DailyCount returns today's action count from the outcome table.
func (s *Scheduler) DailyCount() int {
	if s.outcomeRepo == nil {
		return 0
	}
	_, total := s.outcomeRepo.SuccessRate(domain.ActionContext{}, 1)
	return total
}

// MaxDaily returns the current daily quota, clamped to a safe range.
func (s *Scheduler) MaxDaily() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxDaily
}

// LearnParams tunes thresholds and daily quota from historical outcomes.
func (s *Scheduler) LearnParams() {
	if s.outcomeRepo == nil {
		return
	}
	ctx := domain.ActionContext{}
	accepts, total := s.outcomeRepo.SuccessRate(ctx, 30)
	if total < 20 {
		return
	}
	rate := float64(accepts) / float64(total)

	// Annoyance threshold.
	if rate < 0.3 {
		s.params.AnnoyanceThreshold -= 0.05
	} else if rate > 0.6 {
		s.params.AnnoyanceThreshold += 0.05
	}
	s.params.AnnoyanceThreshold = clamp(s.params.AnnoyanceThreshold, 0.3, 0.7)

	// Adaptive daily quota: high acceptance → more room, low → tighten.
	if rate > 0.6 && s.maxDaily < 70 {
		s.maxDaily += 5
	} else if rate < 0.3 && s.maxDaily > 25 {
		s.maxDaily -= 5
	}
	s.maxDaily = clampInt(s.maxDaily, 25, 70)
}

// ---- adaptive helpers ----

func (s *Scheduler) adaptiveCooldown(source domain.ProactiveSource, in domain.SchedulerInput) time.Duration {
	if s.outcomeRepo == nil {
		switch source {
		case domain.SourceCare:
			return 20 * time.Minute
		case domain.SourceKnowledgeGap:
			return 30 * time.Minute
		default:
			return 10 * time.Minute
		}
	}
	ctx := domain.ActionContext{Source: source}
	accepts, total := s.outcomeRepo.SuccessRate(ctx, 14)
	if total < s.minSamples {
		return 15 * time.Minute
	}
	rate := float64(accepts) / float64(total)
	if rate > 0.5 {
		return 8 * time.Minute // shorter cooldown for successful types
	}
	return 25 * time.Minute // longer cooldown for unsuccessful types
}

func clamp(v, lo, hi float64) float64 {
	if v < lo { return lo }
	if v > hi { return hi }
	return v
}

func clampInt(v, lo, hi int) int {
	if v < lo { return lo }
	if v > hi { return hi }
	return v
}

func clampScore(s float64) float64 { return math.Max(0, math.Min(1, s)) }
