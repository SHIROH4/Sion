package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"desktop-pet/internal/domain"
)

// BrowseTool navigates to a URL and extracts text content.
// Uses a simple HTTP GET for now; can be upgraded to full browser (WebView/playwright).
type BrowseTool struct {
	// OnExtract is called with the page content for LLM summarization.
	OnExtract func(url, rawText string) (summary string, facts []string)
}

func (b *BrowseTool) Name() string     { return "browse" }
func (b *BrowseTool) Category() string { return "learning" }

func (b *BrowseTool) Execute(ctx context.Context, input string) (domain.ToolResult, error) {
	url := strings.TrimSpace(input)
	if url == "" {
		return domain.ToolResult{Success: false, Error: "empty URL"}, nil
	}
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return domain.ToolResult{Success: false, Error: err.Error()}, nil
	}
	req.Header.Set("User-Agent", "Sion/1.0 (AI Desktop Companion)")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return domain.ToolResult{Success: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return domain.ToolResult{Success: false, Error: err.Error()}, nil
	}

	text := extractText(string(body))
	summary := fmt.Sprintf("browse: %s (%d bytes)", url, len(body))

	if b.OnExtract != nil && len(text) > 100 {
		s, facts := b.OnExtract(url, text)
		if s != "" {
			summary = s
		}
		for _, f := range facts {
			summary += "\n  - " + f
		}
	}

	return domain.ToolResult{
		ToolName: "browse",
		Success:  true,
		Output:   summary,
		Duration: time.Since(start),
	}, nil
}

// extractText strips HTML tags and returns plain text.
func extractText(html string) string {
	var b strings.Builder
	inTag := false
	for _, r := range html {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(b.String())
}
