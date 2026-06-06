package memory

import (
	"desktop-pet/internal/domain"
	"sync"
	"time"
)

// SessionBuffer is a ring-buffer short-term memory for the current session.
// Messages older than maxAge are excluded from Recent() calls, mimicking human
// working memory duration (~30 seconds, but extended to 30 minutes for chat).
type SessionBuffer struct {
	mu         sync.Mutex
	messages   []domain.Message
	timestamps []time.Time
	maxSize    int
	maxAge     time.Duration
}

// NewSessionBuffer creates a session buffer that retains up to maxSize messages
// and expires entries older than maxAge (default 30 min when <= 0).
func NewSessionBuffer(maxSize int) *SessionBuffer {
	if maxSize <= 0 {
		maxSize = 20
	}
	return &SessionBuffer{
		messages:   make([]domain.Message, 0, maxSize),
		timestamps: make([]time.Time, 0, maxSize),
		maxSize:    maxSize,
		maxAge:     30 * time.Minute,
	}
}

// Append adds a message, dropping the oldest when capacity is exceeded.
func (b *SessionBuffer) Append(msg domain.Message) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.messages = append(b.messages, msg)
	b.timestamps = append(b.timestamps, time.Now())
	if len(b.messages) > b.maxSize {
		drop := len(b.messages) - b.maxSize
		b.messages = append(b.messages[:0], b.messages[drop:]...)
		b.timestamps = append(b.timestamps[:0], b.timestamps[drop:]...)
	}
}

// Recent returns the last n messages within maxAge, earliest first.
// Messages older than maxAge are excluded, mimicking time-based working memory decay.
func (b *SessionBuffer) Recent(n int) []domain.Message {
	b.mu.Lock()
	defer b.mu.Unlock()

	if n <= 0 || len(b.messages) == 0 {
		return nil
	}

	cutoff := time.Now().Add(-b.maxAge)
	var out []domain.Message
	start := len(b.messages) - n
	if start < 0 {
		start = 0
	}
	for i := start; i < len(b.messages); i++ {
		if b.timestamps[i].After(cutoff) {
			out = append(out, b.messages[i])
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// All returns a copy of all buffered messages, earliest first.
func (b *SessionBuffer) All() []domain.Message {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]domain.Message, len(b.messages))
	copy(out, b.messages)
	return out
}

// Len returns the number of messages currently in the buffer.
func (b *SessionBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.messages)
}

// Clear empties the buffer.
func (b *SessionBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.messages = b.messages[:0]
	b.timestamps = b.timestamps[:0]
}

// SetMaxAge configures the time-based decay window. Use only in tests.
func (b *SessionBuffer) SetMaxAge(d time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.maxAge = d
}
