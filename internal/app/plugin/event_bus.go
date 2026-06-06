package plugin

import (
	"fmt"
	"log/slog"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
)

// Predefined event constants.
const (
	EvtChatSent       = "chat:sent"
	EvtChatResponse   = "chat:response"
	EvtUserActive     = "user:active"
	EvtUserIdle       = "user:idle"
	EvtScreenshot     = "screenshot:new"
	EvtSystemWake     = "system:wake"
	EvtMemoryUpdated  = "memory:updated"
	EvtVoiceAudio     = "voice:audio"
	EvtAutonomousTask = "events:autonomous_task"
)

// Middleware intercepts events. Call next() to proceed to the next middleware
// or the handlers; skip next() to swallow the event.
type Middleware func(event string, payload any, next func())

// CancelFunc unsubscribes a previously registered handler.
type CancelFunc func()

type subscription struct {
	id       uint64
	priority int
	handler  func(payload any)
}

// EventBus is the central event system for inter-plugin communication.
type EventBus struct {
	mu         sync.RWMutex
	handlers   map[string][]*subscription
	middleware []Middleware
	sem        chan struct{}
	nextID     uint64
	logger     *slog.Logger
}

// EventBusOption configures a new EventBus.
type EventBusOption func(*EventBus)

// WithMaxWorkers sets the maximum concurrent handler goroutines.
func WithMaxWorkers(n int) EventBusOption {
	return func(b *EventBus) {
		if n < 1 {
			n = 1
		}
		b.sem = make(chan struct{}, n)
	}
}

// WithLogger sets the logger for panic recovery.
func WithLogger(l *slog.Logger) EventBusOption {
	return func(b *EventBus) {
		b.logger = l
	}
}

// NewEventBus returns an initialized EventBus.
func NewEventBus(opts ...EventBusOption) *EventBus {
	b := &EventBus{
		handlers: make(map[string][]*subscription),
		sem:      make(chan struct{}, runtime.NumCPU()),
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// On registers a handler for the given event and returns a CancelFunc to unsubscribe.
func (b *EventBus) On(event string, handler func(payload any)) CancelFunc {
	return b.OnWithPriority(event, 0, handler)
}

// OnWithPriority registers a handler with an explicit priority (higher runs first).
func (b *EventBus) OnWithPriority(event string, priority int, handler func(payload any)) CancelFunc {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := atomic.AddUint64(&b.nextID, 1)
	sub := &subscription{id: id, priority: priority, handler: handler}
	b.handlers[event] = append(b.handlers[event], sub)

	once := sync.Once{}
	return func() {
		once.Do(func() {
			b.remove(event, id)
		})
	}
}

func (b *EventBus) remove(event string, id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.handlers[event]
	for i, sub := range subs {
		if sub.id == id {
			b.handlers[event] = append(subs[:i], subs[i+1:]...)
			return
		}
	}
}

// Use adds middleware to the event bus. Middleware are applied in registration order.
func (b *EventBus) Use(mw ...Middleware) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.middleware = append(b.middleware, mw...)
}

// Emit asynchronously triggers all handlers for the given event.
// Handlers run with panic recovery, concurrency limiting, and in priority order.
func (b *EventBus) Emit(event string, payload any) {
	b.emit(event, payload, false)
}

// EmitSync triggers all handlers synchronously in priority order.
// Returns only after every handler has completed.
func (b *EventBus) EmitSync(event string, payload any) {
	b.emit(event, payload, true)
}

func (b *EventBus) emit(event string, payload any, synchronous bool) {
	b.mu.RLock()
	// Snapshot and sort by priority descending.
	subs := make([]*subscription, len(b.handlers[event]))
	copy(subs, b.handlers[event])
	middleware := b.middleware
	b.mu.RUnlock()

	sort.Slice(subs, func(i, j int) bool {
		return subs[i].priority > subs[j].priority
	})

	// dispatch is the innermost function: runs all handlers.
	dispatch := func() {
		if synchronous {
			for _, sub := range subs {
				b.safeCall(sub.handler, payload)
			}
		} else {
			var wg sync.WaitGroup
			for _, sub := range subs {
				sub := sub
				wg.Add(1)
				go func() {
					defer wg.Done()
					b.sem <- struct{}{}
					defer func() { <-b.sem }()
					b.safeCall(sub.handler, payload)
				}()
			}
		}
	}

	// Build middleware chain (last registered = outermost).
	chain := dispatch
	for i := len(middleware) - 1; i >= 0; i-- {
		mw := middleware[i]
		prev := chain
		chain = func() {
			mw(event, payload, prev)
		}
	}

	chain()
}

// safeCall invokes handler with panic recovery.
func (b *EventBus) safeCall(handler func(payload any), payload any) {
	defer func() {
		if r := recover(); r != nil {
			b.logger.Error("event handler panic recovered",
				"panic", fmt.Sprintf("%v", r),
			)
		}
	}()
	handler(payload)
}

// SubscriberCount returns the number of handlers registered for an event.
func (b *EventBus) SubscriberCount(event string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.handlers[event])
}

// OnTyped is a generic helper that wraps a typed handler with a type assertion.
func OnTyped[T any](bus *EventBus, event string, handler func(T)) CancelFunc {
	return bus.On(event, func(payload any) {
		if v, ok := payload.(T); ok {
			handler(v)
		}
	})
}

// OnTypedWithPriority is the priority variant of OnTyped.
func OnTypedWithPriority[T any](bus *EventBus, event string, priority int, handler func(T)) CancelFunc {
	return bus.OnWithPriority(event, priority, func(payload any) {
		if v, ok := payload.(T); ok {
			handler(v)
		}
	})
}
