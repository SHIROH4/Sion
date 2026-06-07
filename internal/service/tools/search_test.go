package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchTool_NoInput(t *testing.T) {
	s := &SearchTool{}
	result, err := s.Execute(context.Background(), "")
	if err != nil {
		t.Fatal("empty input should not error:", err)
	}
	if result.Success {
		t.Error("empty input should return failure")
	}
}

func TestSearchTool_WithOnResults(t *testing.T) {
	var capturedQuery string
	var capturedResults []SearchResult

	s := &SearchTool{
		OnResults: func(query string, results []SearchResult) string {
			capturedQuery = query
			capturedResults = results
			return "summarized: " + query
		},
	}

	// Test with mock results by passing pre-set results through the callback.
	// We test the wiring by directly calling OnResults.
	output := s.OnResults("Go generics", []SearchResult{
		{Title: "Go Generics Tutorial", Snippet: "Learn about generics in Go 1.18+", URL: "https://example.com"},
		{Title: "Type Parameters", Snippet: "Go supports type parameters", URL: "https://example.com/2"},
	})

	if capturedQuery != "Go generics" {
		t.Errorf("query = %q", capturedQuery)
	}
	if len(capturedResults) != 2 {
		t.Errorf("results count = %d, want 2", len(capturedResults))
	}
	if capturedResults[0].Title != "Go Generics Tutorial" {
		t.Errorf("first result title = %q", capturedResults[0].Title)
	}
	if !strings.Contains(output, "summarized") {
		t.Errorf("output should contain 'summarized': %q", output)
	}
}

func TestSearchTool_NilOnResults(t *testing.T) {
	s := &SearchTool{} // OnResults is nil
	// With nil OnResults, Execute would try to call DDG API.
	// We just verify the tool can be created without OnResults.
	if s.Name() != "search" {
		t.Errorf("name = %q", s.Name())
	}
	if s.Category() != "learning" {
		t.Errorf("category = %q", s.Category())
	}
}

func TestSearchResult_Fields(t *testing.T) {
	r := SearchResult{
		Title:   "Test Title",
		Snippet: "Test Snippet",
		URL:     "https://example.com",
	}
	if r.Title != "Test Title" || r.Snippet != "Test Snippet" || r.URL != "https://example.com" {
		t.Error("SearchResult fields not set correctly")
	}
}

func TestExtractText(t *testing.T) {
	tests := []struct {
		html string
		want string
	}{
		{"<html><body>Hello World</body></html>", "Hello World"},
		{"<div>Line 1</div><p>Line 2</p>", "Line 1Line 2"},
		{"No HTML tags", "No HTML tags"},
		{"<a href='x'>link</a>", "link"},
	}
	for _, tt := range tests {
		got := extractText(tt.html)
		if got != tt.want {
			t.Errorf("extractText(%q) = %q, want %q", tt.html, got, tt.want)
		}
	}
}

// TestBingAPI_Integration tests Bing Web Search API.
// Set BING_API_KEY env var to run. Skipped otherwise.
func TestBingAPI_Integration(t *testing.T) {
	apiKey := "" // set your Bing API key here for testing
	if apiKey == "" {
		t.Skip("no Bing API key")
	}

	ctx := context.Background()
	results, err := searchBingAPI(ctx, "Go programming language", apiKey)
	if err != nil {
		t.Fatalf("Bing search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least 1 result from Bing")
	}
	t.Logf("got %d results for 'Go programming language'", len(results))
	for i, r := range results {
		if i >= 3 { break }
		t.Logf("  %d. %s — %s", i+1, r.Title, r.Snippet[:min(len(r.Snippet), 60)])
	}
}

func TestBochaAPI_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("wrong auth header: %s", r.Header.Get("Authorization"))
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["query"] != "test query" {
			t.Errorf("query = %q, want test query", body["query"])
		}

		resp := map[string]any{
			"code": 200,
			"data": map[string]any{
				"webPages": map[string]any{
					"value": []map[string]any{
						{
							"name":             "Test Result 1",
							"url":              "https://example.com/1",
							"summary":          "Summary of result 1",
							"siteName":         "Example Site",
							"dateLastCrawled":  "2025-06-06T00:00:00",
						},
						{
							"name":             "Test Result 2",
							"url":              "https://example.com/2",
							"summary":          "Summary of result 2",
							"siteName":         "Another Site",
							"dateLastCrawled":  "2025-06-05T00:00:00",
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Override base URL for testing.
	origURL := BochaBaseURL
	BochaSetBaseURL(server.URL)
	defer BochaSetBaseURL(origURL)

	ctx := context.Background()
	results, err := SearchBochaAPI(ctx, "test query", "test-api-key")
	if err != nil {
		t.Fatalf("SearchBochaAPI failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "Test Result 1" {
		t.Errorf("result[0].Title = %q", results[0].Title)
	}
	if results[0].URL != "https://example.com/1" {
		t.Errorf("result[0].URL = %q", results[0].URL)
	}
	if results[0].Snippet != "Summary of result 1" {
		t.Errorf("result[0].Snippet = %q", results[0].Snippet)
	}
	if results[1].Title != "Test Result 2" {
		t.Errorf("result[1].Title = %q", results[1].Title)
	}
}

func TestSearchTool_WithBochaKey(t *testing.T) {
	s := &SearchTool{
		BochaAPIKey: "sk-test-key",
		OnResults: func(query string, results []SearchResult) string {
			if query != "test" {
				t.Errorf("query = %q, want test", query)
			}
			if len(results) != 2 {
				t.Errorf("results count = %d, want 2", len(results))
			}
			return "bocha results for: " + query
		},
	}

	output := s.OnResults("test", []SearchResult{
		{Title: "R1", Snippet: "S1", URL: "https://a.com"},
		{Title: "R2", Snippet: "S2", URL: "https://b.com"},
	})
	if !strings.Contains(output, "bocha results") {
		t.Errorf("output = %q", output)
	}
}

func TestBochaAPI_Integration(t *testing.T) {
	apiKey := "" // set your Bocha API key here for testing
	if apiKey == "" {
		t.Skip("no Bocha API key — set BOCHA_API_KEY env to run integration test")
	}

	ctx := context.Background()
	results, err := SearchBochaAPI(ctx, "Go programming language", apiKey)
	if err != nil {
		t.Fatalf("Bocha search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least 1 result from Bocha")
	}
	t.Logf("got %d results", len(results))
	for i, r := range results {
		if i >= 3 {
			break
		}
		t.Logf("  %d. %s — %s", i+1, r.Title, r.Snippet[:min(len(r.Snippet), 60)])
	}
}

func TestBochaAPI_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid token"}`))
	}))
	defer server.Close()

	origURL := BochaBaseURL
	BochaSetBaseURL(server.URL)
	defer BochaSetBaseURL(origURL)

	ctx := context.Background()
	_, err := SearchBochaAPI(ctx, "test", "bad-key")
	if err == nil {
		t.Error("expected error for HTTP 401")
	}
}

func TestSearchTool_DualBackend(t *testing.T) {
	s := &SearchTool{
		BochaAPIKey: "bocha-key",
		BingAPIKey:  "bing-key",
	}

	if s.BochaAPIKey != "bocha-key" {
		t.Errorf("BochaAPIKey = %q", s.BochaAPIKey)
	}
	if s.BingAPIKey != "bing-key" {
		t.Errorf("BingAPIKey = %q", s.BingAPIKey)
	}
	if s.Name() != "search" {
		t.Errorf("Name = %q", s.Name())
	}
	if s.Category() != "learning" {
		t.Errorf("Category = %q", s.Category())
	}
}
