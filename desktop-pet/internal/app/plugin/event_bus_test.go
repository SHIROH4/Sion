package plugin

import (
	"sync"
	"testing"
	"time"
)

func TestEventBus_Emit(t *testing.T) {
	bus := NewEventBus()
	received := make(chan string, 1)
	bus.On("test:event", func(payload any) {
		received <- payload.(string)
	})
	bus.Emit("test:event", "hello")
	select {
	case msg := <-received:
		if msg != "hello" {
			t.Errorf("expected 'hello', got '%s'", msg)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for event")
	}
}

func TestEventBus_MultipleHandlers(t *testing.T) {
	bus := NewEventBus()
	var wg sync.WaitGroup
	wg.Add(2)
	bus.On("test:multi", func(payload any) { wg.Done() })
	bus.On("test:multi", func(payload any) { wg.Done() })
	bus.Emit("test:multi", nil)
	wg.Wait()
}

// ❶ 缺陷修复：handler 内部 panic 不应导致程序崩溃
func TestEventBus_PanicRecovery(t *testing.T) {
	bus := NewEventBus()
	done := make(chan struct{})
	bus.On("test:panic", func(payload any) {
		panic("intentional panic for test")
	})
	bus.On("test:panic", func(payload any) {
		close(done)
	})
	bus.Emit("test:panic", nil)
	select {
	case <-done:
		// second handler still executed after the first panicked
	case <-time.After(1 * time.Second):
		t.Error("timeout: second handler was not called after panic")
	}
}

// ❷ 缺陷修复：取消订阅 (On 返回 CancelFunc)
func TestEventBus_Unsubscribe(t *testing.T) {
	bus := NewEventBus()
	count := 0
	cancel := bus.On("test:unsub", func(payload any) {
		count++
	})
	cancel()
	bus.EmitSync("test:unsub", nil)
	if count != 0 {
		t.Errorf("expected 0 calls after unsubscribe, got %d", count)
	}
}

// ❸ 缺陷修复：事件中间件（日志/追踪/限流）
func TestEventBus_Middleware(t *testing.T) {
	bus := NewEventBus()
	var order []string

	bus.Use(func(event string, payload any, next func()) {
		order = append(order, "mw1:before")
		next()
		order = append(order, "mw1:after")
	})
	bus.Use(func(event string, payload any, next func()) {
		order = append(order, "mw2:before")
		next()
		order = append(order, "mw2:after")
	})

	bus.On("test:mw", func(payload any) {
		order = append(order, "handler")
	})
	bus.EmitSync("test:mw", nil)

	expected := []string{"mw1:before", "mw2:before", "handler", "mw2:after", "mw1:after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d entries, got %d: %v", len(expected), len(order), order)
	}
	for i, s := range order {
		if s != expected[i] {
			t.Errorf("order[%d]: expected %q, got %q", i, expected[i], s)
		}
	}
}

// ❸ 补充：中间件可以吞掉事件（不调用 next）
func TestEventBus_MiddlewareSwallow(t *testing.T) {
	bus := NewEventBus()
	bus.Use(func(event string, payload any, next func()) {
		// intentionally skip next()
	})
	called := false
	bus.On("test:swallow", func(payload any) {
		called = true
	})
	bus.EmitSync("test:swallow", nil)
	if called {
		t.Error("handler should not have been called when middleware swallows event")
	}
}

// ❹ 缺陷修复：并发控制 (semaphore 限制并发 worker 数)
func TestEventBus_ConcurrencyLimit(t *testing.T) {
	bus := NewEventBus(WithMaxWorkers(2))

	var mu sync.Mutex
	var maxConcurrent, current int

	const numHandlers = 10
	var wg sync.WaitGroup
	wg.Add(numHandlers)

	for i := 0; i < numHandlers; i++ {
		bus.On("test:concurrency", func(payload any) {
			mu.Lock()
			current++
			if current > maxConcurrent {
				maxConcurrent = current
			}
			mu.Unlock()

			time.Sleep(10 * time.Millisecond) // hold the worker slot briefly

			mu.Lock()
			current--
			mu.Unlock()

			wg.Done()
		})
	}
	bus.Emit("test:concurrency", nil)
	wg.Wait()

	if maxConcurrent > 2 {
		t.Errorf("max concurrent workers exceeded limit: got %d, limit 2", maxConcurrent)
	}
}

// ❺ 缺陷修复：事件优先级（priority 高的 handler 先执行）
func TestEventBus_Priority(t *testing.T) {
	bus := NewEventBus()
	var order []int

	bus.OnWithPriority("test:prio", 0, func(payload any) {
		order = append(order, 0)
	})
	bus.OnWithPriority("test:prio", 10, func(payload any) {
		order = append(order, 10)
	})
	bus.OnWithPriority("test:prio", 5, func(payload any) {
		order = append(order, 5)
	})

	bus.EmitSync("test:prio", nil)

	if len(order) != 3 {
		t.Fatalf("expected 3 handlers, got %d", len(order))
	}
	if order[0] != 10 || order[1] != 5 || order[2] != 0 {
		t.Errorf("expected [10, 5, 0], got %v", order)
	}
}

// ❻ 缺陷修复：类型安全 (OnTyped 泛型 helper)
func TestEventBus_TypedHandler(t *testing.T) {
	bus := NewEventBus()

	type ChatPayload struct {
		User    string
		Message string
	}

	received := make(chan ChatPayload, 1)
	OnTyped(bus, "chat:typed", func(p ChatPayload) {
		received <- p
	})

	payload := ChatPayload{User: "alice", Message: "hello"}
	bus.Emit("chat:typed", payload)

	select {
	case p := <-received:
		if p.User != "alice" || p.Message != "hello" {
			t.Errorf("unexpected payload: %+v", p)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout")
	}
}

// ❻ 补充：类型不匹配时 OnTyped handler 被跳过（不 panic）
func TestEventBus_TypedHandlerMismatch(t *testing.T) {
	bus := NewEventBus()

	type Foo struct{ X int }
	type Bar struct{ Y int }

	called := false
	OnTyped(bus, "test:mismatch", func(f Foo) {
		called = true
	})

	// emit a Bar instead of Foo — handler should be skipped silently
	bus.EmitSync("test:mismatch", Bar{Y: 42})

	if called {
		t.Error("typed handler should not be called with mismatched type")
	}
}

// EmitSync 同步发射验证
func TestEventBus_EmitSync(t *testing.T) {
	bus := NewEventBus()
	counter := 0
	for i := 0; i < 5; i++ {
		bus.On("test:sync", func(payload any) {
			counter++
		})
	}
	bus.EmitSync("test:sync", nil)
	if counter != 5 {
		t.Errorf("expected counter=5 after EmitSync, got %d", counter)
	}
}
