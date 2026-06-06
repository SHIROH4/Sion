package tools

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"desktop-pet/internal/domain"
)

// Tool is an executable action the AI can invoke.
type Tool interface {
	Name() string
	Category() string // "interaction" | "perception" | "learning" | "reflection"
	Execute(ctx context.Context, input string) (domain.ToolResult, error)
}

// Registry holds all available tools and routes decisions to them.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Execute runs the named tool with the given input.
func (r *Registry) Execute(ctx context.Context, name, input string) (domain.ToolResult, error) {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return domain.ToolResult{}, fmt.Errorf("tool %q not found", name)
	}
	slog.Debug("tools: executing", "tool", name, "input", input[:min(len(input), 80)])
	return t.Execute(ctx, input)
}

// List returns all registered tool names grouped by category.
func (r *Registry) List() map[string][]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string][]string)
	for name, t := range r.tools {
		cat := t.Category()
		result[cat] = append(result[cat], name)
	}
	return result
}
