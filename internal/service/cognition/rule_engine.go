package cognition

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"desktop-pet/internal/domain"
)

// ---- Strategy Rule ----

// StrategyRule is a learned behavioral rule that maps conditions to action recommendations.
// Produced by StrategicAgent from action outcomes, these rules can be executed by S1
// (fast path) without LLM involvement once confidence is high enough.
type StrategyRule struct {
	ID                int64         `json:"id"`
	Condition         ConditionExpr `json:"condition"`
	RecommendedAction string        `json:"recommended_action"`
	Suppress          []string      `json:"suppress"` // actions to suppress when rule matches
	Boost             []string      `json:"boost"`    // actions to boost when rule matches
	Confidence        float64       `json:"confidence"`
	Source            string        `json:"source"` // human-readable origin
	SourceType        string        `json:"source_type"` // "strategic_agent" | "immediate_correction" | "manual"
	HitCount          int           `json:"hit_count"`
	AcceptRate        float64       `json:"accept_rate"` // sliding window
	LastHitAt         time.Time     `json:"last_hit_at"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
	Active            bool          `json:"active"`
}

// ConditionExpr is a set of AND-connected constraints on quantified features.
type ConditionExpr struct {
	Constraints []Constraint `json:"constraints"`
}

// Constraint represents a single comparison against a feature field.
type Constraint struct {
	Field string  `json:"field"` // e.g., "U4_ContinuousWorkMins", "R4_RejectionSeverity"
	Op    string  `json:"op"`   // "gt", "lt", "gte", "lte", "eq", "neq"
	Value float64 `json:"value"`
}

// Satisfied checks whether all constraints evaluate to true against the given features.
func (c ConditionExpr) Satisfied(feats *domain.QuantifiedFeatures) bool {
	if feats == nil || len(c.Constraints) == 0 {
		return false
	}
	for _, ct := range c.Constraints {
		v := resolveFeature(feats, ct.Field)
		if !compare(v, ct.Op, ct.Value) {
			return false
		}
	}
	return true
}

// resolveFeature extracts a float64 value from QuantifiedFeatures by field name.
func resolveFeature(f *domain.QuantifiedFeatures, field string) float64 {
	switch field {
	// U group
	case "U3_IsWorking":
		if f.U3_IsWorking > 0 {
			return 1
		}
		return 0
	case "U4_ContinuousWorkMins":
		return f.U4_ContinuousWorkMins
	case "U5_AppSwitchCount":
		return f.U5_AppSwitchCount
	case "U7_LengthTrend":
		return f.U7_LengthTrend
	case "U8_EngagementNorm":
		return f.U8_EngagementNorm
	case "U10_TimeWindowPref":
		return f.U10_TimeWindowPref
	case "U12_NightTime":
		return f.U12_NightTime
	case "U13_IsWeekend":
		if f.U13_IsWeekend > 0 {
			return 1
		}
		return 0
	case "U14_TimeSinceChatMins":
		return f.U14_TimeSinceChatMins

	// A group (emotion)
	case "A1_Affection":
		return f.A1_Affection
	case "A1_Worry":
		return f.A1_Worry
	case "A1_Curiosity":
		return f.A1_Curiosity
	case "A1_Sleepiness":
		return f.A1_Sleepiness
	case "A1_Playfulness":
		return f.A1_Playfulness
	case "A1_Loneliness":
		return f.A1_Loneliness
	case "A1_Confidence":
		return f.A1_Confidence
	case "A1_Annoyance":
		return f.A1_Annoyance
	case "A3_Intensity":
		return f.A3_Intensity
	case "A4_ValenceTrend":
		return f.A4_ValenceTrend
	case "A6_DailyActionCount":
		return f.A6_DailyActionCount
	case "A14_ConsecutiveCount":
		return f.A14_ConsecutiveCount
	case "A11_ActiveInquiries":
		return f.A11_ActiveInquiries
	case "A12_KnowledgeGaps":
		return f.A12_KnowledgeGaps

	// E group
	case "E1_Hour":
		return f.E1_Hour
	case "E3_CooldownNorm":
		return f.E3_CooldownNorm
	case "E4_QuotaRemaining":
		return f.E4_QuotaRemaining
	case "E7_ReflectionDue":
		return f.E7_ReflectionDue

	// R group
	case "R1_OverallAcceptRate":
		return f.R1_OverallAcceptRate
	case "R4_RejectionSeverity":
		return f.R4_RejectionSeverity
	case "R5_NeglectHours":
		return f.R5_NeglectHours
	case "R6_DepthTrend":
		return f.R6_DepthTrend
	case "R8_IntimacyTrend":
		return f.R8_IntimacyTrend

	default:
		return 0
	}
}

func compare(v float64, op string, threshold float64) bool {
	switch op {
	case "gt":
		return v > threshold
	case "lt":
		return v < threshold
	case "gte":
		return v >= threshold
	case "lte":
		return v <= threshold
	case "eq":
		return v == threshold
	case "neq":
		return v != threshold
	default:
		return false
	}
}

// ---- S1 Rule Engine ----

// S1RuleEngine stores and matches strategy rules against feature snapshots.
// It provides the System 1 fast path — when rules with high confidence match,
// the action can be taken immediately without LLM involvement.
type S1RuleEngine struct {
	mu           sync.RWMutex
	rules        []StrategyRule
	fallbackFn   func(feats *domain.QuantifiedFeatures, drives DriveScores) *domain.DecisionOutput
}

// DriveScores bundles the 5 computed drives for fallback scoring.
type DriveScores struct {
	Social, Care, Curious, Quiet, Explore float64
}

// NewS1RuleEngine creates an empty rule engine with no rules.
func NewS1RuleEngine() *S1RuleEngine {
	return &S1RuleEngine{}
}

// SetFallback sets the fallback decision function used when no rules match.
func (e *S1RuleEngine) SetFallback(fn func(feats *domain.QuantifiedFeatures, drives DriveScores) *domain.DecisionOutput) {
	e.fallbackFn = fn
}

// Decide evaluates all active rules against the current feature snapshot.
// Returns the best-matching decision, or nil if no rule matches (caller should escalate).
func (e *S1RuleEngine) Decide(feats *domain.QuantifiedFeatures, drives DriveScores) (*RuleDecision, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Collect all matching rules.
	type match struct {
		rule       StrategyRule
		confidence float64
	}
	var matches []match
	for _, r := range e.rules {
		if !r.Active {
			continue
		}
		if r.Condition.Satisfied(feats) {
			// Weighted confidence: base confidence × freshness decay × acceptance bonus.
			weighted := r.Confidence
			// Rules updated recently get a small freshness bonus.
			if time.Since(r.UpdatedAt) < 24*time.Hour {
				weighted += 0.05
			}
			// Rules with strong acceptance history get a bonus.
			if r.HitCount >= 5 && r.AcceptRate > 0.7 {
				weighted += 0.1
			}
			// Low-confidence rules with few hits get a penalty.
			if r.HitCount < 3 {
				weighted -= 0.1
			}
			weighted = clamp01(weighted)
			matches = append(matches, match{rule: r, confidence: weighted})
		}
	}

	if len(matches) == 0 {
		return nil, false
	}

	// Sort by weighted confidence descending.
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].confidence > matches[j].confidence
	})

	best := matches[0]

	// If the best rule has high enough confidence, return its recommendation.
	if best.confidence >= 0.6 {
		// Build decision from the best rule, applying suppress/boost hints.
		action := best.rule.RecommendedAction
		if action == "" && len(best.rule.Boost) > 0 {
			action = best.rule.Boost[0]
		}
		if action == "" {
			action = "none"
		}

		return &RuleDecision{
			Action:        action,
			Suppress:      best.rule.Suppress,
			Boost:         best.rule.Boost,
			Confidence:    best.confidence,
			RuleID:        best.rule.ID,
			RuleSource:    best.rule.Source,
			MatchedCount:  len(matches),
			TopConfidence: best.confidence,
		}, true
	}

	// Low-confidence match — escalate to S2.
	return &RuleDecision{
		Action:        best.rule.RecommendedAction,
		Confidence:    best.confidence,
		RuleID:        best.rule.ID,
		RuleSource:    best.rule.Source,
		MatchedCount:  len(matches),
		TopConfidence: best.confidence,
		NeedsLLM:      true,
	}, true
}

// RuleDecision is the output of the S1 rule engine.
type RuleDecision struct {
	Action        string   `json:"action"`
	Suppress      []string `json:"suppress"`
	Boost         []string `json:"boost"`
	Confidence    float64  `json:"confidence"`
	RuleID        int64    `json:"rule_id"`
	RuleSource    string   `json:"rule_source"`
	MatchedCount  int      `json:"matched_count"`
	TopConfidence float64  `json:"top_confidence"`
	NeedsLLM      bool     `json:"needs_llm"` // true when match exists but confidence is low
}

// ---- CRUD ----

// AddRule inserts a new strategy rule (or updates if ID already exists).
func (e *S1RuleEngine) AddRule(rule StrategyRule) int64 {
	e.mu.Lock()
	defer e.mu.Unlock()

	if rule.ID == 0 {
		rule.ID = time.Now().UnixNano()
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now()
	}
	if rule.UpdatedAt.IsZero() {
		rule.UpdatedAt = time.Now()
	}
	rule.Active = true

	// Replace existing rule with same ID.
	for i, r := range e.rules {
		if r.ID == rule.ID {
			e.rules[i] = rule
			return rule.ID
		}
	}
	e.rules = append(e.rules, rule)
	return rule.ID
}

// UpdateConfidence adjusts a rule's confidence based on new feedback.
func (e *S1RuleEngine) UpdateConfidence(id int64, accepted bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i, r := range e.rules {
		if r.ID == id {
			e.rules[i].HitCount++
			// Sliding window acceptance rate.
			alpha := 0.2
			reward := 0.0
			if accepted {
				reward = 1.0
			}
			e.rules[i].AcceptRate = e.rules[i].AcceptRate*(1-alpha) + reward*alpha

			// Adjust confidence toward acceptance rate.
			e.rules[i].Confidence = e.rules[i].Confidence*0.7 + e.rules[i].AcceptRate*0.3
			e.rules[i].LastHitAt = time.Now()
			e.rules[i].UpdatedAt = time.Now()

			// Deactivate rules with consistently poor acceptance.
			if e.rules[i].HitCount >= 10 && e.rules[i].AcceptRate < 0.3 {
				e.rules[i].Active = false
			}
			return
		}
	}
}

// DeactivateRule marks a rule as inactive.
func (e *S1RuleEngine) DeactivateRule(id int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, r := range e.rules {
		if r.ID == id {
			e.rules[i].Active = false
			e.rules[i].UpdatedAt = time.Now()
			return
		}
	}
}

// ActiveRules returns all active rules, sorted by confidence descending.
func (e *S1RuleEngine) ActiveRules() []StrategyRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var active []StrategyRule
	for _, r := range e.rules {
		if r.Active {
			active = append(active, r)
		}
	}
	sort.Slice(active, func(i, j int) bool {
		return active[i].Confidence > active[j].Confidence
	})
	return active
}

// Count returns the number of active rules.
func (e *S1RuleEngine) Count() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	n := 0
	for _, r := range e.rules {
		if r.Active {
			n++
		}
	}
	return n
}

// Describe returns a human-readable summary of the best-matching rule for debugging.
func (e *S1RuleEngine) Describe(feats *domain.QuantifiedFeatures) string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("S1 Rule Engine: %d active rules\n", e.Count()))
	matched := 0
	for _, r := range e.rules {
		if !r.Active {
			continue
		}
		if r.Condition.Satisfied(feats) {
			matched++
			sb.WriteString(fmt.Sprintf("  ✓ #%d: %s (conf=%.2f, hits=%d, accept=%.0f%%)\n",
				r.ID, r.Source, r.Confidence, r.HitCount, r.AcceptRate*100))
			if matched >= 5 {
				break
			}
		}
	}
	if matched == 0 {
		sb.WriteString("  (no matching rules)\n")
	}
	return sb.String()
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
