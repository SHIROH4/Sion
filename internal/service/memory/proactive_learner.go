package memory

import (
	"fmt"
	"sync"
	"time"

	"desktop-pet/internal/domain"
)

// ProactiveLearner periodically reviews unprocessed observations and extracts
// knowledge. It runs as part of the background cognition loop.
type ProactiveLearner struct {
	mu sync.Mutex

	pending     []domain.Observation
	lastLearnAt time.Time
	minInterval time.Duration
	maxPending  int
}

// NewProactiveLearner creates a learner with default settings.
func NewProactiveLearner() *ProactiveLearner {
	return &ProactiveLearner{
		pending:     make([]domain.Observation, 0),
		lastLearnAt: time.Now(),
		minInterval: 2 * time.Minute,
		maxPending:  50,
	}
}

// Ingest adds an observation to the pending queue. Non-chat observations are
// queued for learning; chat observations are skipped (already processed by OnAfterChat).
func (l *ProactiveLearner) Ingest(obs domain.Observation) {
	if obs.Source == domain.ObsChat {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pending = append(l.pending, obs)
	if len(l.pending) > l.maxPending {
		drop := len(l.pending) - l.maxPending
		l.pending = append(l.pending[:0], l.pending[drop:]...)
	}
}

// ShouldLearn returns true when enough time has passed and there are pending observations.
func (l *ProactiveLearner) ShouldLearn() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.pending) > 0 && time.Since(l.lastLearnAt) > l.minInterval
}

// Learn processes pending observations through MemCell extraction and fact encoding.
func (l *ProactiveLearner) Learn(
	rawLLM func([]domain.Message) (string, error),
	emo domain.EmotionState,
	saveMemCell func(t string, content string, importance float64, valence, arousal float64, sourceMsg string) error,
	saveFact func(content, source string) error,
) int {
	if rawLLM == nil {
		return 0
	}

	l.mu.Lock()
	if len(l.pending) == 0 {
		l.mu.Unlock()
		return 0
	}
	batchSize := 5
	if batchSize > len(l.pending) {
		batchSize = len(l.pending)
	}
	batch := make([]domain.Observation, batchSize)
	copy(batch, l.pending[:batchSize])
	l.pending = l.pending[batchSize:]
	l.lastLearnAt = time.Now()
	l.mu.Unlock()

	processed := 0
	for _, obs := range batch {
		tag := ""
		if obs.Source != domain.ObsChat {
			tag = fmt.Sprintf("[%s观测] ", obs.Source)
		}
		msgs := []domain.Message{{Role: "user", Content: tag + obs.Content}}
		cells, _ := ExtractMemCells(rawLLM, msgs, emo)
		for _, c := range cells {
			if saveMemCell != nil {
				saveMemCell(string(c.Type), c.Content, c.Importance,
					emo.Valence, emo.Arousal, obs.Content)
			}
			if c.Importance >= 0.5 && saveFact != nil {
				saveFact(c.Content, string(obs.Source))
			}
		}
		processed++
	}
	return processed
}

// PendingCount returns the number of unprocessed observations.
func (l *ProactiveLearner) PendingCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.pending)
}

// SetMinInterval overrides the minimum learning interval (for tests).
func (l *ProactiveLearner) SetMinInterval(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minInterval = d
}
