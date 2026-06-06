package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Manager handles plugin lifecycle, capability discovery, and the chat pipeline.
type Manager struct {
	mu            sync.RWMutex
	plugins       map[string]Plugin
	chatHooks     []ChatProcessor
	msgFilters    []MessageFilter
	funcProviders []FunctionProvider
	uiProviders   []UIProvider
	eventBus      *EventBus
	pokeBuffer    *PokeBuffer
	config        interface{}
	kernelReady   bool
}

// NewManager returns an initialized Manager.
func NewManager(cfg interface{}) *Manager {
	return &Manager{
		plugins:    make(map[string]Plugin),
		eventBus:   NewEventBus(),
		pokeBuffer: NewPokeBuffer(),
		config:     cfg,
	}
}

// Register validates dependencies and registers a plugin, auto-discovering its capabilities.
func (m *Manager) Register(p Plugin) error {
	info := p.Info()

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, req := range info.Requires {
		if _, ok := m.plugins[req]; !ok {
			return fmt.Errorf("plugin %s requires %s which is not registered", info.Name, req)
		}
	}

	m.plugins[info.Name] = p

	if cp, ok := p.(ChatProcessor); ok {
		m.chatHooks = append(m.chatHooks, cp)
	}
	if mf, ok := p.(MessageFilter); ok {
		m.msgFilters = append(m.msgFilters, mf)
	}
	if fp, ok := p.(FunctionProvider); ok {
		m.funcProviders = append(m.funcProviders, fp)
	}
	if up, ok := p.(UIProvider); ok {
		m.uiProviders = append(m.uiProviders, up)
	}

	return nil
}

// InitAll runs the two-phase plugin lifecycle and starts the poke flush loop.
func (m *Manager) InitAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sorted := m.sorted()

	pluginDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		pluginDir = filepath.Join(home, ".desktop-pet")
	}

	// Phase 1: Awake (kernel not ready).
	for _, p := range sorted {
		pctx := PluginContext{
			Ctx:        ctx,
			EventBus:   m.eventBus,
			Config:     m.config,
			Logger:     slog.Default(),
			PluginDir:  pluginDir,
			PokeBuffer: m.pokeBuffer,
			Pipeline:   m,
		}
		if err := p.Awake(pctx); err != nil {
			return fmt.Errorf("plugin %s awake: %w", p.Info().Name, err)
		}
	}

	m.kernelReady = true

	// Phase 2: Start (kernel ready, LLM available).
	for _, p := range sorted {
		if err := p.Start(); err != nil {
			return fmt.Errorf("plugin %s start: %w", p.Info().Name, err)
		}
	}

	// Collect all FunctionProviders into a combined registry and pass to ChatPlugin.
	var allFuncs FunctionRegistry
	for _, p := range sorted {
		if fp, ok := p.(FunctionProvider); ok {
			fp.RegisterFunctions(&allFuncs)
		}
	}
	if chat, ok := m.plugins["chat"]; ok {
		if cp, ok := chat.(interface {
			SetFunctionRegistry(*FunctionRegistry)
		}); ok {
			cp.SetFunctionRegistry(&allFuncs)
		}
	}

	// Wire embedding function from chat gateway to memory diary store.
	if chat, mem := m.plugins["chat"], m.plugins["memory"]; chat != nil && mem != nil {
		cp, hasEmbed := chat.(interface {
			EmbeddingFunc() func(string) ([]float32, error)
		})
		mp, hasSetVec := mem.(interface {
			SetVectorizeFunc(func(string) ([]float32, error))
		})
		if hasEmbed && hasSetVec {
			mp.SetVectorizeFunc(cp.EmbeddingFunc())
		}
	}

	go m.pokeBuffer.FlushLoop(ctx)

	return nil
}

// Shutdown stops all plugins in reverse registration order.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	sorted := m.sorted()
	for i := len(sorted) - 1; i >= 0; i-- {
		sorted[i].Stop()
	}
}

// ProcessChat runs the full chat pipeline: filter → before-hooks → LLM → after-hooks → emit.
func (m *Manager) ProcessChat(input string, llmCall func([]Message, func(string) error) error) (*ChatContext, error) {
	m.mu.RLock()
	msgFilters := m.msgFilters
	chatHooks := m.chatHooks
	m.mu.RUnlock()

	// Step 1: build user message and run through filters.
	msg := &Message{Role: "user", Content: input}
	for _, f := range msgFilters {
		if err := f.FilterMessage(msg); err != nil {
			return nil, fmt.Errorf("filter message: %w", err)
		}
	}

	// Step 2: build chat context and run before-chat hooks.
	ctx := &ChatContext{
		Input:    input,
		Messages: []Message{*msg},
	}
	// Run OnBeforeChat in reverse so the chat plugin (system prompt) runs last
	// and its prepend lands first — closest to the LLM's attention window.
	for i := len(chatHooks) - 1; i >= 0; i-- {
		if err := chatHooks[i].OnBeforeChat(ctx); err != nil {
			return nil, fmt.Errorf("before chat: %w", err)
		}
	}

	// Step 3: call LLM with streaming, accumulating output.
	var output strings.Builder
	err := llmCall(ctx.Messages, func(chunk string) error {
		output.WriteString(chunk)
		m.eventBus.Emit(EvtChatResponse, chunk)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("llm call: %w", err)
	}
	ctx.Output = output.String()

	// Step 4: after-chat hooks.
	for _, h := range chatHooks {
		if err := h.OnAfterChat(ctx); err != nil {
			return nil, fmt.Errorf("after chat: %w", err)
		}
	}

	m.eventBus.Emit(EvtChatSent, ctx)

	return ctx, nil
}

// Poke enqueues a message into the poke buffer with a source prefix.
func (m *Manager) Poke(source, message string) {
	m.pokeBuffer.Poke(fmt.Sprintf("[%s] %s", source, message))
}

// PokeBuffer returns the global poke buffer, allowing callers to configure
// its delivery sink (e.g. to emit care messages to the frontend).
func (m *Manager) PokeBuffer() *PokeBuffer {
	return m.pokeBuffer
}

// EventBus returns the manager's event bus.
func (m *Manager) EventBus() *EventBus { return m.eventBus }

// UIProviders returns all registered UI providers.
func (m *Manager) UIProviders() []UIProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.uiProviders
}

// Plugin returns the registered plugin with the given name, or nil.
func (m *Manager) Plugin(name string) Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.plugins[name]
}

// Plugins returns all registered plugins.
func (m *Manager) Plugins() []Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sorted()
}

// IsKernelReady reports whether InitAll has completed phase 1.
func (m *Manager) IsKernelReady() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.kernelReady
}

// sorted returns plugins ordered by ascending Priority.
func (m *Manager) sorted() []Plugin {
	plugins := make([]Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		plugins = append(plugins, p)
	}
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Info().Priority < plugins[j].Info().Priority
	})
	return plugins
}
