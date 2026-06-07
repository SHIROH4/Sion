package memory

import (
	"os"
	"strings"
	"testing"

	"desktop-pet/internal/service/tools"
	infrastorage "desktop-pet/internal/infra/storage"
)

// newTestStore creates a real SQLite store in a temp directory for testing.
func newTestStore(t *testing.T) *infrastorage.Store {
	t.Helper()
	dir, err := os.MkdirTemp("", "sion-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	db, err := infrastorage.OpenDB(dir + "/memory.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	store := infrastorage.NewStore(db)
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSaveJudgedFacts_HighQuality(t *testing.T) {
	store := newTestStore(t)
	p := &MemoryPlugin{store: store}

	jsonResp := `{
		"facts": [
			{"content": "Go 1.26 introduces range-over-func", "source_url": "https://go.dev/blog/go1.26"}
		],
		"quality": {
			"reliability": 0.9,
			"relevance": 0.9,
			"novelty": 0.8,
			"overall": 0.85
		}
	}`

	summary := p.saveJudgedFacts("Go 1.26 features", jsonResp, []tools.SearchResult{
		{Title: "Go 1.26", Snippet: "new features", URL: "https://go.dev/blog/go1.26"},
	})

	facts := store.ListActiveFacts(0)
	if len(facts) != 1 {
		t.Fatalf("expected 1 saved fact, got %d", len(facts))
	}
	if !strings.Contains(facts[0].Content, "range-over-func") {
		t.Errorf("saved fact content = %q", facts[0].Content)
	}
	if facts[0].Source != "web_search" {
		t.Errorf("fact source = %q, want web_search", facts[0].Source)
	}
	if !strings.Contains(summary, "saved:1") {
		t.Errorf("summary should mention saved:1, got %q", summary)
	}
}

func TestSaveJudgedFacts_LowQuality(t *testing.T) {
	store := newTestStore(t)
	p := &MemoryPlugin{store: store}

	jsonResp := `{
		"facts": [
			{"content": "some unreliable fact", "source_url": "https://untrusted.example.com"}
		],
		"quality": {
			"reliability": 0.2,
			"relevance": 0.2,
			"novelty": 0.1,
			"overall": 0.2
		}
	}`

	summary := p.saveJudgedFacts("test query", jsonResp, nil)

	facts := store.ListActiveFacts(0)
	if len(facts) != 0 {
		t.Errorf("expected 0 saved facts (quality < 0.5), got %d", len(facts))
	}
	if !strings.Contains(summary, "saved:0") {
		t.Errorf("summary should mention saved:0, got %q", summary)
	}
}

func TestSaveJudgedFacts_MultipleFacts(t *testing.T) {
	store := newTestStore(t)
	p := &MemoryPlugin{store: store}

	jsonResp := `{
		"facts": [
			{"content": "high quality fact alpha", "source_url": "https://trusted.com/a"},
			{"content": "high quality fact beta", "source_url": "https://trusted.com/b"}
		],
		"quality": {
			"reliability": 0.8,
			"relevance": 0.7,
			"novelty": 0.6,
			"overall": 0.7
		}
	}`

	summary := p.saveJudgedFacts("test query", jsonResp, nil)

	facts := store.ListActiveFacts(0)
	if len(facts) != 2 {
		t.Errorf("expected 2 saved facts, got %d", len(facts))
	}
	if !strings.Contains(summary, "saved:2") {
		t.Errorf("summary should mention saved:2, got %q", summary)
	}
}

func TestSaveJudgedFacts_EmptyFacts(t *testing.T) {
	store := newTestStore(t)
	p := &MemoryPlugin{store: store}

	jsonResp := `{
		"facts": [],
		"quality": {
			"reliability": 0,
			"relevance": 0,
			"novelty": 0,
			"overall": 0
		}
	}`

	summary := p.saveJudgedFacts("test query", jsonResp, nil)

	// Empty facts array + overall=0 → fallback creates one fact from raw text
	// with overall=0.4, which is below the 0.5 gate, so nothing saved.
	facts := store.ListActiveFacts(0)
	if len(facts) != 0 {
		t.Errorf("expected 0 saved facts, got %d", len(facts))
	}
	if !strings.Contains(summary, "saved:0") {
		t.Errorf("summary should mention saved:0, got %q", summary)
	}
}

func TestSaveJudgedFacts_MalformedJSON(t *testing.T) {
	store := newTestStore(t)
	p := &MemoryPlugin{store: store}

	summary := p.saveJudgedFacts("test query", "this is not JSON at all", nil)

	if summary == "" {
		t.Error("summary should not be empty even on malformed JSON")
	}
	if !strings.Contains(summary, "parse error") {
		t.Errorf("should mention parse error, got %q", summary)
	}
}

func TestSaveJudgedFacts_NoStore(t *testing.T) {
	p := &MemoryPlugin{} // nil store

	jsonResp := `{
		"facts": [{"content": "some researched fact here", "source_url": ""}],
		"quality": {"reliability": 0.9, "relevance": 0.9, "novelty": 0.8, "overall": 0.85}
	}`

	// Should not panic even without store/cogRepo.
	summary := p.saveJudgedFacts("test", jsonResp, nil)
	if !strings.Contains(summary, "test") {
		t.Errorf("summary = %q", summary)
	}
	// saved counter still increments (the save is just a no-op when store is nil).
}

func TestSaveJudgedFacts_BorderlineScore(t *testing.T) {
	store := newTestStore(t)
	p := &MemoryPlugin{store: store}

	jsonResp := `{
		"facts": [{"content": "borderline fact exactly at threshold", "source_url": ""}],
		"quality": {"reliability": 0.5, "relevance": 0.5, "novelty": 0.5, "overall": 0.5}
	}`

	p.saveJudgedFacts("test", jsonResp, nil)

	facts := store.ListActiveFacts(0)
	if len(facts) != 1 {
		t.Errorf("overall=0.5 should still qualify (>= 0.5), got %d facts", len(facts))
	}
}

func TestSaveJudgedFacts_JustBelowThreshold(t *testing.T) {
	store := newTestStore(t)
	p := &MemoryPlugin{store: store}

	jsonResp := `{
		"facts": [{"content": "almost good enough fact", "source_url": ""}],
		"quality": {"reliability": 0.49, "relevance": 0.49, "novelty": 0.49, "overall": 0.49}
	}`

	p.saveJudgedFacts("test", jsonResp, nil)

	facts := store.ListActiveFacts(0)
	if len(facts) != 0 {
		t.Errorf("overall=0.49 should NOT qualify (< 0.5), got %d facts", len(facts))
	}
}

func TestSaveJudgedFacts_ComputedOverallFromDimensions(t *testing.T) {
	store := newTestStore(t)
	p := &MemoryPlugin{store: store}

	// No explicit overall → computed: reliability*0.3 + relevance*0.3 + novelty*0.2 + 0.5*0.2
	// = 0.8*0.3 + 0.8*0.3 + 0.7*0.2 + 0.1 = 0.24 + 0.24 + 0.14 + 0.10 = 0.72
	jsonResp := `{
		"facts": [{"content": "computed score fact here", "source_url": ""}],
		"quality": {"reliability": 0.8, "relevance": 0.8, "novelty": 0.7, "overall": 0}
	}`

	p.saveJudgedFacts("test", jsonResp, nil)

	facts := store.ListActiveFacts(0)
	if len(facts) != 1 {
		t.Errorf("computed overall=0.72 should qualify, got %d facts", len(facts))
	}
}
