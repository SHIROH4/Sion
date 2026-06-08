package cognition

import (
	"strings"

	"desktop-pet/internal/domain"
)

// DecisionPath represents which decision layer should handle the current tick.
type DecisionPath int

const (
	PathNone     DecisionPath = iota // safety gate blocked, do nothing
	PathS1                           // rule engine, 0 token
	PathS2Lite                       // lightweight LLM, ~200 tokens
	PathS2Full                       // full LLM, ~370 tokens
)

func (d DecisionPath) String() string {
	switch d {
	case PathNone:
		return "none"
	case PathS1:
		return "S1"
	case PathS2Lite:
		return "S2-lite"
	case PathS2Full:
		return "S2-full"
	default:
		return "unknown"
	}
}

// MetaReasoner evaluates the current situation and decides whether to use
// S1 (rule engine), S2-lite (lightweight LLM), S2-full (full LLM), or none.
//
// Inspired by SOFAI (IBM/Oxford, 2025) — the metacognitive layer evaluates
// complexity, policy coverage, confidence, and risk to route decisions.
type MetaReasoner struct {
	// Thresholds (tunable).
	LowComplexity    float64 // below this → consider S1
	HighComplexity   float64 // above this → must use S2Full
	HighRisk         float64 // above this → must use S2Full
	MinCoverage      float64 // below this → must use LLM (no S1)
	MinConfidence    float64 // below this → escalate to LLM
}

// DefaultMetaReasoner returns a MetaReasoner with sensible defaults.
func DefaultMetaReasoner() *MetaReasoner {
	return &MetaReasoner{
		LowComplexity:  0.3,
		HighComplexity: 0.6,
		HighRisk:       0.7,
		MinCoverage:    0.5,
		MinConfidence:  0.6,
	}
}

// Route evaluates the current features, matched rules, and context to decide
// which decision path to take.
func (m *MetaReasoner) Route(
	feats *domain.QuantifiedFeatures,
	ruleResult *RuleDecision,
	hasConflict bool,
	hasExtremeEmotion bool,
) DecisionPath {
	// 1. Compute risk level.
	risk := m.computeRisk(feats, hasExtremeEmotion)

	// 2. High risk → must use full LLM for careful handling.
	if risk >= m.HighRisk {
		return PathS2Full
	}

	// 3. If rule engine produced a confident match → S1.
	if ruleResult != nil && !ruleResult.NeedsLLM && ruleResult.Confidence >= m.MinConfidence {
		return PathS1
	}

	// 4. If rule engine matched but confidence is low → escalate.
	if ruleResult != nil && ruleResult.NeedsLLM {
		// Determine whether lightweight or full context is enough.
		if hasConflict || hasExtremeEmotion {
			return PathS2Full
		}
		return PathS2Lite
	}

	// 5. No rule match → compute complexity to decide lite vs full.
	complexity := m.computeComplexity(feats, hasConflict)

	if complexity >= m.HighComplexity || hasExtremeEmotion {
		return PathS2Full
	}
	if complexity >= m.LowComplexity {
		return PathS2Lite
	}

	// 6. Low complexity, no matching rules → default to S2-lite for safety.
	return PathS2Lite
}

// computeRisk estimates how risky it would be to make a wrong decision right now.
// High risk = night time, recent rejections, user working intensively, extreme emotion.
func (m *MetaReasoner) computeRisk(feats *domain.QuantifiedFeatures, hasExtremeEmotion bool) float64 {
	if feats == nil {
		return 0.5
	}

	risk := 0.0

	// Night time: moderate risk of disturbing user.
	if feats.U12_NightTime > 0 {
		risk += 0.3
	}

	// Recent rejections: user is signaling "leave me alone".
	if feats.R4_RejectionSeverity > 0.5 {
		risk += 0.4
	} else if feats.R4_RejectionSeverity > 0.2 {
		risk += 0.2
	}

	// Deep work: user is focused, interruption costly.
	if feats.U3_IsWorking > 0 && feats.U4_ContinuousWorkMins > 60 {
		risk += 0.2
	}

	// Extreme emotion: wrong response could escalate the situation.
	if hasExtremeEmotion {
		risk += 0.3
	}

	// Low intimacy trend: relationship is fragile.
	if feats.R8_IntimacyTrend < -0.1 {
		risk += 0.15
	}

	return clamp01(risk)
}

// computeComplexity estimates how complicated the current situation is.
// High complexity = conflicting signals, unusual patterns, multiple factors.
func (m *MetaReasoner) computeComplexity(feats *domain.QuantifiedFeatures, hasConflict bool) float64 {
	if feats == nil {
		return 0.5
	}

	complexity := 0.0

	// Multiple rules matched → conflicting advice.
	if hasConflict {
		complexity += 0.4
	}

	// Working + night + weekend = unusual combination.
	unusualFactors := 0
	if feats.U3_IsWorking > 0 {
		unusualFactors++
	}
	if feats.U12_NightTime > 0 {
		unusualFactors++
	}
	if feats.U13_IsWeekend > 0 {
		unusualFactors++
	}
	if unusualFactors >= 2 {
		complexity += 0.2
	}

	// High emotion intensity + working = complex emotional-work boundary.
	if feats.A3_Intensity > 0.6 && feats.U3_IsWorking > 0 {
		complexity += 0.15
	}

	// Extreme needs (very high or very low) suggest an unusual state.
	if feats.U7_LengthTrend < -0.3 || feats.U7_LengthTrend > 0.3 {
		complexity += 0.1
	}

	// Unusually high or low engagement.
	if feats.U8_EngagementNorm < 0.3 {
		complexity += 0.1
	}

	return clamp01(complexity)
}

// buildS2LitePrompt produces a compact decision prompt (~200 tokens).
func buildS2LitePrompt(feats *domain.QuantifiedFeatures, ruleResult *RuleDecision) string {
	var sb strings.Builder
	sb.WriteString("你是诗音。快速判断: ")

	if feats != nil {
		workLabel := "休闲"
		if feats.U3_IsWorking > 0 && feats.U4_ContinuousWorkMins >= 5 {
			workLabel = "工作中"
		}
		sb.WriteString(feats.U1_AppCategory + "/" + workLabel)
		if feats.U12_NightTime > 0 {
			sb.WriteString(" 深夜")
		}
	}
	if ruleResult != nil {
		sb.WriteString(" | 规则建议: " + ruleResult.Action)
		sb.WriteString(" (置信度" + formatPct(ruleResult.Confidence) + ")")
	}
	sb.WriteString("。选择一个动作。")

	return sb.String()
}

// buildS2FullPrompt produces a comprehensive decision prompt (~370 tokens).
func buildS2FullPrompt(
	feats *domain.QuantifiedFeatures,
	needs *domain.IntrinsicNeeds,
	emotionVec *domain.EmotionVector,
	emotionState *domain.EmotionState,
	ruleResult *RuleDecision,
	recentExperiences []ExperienceRecord,
	timeSinceLastChat float64,
) string {
	var sb strings.Builder
	sb.WriteString("你是诗音，一只关心主人的猫娘桌面宠物。现在需要你判断最合适的动作。\n\n")

	// User context.
	if feats != nil {
		sb.WriteString("[主人]\n")
		workLabel := "休闲中"
		if feats.U3_IsWorking > 0 && feats.U4_ContinuousWorkMins >= 5 {
			workLabel = "工作中(已" + formatMins(feats.U4_ContinuousWorkMins) + ")"
		}
		sb.WriteString("当前: " + feats.U1_AppCategory + " " + workLabel)
		if feats.U12_NightTime > 0 {
			sb.WriteString(" ⚠️深夜")
		}
		if feats.R4_RejectionSeverity > 0.3 {
			sb.WriteString(" ⚠️最近被拒")
		}
		sb.WriteString("\n")
		sb.WriteString("时段: " + formatMins(timeSinceLastChat) + "前互动\n")
	}

	// Emotion.
	if emotionVec != nil && emotionState != nil {
		sb.WriteString("[情绪] " + emotionState.Primary)
		sb.WriteString(" 亲密度" + formatPct(emotionVec.Affection))
		sb.WriteString(" 困倦" + formatPct(emotionVec.Sleepiness))
		sb.WriteString(" 烦躁" + formatPct(emotionVec.Annoyance) + "\n")
	}

	// Needs.
	if needs != nil {
		sb.WriteString("[需求] ")
		sb.WriteString("陪伴" + formatPct(needs.Companionship))
		sb.WriteString(" 关怀" + formatPct(needs.Care))
		sb.WriteString(" 好奇" + formatPct(needs.Curiosity))
		sb.WriteString(" 休息" + formatPct(needs.Rest) + "\n")
	}

	// Rule recommendation (if any).
	if ruleResult != nil {
		sb.WriteString("[规则] 建议: " + ruleResult.Action)
		sb.WriteString(" (" + ruleResult.RuleSource + ", 置信度" + formatPct(ruleResult.Confidence) + ")")
		if ruleResult.MatchedCount > 1 {
			sb.WriteString(" 另有" + itoa(ruleResult.MatchedCount-1) + "条规则也匹配")
		}
		sb.WriteString("\n")
	}

	// Recent experiences.
	if len(recentExperiences) > 0 {
		sb.WriteString("[最近]\n")
		for i, exp := range recentExperiences {
			if i >= 3 {
				break
			}
			sb.WriteString("- " + exp.Summary + "\n")
		}
	}

	sb.WriteString("\n选择一个动作。如果规则建议合理就采纳；如果有更好的选择也可以不采纳。")
	return sb.String()
}

// ---- Helpers ----

func formatPct(v float64) string {
	return itoa(int(v * 100)) + "%"
}

func formatMins(v float64) string {
	return itoa(int(v)) + "分钟"
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + itoa(n%10)
}
