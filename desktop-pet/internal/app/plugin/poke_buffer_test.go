package plugin

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestPokeBuffer_Flush(t *testing.T) {
	buf := NewPokeBuffer()
	flushed := make(chan string, 1)
	buf.SetSink(func(merged string) error {
		flushed <- merged
		return nil
	})
	buf.Poke("msg1")
	buf.Poke("msg2")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go buf.FlushLoop(ctx)
	select {
	case result := <-flushed:
		if !strings.Contains(result, "msg1") || !strings.Contains(result, "msg2") {
			t.Errorf("expected msg1 and msg2 in merged, got: %s", result)
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for flush")
	}
}

func TestPokeBuffer_MaxCapacity(t *testing.T) {
	buf := NewPokeBuffer()
	for i := 0; i < 25; i++ {
		buf.Poke(fmt.Sprintf("msg%d", i))
	}
	buf.mu.Lock()
	count := len(buf.pending)
	first := buf.pending[0]
	last := buf.pending[count-1]
	buf.mu.Unlock()

	if count != maxPending {
		t.Fatalf("expected %d pending, got %d", maxPending, count)
	}
	if first != "msg5" {
		t.Errorf("expected oldest to be msg5, got %s", first)
	}
	if last != "msg24" {
		t.Errorf("expected newest to be msg24, got %s", last)
	}
}

func TestPokeBuffer_NilSinkSkipsFlush(t *testing.T) {
	buf := NewPokeBuffer()
	buf.Poke("msg1")
	ctx, cancel := context.WithCancel(context.Background())
	go buf.FlushLoop(ctx)
	// Let a tick fire.
	time.Sleep(2500 * time.Millisecond)
	buf.mu.Lock()
	count := len(buf.pending)
	buf.mu.Unlock()
	cancel()

	if count != 1 {
		t.Errorf("pending should not be drained without sink, got %d", count)
	}
}

func TestPokeBuffer_EmptyPendingSkipsFlush(t *testing.T) {
	buf := NewPokeBuffer()
	flushed := make(chan string, 1)
	buf.SetSink(func(merged string) error {
		flushed <- merged
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go buf.FlushLoop(ctx)
	select {
	case <-flushed:
		t.Error("should not flush with empty pending")
	case <-time.After(3 * time.Second):
		// expected: no flush
	}
}

func TestPokeBuffer_SlowSinkDoesNotBlock(t *testing.T) {
	buf := NewPokeBuffer()
	block := make(chan struct{})
	done := make(chan struct{})
	buf.SetSink(func(merged string) error {
		<-block // block the first flush
		done <- struct{}{}
		return nil
	})
	buf.Poke("msg1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go buf.FlushLoop(ctx)

	// Wait for first flush to start and block.
	time.Sleep(2500 * time.Millisecond)

	// Add more messages while sink is blocked.
	buf.Poke("msg2")
	buf.Poke("msg3")

	// Wait another tick — should skip because flushMu is held.
	time.Sleep(2 * time.Second)

	// Unblock the first flush.
	close(block)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("first flush should complete after unblock")
	}
}

func TestPokeBuffer_FlushLoopExitsOnCancel(t *testing.T) {
	buf := NewPokeBuffer()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		buf.FlushLoop(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
		// expected
	case <-time.After(2 * time.Second):
		t.Error("FlushLoop should exit on cancel")
	}
}
