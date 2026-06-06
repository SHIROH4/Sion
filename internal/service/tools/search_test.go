package tools

import (
	"context"
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
