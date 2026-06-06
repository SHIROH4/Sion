package tools

import (
	"context"
	"strings"
	"time"

	"desktop-pet/internal/domain"
)

// SpeakTool wraps the proactive chat generator as a tool.
type SpeakTool struct {
	SpeakFunc func(source domain.ProactiveSource, mood string, reason string) error
}

func (s *SpeakTool) Name() string     { return "speak" }
func (s *SpeakTool) Category() string { return "interaction" }

func (s *SpeakTool) Execute(ctx context.Context, input string) (domain.ToolResult, error) {
	if s.SpeakFunc == nil {
		return domain.ToolResult{Success: false, Error: "speak tool not wired"}, nil
	}
	start := time.Now()

	// Parse: "casual|reason text|mood" or just "casual"
	parts := strings.SplitN(input, "|", 3)
	source := domain.ProactiveSource(parts[0])
	if source == "" {
		source = domain.SourceCasual
	}
	reason := ""
	mood := ""
	if len(parts) > 1 {
		reason = parts[1]
	}
	if len(parts) > 2 {
		mood = parts[2]
	}

	err := s.SpeakFunc(source, mood, reason)
	return domain.ToolResult{
		ToolName: "speak",
		Success:  err == nil,
		Output:   reason,
		Duration: time.Since(start),
	}, err
}
