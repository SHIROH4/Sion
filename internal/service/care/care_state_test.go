package care

import (
	"desktop-pet/internal/domain"
	"testing"
	"time"
)

func TestUserCareState_New(t *testing.T) {
	s := domain.NewUserCareState()
	if s == nil {
		t.Fatal("domain.NewUserCareState returned nil")
	}
	if s.MoodTrend != "stable" {
		t.Errorf("expected MoodTrend=stable, got %s", s.MoodTrend)
	}
	if s.TaskComplexity != "medium" {
		t.Errorf("expected TaskComplexity=medium, got %s", s.TaskComplexity)
	}
	if s.StressLevel != 0 {
		t.Errorf("expected StressLevel=0, got %.2f", s.StressLevel)
	}
	if s.FocusLevel != 0 {
		t.Errorf("expected FocusLevel=0, got %.2f", s.FocusLevel)
	}
	if s.SocialActivity != 0 {
		t.Errorf("expected SocialActivity=0, got %.2f", s.SocialActivity)
	}
	if s.BurnoutRisk != 0 {
		t.Errorf("expected BurnoutRisk=0, got %.2f", s.BurnoutRisk)
	}
	if !s.LastMealAt.IsZero() {
		t.Error("expected LastMealAt to be zero")
	}
	if !s.LastDrinkAt.IsZero() {
		t.Error("expected LastDrinkAt to be zero")
	}
}

func TestUserCareState_UpdateFromScreenObs(t *testing.T) {
	s := domain.NewUserCareState()

	obs := NewObservation(domain.ObsScreen, "VSCode活跃120分钟 正在编辑代码")
	s.UpdateFromObservation(obs)

	snap := s.Snapshot()
	if snap.ContinuousWork != 120 {
		t.Errorf("expected ContinuousWork=120, got %d", snap.ContinuousWork)
	}
	if snap.FocusLevel <= 0 {
		t.Errorf("expected FocusLevel > 0 after coding observation, got %.3f", snap.FocusLevel)
	}

	// Second observation: browser → medium focus, EMA-smoothed with previous.
	obs2 := NewObservation(domain.ObsScreen, "浏览器活跃30分钟 查看技术文档")
	s.UpdateFromObservation(obs2)

	snap2 := s.Snapshot()
	if snap2.FocusLevel <= 0 {
		t.Errorf("expected FocusLevel > 0 after browser observation, got %.3f", snap2.FocusLevel)
	}
}

func TestUserCareState_UpdateFromQQObs(t *testing.T) {
	s := domain.NewUserCareState()

	// Multiple QQ observations should raise SocialActivity via EMA.
	obs := NewObservation(domain.ObsQQ, "群聊: 今天讨论Rust编译器")
	s.UpdateFromObservation(obs)

	snap := s.Snapshot()
	if snap.SocialActivity <= 0 {
		t.Errorf("expected SocialActivity > 0 after QQ obs, got %.3f", snap.SocialActivity)
	}
	if snap.IsolationHours != 0 {
		t.Errorf("expected IsolationHours=0 after QQ activity, got %.1f", snap.IsolationHours)
	}

	// Second QQ obs → EMA should further increase SocialActivity.
	obs2 := NewObservation(domain.ObsQQ, "私聊: 周末一起吃饭吗")
	s.UpdateFromObservation(obs2)

	snap2 := s.Snapshot()
	if snap2.SocialActivity <= snap.SocialActivity {
		t.Errorf("expected SocialActivity to increase via EMA after second QQ obs, got %.3f -> %.3f",
			snap.SocialActivity, snap2.SocialActivity)
	}
}

func TestUserCareState_UpdateFromChatObs_Meal(t *testing.T) {
	s := domain.NewUserCareState()
	now := time.Now()

	obs := NewObservation(domain.ObsChat, "刚吃了饭，今天的午餐还不错")
	obs.Timestamp = now
	s.UpdateFromObservation(obs)

	snap := s.Snapshot()
	if snap.LastMealAt.IsZero() {
		t.Error("expected LastMealAt to be set after '刚吃了饭'")
	}
	if !snap.LastMealAt.Equal(now) {
		t.Errorf("expected LastMealAt=%v, got %v", now, snap.LastMealAt)
	}
}

func TestUserCareState_UpdateFromChatObs_Drink(t *testing.T) {
	s := domain.NewUserCareState()
	now := time.Now()

	obs := NewObservation(domain.ObsChat, "去喝杯水，有点渴了")
	obs.Timestamp = now
	s.UpdateFromObservation(obs)

	snap := s.Snapshot()
	if snap.LastDrinkAt.IsZero() {
		t.Error("expected LastDrinkAt to be set after '去喝杯水'")
	}
	if !snap.LastDrinkAt.Equal(now) {
		t.Errorf("expected LastDrinkAt=%v, got %v", now, snap.LastDrinkAt)
	}
}

func TestUserCareState_BurnoutRisk(t *testing.T) {
	s := domain.NewUserCareState()

	// Apply multiple screen observations to build up FocusLevel via EMA.
	// EMA: new = 0.3*0.7 + 0.7*old. Starting from 0, after ~8 obs FocusLevel > 0.65.
	// The last observation also sets ContinuousWork=240.
	for i := 0; i < 7; i++ {
		s.UpdateFromObservation(NewObservation(domain.ObsScreen, "VSCode活跃60分钟 正在编辑代码"))
	}
	// Final observation: 4h continuous work + high focus → BurnoutRisk fires.
	s.UpdateFromObservation(NewObservation(domain.ObsScreen, "VSCode活跃240分钟 终端运行测试"))

	snap := s.Snapshot()
	if snap.ContinuousWork != 240 {
		t.Errorf("expected ContinuousWork=240, got %d", snap.ContinuousWork)
	}
	if snap.FocusLevel <= 0.6 {
		t.Errorf("expected FocusLevel > 0.6 after repeated coding obs, got %.3f", snap.FocusLevel)
	}
	if snap.BurnoutRisk <= 0 {
		t.Errorf("expected BurnoutRisk > 0 after 4h work + high focus, got %.3f", snap.BurnoutRisk)
	}

	// Short work duration should NOT trigger burnout risk.
	s2 := domain.NewUserCareState()
	s2.UpdateFromObservation(NewObservation(domain.ObsScreen, "VSCode活跃30分钟 正在编辑代码"))

	snap2 := s2.Snapshot()
	if snap2.BurnoutRisk != 0 {
		t.Errorf("expected BurnoutRisk=0 for short work duration, got %.3f", snap2.BurnoutRisk)
	}
}

func TestUserCareState_UpdateFromLLM(t *testing.T) {
	s := domain.NewUserCareState()

	// Set initial values that should be overwritten.
	s.Mu.Lock()
	s.StressLevel = 0.2
	s.FocusLevel = 0.3
	s.MoodTrend = "stable"
	s.Mu.Unlock()

	result := &domain.LLMCareStateResult{
		StressLevel:    0.6,
		MoodTrend:      "falling",
		BurnoutRisk:    0.4,
		FocusLevel:     0.7,
		TaskComplexity: "high",
		SocialActivity: 0.5,
	}
	s.UpdateFromLLM(result)

	snap := s.Snapshot()
	// Non-zero fields should be overwritten.
	if snap.StressLevel != 0.6 {
		t.Errorf("expected StressLevel=0.6, got %.2f", snap.StressLevel)
	}
	if snap.MoodTrend != "falling" {
		t.Errorf("expected MoodTrend=falling, got %s", snap.MoodTrend)
	}
	if snap.BurnoutRisk != 0.4 {
		t.Errorf("expected BurnoutRisk=0.4, got %.2f", snap.BurnoutRisk)
	}
	if snap.FocusLevel != 0.7 {
		t.Errorf("expected FocusLevel=0.7, got %.2f", snap.FocusLevel)
	}
	if snap.TaskComplexity != "high" {
		t.Errorf("expected TaskComplexity=high, got %s", snap.TaskComplexity)
	}
	if snap.SocialActivity != 0.5 {
		t.Errorf("expected SocialActivity=0.5, got %.2f", snap.SocialActivity)
	}

	// Now send a result with only one field set — zeros/empties must NOT overwrite.
	result2 := &domain.LLMCareStateResult{
		StressLevel: 0.9,
		// All other fields are zero/empty.
	}
	s.UpdateFromLLM(result2)

	snap2 := s.Snapshot()
	if snap2.StressLevel != 0.9 {
		t.Errorf("expected StressLevel=0.9 (overwritten), got %.2f", snap2.StressLevel)
	}
	// These should retain the previous values, not be zeroed out.
	if snap2.FocusLevel != 0.7 {
		t.Errorf("expected FocusLevel=0.7 (not overwritten by zero), got %.2f", snap2.FocusLevel)
	}
	if snap2.MoodTrend != "falling" {
		t.Errorf("expected MoodTrend=falling (not overwritten by empty), got %s", snap2.MoodTrend)
	}
	if snap2.BurnoutRisk != 0.4 {
		t.Errorf("expected BurnoutRisk=0.4 (not overwritten by zero), got %.2f", snap2.BurnoutRisk)
	}
}

func TestUserCareState_Snapshot(t *testing.T) {
	s := domain.NewUserCareState()
	now := time.Now()

	s.Mu.Lock()
	s.LastMealAt = now
	s.LastDrinkAt = now.Add(-30 * time.Minute)
	s.ContinuousWork = 120
	s.PostureWarning = true
	s.StressLevel = 0.4
	s.MoodTrend = "rising"
	s.BurnoutRisk = 0.2
	s.SocialActivity = 0.6
	s.IsolationHours = 2.5
	s.FocusLevel = 0.6
	s.TaskComplexity = "high"
	s.DeadlinesNear = true
	s.LastUpdated = now
	s.Mu.Unlock()

	snap := s.Snapshot()

	// Modify snapshot — original must be unaffected.
	snap.StressLevel = 0.9
	snap.FocusLevel = 0.1
	snap.MoodTrend = "falling"

	orig := s.Snapshot()
	if orig.StressLevel != 0.4 {
		t.Errorf("original StressLevel should be 0.4, got %.2f", orig.StressLevel)
	}
	if orig.FocusLevel != 0.6 {
		t.Errorf("original FocusLevel should be 0.6, got %.2f", orig.FocusLevel)
	}
	if orig.MoodTrend != "rising" {
		t.Errorf("original MoodTrend should be rising, got %s", orig.MoodTrend)
	}
	if !orig.LastMealAt.Equal(now) {
		t.Error("original LastMealAt should be unchanged")
	}
	if orig.ContinuousWork != 120 {
		t.Errorf("original ContinuousWork should be 120, got %d", orig.ContinuousWork)
	}
	if !orig.PostureWarning {
		t.Error("original PostureWarning should be true")
	}
	if orig.BurnoutRisk != 0.2 {
		t.Errorf("original BurnoutRisk should be 0.2, got %.2f", orig.BurnoutRisk)
	}
	if orig.SocialActivity != 0.6 {
		t.Errorf("original SocialActivity should be 0.6, got %.2f", orig.SocialActivity)
	}
	if orig.IsolationHours != 2.5 {
		t.Errorf("original IsolationHours should be 2.5, got %.1f", orig.IsolationHours)
	}
	if orig.TaskComplexity != "high" {
		t.Errorf("original TaskComplexity should be high, got %s", orig.TaskComplexity)
	}
	if !orig.DeadlinesNear {
		t.Error("original DeadlinesNear should be true")
	}
}
