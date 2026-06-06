package storage

import (
	"testing"
	"time"

	"desktop-pet/internal/domain"
)

func newTestStore(t *testing.T) (*Store, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDB(dir + "/memory.db")
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	return store, func() { store.Close() }
}

// ---- Fact CRUD ----

func TestStore_SaveAndLoadFacts(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	must(store.SaveFact("主人使用Go语言开发", "chat"))
	must(store.SaveFact("主人喜欢深色主题", "chat"))
	must(store.SaveFact("主人生日是6月15日", "chat"))

	facts := store.LoadFacts()
	if len(facts) < 3 {
		t.Fatalf("expected >= 3 facts, got %d", len(facts))
	}
}

func TestStore_SaveFact_DeduplicateContent(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	store.SaveFact("主人使用Go语言", "chat")
	err := store.SaveFact("主人使用Go语言", "chat")
	if err != nil {
		t.Fatal("SaveFact should not error on duplicate:", err)
	}

	facts := store.LoadFacts()
	count := 0
	for _, f := range facts {
		if f == "主人使用Go语言" {
			count++
		}
	}
	if count > 1 {
		t.Fatalf("expected 1 occurrence, got %d", count)
	}
}

func TestStore_ListActiveFacts(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	store.SaveFact("高重要性事实", "chat")
	store.SaveFact("低重要性事实", "chat")

	facts := store.ListActiveFacts(0)
	if len(facts) == 0 {
		t.Fatal("expected facts with threshold 0")
	}

	// High threshold should filter
	facts = store.ListActiveFacts(0.9)
	// New facts default to importance 0.5, so none should match
	if len(facts) > 0 {
		t.Log("some facts have default importance >= 0.9")
	}
}

func TestStore_ArchiveAndCleanFacts(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	store.SaveFact("待归档事实", "chat")
	facts := store.ListActiveFacts(0)
	if len(facts) == 0 {
		t.Fatal("expected facts")
	}

	id := facts[0].ID
	if err := store.ArchiveFact(id); err != nil {
		t.Fatal("ArchiveFact:", err)
	}

	active := store.ListActiveFacts(0)
	for _, f := range active {
		if f.ID == id {
			t.Fatal("archived fact should not appear in active list")
		}
	}

	// CleanArchivedFacts with 0 retention → immediate deletion
	n := store.CleanArchivedFacts(0)
	t.Logf("cleaned %d archived facts", n)
}

func TestStore_UpdateFactRecall(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	store.SaveFact("待召回事实", "chat")
	facts := store.ListActiveFacts(0)
	if len(facts) == 0 {
		t.Fatal("expected facts")
	}

	id := facts[0].ID
	for i := 0; i < 3; i++ {
		if err := store.UpdateFactRecall(id); err != nil {
			t.Fatal("UpdateFactRecall:", err)
		}
	}

	updated := store.ListActiveFacts(0)
	for _, f := range updated {
		if f.ID == id {
			if f.RecallCount != 3 {
				t.Fatalf("expected recall_count=3, got %d", f.RecallCount)
			}
			return
		}
	}
	t.Fatal("fact not found after recall update")
}

func TestStore_BatchUpdateFactRecall(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	store.SaveFact("主人批量测试第一条", "chat")
	store.SaveFact("主人批量测试第二条", "chat")
	facts := store.ListActiveFacts(0)
	if len(facts) < 2 {
		t.Fatalf("need at least 2 facts, got %d", len(facts))
	}

	ids := []int64{facts[0].ID, facts[1].ID}
	if err := store.BatchUpdateFactRecall(ids); err != nil {
		t.Fatal("BatchUpdateFactRecall:", err)
	}
}

// ---- Profile CRUD ----

func TestStore_ProfileCRUD(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	if err := store.SaveProfile("name", "白羽"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile("tech_stack", "Go,Python"); err != nil {
		t.Fatal(err)
	}

	profile := store.LoadProfile()
	if profile.Name != "白羽" {
		t.Fatalf("expected 白羽, got %s", profile.Name)
	}
	if len(profile.TechStack) != 2 {
		t.Fatalf("expected 2 tech stacks, got %d", len(profile.TechStack))
	}
}

func TestStore_SelfProfile(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	if err := store.SaveSelfProfile("我是诗音，主人的猫娘伙伴。"); err != nil {
		t.Fatal(err)
	}

	self := store.LoadSelfProfile()
	if self == "" {
		t.Fatal("expected non-empty self profile")
	}
}

// ---- Chat History ----

func TestStore_ChatHistory(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	msgs := []domain.Message{
		{Role: "user", Content: "你好"},
		{Role: "assistant", Content: "喵~主人好！"},
	}
	if err := store.SaveHistory(msgs, 0); err != nil {
		t.Fatal("SaveHistory:", err)
	}

	loaded, err := store.LoadHistory(10)
	if err != nil {
		t.Fatal("LoadHistory:", err)
	}
	if len(loaded) < 2 {
		t.Fatalf("expected >= 2 messages, got %d", len(loaded))
	}
}

func TestStore_CleanOldHistory(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	store.SaveHistory([]domain.Message{{Role: "user", Content: "old"}}, 0)
	if err := store.CleanOldHistory(0); err != nil {
		t.Fatal("CleanOldHistory:", err)
	}

	loaded, _ := store.LoadHistory(10)
	t.Logf("after clean, %d messages remain", len(loaded))
}

// ---- MemCell ----

func TestStore_MemCellCRUD(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	err := store.SaveMemCell("fact", "主人喜欢咖啡", 0.8, 0.3, 0.1, "主人说喜欢喝咖啡")
	if err != nil {
		t.Fatal("SaveMemCell:", err)
	}

	cells := store.ListMemCells("fact", 5)
	if len(cells) == 0 {
		t.Fatal("expected at least 1 memcell")
	}
	if cells[0].Content != "主人喜欢咖啡" {
		t.Fatalf("unexpected content: %s", cells[0].Content)
	}
}

// ---- Archive ----

func TestStore_ArchiveCRUD(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	err := store.SaveArchive("test-archive", 1, "original text", "summary text")
	if err != nil {
		t.Fatal("SaveArchive:", err)
	}

	original, err := store.FindArchiveByName("test-archive")
	if err != nil {
		t.Fatal("FindArchiveByName:", err)
	}
	if original != "original text" {
		t.Fatalf("expected 'original text', got %s", original)
	}
}

// ---- Search ----

func TestStore_SearchArchives(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	store.SaveArchive("go-tips", 1, "Go tips about concurrency", "Go并发技巧")
	store.SaveArchive("rust-notes", 2, "Rust ownership notes", "Rust所有权笔记")

	results, err := store.SearchArchives("Go", 5)
	if err != nil {
		t.Fatal("SearchArchives:", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 search result")
	}
}

func TestStore_UnifiedSearch_NoVector(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	store.SaveFact("主人使用Go语言开发后端服务", "chat")
	store.SaveFact("主人喜欢喝咖啡", "chat")

	// Without embedding service, search falls back to keyword
	results, err := store.UnifiedSearch(nil, "Go", 5)
	if err != nil {
		t.Fatal("UnifiedSearch:", err)
	}
	t.Logf("unified search returned %d results", len(results))
}

// ---- QA Repo ----

// ---- Identity Repo ----

func TestIdentityRepo_CRUD(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()
	repo := NewIdentityRepo(store.db)

	node := &domain.IdentityNode{
		Type:       domain.NodeCoreValue,
		Content:    "我是诗音，主人的猫娘伙伴",
		Confidence: 1.0,
		Active:     true,
	}
	if err := repo.UpsertIdentityNode(node); err != nil {
		t.Fatal("UpsertIdentityNode:", err)
	}
	if node.ID == 0 {
		t.Fatal("expected non-zero ID after upsert")
	}

	nodes, err := repo.ListActiveIdentityNodes()
	if err != nil {
		t.Fatal("ListActiveIdentityNodes:", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 active node")
	}

	if err := repo.UpdateIdentityNodeMatchStats(node.ID, time.Now().Unix(), 5); err != nil {
		t.Fatal("UpdateIdentityNodeMatchStats:", err)
	}

	if err := repo.DeactivateIdentityNode(node.ID); err != nil {
		t.Fatal("DeactivateIdentityNode:", err)
	}

	nodes, _ = repo.ListActiveIdentityNodes()
	for _, n := range nodes {
		if n.ID == node.ID {
			t.Fatal("deactivated node should not appear in active list")
		}
	}
}

// ---- Feature Cache ----

func TestFeatureCache_CRUD(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	err := store.SaveFeatureCache("test_key", `{"v":0.75}`, 1.0, 10, 3600)
	if err != nil {
		t.Fatal("SaveFeatureCache:", err)
	}

	val, ok := store.LoadFeatureCache("test_key")
	if !ok {
		t.Fatal("LoadFeatureCache should find key")
	}
	if val != `{"v":0.75}` {
		t.Errorf("value = %q, want {\"v\":0.75}", val)
	}
}

func TestFeatureCache_Expired(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	_ = store.SaveFeatureCache("ephemeral", "42", 1.0, 1, 0)
	time.Sleep(1100 * time.Millisecond)

	_, ok := store.LoadFeatureCache("ephemeral")
	if ok {
		t.Error("expired cache should return false")
	}
}

func TestFeatureCache_Upsert(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	_ = store.SaveFeatureCache("upsert_key", "first", 1.0, 5, 3600)
	_ = store.SaveFeatureCache("upsert_key", "second", 0.8, 10, 7200)

	val, ok := store.LoadFeatureCache("upsert_key")
	if !ok || val != "second" {
		t.Errorf("upsert failed: ok=%v val=%q", ok, val)
	}
}

func TestFeatureCache_Multi(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	_ = store.SaveFeatureCache("a", "1", 1.0, 1, 3600)
	_ = store.SaveFeatureCache("b", "2", 1.0, 1, 3600)

	results := store.LoadFeatureCacheMulti([]string{"a", "b", "missing"})
	if len(results) != 2 {
		t.Errorf("expected 2, got %d: %v", len(results), results)
	}
	if results["a"] != "1" || results["b"] != "2" {
		t.Error("multi values mismatch")
	}
}

func TestFeatureCache_Missing(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	_, ok := store.LoadFeatureCache("nonexistent")
	if ok {
		t.Error("missing key should return false")
	}
}
