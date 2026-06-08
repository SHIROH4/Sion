package cognition

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"desktop-pet/internal/domain"
)

// ---- Experience Record ----

// ExperienceRecord is a decision+outcome pair stored for experience injection.
// Inspired by RaDAgent (ICLR 2025) — similar-scene experience comparison.
type ExperienceRecord struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"`
	Outcome   string    `json:"outcome"` // "accepted", "ignored", "rejected"
	Context   string    `json:"context"` // compact scene description
	Summary   string    `json:"summary"` // "speak_casual '在写什么呀' → 无视"
	FeatSnap  string    `json:"feat_snap"` // key features at decision time (compact)
	CreatedAt time.Time `json:"created_at"`
}

// featKey produces a compact scene fingerprint for similarity matching.
func featKey(feats *domain.QuantifiedFeatures) string {
	if feats == nil {
		return ""
	}
	work := "idle"
	if feats.U3_IsWorking > 0 {
		if feats.U4_ContinuousWorkMins > 60 {
			work = "deepwork"
		} else {
			work = "working"
		}
	}
	night := "day"
	if feats.U12_NightTime > 0 {
		night = "night"
	}
	rej := "ok"
	if feats.R4_RejectionSeverity > 0.3 {
		rej = "rej"
	}
	return fmt.Sprintf("%s-%s-%s", work, night, rej)
}

// ---- Immediate Corrector ----

// ImmediateCorrector handles sub-minute corrections to behavior based on
// immediate user feedback. Inspired by Agent-R (2025).
type ImmediateCorrector struct {
	mu           sync.RWMutex
	suppressions map[string]time.Time // action → suppressed until
	ruleEngine   *S1RuleEngine
}

// NewImmediateCorrector creates a new immediate corrector.
func NewImmediateCorrector(ruleEngine *S1RuleEngine) *ImmediateCorrector {
	return &ImmediateCorrector{
		suppressions: make(map[string]time.Time),
		ruleEngine:   ruleEngine,
	}
}

// OnOutcome processes a single outcome and applies immediate corrections.
func (c *ImmediateCorrector) OnOutcome(outcome domain.ActionOutcome, decisionAction string, feats *domain.QuantifiedFeatures) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Clean expired suppressions.
	for action, until := range c.suppressions {
		if now.After(until) {
			delete(c.suppressions, action)
		}
	}

	// Rejected → suppress briefly.
	if outcome.Outcome == domain.OutcomeRejected {
		c.suppressions[decisionAction] = now.Add(30 * time.Minute)
		if strings.HasPrefix(decisionAction, "speak") {
			c.suppressions["speak_casual"] = now.Add(15 * time.Minute)
			c.suppressions["speak_inquiry"] = now.Add(15 * time.Minute)
		}
		if feats != nil {
			c.createQuickRule(decisionAction, feats)
		}
	}

	// Accepted (replied or engaged) → clear suppression.
	if outcome.Outcome == domain.OutcomeReplied || outcome.Outcome == domain.OutcomeEngaged {
		delete(c.suppressions, decisionAction)
	}
}

// IsSuppressed checks if an action is currently under immediate suppression.
func (c *ImmediateCorrector) IsSuppressed(action string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	until, ok := c.suppressions[action]
	return ok && time.Now().Before(until)
}

// SuppressedActions returns all currently suppressed actions.
func (c *ImmediateCorrector) SuppressedActions() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := time.Now()
	var actions []string
	for action, until := range c.suppressions {
		if now.Before(until) {
			actions = append(actions, action)
		}
	}
	return actions
}

// createQuickRule generates a low-confidence temporary rule from a rejection.
func (c *ImmediateCorrector) createQuickRule(action string, feats *domain.QuantifiedFeatures) {
	if c.ruleEngine == nil {
		return
	}
	if feats != nil && feats.U3_IsWorking > 0 {
		c.ruleEngine.AddRule(StrategyRule{
			Condition: ConditionExpr{
				Constraints: []Constraint{
					{Field: "U3_IsWorking", Op: "gt", Value: 0},
				},
			},
			Suppress:   []string{action},
			Confidence: 0.45,
			Source:     "主人工作时被拒绝，暂不主动搭话",
			SourceType: "immediate_correction",
		})
	}
}

// ---- Unified Feedback Processor ----

// UnifiedFeedbackProcessor is the single entry point for all learning.
// Replaces scattered Learner + StrategicAgent + Personality updates.
//
// Three time scales:
//   Immediate: sub-second → minute (ImmediateCorrector)
//   Strategic: hourly → daily (StrategyDistiller)
//   Personality: weekly → monthly (PersonalityAdapter)
type UnifiedFeedbackProcessor struct {
	Immediate  *ImmediateCorrector
	RuleEngine *S1RuleEngine

	mu          sync.RWMutex
	experiences []ExperienceRecord
	maxExp      int

	OnStrategicDistill func(experiences []ExperienceRecord, rules []StrategyRule) error
	lastDistillAt      time.Time
	distillInterval    time.Duration
}

// NewUnifiedFeedbackProcessor creates a new processor.
func NewUnifiedFeedbackProcessor(ruleEngine *S1RuleEngine, maxExperiences int) *UnifiedFeedbackProcessor {
	return &UnifiedFeedbackProcessor{
		Immediate:       NewImmediateCorrector(ruleEngine),
		RuleEngine:      ruleEngine,
		experiences:     make([]ExperienceRecord, 0, maxExperiences),
		maxExp:          maxExperiences,
		distillInterval: 6 * time.Hour,
	}
}

// Process is called after every action outcome is recorded.
func (p *UnifiedFeedbackProcessor) Process(
	outcome domain.ActionOutcome,
	decisionAction string,
	feats *domain.QuantifiedFeatures,
) {
	// 1. Immediate correction.
	p.Immediate.OnOutcome(outcome, decisionAction, feats)

	// 2. Store experience.
	p.storeExperience(outcome, decisionAction, feats)

	// 3. Check if it's time for strategic distillation.
	if p.shouldDistill() {
		p.distill()
	}
}

func (p *UnifiedFeedbackProcessor) storeExperience(outcome domain.ActionOutcome, decisionAction string, feats *domain.QuantifiedFeatures) {
	p.mu.Lock()
	defer p.mu.Unlock()

	outcomeLabel := "?"
	switch outcome.Outcome {
	case domain.OutcomeReplied, domain.OutcomeEngaged:
		outcomeLabel = "✅ 被接受"
	case domain.OutcomeRejected:
		outcomeLabel = "❌ 被拒绝"
	case domain.OutcomeIgnored:
		outcomeLabel = "⏳ 被无视"
	}

	exp := ExperienceRecord{
		ID:        time.Now().UnixNano(),
		Action:    decisionAction,
		Outcome:   outcomeLabel,
		Context:   featKey(feats),
		FeatSnap:  featKey(feats),
		Summary:   fmt.Sprintf("%s → %s (场景: %s)", decisionAction, outcomeLabel, featKey(feats)),
		CreatedAt: time.Now(),
	}

	p.experiences = append(p.experiences, exp)
	if len(p.experiences) > p.maxExp {
		p.experiences = p.experiences[len(p.experiences)-p.maxExp:]
	}
}

// RecentExperiences returns the most recent N experiences for context injection.
func (p *UnifiedFeedbackProcessor) RecentExperiences(n int) []ExperienceRecord {
	p.mu.RLock()
	defer p.mu.RUnlock()

	start := len(p.experiences) - n
	if start < 0 {
		start = 0
	}
	result := make([]ExperienceRecord, len(p.experiences)-start)
	copy(result, p.experiences[start:])
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// SimilarExperiences returns experiences with the same scene fingerprint.
func (p *UnifiedFeedbackProcessor) SimilarExperiences(feats *domain.QuantifiedFeatures, n int) []ExperienceRecord {
	p.mu.RLock()
	defer p.mu.RUnlock()

	key := featKey(feats)
	var matches []ExperienceRecord
	for i := len(p.experiences) - 1; i >= 0 && len(matches) < n; i-- {
		if p.experiences[i].FeatSnap == key {
			matches = append(matches, p.experiences[i])
		}
	}
	return matches
}

func (p *UnifiedFeedbackProcessor) shouldDistill() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return time.Since(p.lastDistillAt) > p.distillInterval &&
		len(p.experiences) >= 10
}

func (p *UnifiedFeedbackProcessor) distill() {
	if p.OnStrategicDistill == nil {
		return
	}
	p.mu.Lock()
	exps := make([]ExperienceRecord, len(p.experiences))
	copy(exps, p.experiences)
	p.lastDistillAt = time.Now()
	p.mu.Unlock()

	rules := p.RuleEngine.ActiveRules()
	_ = p.OnStrategicDistill(exps, rules)
}

// InjectExperiences formats recent similar experiences for LLM context injection.
func (p *UnifiedFeedbackProcessor) InjectExperiences(feats *domain.QuantifiedFeatures) string {
	similar := p.SimilarExperiences(feats, 3)
	if len(similar) == 0 {
		return ""
	}
	result := ""
	for _, exp := range similar {
		result += "- " + exp.Summary + "\n"
	}
	return result
}
