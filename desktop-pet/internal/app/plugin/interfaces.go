package plugin

import (
	"context"
	"log/slog"

	"desktop-pet/internal/domain"
)

// ---- Plugin lifecycle ----

// Plugin is the base interface all plugins must implement.
type Plugin interface {
	Info() PluginInfo
	Awake(ctx PluginContext) error
	Start() error
	Stop() error
	IsRunning() bool
}

// PluginInfo holds metadata for a plugin.
type PluginInfo struct {
	Name        string
	Version     string
	Description string
	Priority    int      // 数字小先加载，销毁时逆序
	Requires    []string // 依赖的其他插件名
}

// PluginContext is passed to a plugin during Awake, providing access to shared services.
type PluginContext struct {
	Ctx        context.Context
	EventBus   *EventBus
	Config     interface{}
	Logger     *slog.Logger
	PluginDir  string
	PokeBuffer *PokeBuffer
	Pipeline   domain.ChatPipeline
}

// FunctionProvider registers AI-callable functions.
type FunctionProvider interface {
	RegisterFunctions(reg *FunctionRegistry)
}

// UIProvider supplies frontend settings components and default config.
type UIProvider interface {
	SettingsComponent() string
	DefaultConfig() map[string]any
}

// ---- Data types (not yet in domain) ----

// UserState captures the user's current state for scheduled tasks.
type UserState struct {
	CurrentHour           int
	IsActive              bool
	ContinuousWorkMinutes int
	IsFirstActive         bool
	GreetedToday          bool
	LastInteractionAt     int64
}

// Action is a behavior command emitted by plugins.
type Action struct {
	Type     string // "say", "expression", "motion", "silent"
	Content  string
	Priority int
}

// ---- Function registry ----

// FunctionRegistry maintains a registry of AI-callable functions.
type FunctionRegistry struct {
	entries []FunctionEntry
}

// FunctionEntry describes a single registered function.
type FunctionEntry struct {
	Name        string
	Description string
	Handler     interface{}
	Parameters  map[string]any // JSON Schema properties; nil = auto-generate {"query": "string"}
}

// Register adds a function to the registry.
func (r *FunctionRegistry) Register(name, description string, handler interface{}) {
	r.entries = append(r.entries, FunctionEntry{Name: name, Description: description, Handler: handler})
}

// RegisterWithParams adds a function with explicit JSON Schema parameters.
func (r *FunctionRegistry) RegisterWithParams(name, description string, handler interface{}, params map[string]any) {
	r.entries = append(r.entries, FunctionEntry{
		Name: name, Description: description, Handler: handler, Parameters: params,
	})
}

// Entries returns all registered functions.
func (r *FunctionRegistry) Entries() []FunctionEntry { return r.entries }

// ---- Backward-compat type aliases (pointing to domain) ----

type (
	ChatContext      = domain.ChatContext
	Message          = domain.Message
	Image            = domain.Image
	ToolCall         = domain.ToolCall
	ToolCallFunction = domain.ToolCallFunction
	ChatProcessor    = domain.ChatProcessor
	MessageFilter    = domain.MessageFilter
	ChatPipeline     = domain.ChatPipeline
)
