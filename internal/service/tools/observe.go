package tools

import (
	"context"
	"time"

	"desktop-pet/internal/domain"
)

// ObserveTool captures the screen and runs visual analysis.
type ObserveTool struct {
	// ObserveFunc takes a screenshot and analyzes it, returning gaps found.
	ObserveFunc func() (gapsFound int, screenSummary string, err error)
}

func (o *ObserveTool) Name() string     { return "observe" }
func (o *ObserveTool) Category() string { return "perception" }

func (o *ObserveTool) Execute(ctx context.Context, input string) (domain.ToolResult, error) {
	if o.ObserveFunc == nil {
		return domain.ToolResult{Success: false, Error: "observe tool not wired"}, nil
	}
	start := time.Now()
	_, summary, err := o.ObserveFunc()
	return domain.ToolResult{
		ToolName: "observe",
		Success:  err == nil,
		Output:   summary,
		Duration: time.Since(start),
	}, err
}
