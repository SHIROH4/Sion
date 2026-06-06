package background

import (
	"log/slog"
	"sync"
	"time"

	"desktop-pet/internal/domain"
	"desktop-pet/internal/infra/native"
)

// ConsolidationResult is the outcome of a fact consolidation run.
type ConsolidationResult struct {
	ClustersFound int
	Merged        int
	Discarded     int
	Kept          int
	FactsArchived int
}

// BackgroundLoop manages a periodic cognitive loop that runs consolidation,
// forgetting, care, and proactive scheduling when the user has been idle long enough.
// The System 3 strategic agent replaces the legacy reflect/dream cycle.
type BackgroundLoop struct {
	// Dependencies.
	RawLLM  func([]domain.Message) (string, error)
	Emotion domain.EmotionEvaluator
	Care    domain.CareProvider
	Sched   domain.Scheduler

	// Proactive learner callbacks.
	ShouldLearn func() bool
	Learn       func(
		rawLLM func([]domain.Message) (string, error),
		emotion domain.EmotionState,
		saveMemCell func(t string, content string, importance float64, valence, arousal float64, sourceMsg string) error,
		saveFact func(content, source string) error,
	)

	// Memory layer callbacks.
	Consolidate func(rawLLM func([]domain.Message) (string, error))
	SaveFact    func(content, source string) error
	SaveMemCell func(t string, content string, importance float64, valence, arousal float64, sourceMsg string) error
	Forget      func()

	// Fact consolidator callbacks.
	ShouldConsolidate func() bool
	RunConsolidation  func() (ConsolidationResult, error)

	// Topic store callbacks.
	ListTopics     func() []domain.TopicEntry
	UpdateCentroid func(id int64)

	// External callbacks.
	OnProactive  func(result domain.SchedulerResult)
	OnCleanup    func()
	OnReflect    func()
	OnDetectGaps       func()
	OnSystem2Decision  func()

	// Strategic agent (System 3).
	StrategyAgent      func()
	TacticalDirectives []string

	// Screen observation.
	OnScreenObserve func(native.ScreenObservation)

	// Intervals (set before Start; zero means use defaults).
	TickInterval   time.Duration
	ScreenInterval time.Duration

	// IntervalFunc returns the current optimal decision interval. Called after each
	// tick to dynamically adjust pacing. When nil, TickInterval is used as a fixed value.
	IntervalFunc func() time.Duration

	// Mutable state (public for thin wrapper access).
	LastActive         time.Time
	LastScreenObs      native.ScreenObservation
	LastUserMsg        string
	LoopCycles         int
	EpisodesSinceTopic int

	// Internal lifecycle.
	mu      sync.RWMutex
	running bool
	stopCh  chan struct{}
	wakeCh  chan struct{} // event-driven trigger: non-blocking send wakes the loop

	lastForgetAt        time.Time
	lastScreenObserveAt time.Time
}

// NewBackgroundLoop creates a BackgroundLoop initialised to "now" so that
// background tasks don't fire immediately on the first tick.
func NewBackgroundLoop() *BackgroundLoop {
	now := time.Now()
	return &BackgroundLoop{
		stopCh:     make(chan struct{}),
		wakeCh:     make(chan struct{}, 1), // buffered so Wake() never blocks
		LastActive: time.Now().Add(-35 * time.Second),
		lastForgetAt: now,
	}
}

// Wake triggers an immediate tick if the loop is sleeping. Non-blocking — if a
// wake is already pending, this is a no-op. Use for event-driven decision triggers
// (user message, app switch, emotion spike).
func (l *BackgroundLoop) Wake() {
	select {
	case l.wakeCh <- struct{}{}:
	default:
	}
}

// Start launches the background loop in a new goroutine. Idempotent.
func (l *BackgroundLoop) Start() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.running {
		return
	}
	l.running = true
	l.stopCh = make(chan struct{})
	go l.loop()
}

// Stop signals the background loop to shut down. Idempotent.
func (l *BackgroundLoop) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.running {
		return
	}
	l.running = false
	close(l.stopCh)
}

// NotifyActivity records a user interaction, resetting the idle timer.
func (l *BackgroundLoop) NotifyActivity() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.LastActive = time.Now()
}

// SetScreenInterval overrides the default screen observation interval.
func (l *BackgroundLoop) SetScreenInterval(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ScreenInterval = d
}

// SetOnScreenObserve registers the callback for L1 screen observations.
func (l *BackgroundLoop) SetOnScreenObserve(fn func(native.ScreenObservation)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.OnScreenObserve = fn
}

// NotifyEpisodeCreated increments the episode counter for topic maintenance.
func (l *BackgroundLoop) NotifyEpisodeCreated() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.EpisodesSinceTopic++
}

// loop is the main cognitive cycle.
func (l *BackgroundLoop) loop() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("background: loop panicked", "panic", r)
		}
	}()

	slog.Info("background: loop started", "interval", l.TickInterval)

	// Snapshot callback fields under lock to prevent data races.
	l.mu.RLock()
	onSys2 := l.OnSystem2Decision
	l.mu.RUnlock()

	// Fire System 2 decision shortly after startup.
	go func() {
		time.Sleep(15 * time.Second)
		if onSys2 != nil {
			onSys2()
		}
	}()

	interval := l.TickInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	screenInterval := l.ScreenInterval
	if screenInterval <= 0 {
		screenInterval = 60 * time.Second
	}
	screenTicker := time.NewTicker(screenInterval)
	defer screenTicker.Stop()

	for {
		select {
		case <-l.stopCh:
			return
		case <-l.wakeCh:
			l.runTick()
			// After event-driven tick, recompute intervals.
			if l.IntervalFunc != nil {
				newInterval := l.IntervalFunc()
				if newInterval > 0 && newInterval != interval {
					ticker.Reset(newInterval)
					interval = newInterval
					if newInterval >= 60*time.Second {
						screenTicker.Reset(newInterval / 3)
					}
				}
			}
		case <-screenTicker.C:
			l.observeScreen()
		case <-ticker.C:
			l.runTick()
			// After each tick, recompute interval dynamically.
			if l.IntervalFunc != nil {
				newInterval := l.IntervalFunc()
				if newInterval > 0 && newInterval != interval {
					ticker.Reset(newInterval)
					interval = newInterval
					if newInterval >= 60*time.Second {
						screenTicker.Reset(newInterval / 3)
					}
				}
			}
		}
	}
}

// runTick executes one cognitive cycle: snapshot callbacks and run all periodic tasks.
func (l *BackgroundLoop) runTick() {
	l.mu.Lock()
	l.LoopCycles++
	cycle := l.LoopCycles
	l.mu.Unlock()

	// Snapshot callback fields under lock to prevent data races on concurrent Set* calls.
	l.mu.RLock()
	shouldConsolidate := l.ShouldConsolidate
	runConsolidation := l.RunConsolidation
	onDetectGaps := l.OnDetectGaps
	onReflect := l.OnReflect
	care := l.Care
	onSys2 := l.OnSystem2Decision
	strategyAgent := l.StrategyAgent
	consolidate := l.Consolidate
	rawLLM := l.RawLLM
	forget := l.Forget
	shouldLearn := l.ShouldLearn
	learn := l.Learn
	emotion := l.Emotion
	saveMemCell := l.SaveMemCell
	saveFact := l.SaveFact
	listTopics := l.ListTopics
	updateCentroid := l.UpdateCentroid
	onCleanup := l.OnCleanup
	l.mu.RUnlock()

	// Fact consolidation (daily).
	if shouldConsolidate != nil && shouldConsolidate() {
		result, err := runConsolidation()
		if err != nil {
			slog.Warn("memory: fact consolidation failed", "err", err)
		} else if result.ClustersFound > 0 {
			slog.Info("memory: fact consolidation done",
				"clusters", result.ClustersFound,
				"merged", result.Merged,
				"discarded", result.Discarded,
				"kept", result.Kept,
				"archived", result.FactsArchived,
			)
		}
	}

	// Detect knowledge gaps (curiosity engine).
	if cycle%10 == 0 && onDetectGaps != nil {
		onDetectGaps()
	}

	// ReflectAndForget (fact maintenance).
	if cycle%100 == 0 && onReflect != nil {
		onReflect()
	}

	// Tick isolation counter.
	if care != nil {
		care.TickIsolation(5 * time.Minute)
	}

	// System 2 LLM autonomous decision.
	if onSys2 != nil {
		onSys2()
	}

	// ---- idle only below ----
	l.mu.Lock()
	idle := time.Since(l.LastActive)
	l.mu.Unlock()

	if idle < 30*time.Minute {
		return
	}

	// Strategic agent (System 3 — daily reflection).
	if strategyAgent != nil {
		strategyAgent()
	}

	// Consolidation.
	if consolidate != nil && rawLLM != nil {
		consolidate(rawLLM)
	}

	// Forget (daily).
	if l.shouldForget() {
		if forget != nil {
			forget()
		}
	}

	// Proactive learning.
	if shouldLearn != nil && shouldLearn() && emotion != nil && learn != nil {
		learn(rawLLM, emotion.Current(), saveMemCell, saveFact)
	}

	// Topic centroid maintenance.
	l.mu.Lock()
	need := l.EpisodesSinceTopic >= 100
	if need {
		l.EpisodesSinceTopic = 0
	}
	l.mu.Unlock()
	if need && listTopics != nil && updateCentroid != nil {
		for _, t := range listTopics() {
			updateCentroid(t.ID)
		}
	}

	// Every 100 cycles: cleanup.
	if cycle%100 == 0 {
		if onCleanup != nil {
			onCleanup()
		}
	}
}

// ---- trigger guards ----

func (l *BackgroundLoop) shouldForget() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return time.Since(l.lastForgetAt) > 24*time.Hour
}

// ---- screen observation ----

func (l *BackgroundLoop) observeScreen() {
	l.mu.Lock()
	fn := l.OnScreenObserve
	l.mu.Unlock()

	if fn == nil {
		return
	}

	obs, err := native.OCRActiveScreen()
	if err != nil {
		slog.Debug("memory: screen observe partial", "err", err,
			"app", obs.AppName, "title", obs.WindowTitle)
	}
	if obs.AppName == "" {
		return
	}

	l.mu.Lock()
	l.lastScreenObserveAt = time.Now()
	l.LastScreenObs = obs
	l.mu.Unlock()

	fn(obs)
}
