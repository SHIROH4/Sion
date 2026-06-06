package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// testPlugin is a minimal Plugin implementation for testing.
type testPlugin struct {
	name     string
	priority int
	requires []string

	awakeCalled bool
	startCalled bool
	stopCalled  bool

	awakeErr error
	startErr error

	stopOrder *[]string // if set, Stop appends name here
}

func (p *testPlugin) Info() PluginInfo {
	return PluginInfo{Name: p.name, Priority: p.priority, Requires: p.requires}
}

func (p *testPlugin) Awake(ctx PluginContext) error {
	p.awakeCalled = true
	return p.awakeErr
}

func (p *testPlugin) Start() error {
	p.startCalled = true
	return p.startErr
}

func (p *testPlugin) Stop() error {
	p.stopCalled = true
	if p.stopOrder != nil {
		*p.stopOrder = append(*p.stopOrder, p.name)
	}
	return nil
}

func (p *testPlugin) IsRunning() bool { return p.awakeCalled && p.startCalled }

func TestManager_RegisterAndInit(t *testing.T) {
	m := NewManager(nil)
	p := &testPlugin{name: "test", priority: 10}
	if err := m.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := m.InitAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !p.awakeCalled {
		t.Error("Awake not called")
	}
	if !p.startCalled {
		t.Error("Start not called")
	}
	if !m.IsKernelReady() {
		t.Error("kernelReady should be true")
	}
}

func TestManager_DependencyCheck(t *testing.T) {
	m := NewManager(nil)
	p := &testPlugin{name: "dep", priority: 20, requires: []string{"missing"}}
	err := m.Register(p)
	if err == nil {
		t.Error("expected error for missing dependency")
	}
}

func TestManager_DependencySatisfied(t *testing.T) {
	m := NewManager(nil)
	m.Register(&testPlugin{name: "base", priority: 10})
	err := m.Register(&testPlugin{name: "dep", priority: 20, requires: []string{"base"}})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestManager_ShutdownReverseOrder(t *testing.T) {
	var stopped []string

	m := NewManager(nil)
	p1 := &testPlugin{name: "p1", priority: 10, stopOrder: &stopped}
	p2 := &testPlugin{name: "p2", priority: 20, stopOrder: &stopped}

	m.Register(p1)
	m.Register(p2)
	m.InitAll(context.Background())
	m.Shutdown()

	if len(stopped) != 2 {
		t.Fatalf("expected 2 stops, got %d", len(stopped))
	}
	// Higher priority (later in sorted order) stops first.
	if stopped[0] != "p2" {
		t.Errorf("first stopped should be p2 (higher priority stops first), got %s", stopped[0])
	}
	if stopped[1] != "p1" {
		t.Errorf("second stopped should be p1, got %s", stopped[1])
	}
}

func TestManager_InitAllAwakeError(t *testing.T) {
	m := NewManager(nil)
	m.Register(&testPlugin{name: "ok", priority: 10})
	m.Register(&testPlugin{name: "bad", priority: 20, awakeErr: errors.New("awake failed")})
	err := m.InitAll(context.Background())
	if err == nil {
		t.Error("expected awake error")
	}
	if m.IsKernelReady() {
		t.Error("kernelReady should be false on awake failure")
	}
}

func TestManager_InitAllStartError(t *testing.T) {
	m := NewManager(nil)
	m.Register(&testPlugin{name: "bad", priority: 10, startErr: errors.New("start failed")})
	err := m.InitAll(context.Background())
	if err == nil {
		t.Error("expected start error")
	}
}

func TestManager_ProcessChat(t *testing.T) {
	m := NewManager(nil)

	// Register a plugin that implements ChatProcessor.
	cp := &chatProcessorPlugin{name: "cp"}
	m.Register(cp)
	m.InitAll(context.Background())

	var chunks []string
	llmCall := func(msgs []Message, emit func(string) error) error {
		emit("Hello")
		emit(" World")
		return nil
	}

	ctx, err := m.ProcessChat("hi", llmCall)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Input != "hi" {
		t.Errorf("expected input 'hi', got %q", ctx.Input)
	}
	if ctx.Output != "Hello World" {
		t.Errorf("expected output 'Hello World', got %q", ctx.Output)
	}
	if len(ctx.Messages) == 0 || ctx.Messages[0].Role != "user" {
		t.Error("expected user message in context")
	}
	if !cp.beforeCalled {
		t.Error("OnBeforeChat not called")
	}
	if !cp.afterCalled {
		t.Error("OnAfterChat not called")
	}
	_ = chunks
}

func TestManager_ProcessChatFilterChain(t *testing.T) {
	m := NewManager(nil)

	// Filter that uppercases content.
	m.Register(&upperFilterPlugin{name: "upper"})
	m.InitAll(context.Background())

	llmCall := func(msgs []Message, emit func(string) error) error {
		if msgs[0].Content != "HI" {
			t.Errorf("expected filtered content 'HI', got %q", msgs[0].Content)
		}
		return nil
	}

	_, err := m.ProcessChat("hi", llmCall)
	if err != nil {
		t.Fatal(err)
	}
}

func TestManager_ProcessChatLLMError(t *testing.T) {
	m := NewManager(nil)
	m.InitAll(context.Background())

	llmCall := func(msgs []Message, emit func(string) error) error {
		return errors.New("llm down")
	}

	_, err := m.ProcessChat("hi", llmCall)
	if err == nil {
		t.Error("expected llm error")
	}
}

func TestManager_CapabilityDiscovery(t *testing.T) {
	m := NewManager(nil)

	m.Register(&chatProcessorPlugin{name: "cp"})
	m.Register(&upperFilterPlugin{name: "mf"})
	m.Register(&funcProviderPlugin{name: "fp"})

	if len(m.chatHooks) != 1 {
		t.Errorf("expected 1 ChatProcessor, got %d", len(m.chatHooks))
	}
	if len(m.msgFilters) != 1 {
		t.Errorf("expected 1 MessageFilter, got %d", len(m.msgFilters))
	}
	if len(m.funcProviders) != 1 {
		t.Errorf("expected 1 FunctionProvider, got %d", len(m.funcProviders))
	}
}

func TestManager_Poke(t *testing.T) {
	m := NewManager(nil)
	flushed := make(chan string, 1)
	m.pokeBuffer.SetSink(func(merged string) error {
		flushed <- merged
		return nil
	})
	m.Poke("test", "hello")
	m.Poke("test", "world")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.pokeBuffer.FlushLoop(ctx)

	select {
	case result := <-flushed:
		if !strings.Contains(result, "[test] hello") || !strings.Contains(result, "[test] world") {
			t.Errorf("expected formatted messages, got: %s", result)
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for poke flush")
	}
}

// --- Capability plugin stubs ---

type chatProcessorPlugin struct {
	name         string
	beforeCalled bool
	afterCalled  bool
}

func (p *chatProcessorPlugin) Info() PluginInfo          { return PluginInfo{Name: p.name} }
func (p *chatProcessorPlugin) Awake(PluginContext) error { return nil }
func (p *chatProcessorPlugin) Start() error              { return nil }
func (p *chatProcessorPlugin) Stop() error               { return nil }
func (p *chatProcessorPlugin) IsRunning() bool           { return true }
func (p *chatProcessorPlugin) OnBeforeChat(ctx *ChatContext) error {
	p.beforeCalled = true
	return nil
}
func (p *chatProcessorPlugin) OnAfterChat(ctx *ChatContext) error {
	p.afterCalled = true
	return nil
}

type upperFilterPlugin struct{ name string }

func (p *upperFilterPlugin) Info() PluginInfo          { return PluginInfo{Name: p.name} }
func (p *upperFilterPlugin) Awake(PluginContext) error { return nil }
func (p *upperFilterPlugin) Start() error              { return nil }
func (p *upperFilterPlugin) Stop() error               { return nil }
func (p *upperFilterPlugin) IsRunning() bool           { return true }
func (p *upperFilterPlugin) FilterMessage(msg *Message) error {
	msg.Content = strings.ToUpper(msg.Content)
	return nil
}

type funcProviderPlugin struct{ name string }

func (p *funcProviderPlugin) Info() PluginInfo          { return PluginInfo{Name: p.name} }
func (p *funcProviderPlugin) Awake(PluginContext) error { return nil }
func (p *funcProviderPlugin) Start() error              { return nil }
func (p *funcProviderPlugin) Stop() error               { return nil }
func (p *funcProviderPlugin) IsRunning() bool           { return true }
func (p *funcProviderPlugin) RegisterFunctions(reg *FunctionRegistry) {
	reg.Register("test", "a test function", nil)
}
