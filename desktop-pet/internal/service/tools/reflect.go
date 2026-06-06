package tools

import (
	"context"
	"time"

	"desktop-pet/internal/domain"
)

// ReflectTool wraps the strategic agent's daily reflection.
type ReflectTool struct {
	ReflectFunc func() (*domain.DailyReflectionOutput, error)
}

func (r *ReflectTool) Name() string     { return "reflect" }
func (r *ReflectTool) Category() string { return "reflection" }

func (r *ReflectTool) Execute(ctx context.Context, input string) (domain.ToolResult, error) {
	if r.ReflectFunc == nil {
		return domain.ToolResult{Success: false, Error: "reflect tool not wired"}, nil
	}
	start := time.Now()
	output, err := r.ReflectFunc()
	summary := ""
	if output != nil {
		summary = output.NarrativeSummary
	}
	return domain.ToolResult{
		ToolName: "reflect",
		Success:  err == nil,
		Output:   summary,
		Duration: time.Since(start),
	}, err
}
