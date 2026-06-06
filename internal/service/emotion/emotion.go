package emotion

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	"desktop-pet/internal/domain"; "desktop-pet/internal/infra"; infrastorage "desktop-pet/internal/infra/storage"
)

var _ domain.EmotionEvaluator = (*EmotionModel)(nil)

type (
	EmotionState  = domain.EmotionState
	EmotionVector = domain.EmotionVector
)

// Neutral points — decay drifts toward these (not floors, just attractors).
// Positive traits (affection, confidence) neutral at 0.5;
// negative/reactive traits (worry, annoyance) neutral at 0.
const (
	neutralAffection   = 0.5
	neutralConfidence  = 0.5
	neutralCuriosity   = 0.5
	neutralPlayfulness = 0.5
	neutralWorry       = 0.0
	neutralAnnoyance   = 0.0
	neutralLoneliness  = 0.0
)

// Decay rates per hour — higher = faster drift to neutral.
const (
	decayAffection   = 0.005
	decayConfidence  = 0.008
	decayCuriosity   = 0.05
	decayPlayfulness = 0.05
	decayWorry       = 0.08
	decayAnnoyance   = 0.12
	decayLoneliness  = 0.03
)

// Smoothing: alpha = weight of NEW evaluation, 1-alpha = weight of old.
const (
	smoothAffection   = 0.10
	smoothConfidence  = 0.15
	smoothCuriosity   = 0.30
	smoothPlayfulness = 0.30
	smoothWorry       = 0.60
	smoothAnnoyance   = 0.70
)

// History depth.
const maxHistory = 100

// PersonalityScale modulates how the emotion model reacts to user input.
// It learns from action_outcomes to personalize the cat's emotional personality.
type PersonalityScale struct {
	AnnoyanceSensitivity float64 // 0~1: how easily annoyed. Low = thick-skinned.
	AffectionWarmth      float64 // 0~1: how quickly affection grows.
	WorryTendency        float64 // 0~1: how much the cat worries about the user.
}

// DefaultPersonality returns a balanced starting personality.
func DefaultPersonality() PersonalityScale {
	return PersonalityScale{
		AnnoyanceSensitivity: 0.5,
		AffectionWarmth:      0.5,
		WorryTendency:        0.5,
	}
}

// EmotionModel manages the AI's emotional state with human-like dynamics.
type EmotionModel struct {
	mu            sync.Mutex
	current       EmotionState
	vector        EmotionVector
	history       []EmotionState
	vectorHistory []EmotionVector
	lastUpdate    time.Time
	lastInteract  time.Time // last chat/poke interaction
	lastActivity  time.Time // last ANY user activity (mouse, keyboard, chat)
	llmEval       func(prompt string) (string, error)
	ruleEval      *RuleBasedEmotionEvaluator
	logFn         func(msg string, args ...any)
	decayTicker   *time.Ticker
	stopDecay     chan struct{}
	store         EmotionStore
	personality   PersonalityScale
	outcomeRepo   domain.ActionOutcomeRepository
	// Activity tracking for sleep/idle detection.
	activityHours  [24]int                   // rolling interaction count per hour (for sleep inference)
	needModulation *domain.NeedModulation     // intrinsic need modulation (set by NeedModel)
}

type EmotionStore interface {
	SaveEmotion(domain.EmotionState, domain.EmotionVector, time.Time) error
	LoadEmotion() (domain.EmotionState, domain.EmotionVector, time.Time, bool)
}

func (e *EmotionModel) log(msg string, args ...any) {
	if e.logFn != nil {
		e.logFn(msg, args...)
	}
}

// NewEmotionModel creates an EmotionModel with human-like defaults.
func NewEmotionModel(llmEval func(string) (string, error)) *EmotionModel {
	e := &EmotionModel{
		current: EmotionState{
			Valence:   0.25,
			Arousal:   0.1,
			Dominance: 0.15,
			Primary:   "neutral",
			Intensity: 0.1,
		},
		vector: EmotionVector{
			Affection:   0.45, // slightly below neutral — needs to warm up
			Worry:       0.1,
			Curiosity:   0.4,
			Sleepiness:  sleepinessForHour(time.Now().Hour()),
			Playfulness: 0.3,
			Loneliness:  0.15,
			Confidence:  0.45,
			Annoyance:   0.02,
		},
		lastUpdate:   time.Now(),
		lastInteract: time.Now(),
		lastActivity: time.Now(),
		llmEval:      llmEval,
		ruleEval:     NewRuleBasedEmotionEvaluator(),
		personality:  DefaultPersonality(),
		stopDecay:    make(chan struct{}),
	}
	e.decayTicker = time.NewTicker(5 * time.Minute)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("emotion: decay goroutine panicked", "panic", r)
			}
		}()
		for {
			select {
			case <-e.decayTicker.C:
				e.decay()
			case <-e.stopDecay:
				return
			}
		}
	}()
	return e
}

func (e *EmotionModel) StopDecay() {
	close(e.stopDecay)
	if e.decayTicker != nil {
		e.decayTicker.Stop()
	}
}

func (e *EmotionModel) SetStore(s EmotionStore) { e.store = s }

// NotifyActivity records user presence (mouse, keyboard, or chat).
// Call this periodically from the UI layer.
func (e *EmotionModel) NotifyActivity() {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	e.lastActivity = now

	// Track interaction per hour for sleep schedule inference.
	hour := now.Hour()
	if e.activityHours[hour] < 10000 {
		e.activityHours[hour]++
	}

	// Reunion effect: user returned after a long idle.
	idleMinutes := now.Sub(e.lastInteract).Minutes()
	if idleMinutes > 60 {
		e.vector.Affection = infrastorage.Clamp01(e.vector.Affection + 0.03)
		e.vector.Playfulness = infrastorage.Clamp01(e.vector.Playfulness + 0.08)
		e.vector.Loneliness = infrastorage.Clamp01(e.vector.Loneliness - 0.3)
		e.vector.Worry = infrastorage.Clamp01(e.vector.Worry - 0.1)
		e.computeState()
	}
}

// isProbablyIdle returns true if the user has been inactive for >30 min.
func (e *EmotionModel) isProbablyIdle() bool {
	return time.Since(e.lastActivity) > 30*time.Minute
}

// isProbablyAsleep returns true if the user is likely sleeping:
// idle > 2 hours AND current hour is in a historically quiet window.
func (e *EmotionModel) isProbablyAsleep() bool {
	if time.Since(e.lastActivity) < 2*time.Hour {
		return false
	}
	hour := time.Now().Hour()
	return e.isQuietHour(hour)
}

// isQuietHour checks whether the given hour historically has low activity.
// Falls back to a default 0-7 quiet window before enough data is collected.
func (e *EmotionModel) isQuietHour(hour int) bool {
	total := 0
	for _, c := range e.activityHours {
		total += c
	}
	if total < 50 { // not enough data yet — use default sleep window.
		return hour >= 0 && hour < 7
	}
	// If this hour's activity is < 30% of the busiest hour, treat as quiet.
	maxCount := 0
	for _, c := range e.activityHours {
		if c > maxCount {
			maxCount = c
		}
	}
	if maxCount == 0 {
		return hour >= 0 && hour < 7
	}
	return float64(e.activityHours[hour]) < float64(maxCount)*0.3
}
func (e *EmotionModel) SetLLMEval(fn func(string) (string, error)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.llmEval = fn
}
func (e *EmotionModel) SetLogger(fn func(msg string, args ...any)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.logFn = fn
}
func (e *EmotionModel) SetPersonality(p PersonalityScale) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.personality = p
}

// SetNeedModulation injects intrinsic need modulation factors.
// Called each tick by NeedModel to modulate emotion decay/growth rates.
func (e *EmotionModel) SetNeedModulation(mod *domain.NeedModulation) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.needModulation = mod
}

// Personality returns the current learned personality scale.
func (e *EmotionModel) Personality() PersonalityScale {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.personality
}

func (e *EmotionModel) SetOutcomeRepo(repo domain.ActionOutcomeRepository) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.outcomeRepo = repo
}

// LearnPersonality adjusts the personality scale based on historical outcomes.
// Called periodically (e.g., daily) by the strategic agent.
func (e *EmotionModel) LearnPersonality() {
	if e.outcomeRepo == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	ctx := domain.ActionContext{}
	accepts, total := e.outcomeRepo.SuccessRate(ctx, 30)
	if total < 20 {
		return
	}
	rate := float64(accepts) / float64(total)

	// If user often rejects → they may be easily annoyed → lower sensitivity
	// (thick-skinned cat doesn't get annoyed as easily).
	if rate < 0.3 {
		e.personality.AnnoyanceSensitivity = infrastorage.Clamp01(e.personality.AnnoyanceSensitivity - 0.05)
		e.personality.AffectionWarmth = infrastorage.Clamp01(e.personality.AffectionWarmth - 0.03)
	} else if rate > 0.6 {
		// User responds warmly → cat warms up faster.
		e.personality.AffectionWarmth = infrastorage.Clamp01(e.personality.AffectionWarmth + 0.05)
	}

	// Worry tendency: if most interactions are care-related → user needs more care.
	careCtx := domain.ActionContext{Source: domain.SourceCare}
	careAccepts, careTotal := e.outcomeRepo.SuccessRate(careCtx, 30)
	if careTotal > 10 {
		careRate := float64(careAccepts) / float64(careTotal)
		if careRate > 0.5 {
			e.personality.WorryTendency = infrastorage.Clamp01(e.personality.WorryTendency + 0.03)
		}
	}
}

func (e *EmotionModel) UseMockMode() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ruleEval = nil
}

// ---- Decay: drift toward neutral (no floor) ----

func (e *EmotionModel) decay() {
	e.mu.Lock()
	defer e.mu.Unlock()

	elapsed := time.Since(e.lastUpdate).Hours()
	if elapsed <= 0 {
		return
	}

	asleep := e.isProbablyAsleep()
	idle := e.isProbablyIdle()
	hour := time.Now().Hour()

	// During sleep, freeze most emotions — the cat is "asleep" too.
	decayMult := 1.0
	if asleep {
		decayMult = 0.15 // very slow decay during sleep
	}

	// Time-driven emotions.
	timeSleepiness := sleepinessForHour(hour)

	// Loneliness: only grows when user is ACTIVE but ignoring the cat.
	// If user is idle/asleep, loneliness stays flat (cat understands).
	var timeLoneliness float64
	if idle || asleep {
		// User is away — loneliness doesn't increase (cat rests too).
		timeLoneliness = e.vector.Loneliness // hold current
	} else {
		// User is around but not interacting — loneliness grows.
		idleSinceInteract := time.Since(e.lastInteract).Hours()
		timeLoneliness = lonelinessFromIdle(idleSinceInteract)
	}

	// Apply intrinsic need modulation to decay rates.
	confDecayMul := 1.0
	curDecayMul := 1.0
	playDecayMul := 1.0
	worryDecayMul := 1.0
	loneDecayMul := 1.0
	sleepGrowMul := 1.0
	if e.needModulation != nil {
		confDecayMul = e.needModulation.ConfidenceDecayMul
		curDecayMul = e.needModulation.CuriosityDecayMul
		playDecayMul = e.needModulation.PlayfulnessDecayMul
		worryDecayMul = e.needModulation.WorryDecayMul
		loneDecayMul = e.needModulation.LonelinessDecayMul
		sleepGrowMul = e.needModulation.SleepinessGrowthMul
	}

	e.vector.Affection = decayToward(e.vector.Affection, neutralAffection, decayAffection*decayMult, elapsed)
	e.vector.Confidence = decayToward(e.vector.Confidence, neutralConfidence, decayConfidence*decayMult*confDecayMul, elapsed)
	e.vector.Curiosity = decayToward(e.vector.Curiosity, neutralCuriosity, decayCuriosity*decayMult*curDecayMul, elapsed)
	e.vector.Playfulness = decayToward(e.vector.Playfulness, neutralPlayfulness, decayPlayfulness*decayMult*playDecayMul, elapsed)
	e.vector.Worry = decayToward(e.vector.Worry, neutralWorry, decayWorry*decayMult*worryDecayMul, elapsed)
	e.vector.Annoyance = decayToward(e.vector.Annoyance, neutralAnnoyance, decayAnnoyance*decayMult, elapsed)

	// Sleepiness: blend circadian target with current inertia, modulated by rest need.
	targetSleepiness := (timeSleepiness*0.6 + e.vector.Sleepiness*0.4) * sleepGrowMul
	targetSleepiness = math.Min(targetSleepiness, 1.0)
	e.vector.Sleepiness = decayToward(e.vector.Sleepiness, targetSleepiness, decayPlayfulness*decayMult, elapsed)

	// Loneliness: driven by idle-vs-ignore detection, modulated by companionship need.
	e.vector.Loneliness = decayToward(e.vector.Loneliness, timeLoneliness, decayLoneliness*decayMult*loneDecayMul, elapsed)

	e.computeState()
	e.lastUpdate = time.Now()
}

func decayToward(current, neutral, ratePerHr, elapsedHours float64) float64 {
	delta := (neutral - current) * math.Min(ratePerHr*elapsedHours, 1.0)
	return current + delta
}

func sleepinessForHour(hour int) float64 {
	switch {
	case hour >= 23 || hour < 6:
		return 0.75
	case hour >= 6 && hour < 8:
		return 0.50
	case hour >= 8 && hour < 12:
		return 0.20
	case hour >= 12 && hour < 14:
		return 0.15
	case hour >= 14 && hour < 16:
		return 0.35
	case hour >= 16 && hour < 22:
		return 0.20
	default: // 22-23
		return 0.45
	}
}

func lonelinessFromIdle(idleHours float64) float64 {
	return infrastorage.Clamp01(0.1 + idleHours*0.04)
}

// ---- Evaluation ----

// maxCacheSize is the maximum number of cached emotion evaluations.
const maxCacheSize = 64

// cachedEval stores a recent LLM emotion evaluation for reuse.
type cachedEval struct {
	sig   uint64 // simhash signature of the input
	state EmotionState
	vec   EmotionVector
	at    time.Time
}

var (
	emotionCache   []cachedEval
	emotionCacheMu sync.Mutex
)

func (e *EmotionModel) Evaluate(recentTurns string) error {
	e.mu.Lock()
	e.lastInteract = time.Now()
	e.vector.Sleepiness = infrastorage.Clamp01(e.vector.Sleepiness - 0.15)
	e.mu.Unlock()

	// Tier 1: Check cache for similar recent turns (fast regex-like reuse).
	sig := simhash(recentTurns)
	emotionCacheMu.Lock()
	cached := lookupCache(sig)
	emotionCacheMu.Unlock()
	if cached != nil && time.Since(cached.at) < 30*time.Second {
		e.applySmoothing(cached.state, cached.vec)
		return nil
	}

	// Tier 2: LLM evaluation (primary path — Kardia-R1 / Echo-N1 inspired).
	if e.llmEval != nil {
		prompt := BuildEmotionPrompt(recentTurns)
		result, err := e.llmEval(prompt)
		if err == nil {
			parsed, vec, err := parseEmotionJSON(result)
			if err == nil {
				storeCache(sig, parsed, vec)
				e.applySmoothing(parsed, vec)
				return nil
			}
		}
	}

	// Tier 3: Rule engine (fallback when LLM unavailable or fails).
	if e.ruleEval != nil {
		state, vec, matched := e.ruleEval.Evaluate(recentTurns)
		if matched {
			e.applySmoothing(state, vec)
			return nil
		}
	}

	return nil
}

// simhash computes a simple rolling-hash signature for cache lookup.
func simhash(text string) uint64 {
	var h uint64
	for i := 0; i < len(text); i++ {
		h = h*31 + uint64(text[i])
	}
	return h
}

func lookupCache(sig uint64) *cachedEval {
	for i := range emotionCache {
		if emotionCache[i].sig == sig {
			return &emotionCache[i]
		}
	}
	return nil
}

func storeCache(sig uint64, state EmotionState, vec EmotionVector) {
	emotionCacheMu.Lock()
	defer emotionCacheMu.Unlock()
	entry := cachedEval{sig: sig, state: state, vec: vec, at: time.Now()}
	// Evict oldest if full.
	if len(emotionCache) >= maxCacheSize {
		oldest := 0
		for i := range emotionCache {
			if emotionCache[i].at.Before(emotionCache[oldest].at) {
				oldest = i
			}
		}
		emotionCache[oldest] = entry
		return
	}
	emotionCache = append(emotionCache, entry)
}

// computeState derives PAD (Valence/Arousal/Dominance), Primary emotion,
// and Intensity from the current 8-dimension vector.
func (e *EmotionModel) computeState() {
	v := e.vector

	// Valence: affection pulls up, annoyance pulls down.
	e.current.Valence = clamp1(v.Affection - v.Annoyance)

	// Arousal: playfulness + curiosity energise, sleepiness depresses.
	e.current.Arousal = clamp1((v.Playfulness+v.Curiosity)/2 - v.Sleepiness)

	// Dominance: confidence gives control, worry + low affection reduce it.
	e.current.Dominance = clamp1(v.Confidence - 0.5 - v.Worry*0.3)

	// Intensity: max deviation from neutral across all vector dimensions.
	intensity := 0.0
	for _, d := range []float64{
		math.Abs(v.Affection-0.5) / 0.5,
		math.Abs(v.Worry) / 0.7,
		math.Abs(v.Curiosity-0.5) / 0.5,
		math.Abs(v.Playfulness-0.5) / 0.5,
		math.Abs(v.Loneliness) / 0.7,
		math.Abs(v.Confidence-0.5) / 0.5,
		math.Abs(v.Annoyance) / 0.7,
	} {
		if d > intensity {
			intensity = d
		}
	}
	e.current.Intensity = infrastorage.Clamp01(intensity)

	// Primary emotion label.
	e.current.Primary = inferPrimaryFromVector(v)
}

func inferPrimaryFromVector(v EmotionVector) string {
	switch {
	case v.Annoyance > 0.5:
		return "anger"
	case v.Worry > 0.5:
		return "fear"
	case v.Sleepiness > 0.7:
		return "neutral"
	case v.Playfulness > 0.6:
		return "joy"
	case v.Affection > 0.7:
		return "joy"
	case v.Loneliness > 0.6:
		return "sadness"
	case v.Curiosity > 0.6:
		return "surprise"
	default:
		return "neutral"
	}
}

// applySmoothing blends a new evaluation into the current vector using per-dimension
// smoothing coefficients. PAD is derived from the vector afterward.
func (e *EmotionModel) applySmoothing(state EmotionState, vec EmotionVector) {
	e.mu.Lock()
	defer e.mu.Unlock()

	p := e.personality

	// Affection blending modulated by warmth: warmer cat adopts affection faster.
	affectionAlpha := smoothAffection * (0.5 + p.AffectionWarmth)
	e.vector.Affection = infrastorage.Clamp01(blend(e.vector.Affection, vec.Affection, affectionAlpha))

	e.vector.Confidence = infrastorage.Clamp01(blend(e.vector.Confidence, vec.Confidence, smoothConfidence))
	e.vector.Curiosity = infrastorage.Clamp01(blend(e.vector.Curiosity, vec.Curiosity, smoothCuriosity))
	e.vector.Playfulness = infrastorage.Clamp01(blend(e.vector.Playfulness, vec.Playfulness, smoothPlayfulness))

	// Worry blending modulated by tendency.
	worryAlpha := smoothWorry * (0.5 + p.WorryTendency)
	e.vector.Worry = infrastorage.Clamp01(blend(e.vector.Worry, vec.Worry, worryAlpha))

	// Annoyance modulated by sensitivity: low sensitivity = annoyance changes less.
	annoyAlpha := smoothAnnoyance * p.AnnoyanceSensitivity
	e.vector.Annoyance = infrastorage.Clamp01(blend(e.vector.Annoyance, vec.Annoyance, annoyAlpha))

	e.computeState()

	// Respect the LLM's explicit primary emotion judgment.
	// The LLM does structured reasoning — trust its output over the vector-derived label.
	if state.Primary != "" && state.Primary != "neutral" {
		e.current.Primary = state.Primary
	}

	e.history = append(e.history, e.current)
	if len(e.history) > maxHistory {
		e.history = e.history[len(e.history)-maxHistory:]
	}
	e.vectorHistory = append(e.vectorHistory, e.vector)
	if len(e.vectorHistory) > maxHistory {
		e.vectorHistory = e.vectorHistory[len(e.vectorHistory)-maxHistory:]
	}
	e.lastUpdate = time.Now()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("emotion: Save goroutine panicked", "panic", r)
			}
		}()
		if err := e.Save(); err != nil {
			slog.Warn("emotion: failed to save", "err", err)
		}
	}()
}

func blend(oldVal, newVal, alpha float64) float64 {
	return oldVal*(1-alpha) + newVal*alpha
}

// ---- Accessors ----

func (e *EmotionModel) Current() EmotionState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.current
}

func (e *EmotionModel) CurrentVector() EmotionVector {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.vector
}

func (e *EmotionModel) History() []EmotionState {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]EmotionState, len(e.history))
	copy(out, e.history)
	return out
}

func (e *EmotionModel) VectorHistory() []EmotionVector {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]EmotionVector, len(e.vectorHistory))
	copy(out, e.vectorHistory)
	return out
}

// ---- SQLite Persistence ----

func (e *EmotionModel) Save() error {
	if e.store == nil {
		return nil
	}
	e.mu.Lock()
	es := e.current
	ev := e.vector
	li := e.lastInteract
	e.mu.Unlock()
	// Round floats to 3 decimal places to avoid noise accumulation.
	es.Valence = math.Round(es.Valence*1000) / 1000
	es.Arousal = math.Round(es.Arousal*1000) / 1000
	es.Dominance = math.Round(es.Dominance*1000) / 1000
	es.Intensity = math.Round(es.Intensity*1000) / 1000
	ev.Affection = math.Round(ev.Affection*1000) / 1000
	ev.Worry = math.Round(ev.Worry*1000) / 1000
	ev.Curiosity = math.Round(ev.Curiosity*1000) / 1000
	ev.Sleepiness = math.Round(ev.Sleepiness*1000) / 1000
	ev.Playfulness = math.Round(ev.Playfulness*1000) / 1000
	ev.Loneliness = math.Round(ev.Loneliness*1000) / 1000
	ev.Confidence = math.Round(ev.Confidence*1000) / 1000
	ev.Annoyance = math.Round(ev.Annoyance*1000) / 1000
	return e.store.SaveEmotion(es, ev, li)
}

func (e *EmotionModel) Load() error {
	if e.store == nil {
		return nil
	}
	es, ev, li, ok := e.store.LoadEmotion()
	if !ok {
		return nil
	}
	e.mu.Lock()
	e.current = es
	e.vector = ev
	e.lastInteract = li
	e.lastUpdate = time.Now()
	e.mu.Unlock()

	e.decay()
	return nil
}

// ---- Rule-Based Evaluator ----

type emotionRule struct {
	re     *regexp.Regexp
	state  EmotionState
	vector EmotionVector
}

type RuleBasedEmotionEvaluator struct {
	rules []emotionRule
}

func NewRuleBasedEmotionEvaluator() *RuleBasedEmotionEvaluator {
	return &RuleBasedEmotionEvaluator{rules: defaultEmotionRules()}
}

func (r *RuleBasedEmotionEvaluator) Evaluate(recentTurns string) (EmotionState, EmotionVector, bool) {
	lower := strings.ToLower(recentTurns)
	state := EmotionState{}
	vec := EmotionVector{}
	matched := false

	for _, rule := range r.rules {
		if rule.re.MatchString(lower) {
			state.Valence += rule.state.Valence
			state.Arousal += rule.state.Arousal
			state.Dominance += rule.state.Dominance
			state.Intensity += rule.state.Intensity
			vec.Affection += rule.vector.Affection
			vec.Worry += rule.vector.Worry
			vec.Curiosity += rule.vector.Curiosity
			vec.Sleepiness += rule.vector.Sleepiness
			vec.Playfulness += rule.vector.Playfulness
			vec.Loneliness += rule.vector.Loneliness
			vec.Confidence += rule.vector.Confidence
			vec.Annoyance += rule.vector.Annoyance
			matched = true
		}
	}

	if !matched {
		return EmotionState{}, EmotionVector{}, false
	}

	state.Primary = inferPrimary(state.Valence, state.Arousal)
	state.Valence = clamp1(state.Valence)
	state.Arousal = clamp1(state.Arousal)
	state.Dominance = clamp1(state.Dominance)
	state.Intensity = infrastorage.Clamp01(state.Intensity)
	vec.Affection = infrastorage.Clamp01(vec.Affection)
	vec.Worry = infrastorage.Clamp01(vec.Worry)
	vec.Curiosity = infrastorage.Clamp01(vec.Curiosity)
	vec.Sleepiness = infrastorage.Clamp01(vec.Sleepiness)
	vec.Playfulness = infrastorage.Clamp01(vec.Playfulness)
	vec.Loneliness = infrastorage.Clamp01(vec.Loneliness)
	vec.Confidence = infrastorage.Clamp01(vec.Confidence)
	vec.Annoyance = infrastorage.Clamp01(vec.Annoyance)

	return state, vec, true
}

func inferPrimary(valence, arousal float64) string {
	switch {
	case valence > 0.3 && arousal > 0.2:
		return "joy"
	case valence < -0.3 && arousal > 0.2:
		return "anger"
	case valence < -0.3 && arousal < 0:
		return "sadness"
	case valence > 0.3 && arousal < 0:
		return "neutral"
	case arousal > 0.5:
		return "surprise"
	case valence < -0.5:
		return "fear"
	default:
		return "neutral"
	}
}

func defaultEmotionRules() []emotionRule {
	return []emotionRule{
		// Affection + joy — scaled for the new smoothing (0.10-0.15 alpha)
		{re: regexp.MustCompile(`(诗音|喵).*(可爱|乖|棒|厉害|好|聪明)`), state: EmotionState{}, vector: EmotionVector{Affection: 0.5, Confidence: 0.3, Playfulness: 0.3}},
		{re: regexp.MustCompile(`(哈哈|笑死|有趣|好玩|开心|高兴|太好了|nice)`), state: EmotionState{}, vector: EmotionVector{Playfulness: 0.4, Affection: 0.2}},
		{re: regexp.MustCompile(`(夸|表扬|赞|棒|优秀|不错)`), state: EmotionState{}, vector: EmotionVector{Confidence: 0.4, Affection: 0.2}},
		{re: regexp.MustCompile(`(喜欢|爱).*(你|诗音|猫娘)`), state: EmotionState{}, vector: EmotionVector{Affection: 0.6, Confidence: 0.4, Playfulness: 0.4}},
		{re: regexp.MustCompile(`(摸摸|摸头|撸猫|顺毛|乖)`), state: EmotionState{}, vector: EmotionVector{Affection: 0.5, Sleepiness: 0.3}},
		{re: regexp.MustCompile(`(谢谢|多谢|thanks|thank)`), state: EmotionState{}, vector: EmotionVector{Affection: 0.3, Confidence: 0.2}},
		// Worry
		{re: regexp.MustCompile(`(熬夜|通宵|失眠|睡不着|好晚|凌晨)`), state: EmotionState{}, vector: EmotionVector{Worry: 0.7, Sleepiness: 0.2}},
		{re: regexp.MustCompile(`(累|疲惫|好困|没精神|虚脱)`), state: EmotionState{}, vector: EmotionVector{Worry: 0.5, Sleepiness: 0.2}},
		{re: regexp.MustCompile(`(压力|焦虑|紧张|担心|焦躁|崩溃)`), state: EmotionState{}, vector: EmotionVector{Worry: 0.6, Confidence: -0.1, Curiosity: -0.2}},
		{re: regexp.MustCompile(`(bug|报错|crash|error|挂了|崩了|不行|失败)`), state: EmotionState{}, vector: EmotionVector{Curiosity: 0.3, Worry: 0.2}},
		{re: regexp.MustCompile(`(没吃|不饿|忘吃|没吃饭|饿)`), state: EmotionState{}, vector: EmotionVector{Worry: 0.5}},
		// Annoyance — high delta for fast response (smoothAnnoyance=0.70 makes it stick)
		{re: regexp.MustCompile(`(别烦|别吵|走开|滚|闭嘴|别管|烦死了|你好烦)`), state: EmotionState{}, vector: EmotionVector{Annoyance: 0.9, Confidence: -0.4, Affection: -0.3}},
		{re: regexp.MustCompile(`(AI|工具|机器人|程序|代码写).*(你|诗音)`), state: EmotionState{}, vector: EmotionVector{Annoyance: 0.95, Confidence: -0.5, Affection: -0.1}},
		{re: regexp.MustCompile(`(不想|懒得|算了|随便|无所谓)`), state: EmotionState{}, vector: EmotionVector{Playfulness: -0.1}},
		{re: regexp.MustCompile(`(对不起|抱歉|道歉|我的错|sorry)`), state: EmotionState{}, vector: EmotionVector{Annoyance: -0.3, Affection: 0.2}},
		// Curiosity
		{re: regexp.MustCompile(`(代码|编程|写|开发|debug|算法|bug|refactor|编译|部署|docker|k8s)`), state: EmotionState{}, vector: EmotionVector{Curiosity: 0.5}},
		{re: regexp.MustCompile(`(新.*(技术|语言|框架|工具|库|方法|思路))|(学|教|教教|教我).*(你|诗音)`), state: EmotionState{}, vector: EmotionVector{Curiosity: 0.6, Confidence: 0.2}},
		// Loneliness — spikes when user returns
		{re: regexp.MustCompile(`(好久不见|终于|回来了|想.*你|想.*诗音|一直.*在)`), state: EmotionState{}, vector: EmotionVector{Loneliness: 0.0, Affection: 0.4}},
		// Sleepiness
		{re: regexp.MustCompile(`(睡了|晚安|bye|拜拜|明天见)`), state: EmotionState{}, vector: EmotionVector{Sleepiness: 0.5, Affection: 0.2}},
		// Confidence
		{re: regexp.MustCompile(`(厉害|强|大佬|膜拜|天才|nb|666|respect)`), state: EmotionState{}, vector: EmotionVector{Confidence: 0.5, Affection: 0.2}},
	}
}

// ---- JSON parsing ----

func BuildEmotionPrompt(recentTurns string) string {
	return fmt.Sprintf(emotionPromptTemplate, recentTurns)
}

type parsedEmotion struct {
	Valence       float64       `json:"valence"`
	Arousal       float64       `json:"arousal"`
	Dominance     float64       `json:"dominance"`
	Primary       string        `json:"primary"`
	Intensity     float64       `json:"intensity"`
	EmotionVector EmotionVector `json:"emotion_vector"`
}

func parseEmotionJSON(raw string) (EmotionState, EmotionVector, error) {
	raw = infra.CleanJSON(raw)
	var p parsedEmotion
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return EmotionState{}, EmotionVector{}, err
	}
	s := EmotionState{
		Valence:   clamp1(p.Valence),
		Arousal:   clamp1(p.Arousal),
		Dominance: clamp1(p.Dominance),
		Intensity: math.Min(math.Max(p.Intensity, 0), 1),
		Primary:   p.Primary,
	}
	if s.Primary == "" {
		s.Primary = "neutral"
	}
	v := EmotionVector{
		Affection:   infrastorage.Clamp01(p.EmotionVector.Affection),
		Worry:       infrastorage.Clamp01(p.EmotionVector.Worry),
		Curiosity:   infrastorage.Clamp01(p.EmotionVector.Curiosity),
		Sleepiness:  infrastorage.Clamp01(p.EmotionVector.Sleepiness),
		Playfulness: infrastorage.Clamp01(p.EmotionVector.Playfulness),
		Loneliness:  infrastorage.Clamp01(p.EmotionVector.Loneliness),
		Confidence:  infrastorage.Clamp01(p.EmotionVector.Confidence),
		Annoyance:   infrastorage.Clamp01(p.EmotionVector.Annoyance),
	}
	return s, v, nil
}

const emotionPromptTemplate = `## 情绪评估（Kardia-R1 风格结构化推理）

你是诗音的情绪感知模块。根据最近对话，分三步推理诗音当前的情绪状态。

### 最近对话
%s

### 推理步骤

**Step 1 — 理解主人**: 主人说了什么？情绪如何？是在工作、闲聊、求助、还是发泄？
**Step 2 — 自我感知**: 作为猫娘，主人这样的话会让我有什么感受？被夸奖会开心、被冷落会寂寞、被骂会委屈。
**Step 3 — 量化输出**: 基于以上两步，给出精确的数值。

### 输出格式
{
  "reasoning": "一句话的情绪推理（如：主人夸我可爱，我很开心但假装不在意）",
  "valence": -1到1(愉悦度),
  "arousal": -1到1(唤醒度),
  "dominance": -1到1(支配感),
  "primary": "joy/sadness/anger/fear/surprise/neutral",
  "intensity": 0到1,
  "emotion_vector": {
    "affection": 0到1,
    "worry": 0到1,
    "curiosity": 0到1,
    "sleepiness": 0到1,
    "playfulness": 0到1,
    "loneliness": 0到1,
    "confidence": 0到1,
    "annoyance": 0到1
  }
}

### 指引
- 中性情绪时 vector 各维度约 0.5
- 被夸奖 → affection↑ confidence↑ playfulness↑
- 被冷落/忽视 → loneliness↑
- 被说"别烦" → annoyance↑↑ confidence↓↓
- 主人压力大 → worry↑ playfulness↓
- 只输出 JSON，不要有其他文字。`


func clamp1(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}

