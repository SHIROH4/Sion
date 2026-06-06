package background

import (
	"testing"
	"time"

	"desktop-pet/internal/domain"
	infrastorage "desktop-pet/internal/infra/storage"
)

func TestReflectAndForget_MergeDuplicates(t *testing.T) {
	dir := t.TempDir()
	db, err := infrastorage.OpenDB(dir + "/memory.db")
	if err != nil {
		t.Fatal(err)
	}
	store := infrastorage.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now().Unix()
	db.Exec(`INSERT INTO facts (content, importance, fact_role, created_at) VALUES (?, ?, ?, ?)`,
		"白羽生日是10月14日", 0.9, "core", now)
	db.Exec(`INSERT INTO facts (content, importance, fact_role, created_at) VALUES (?, ?, ?, ?)`,
		"主人生日10月14", 0.5, "core", now)
	db.Exec(`INSERT INTO facts (content, importance, fact_role, created_at) VALUES (?, ?, ?, ?)`,
		"主人喜欢喝咖啡", 0.8, "core", now)
	for i := 0; i < 8; i++ {
		db.Exec(`INSERT INTO facts (content, importance, fact_role, created_at) VALUES (?, ?, ?, ?)`,
			"填充", 0.5, "core", now)
	}

	rawLLM := func(msgs []domain.Message) (string, error) {
		return `[{"action": "merge", "keep_id": 1, "duplicate_ids": [2]}]`, nil
	}
	lastReflectAt := time.Time{}
	ReflectAndForget(store, rawLLM, &lastReflectAt, nil)

	var archived int
	db.QueryRow(`SELECT COUNT(*) FROM facts WHERE archived = 1`).Scan(&archived)
	if archived != 1 {
		t.Errorf("expected 1 archived, got %d", archived)
	}
}

func TestReflectAndForget_CorrectContradiction(t *testing.T) {
	dir := t.TempDir()
	db, err := infrastorage.OpenDB(dir + "/memory.db")
	if err != nil {
		t.Fatal(err)
	}
	store := infrastorage.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now().Unix()
	db.Exec(`INSERT INTO facts (content, importance, fact_role, created_at) VALUES (?, ?, ?, ?)`,
		"主人常用Go", 0.8, "core", now-3600)
	db.Exec(`INSERT INTO facts (content, importance, fact_role, created_at) VALUES (?, ?, ?, ?)`,
		"主人改用Rust了", 0.7, "core", now)
	for i := 0; i < 9; i++ {
		db.Exec(`INSERT INTO facts (content, importance, fact_role, created_at) VALUES (?, ?, ?, ?)`,
			"填充", 0.5, "core", now)
	}

	rawLLM := func(msgs []domain.Message) (string, error) {
		return `[{"action": "correct", "old_id": 1, "new_id": 2}]`, nil
	}
	lastReflectAt := time.Time{}
	ReflectAndForget(store, rawLLM, &lastReflectAt, nil)

	var archived, replacedBy int
	db.QueryRow(`SELECT archived, replaced_by FROM facts WHERE id = 1`).Scan(&archived, &replacedBy)
	if archived != 1 {
		t.Error("old fact should be archived")
	}
	if replacedBy != 2 {
		t.Errorf("expected replaced_by=2, got %d", replacedBy)
	}
}

func TestReflectAndForget_StaleTemporal(t *testing.T) {
	dir := t.TempDir()
	db, err := infrastorage.OpenDB(dir + "/memory.db")
	if err != nil {
		t.Fatal(err)
	}
	store := infrastorage.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now().Unix()
	db.Exec(`INSERT INTO facts (content, importance, fact_role, created_at) VALUES (?, ?, ?, ?)`,
		"主人正在改bug", 0.4, "temporal", now-3*86400)
	for i := 0; i < 10; i++ {
		db.Exec(`INSERT INTO facts (content, importance, fact_role, created_at) VALUES (?, ?, ?, ?)`,
			"填充", 0.5, "core", now)
	}

	rawLLM := func(msgs []domain.Message) (string, error) {
		return `[{"action": "stale", "id": 1}]`, nil
	}
	lastReflectAt := time.Time{}
	ReflectAndForget(store, rawLLM, &lastReflectAt, nil)

	var archived int
	db.QueryRow(`SELECT archived FROM facts WHERE id = 1`).Scan(&archived)
	if archived != 1 {
		t.Error("stale temporal fact should be archived")
	}
}

func TestReflectAndForget_InsufficientFacts(t *testing.T) {
	dir := t.TempDir()
	db, err := infrastorage.OpenDB(dir + "/memory.db")
	if err != nil {
		t.Fatal(err)
	}
	store := infrastorage.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now().Unix()
	for i := 0; i < 5; i++ {
		db.Exec(`INSERT INTO facts (content, importance, fact_role, created_at) VALUES (?, ?, ?, ?)`,
			"测试", 0.5, "core", now)
	}

	llmCalled := false
	rawLLM := func(msgs []domain.Message) (string, error) {
		llmCalled = true
		return "[]", nil
	}
	lastReflectAt := time.Now().Add(-1 * time.Hour)
	ReflectAndForget(store, rawLLM, &lastReflectAt, nil)
	if llmCalled {
		t.Error("should not call LLM when < 10 new facts")
	}
}

func TestReflectAndForget_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	db, err := infrastorage.OpenDB(dir + "/memory.db")
	if err != nil {
		t.Fatal(err)
	}
	store := infrastorage.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now().Unix()
	for i := 0; i < 15; i++ {
		db.Exec(`INSERT INTO facts (content, importance, fact_role, created_at) VALUES (?, ?, ?, ?)`,
			"测试", 0.5, "core", now)
	}

	rawLLM := func(msgs []domain.Message) (string, error) {
		return "not json at all", nil
	}
	lastReflectAt := time.Time{}
	// Should not panic.
	ReflectAndForget(store, rawLLM, &lastReflectAt, nil)
}
