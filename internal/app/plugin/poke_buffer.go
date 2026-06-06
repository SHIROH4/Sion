package plugin

import (
	"context"
	"strings"
	"sync"
	"time"
)

const maxPending = 20

// PokeBuffer buffers poke interactions from the user,
// merging them into periodic batched deliveries via a configurable sink.
type PokeBuffer struct {
	mu      sync.Mutex
	flushMu sync.Mutex // ensures at most one flush in flight
	pending []string
	sink    func(merged string) error
}

// NewPokeBuffer returns an initialized PokeBuffer.
func NewPokeBuffer() *PokeBuffer {
	return &PokeBuffer{}
}

// SetSink sets the callback invoked each flush tick with merged pending messages.
func (b *PokeBuffer) SetSink(fn func(merged string) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sink = fn
}

// Poke enqueues a message for the next flush cycle.
// If pending exceeds 20, the oldest message is dropped.
func (b *PokeBuffer) Poke(msg string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.pending = append(b.pending, msg)
	if len(b.pending) > maxPending {
		b.pending = b.pending[len(b.pending)-maxPending:]
	}
}

// FlushLoop runs a blocking tick loop that merges and flushes pending messages
// every 2 seconds. It exits when ctx is cancelled.
func (b *PokeBuffer) FlushLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			go b.flush()
		}
	}
}

func (b *PokeBuffer) flush() {
	if !b.flushMu.TryLock() {
		return // previous flush still running, skip this tick
	}
	defer b.flushMu.Unlock()

	b.mu.Lock()
	if len(b.pending) == 0 || b.sink == nil {
		b.mu.Unlock()
		return
	}
	merged := strings.Join(b.pending, "\n")
	b.pending = b.pending[:0]
	sink := b.sink
	b.mu.Unlock()

	sink(merged)
}
