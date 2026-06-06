package identity

import (
	"testing"

	"desktop-pet/internal/domain"
	infrastorage "desktop-pet/internal/infra/storage"
)

func newTestGraph(t *testing.T) (*IdentityGraph, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := infrastorage.OpenDB(dir + "/memory.db")
	if err != nil {
		t.Fatal(err)
	}
	repo := infrastorage.NewIdentityRepo(db)
	graph := NewIdentityGraph(repo, nil) // no vectorizer → keyword fallback
	return graph, func() { db.Close() }
}

func TestIdentityGraph_LoadDefaults(t *testing.T) {
	graph, cleanup := newTestGraph(t)
	defer cleanup()

	if err := graph.Load(); err != nil {
		t.Fatal("Load:", err)
	}

	if graph.NodeCount() == 0 {
		t.Fatal("expected default nodes to be initialised")
	}
	t.Logf("initialised %d default nodes", graph.NodeCount())
}

func TestIdentityGraph_RetrieveFallback(t *testing.T) {
	graph, cleanup := newTestGraph(t)
	defer cleanup()
	if err := graph.Load(); err != nil {
		t.Fatal("Load:", err)
	}

	results := graph.Retrieve("主人 程序员 代码", 3)
	if len(results) == 0 {
		t.Fatal("expected retrieval results for '程序员'")
	}
	t.Logf("retrieved %d results", len(results))
	for _, r := range results {
		t.Logf("  - %s", r)
	}
}

func TestIdentityGraph_UpsertAndDeactivate(t *testing.T) {
	graph, cleanup := newTestGraph(t)
	defer cleanup()
	if err := graph.Load(); err != nil {
		t.Fatal("Load:", err)
	}

	initial := graph.NodeCount()

	// Upsert new node.
	node := &domain.IdentityNode{
		Type:       domain.NodePreference,
		Content:    "主人喜欢在深夜写代码",
		Confidence: 0.7,
		Active:     true,
	}
	if err := graph.Upsert(node); err != nil {
		t.Fatal("Upsert:", err)
	}
	if node.ID == 0 {
		t.Fatal("expected non-zero ID after upsert")
	}

	if graph.NodeCount() != initial+1 {
		t.Fatalf("expected %d nodes, got %d", initial+1, graph.NodeCount())
	}

	// Deactivate.
	if err := graph.Deactivate(node.ID); err != nil {
		t.Fatal("Deactivate:", err)
	}
	if graph.NodeCount() != initial {
		t.Fatalf("expected %d nodes after deactivate, got %d", initial, graph.NodeCount())
	}
}

func TestIdentityGraph_UpdateExisting(t *testing.T) {
	graph, cleanup := newTestGraph(t)
	defer cleanup()
	if err := graph.Load(); err != nil {
		t.Fatal("Load:", err)
	}

	results := graph.Retrieve("猫娘 伙伴", 1)
	if len(results) == 0 {
		t.Fatal("expected to find '猫娘' related node")
	}

	// Find the node ID from retrieve results.
	graph.mu.Lock()
	var target *domain.IdentityNode
	for _, n := range graph.nodes {
		if n.Content == results[0] {
			target = n
			break
		}
	}
	graph.mu.Unlock()

	if target == nil {
		t.Fatal("could not find retrieved node")
	}

	original := target.Content
	target.Content = "我是诗音，主人最好的猫娘伙伴（测试更新）"
	if err := graph.Upsert(target); err != nil {
		t.Fatal("Upsert update:", err)
	}

	_ = original
	t.Logf("updated node %d content", target.ID)
}

// Ensure IdentityGraph implements domain.IdentityRepository.
var _ domain.IdentityRepository = (*mockIdentityRepo)(nil)

type mockIdentityRepo struct{}

func (m *mockIdentityRepo) ListActiveIdentityNodes() ([]domain.IdentityNode, error) { return nil, nil }
func (m *mockIdentityRepo) UpsertIdentityNode(*domain.IdentityNode) error           { return nil }
func (m *mockIdentityRepo) DeactivateIdentityNode(int64) error                      { return nil }
func (m *mockIdentityRepo) UpdateIdentityNodeMatchStats(int64, int64, int) error    { return nil }
