package cognition

import (
	"encoding/json"
	"log/slog"
	"math"
	"os"
	"sync"
	"time"

	"desktop-pet/internal/domain"
	"desktop-pet/internal/infra/storage"
)

// ---- Drive Calculator ----

// ComputeDrives calculates the 5 internal drives from quantified features and intrinsic needs.
// Each drive blends: emotion (base, ~50%) + need push (~15%) + user context (~20%) + relationship gate (~15%).
// feats and needs may be nil — the function degrades to emotion-only computation.
func ComputeDrives(feats *domain.QuantifiedFeatures, needs *domain.IntrinsicNeeds) (social, care, curious, quiet, explore float64) {
	// ---- Extract base values (emotion or defaults) ----
	loneliness := 0.4
	playfulness := 0.35
	affection := 0.45
	annoyance := 0.1
	worry := 0.3
	curiosity := 0.4
	sleepiness := 0.2
	hour := time.Now().Hour()
	timeSinceMins := 15.0 // assume 15 min idle when no data
	dailyCount := 0

	if feats != nil {
		loneliness = feats.A1_Loneliness
		playfulness = feats.A1_Playfulness
		affection = feats.A1_Affection
		annoyance = feats.A1_Annoyance
		worry = feats.A1_Worry
		curiosity = feats.A1_Curiosity
		sleepiness = feats.A1_Sleepiness
		hour = int(feats.E1_Hour)
		timeSinceMins = feats.U14_TimeSinceChatMins
		dailyCount = int(feats.A6_DailyActionCount)
	}

	// ---- Time-based factors (from feats or computed) ----
	timeFactor := storage.Clamp01(1.0 - timeSinceMins/30.0) // 1.0 when just chatted, 0.0 after 30min
	idleBonus := storage.Clamp01(timeSinceMins / 120.0)      // grows to 1.0 after 2h silence
	nightBonus := 0.0
	if hour >= 23 || hour <= 4 {
		nightBonus = 0.6
	}
	mealBonus := 0.0
	if (hour >= 11 && hour <= 13) || (hour >= 17 && hour <= 20) {
		mealBonus = 0.5
	}

	hasInquiry := 0.0
	hasGaps := 0.0
	if feats != nil {
		if feats.A11_ActiveInquiries > 0 {
			hasInquiry = 0.6
		}
		if feats.A12_KnowledgeGaps > 0 {
			hasGaps = 0.4
		}
	}

	// ================================================================
	// Social Drive
	// ================================================================
	social = 0.40*storage.Clamp01(loneliness) +
		0.25*storage.Clamp01(playfulness) +
		0.20*idleBonus +
		0.10*storage.Clamp01(affection) +
		0.05*(1.0-storage.Clamp01(annoyance))

	// Need push.
	if needs != nil {
		social += needs.Companionship * 0.12
		social += needs.Play * 0.08
	}

	// User context modulation.
	if feats != nil {
		// Working -> suppress social.
		social -= feats.U3_IsWorking * 0.15
		// High app switching + working = distracted -> don't bother.
		if feats.U3_IsWorking > 0 && feats.U5_AppSwitchNorm > 0.6 {
			social -= 0.10
		}
		// Night time -> reduce social (unless care is driving it).
		social -= feats.U12_NightTime * 0.15
	}

	// Relationship gate.
	social = applyInteractionGate(social, feats)

	// Rejection severity -> strong suppression.
	if feats != nil {
		social -= feats.R4_RejectionSeverity * 0.35
	}

	social = storage.Clamp01(social)

	// ================================================================
	// Care Drive
	// ================================================================
	care = 0.40*storage.Clamp01(worry) +
		0.20*storage.Clamp01(affection) +
		0.15*nightBonus +
		0.10*mealBonus

	// Need push.
	if needs != nil {
		care += needs.Care * 0.18
	}

	// User context modulation.
	if feats != nil {
		// Continuous work -> increase care.
		care += feats.U4_ContinuousWorkNorm * 0.15
		// Late night + working -> extra care.
		if feats.U12_NightTime > 0 && feats.U3_IsWorking > 0 {
			care += 0.10
		}
		// Weekend -> slightly more care (user might forget self-care).
		care += feats.U13_IsWeekend * 0.05
	}

	// Relationship gate.
	care = applyInteractionGate(care, feats)

	care = storage.Clamp01(care)

	// ================================================================
	// Curious Drive
	// ================================================================
	curious = 0.35*storage.Clamp01(curiosity) +
		0.25*hasInquiry +
		0.20*hasGaps +
		0.15*(1.0-timeFactor)

	// Need push.
	if needs != nil {
		curious += needs.Curiosity * 0.18
	}

	// User context modulation.
	if feats != nil {
		// Learning momentum -> boost curiosity.
		curious += feats.A13_LearningMomentum * 0.07
		// Preference diversity -> more to explore.
		curious += feats.U16_PrefDiversity * 0.05
	}

	// Relationship gate (milder — curiosity is less sensitive to rejection).
	if feats != nil {
		gate := interactionGate(feats.R1_OverallAcceptRate)
		curious *= 0.7 + gate*0.3 // only 30% modulated by rejection
	}

	curious = storage.Clamp01(curious)

	// ================================================================
	// Quiet Drive
	// ================================================================
	quiet = 0.20*storage.Clamp01(sleepiness) +
		0.15*timeFactor +
		0.25*storage.Clamp01(annoyance) +
		0.10*storage.Clamp01(float64(idleBias(dailyCount))*0.1)

	// Need push.
	if needs != nil {
		quiet += needs.Rest * 0.18
	}

	// User context modulation.
	if feats != nil {
		// Recent action -> cooldown boost (gradually fades over 30min).
		quiet += (1.0 - feats.E3_CooldownNorm) * 0.15
		// Working -> prefer quiet.
		quiet += feats.U3_IsWorking * 0.12
		// Night time -> prefer quiet.
		quiet += feats.U12_NightTime * 0.08
		// Low quota remaining -> conserve actions.
		if feats.E4_QuotaRemaining < 5 {
			quiet += 0.10
		}
	}

	// Relationship: rejection -> be quiet (inverse of social suppression).
	if feats != nil {
		quiet += feats.R4_RejectionSeverity * 0.40
	}

	quiet = storage.Clamp01(quiet)

	// ================================================================
	// Explore Drive
	// ================================================================
	// Base: curiosity + idle time + knowledge gaps.
	gapBoost := 0.0
	if feats != nil {
		gapBoost = saturateNorm(float64(feats.A12_KnowledgeGaps), 5) * 0.25
	}
	explore = 0.30*storage.Clamp01(curiosity) +
		0.20*(1.0-timeFactor) +
		gapBoost

	// Need push.
	if needs != nil {
		explore += needs.Curiosity * 0.15
		explore += needs.Autonomy * 0.15
	}

	// User context modulation.
	if feats != nil {
		// Not working -> more room for exploration.
		if feats.U3_IsWorking < 0.5 {
			explore += 0.08
		}
		// Reflection due -> push explore.
		explore += feats.E7_ReflectionDue * 0.10
		// Active inquiries -> push explore (wants to learn things to tell user).
		explore += saturateNorm(float64(feats.A11_ActiveInquiries), 5) * 0.12
		// Low recent activity -> good time for background exploration.
		if feats.E3_MinsSinceAction > 30 {
			explore += 0.10
		}
	}

	// Relationship gate (mild — exploration is background activity).
	if feats != nil {
		gate := interactionGate(feats.R1_OverallAcceptRate)
		explore *= 0.8 + gate*0.2
	}

	explore = storage.Clamp01(explore)

	return
}

// ---- Relationship Gate ----

// interactionGate returns a multiplier [0,1] based on overall acceptance rate.
// Low acceptance -> strong suppression of proactive drives.
func interactionGate(acceptRate float64) float64 {
	if acceptRate <= 0 {
		return 1.0 // no data -> no suppression
	}
	if acceptRate >= 0.5 {
		return 1.0 // healthy -> no suppression
	}
	// 0.0 -> 0.5, 0.25 -> 0.75, 0.5 -> 1.0
	return 0.5 + acceptRate
}

// applyInteractionGate applies the acceptance-rate gate to a drive value.
func applyInteractionGate(drive float64, feats *domain.QuantifiedFeatures) float64 {
	if feats == nil || feats.R1_SampleCount < 3 {
		return drive // insufficient data
	}
	return drive * interactionGate(feats.R1_OverallAcceptRate)
}

// ---- Action-Weight Matrix ----

type ActionWeight struct {
	Social  float64 `json:"social"`
	Care    float64 `json:"care"`
	Curious float64 `json:"curious"`
	Quiet   float64 `json:"quiet"`
	Explore float64 `json:"explore"`
}

// defaultWeights are now defined in actions.go — use BuildWeightsMap() at runtime.

// actionToType moved to actions.go

// actionToSource maps action names to ProactiveSource for R3 lookup.
// actionToSource moved to actions.go

// isSpeakAction moved to actions.go


// ---- Motivator ----

type Motivator struct {
	mu      sync.RWMutex
	weights map[string]ActionWeight
	path    string
}

func NewMotivator() *Motivator {
	return &Motivator{weights: BuildWeightsMap()}
}

func (m *Motivator) SetStoragePath(path string) {
	m.path = path
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var loaded map[string]ActionWeight
	if json.Unmarshal(data, &loaded) == nil {
		m.mu.Lock()
		for k, v := range loaded {
			m.weights[k] = v
		}
		m.mu.Unlock()
	}
}

func (m *Motivator) Save() {
	if m.path == "" {
		return
	}
	m.mu.RLock()
	data, _ := json.Marshal(m.weights)
	m.mu.RUnlock()
	if err := os.WriteFile(m.path, data, 0644); err != nil {
		slog.Warn("motivator: failed to save weights", "err", err)
	}
}

// ScoreActions computes the weighted score for each action and returns the winner,
// its score, and the second-best score (for conflict detection via RouteToLLMWithScores).
// suggestions are active care triggers from CareEngine — they add a priority-based
// bonus to the corresponding care action, making them competitive with organic actions.
func (m *Motivator) ScoreActions(social, care, curious, quiet, explore float64, feats *domain.QuantifiedFeatures, suggestions []domain.CareSuggestion) (action string, score float64, secondScore float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Hard night gate (22:00-08:00): only rest/health care actions allowed.
	// All social/casual/inquiry/explore actions are suppressed to avoid disturbing sleep.
	if feats != nil && feats.U12_NightTime > 0 {
		nightActions := BuildNightActions()
		// Filter weights to only night-safe actions.
		filtered := make(map[string]ActionWeight, len(nightActions))
		for name := range nightActions {
			if w, ok := m.weights[name]; ok {
				filtered[name] = w
			}
		}
		return m.pickBest(social, care, curious, quiet, explore, feats, suggestions, filtered)
	}

	return m.pickBest(social, care, curious, quiet, explore, feats, suggestions, m.weights)
}

// pickBest selects the highest-scoring action from the given weight set.
func (m *Motivator) pickBest(social, care, curious, quiet, explore float64, feats *domain.QuantifiedFeatures, suggestions []domain.CareSuggestion, weights map[string]ActionWeight) (action string, score float64, secondScore float64) {
	// Build a lookup: actionName -> suggestion bonus.
	suggestionBonus := make(map[string]float64, len(suggestions))
	for _, s := range suggestions {
		name := s.ActionName()
		if name != "" {
			// Priority 1 -> +0.25, 2 -> +0.20, 3 -> +0.15, 4 -> +0.10
			bonus := 0.30 - float64(s.Priority)*0.05
			if bonus < 0.05 {
				bonus = 0.05
			}
			suggestionBonus[name] = bonus
		}
	}

	best := "none"
	bestScore := math.Inf(-1)
	secondBest := math.Inf(-1)

	for name, w := range m.weights {
		baseScore := social*w.Social + care*w.Care + curious*w.Curious + quiet*w.Quiet + explore*w.Explore
		// Apply care suggestion bonus (only for care_* actions with active triggers).
		if bonus, ok := suggestionBonus[name]; ok {
			baseScore += bonus
		}
		mod := contextModulator(name, feats)
		finalScore := baseScore * mod
		if finalScore > bestScore {
			secondBest = bestScore
			bestScore = finalScore
			best = name
		} else if finalScore > secondBest {
			secondBest = finalScore
		}
	}

	return best, storage.Clamp01(bestScore), storage.Clamp01(secondBest)
}

// contextModulator computes a per-action multiplier based on historical outcomes
// and current context. Returns 1.0 (no modulation) when feats is nil.
func contextModulator(action string, feats *domain.QuantifiedFeatures) float64 {
	if feats == nil {
		return 1.0
	}
	m := 1.0

	// A7: Historical success rate for this action type.
	if def := ActionByName(action); def != nil && def.OutcomeType != "" {
		if rate, ok := feats.A7_ActionSuccessRate[def.OutcomeType]; ok && rate >= 0 {
			m *= 0.4 + rate*0.6 // rate=0→×0.4, rate=1→×1.0
		}
	}
	// R3: Acceptance rate for this action's source.
	if def := ActionByName(action); def != nil && def.Source != "" {
		if rate, ok := feats.R3_SourceAcceptRate[def.Source]; ok && rate >= 0 {
			m *= 0.5 + rate*0.5
		}
	}

	// U10 / R2: Time-window preference — only for social speak actions.
	// Care actions are need-driven, not socially timed; they bypass this check.
	if action == "speak_casual" || action == "speak_inquiry" || action == "speak_care" {
		m *= 0.4 + feats.U10_TimeWindowPref*0.6
	}

	// U8: User engagement (response speed) — boosts social actions only.
	if action == "speak_casual" || action == "speak_care" {
		if feats.U8_EngagementNorm > 0 {
			m *= 0.6 + feats.U8_EngagementNorm*0.4
		}
	}

	// R6: Conversation depth — deepening conversations allow deeper topics.
	if action == "speak_inquiry" && feats.R6_DepthTrend > 0.2 {
		m *= 1.0 + feats.R6_DepthTrend*0.3 // up to ×1.3 for deepening convos
	}
	// Active inquiries/curiosity -> boost speak_inquiry (has something to talk about).
	if action == "speak_inquiry" && feats.A11_ActiveInquiries > 0 {
		m *= 1.0 + saturateNorm(float64(feats.A11_ActiveInquiries), 3)*0.3
	}

	// U7: Message trend negative -> user disengaging -> reduce social speak.
	// Care actions are need-driven, exempt from social disengagement penalty.
	if (action == "speak_casual" || action == "speak_inquiry") && feats.U7_LengthTrend < -0.3 {
		m *= 1.0 + feats.U7_LengthTrend*0.4 // trend=-1->×0.6
	}

	// Clamp to reasonable range.
	if m < 0.1 {
		m = 0.1
	}
	if m > 1.5 {
		m = 1.5
	}
	return m
}

// GetWeights returns a copy of the current weight matrix.
func (m *Motivator) GetWeights() map[string]ActionWeight {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]ActionWeight, len(m.weights))
	for k, v := range m.weights {
		result[k] = v
	}
	return result
}

func (m *Motivator) UpdateWeight(action string, field string, delta float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, ok := m.weights[action]
	if !ok {
		return
	}
	switch field {
	case "social":
		w.Social = clampWeight(w.Social + delta)
	case "care":
		w.Care = clampWeight(w.Care + delta)
	case "curious":
		w.Curious = clampWeight(w.Curious + delta)
	case "quiet":
		w.Quiet = clampWeight(w.Quiet + delta)
	case "explore":
		w.Explore = clampWeight(w.Explore + delta)
	}
	m.weights[action] = w
}

func (m *Motivator) UpdateWeightsFromOutcome(action string, reward float64, social, care, curious, quiet, explore float64) {
	if action == "none" {
		return
	}
	step := 0.003 * reward
	m.UpdateWeight(action, "social", step*social)
	m.UpdateWeight(action, "care", step*care)
	m.UpdateWeight(action, "curious", step*curious)
	m.UpdateWeight(action, "quiet", step*quiet)
	m.UpdateWeight(action, "explore", step*explore)
}

// ---- LLM Routing ----

// RouteToLLM checks conditions to decide if this scenario needs LLM creativity.
// feats and needs may be nil — conditions that require them are skipped.
// topAction is the action chosen by ScoreActions (used for conflict detection).
func RouteToLLM(ctx domain.DecisionContext, lastAction string, consecutiveCount int, feats *domain.QuantifiedFeatures, needs *domain.IntrinsicNeeds, topAction string) bool {
	// ① Extreme sleepiness — cat is barely awake, LLM handles gracefully.
	if ctx.EmotionVec.Sleepiness > 0.85 {
		return true
	}

	// ② Extreme user emotion: anger or fear with high intensity.
	if (ctx.EmotionState.Primary == "anger" || ctx.EmotionState.Primary == "fear") &&
		ctx.EmotionState.Intensity > 0.8 {
		return true
	}

	// ③ Repetition loop: same action 3+ times consecutively (exclude "none").
	if consecutiveCount >= 3 && lastAction != "none" {
		return true
	}

	// ④ Rejection cascade: 3+ rejections in recent 5 outcomes.
	if feats != nil && feats.R4_RecentRejections >= 3 {
		return true
	}

	// ⑤ Acceptance collapse: was healthy (intimacy trending down) AND now critically low.
	// R8_IntimacyTrend < -0.15 means "getting worse over time" — was better before.
	// R1 < 0.3 means current trajectory is unsustainable. Together = genuine collapse.
	if feats != nil && feats.R1_OverallAcceptRate < 0.3 && feats.R1_SampleCount >= 10 &&
		feats.R8_IntimacyTrend < -0.15 {
		return true
	}

	// ⑥ Need-action conflict: high internal need but motivator chose none/quiet.
	// LLM should resolve the tension between what the cat feels and what it chose.
	if needs != nil && feats != nil && topAction == "none" {
		available := feats.R4_RejectionSeverity < 0.3 && feats.U3_IsWorking < 0.5
		if needs.Companionship > 0.8 && available {
			return true // wants company, not working, not rejected — should not be silent
		}
		if needs.Care > 0.85 && available {
			return true // worried about user, available — should express concern
		}
	}

	// ⑦ Emotion valence crash: user's emotional trajectory suddenly negative.
	// A4_ValenceTrend < -0.5 means valence dropped significantly in the last hour.
	if feats != nil && feats.A4_ValenceTrend < -0.5 {
		return true
	}

	// ⑧ Long silence re-engagement: >4h idle, first re-engagement tick.
	// LLM crafts a contextual entrance. Shorter gaps use scorer (fast path).
	if feats != nil && feats.U14_TimeSinceChatMins > 240 && consecutiveCount <= 1 {
		return true
	}

	return false
}

// RouteToLLMWithScores checks decision conflict: top 2 scores very close AND the winner
// involves speaking (where creativity matters). Background actions don't need LLM tiebreak.
func RouteToLLMWithScores(topScore, secondScore float64, topAction string) bool {
	if !isSpeakAction(topAction) {
		return false // observe/reflect/none conflicts don't need LLM
	}
	return math.Abs(topScore-secondScore) < 0.03
}


func mealBonus(hour int) float64 {
	if (hour >= 11 && hour <= 13) || (hour >= 17 && hour <= 20) {
		return 0.5
	}
	return 0.0
}

func idleBias(dailyCount int) int {
	if dailyCount > 10 {
		return 10
	}
	return dailyCount
}

func clampWeight(v float64) float64 {
	if v < -1.0 {
		return -1.0
	}
	if v > 1.0 {
		return 1.0
	}
	return v
}
