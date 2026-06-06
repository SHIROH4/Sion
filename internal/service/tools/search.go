package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"desktop-pet/internal/domain"
)

// SearchTool queries a search engine and returns results.
// Primary: Bing Web Search API (free tier, accessible in China).
// Fallback: LLM-as-search when no API key configured.
type SearchTool struct {
	// OnResults is called with raw search results for LLM processing.
	OnResults func(query string, results []SearchResult) string
	// BingAPIKey is the Azure Bing Web Search API v7 key (optional).
	BingAPIKey string
}

type SearchResult struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

func (s *SearchTool) Name() string     { return "search" }
func (s *SearchTool) Category() string { return "learning" }

func (s *SearchTool) Execute(ctx context.Context, input string) (domain.ToolResult, error) {
	if input == "" {
		return domain.ToolResult{Success: false, Error: "empty query"}, nil
	}

	start := time.Now()

	// Try Bing API if key configured, otherwise LLM handles the query directly.
	var results []SearchResult
	if s.BingAPIKey != "" {
		results, _ = searchBingAPI(ctx, input, s.BingAPIKey)
	}

	if results == nil && s.OnResults == nil {
		return domain.ToolResult{Success: false, Error: "search unavailable"}, nil
	}

	output := fmt.Sprintf("search: %q → %d results", input, len(results))
	if s.OnResults != nil {
		output = s.OnResults(input, results)
	}

	return domain.ToolResult{
		ToolName: "search",
		Success:  true,
		Output:   output,
		Duration: time.Since(start),
	}, nil
}

// searchBingAPI queries the Bing Web Search API v7.
// Free tier: 1000 transactions/month. Sign up at https://portal.azure.com →
// "Create a resource" → "Bing Search v7" → get key from "Keys and Endpoint".
func searchBingAPI(ctx context.Context, query, apiKey string) ([]SearchResult, error) {
	apiURL := "https://api.bing.microsoft.com/v7.0/search?q=" + url.QueryEscape(query) +
		"&count=5&mkt=zh-CN&setlang=zh-Hans"
	req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	req.Header.Set("Ocp-Apim-Subscription-Key", apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("bing API: HTTP %d — %s", resp.StatusCode, string(body))
	}

	var bingResp struct {
		WebPages struct {
			Value []struct {
				Name    string `json:"name"`
				Snippet string `json:"snippet"`
				URL     string `json:"url"`
			} `json:"value"`
		} `json:"webPages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&bingResp); err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, r := range bingResp.WebPages.Value {
		results = append(results, SearchResult{Title: r.Name, Snippet: r.Snippet, URL: r.URL})
	}
	return results, nil
}

