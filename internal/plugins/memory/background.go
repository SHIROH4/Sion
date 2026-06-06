package memory

import (
	"sync"
	"time"

	"desktop-pet/internal/app/background"
	"desktop-pet/internal/domain"
	"desktop-pet/internal/infra"
	"desktop-pet/internal/infra/native"
	svcmemory "desktop-pet/internal/service/memory"
)

type FactConsolidator = svcmemory.FactConsolidator

var CleanJSON = infra.CleanJSON

func ConvertScreenObs(obs native.ScreenObservation) domain.ScreenObservation {
	return domain.ScreenObservation{
		AppName: obs.AppName, WindowTitle: obs.WindowTitle,
		OCRText: obs.OCRText, CapturedAt: obs.CapturedAt,
		IsWorking: obs.IsWorking,
	}
}

// BackgroundCognition wraps background.BackgroundLoop, keeping the same
// public API for the MemoryPlugin.
type BackgroundCognition struct {
	loop *background.BackgroundLoop

	mu            sync.Mutex
	lastUserMsg   string
	lastScreenObs native.ScreenObservation
}

// NewBackgroundCognition creates a BackgroundCognition wired to a BackgroundLoop.
func NewBackgroundCognition(
	layers *MemoryLayer,
	rawLLM func([]domain.Message) (string, error),
	emotion domain.EmotionEvaluator,
) *BackgroundCognition {
	loop := background.NewBackgroundLoop()
	loop.RawLLM = rawLLM
	loop.Emotion = emotion

	if layers != nil {
		loop.Consolidate = func(rllm func([]domain.Message) (string, error)) {
			layers.Consolidate(rllm)
		}
		if layers.Semantic != nil {
			loop.SaveFact = layers.Semantic.SaveFact
			loop.SaveMemCell = layers.Semantic.SaveMemCell
		}
		loop.Forget = layers.Forget
	}


	return &BackgroundCognition{loop: loop}
}

// Start launches the background loop.
func (b *BackgroundCognition) Start() { b.loop.Start() }

// Stop signals the background loop to shut down.
func (b *BackgroundCognition) Stop() { b.loop.Stop() }

// NotifyActivity records a user interaction. Also wakes the loop for event-driven decision.
func (b *BackgroundCognition) NotifyActivity() { b.loop.NotifyActivity(); b.loop.Wake() }

// SetScreenInterval configures the screen observation interval.
func (b *BackgroundCognition) SetScreenInterval(d time.Duration) { b.loop.SetScreenInterval(d) }

// SetIntervalFunc wires the dynamic interval computation callback.
func (b *BackgroundCognition) SetIntervalFunc(fn func() time.Duration) { b.loop.IntervalFunc = fn }

// SetOnScreenObserve registers the L1 screen observation callback.
func (b *BackgroundCognition) SetOnScreenObserve(fn func(native.ScreenObservation)) {
	b.loop.SetOnScreenObserve(func(obs native.ScreenObservation) {
		b.mu.Lock()
		b.lastScreenObs = obs
		b.mu.Unlock()
		if fn != nil {
			fn(obs)
		}
	})
}

// NotifyEpisodeCreated increments the episode counter.
func (b *BackgroundCognition) NotifyEpisodeCreated() { b.loop.NotifyEpisodeCreated() }

// SetScheduler sets or updates the scheduler.
func (b *BackgroundCognition) SetScheduler(s domain.Scheduler) { b.loop.Sched = s }

// SetCare sets or updates the care provider.
func (b *BackgroundCognition) SetCare(c domain.CareProvider) { b.loop.Care = c }

// SetOnCleanup registers the cleanup callback.
func (b *BackgroundCognition) SetOnCleanup(fn func()) { b.loop.OnCleanup = fn }

// SetOnReflect registers the reflection callback.
func (b *BackgroundCognition) SetOnReflect(fn func()) { b.loop.OnReflect = fn }

// SetOnDetectGaps registers the knowledge gap detection callback.
func (b *BackgroundCognition) SetOnDetectGaps(fn func()) { b.loop.OnDetectGaps = fn }

// SetOnSystem2Decision registers the System 2 LLM autonomous decision callback.
func (b *BackgroundCognition) SetOnSystem2Decision(fn func()) { b.loop.OnSystem2Decision = fn }

// SetStrategyAgent registers the daily strategic reflection callback.
func (b *BackgroundCognition) SetStrategyAgent(fn func()) { b.loop.StrategyAgent = fn }

// TacticalDirectives returns the current tactical directives set by the strategic agent.
func (b *BackgroundCognition) TacticalDirectives() []string { return b.loop.TacticalDirectives }

// SetProactiveLearner sets the proactive learner callbacks.
func (b *BackgroundCognition) SetProactiveLearner(p *ProactiveLearner) {
	if p != nil {
		b.loop.ShouldLearn = p.ShouldLearn
		b.loop.Learn = func(
			rawLLM func([]domain.Message) (string, error),
			emo domain.EmotionState,
			saveMC func(string, string, float64, float64, float64, string) error,
			saveF func(string, string) error,
		) {
			p.Learn(rawLLM, emo, saveMC, saveF)
		}
	}
}

// SetTopicStore sets the topic store callbacks.
func (b *BackgroundCognition) SetTopicStore(ts *TopicStore) {
	if ts != nil {
		b.loop.ListTopics = ts.ListTopics
		b.loop.UpdateCentroid = func(id int64) { _ = ts.UpdateCentroid(id) }
	}
}

// SetFactConsolidator sets the fact consolidator callbacks.
func (b *BackgroundCognition) SetFactConsolidator(fc *FactConsolidator) {
	if fc != nil {
		b.loop.ShouldConsolidate = fc.ShouldRun
		b.loop.RunConsolidation = func() (background.ConsolidationResult, error) {
			r, err := fc.Run()
			return background.ConsolidationResult{
				ClustersFound: r.ClustersFound,
				Merged:        r.Merged,
				Discarded:     r.Discarded,
				Kept:          r.Kept,
				FactsArchived: r.FactsArchived,
			}, err
		}
	}
}

// ---- field accessors (used by MemoryPlugin and generator) ----

// SetUserMsg sets the last user message for context anchoring.
func (b *BackgroundCognition) SetUserMsg(msg string) {
	b.mu.Lock()
	b.lastUserMsg = msg
	b.mu.Unlock()
	b.loop.LastUserMsg = msg
}

// LastScreenObs returns the latest screen observation.
func (b *BackgroundCognition) LastScreenObs() native.ScreenObservation {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastScreenObs
}
