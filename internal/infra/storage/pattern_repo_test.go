package storage

import (
	"testing"
	"time"

	"desktop-pet/internal/domain"
)

func TestActivitySessionRepo_RecordAndList(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()
	repo := NewActivityEventRepo(store.db)

	now := time.Now().Unix()
	s1 := domain.ActivityEvent{
		AppName: "Code", WindowTitle: "main.go", IsWorking: true,
		StartTime: now, EndTime: now + 600,
	}
	s2 := domain.ActivityEvent{
		AppName: "WeChat", WindowTitle: "微信", IsWorking: false,
		StartTime: now + 601, EndTime: now + 900,
	}

	id1, err := repo.RecordSession(s1)
	if err != nil {
		t.Fatal("RecordSession s1:", err)
	}
	id2, err := repo.RecordSession(s2)
	if err != nil {
		t.Fatal("RecordSession s2:", err)
	}
	if id1 == id2 {
		t.Error("expected different IDs")
	}

	if err := repo.UpdateSessionEnd(id1, now+1200); err != nil {
		t.Fatal("UpdateSessionEnd:", err)
	}

	today, err := repo.ListToday()
	if err != nil {
		t.Fatal("ListToday:", err)
	}
	if len(today) < 2 {
		t.Fatalf("expected >=2 sessions, got %d", len(today))
	}
	found := false
	for _, s := range today {
		if s.ID == id1 && s.EndTime == now+1200 {
			found = true
		}
	}
	if !found {
		t.Error("UpdateSessionEnd not reflected in ListToday")
	}

	results, err := repo.ListRange(now, now+2000)
	if err != nil {
		t.Fatal("ListRange:", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2, got %d", len(results))
	}
}

func TestActivitySessionRepo_CleanOld(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()
	repo := NewActivityEventRepo(store.db)

	old := time.Now().AddDate(0, 0, -10).Unix()
	s := domain.ActivityEvent{
		AppName: "OldApp", StartTime: old, EndTime: old + 60,
	}
	repo.RecordSession(s)

	n := repo.CleanOld(7)
	if n != 1 {
		t.Errorf("expected 1 cleaned, got %d", n)
	}
}

func TestPatternRepo_SaveAndList(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()
	repo := NewPatternRepo(store.db)

	p := domain.BehaviorPattern{
		Pattern: "工作日15:00左右出现注意力分散", Type: "daily_rhythm",
		Evidence: "周一15:05切到Bilibili, 周二15:12切到微博",
		Confidence: 0.75, Implication: "在14:55送鼓励", Active: true,
	}
	id, err := repo.SavePattern(p)
	if err != nil {
		t.Fatal("SavePattern:", err)
	}
	if id <= 0 {
		t.Error("expected positive ID")
	}

	active, err := repo.ListActive()
	if err != nil {
		t.Fatal("ListActive:", err)
	}
	if len(active) < 1 {
		t.Fatal("expected >=1 active patterns")
	}

	byType, err := repo.ListByType("daily_rhythm")
	if err != nil {
		t.Fatal("ListByType:", err)
	}
	if len(byType) < 1 {
		t.Fatal("expected >=1 daily_rhythm patterns")
	}

	if err := repo.UpdateConfidence(id, 0.1); err != nil {
		t.Fatal("UpdateConfidence:", err)
	}

	if err := repo.Deactivate(id); err != nil {
		t.Fatal("Deactivate:", err)
	}

	remaining, _ := repo.ListActive()
	if len(remaining) != 0 {
		t.Error("expected 0 active after deactivate")
	}
}
