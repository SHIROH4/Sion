package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"desktop-pet/internal/domain"
)

// SearchTool queries a search engine and returns results.
// Primary: Bocha Web Search API (domestic, no VPN needed).
// Secondary: Bing Web Search API (free tier).
// Fallback: LLM-as-search when no API key configured.
type SearchTool struct {
	// OnResults is called with raw search results for LLM processing.
	OnResults func(query string, results []SearchResult) string
	// BingAPIKey is the Azure Bing Web Search API v7 key (optional).
	BingAPIKey string
	// BochaAPIKey is the Bocha Web Search API key (optional, preferred).
	BochaAPIKey string
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

	// Try Bocha API first (domestic, no VPN needed), then Bing, then fallback to LLM.
	var results []SearchResult
	if s.BochaAPIKey != "" {
		results, _ = SearchBochaAPI(ctx, input, s.BochaAPIKey)
	}
	if results == nil && s.BingAPIKey != "" {
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

// BochaBaseURL is the base URL for the Bocha Web Search API.
// Overridable in tests via BochaSetBaseURL.
var BochaBaseURL = "https://api.bochaai.com/v1/web-search"

// BochaSetBaseURL overrides the Bocha API base URL (for testing).
func BochaSetBaseURL(url string) { BochaBaseURL = url }

// SearchBochaAPI queries the Bocha Web Search API.
// Domestic service (no VPN needed). Register at https://open.bochaai.com →
// WeChat login → create API Key. Free tier: 1000 calls.
// Docs: https://bocha-ai.feishu.cn/wiki/HmtOw1z6vik14Fkdu5uc9VaInBb
func SearchBochaAPI(ctx context.Context, query, apiKey string) ([]SearchResult, error) {
	apiURL := BochaBaseURL
	payload, _ := json.Marshal(map[string]interface{}{
		"query":     query,
		"freshness": "noLimit",
		"summary":   true,
		"count":     5,
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(string(payload)))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("bocha API: HTTP %d — %s", resp.StatusCode, string(body))
	}

	var bochaResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			WebPages struct {
				Value []struct {
					Name            string `json:"name"`
					URL             string `json:"url"`
					Snippet         string `json:"snippet"`
					Summary         string `json:"summary"`
					SiteName        string `json:"siteName"`
					SiteIcon        string `json:"siteIcon"`
					DateLastCrawled string `json:"dateLastCrawled"`
				} `json:"value"`
			} `json:"webPages"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&bochaResp); err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, r := range bochaResp.Data.WebPages.Value {
		snippet := r.Summary
		if snippet == "" {
			snippet = r.Snippet
		}
		results = append(results, SearchResult{
			Title:   r.Name,
			Snippet: snippet,
			URL:     r.URL,
		})
	}
	return results, nil
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

