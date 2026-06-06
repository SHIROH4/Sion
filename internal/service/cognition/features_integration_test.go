package cognition

import (
	"database/sql"
	"math"
	"testing"
	"time"

	"desktop-pet/internal/domain"
	"desktop-pet/internal/infra/storage"
)

// TestComputeFull_Integration_AllDataSources verifies every Tier 2 factor's
// data source produces correct values from a fully-seeded SQLite database.
func TestComputeFull_Integration_AllDataSources(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.OpenDB(dir + "/memory.db")
	if err != nil {
		t.Fatal("OpenDB:", err)
	}
	defer db.Close()

	// Run schema migration.
	store := storage.NewStore(db)
	defer store.Close()

	now := time.Now()
	ts := func(offsetSec int) int64 { return now.Add(time.Duration(offsetSec) * time.Second).Unix() }
	_ = ts // used in seed helpers

	// ---- Seed all tables ----
	seedAS(t, db, now)
	seedAO(t, db, now)
	seedCH(t, db, now)
	seedFacts(t, db, now)
	seedIdent(t, db, now)
	seedThreads(t, db, now)

	// Build FeatureComputer with real DB.
	outcomeRepo := storage.NewActionOutcomeRepo(db)
	fc := NewFeatureComputer(db, outcomeRepo)
	fc.PushEmotionSnapshot(0.0, domain.EmotionVector{
		Affection: 0.3, Worry: 0.3, Curiosity: 0.3, Sleepiness: 0.3,
		Playfulness: 0.3, Loneliness: 0.3, Confidence: 0.3, Annoyance: 0.3,
	})

	f := fc.ComputeFull(
		now,
		domain.EmotionVector{Affection: 0.6, Worry: 0.4, Curiosity: 0.5, Sleepiness: 0.2, Playfulness: 0.5, Loneliness: 0.3, Confidence: 0.7, Annoyance: 0.1},
		domain.EmotionState{Primary: "neutral", Intensity: 0.5, Valence: 0.3},
		10*time.Minute, true,
		3, 20, 1, 2, 1, 4, 2, 5,
		0.5, 0.6, 0.4,
	)

	// ============ User Factors ============
	if f.U1_AppCategory != "work" {
		t.Errorf("U1: want work, got %q", f.U1_AppCategory)
	}
	// Latest window title is "fix bug in auth.go" → matches "debugging".
	if f.U2_WindowSubtype != "debugging" {
		t.Errorf("U2: want debugging, got %q", f.U2_WindowSubtype)
	}
	if f.U3_IsWorking != 1.0 {
		t.Errorf("U3: want 1.0, got %.2f", f.U3_IsWorking)
	}
	if f.U4_ContinuousWorkMins < 90 {
		t.Errorf("U4: want >=90min, got %.1f", f.U4_ContinuousWorkMins)
	}
	if f.U5_AppSwitchCount < 2 {
		t.Errorf("U5: want >=2 switches, got %.0f", f.U5_AppSwitchCount)
	}
	if f.U7_LengthTrend >= -0.5 {
		t.Errorf("U7: want <-0.5 (short recent vs long baseline), got %.3f", f.U7_LengthTrend)
	}
	if f.U8_ResponseDelayEMA <= 0 {
		t.Errorf("U8 delay: want >0, got %.1f", f.U8_ResponseDelayEMA)
	}
	if f.U10_TimeWindowPref < 0 || f.U10_TimeWindowPref > 1 {
		t.Errorf("U10: out of range: %.3f", f.U10_TimeWindowPref)
	}
	if f.U15_FatigueMentionHrs < 0.4 || f.U15_FatigueMentionHrs > 1.5 {
		t.Errorf("U15: want ~1h (3600s ago), got %.2fh", f.U15_FatigueMentionHrs)
	}
	if f.U16_PrefDiversity <= 0 {
		t.Errorf("U16: want >0, got %.3f", f.U16_PrefDiversity)
	}

	// ============ Agent Factors ============
	if len(f.A7_ActionSuccessRate) == 0 {
		t.Error("A7: empty map")
	}
	if len(f.A8_TimeBlockRate) == 0 {
		t.Error("A8: empty map")
	}
	if f.A10_ActiveGoals < 2 {
		t.Errorf("A10: want >=2, got %.0f", f.A10_ActiveGoals)
	}
	if f.A13_NewFacts24h < 2 {
		t.Errorf("A13: want >=2, got %.0f", f.A13_NewFacts24h)
	}

	// ============ Relationship Factors ============
	if f.R1_OverallAcceptRate < 0 || f.R1_OverallAcceptRate > 1 {
		t.Errorf("R1: out of range: %.3f", f.R1_OverallAcceptRate)
	}
	if f.R1_SampleCount < 5 {
		t.Errorf("R1 sample: want >=5, got %.0f", f.R1_SampleCount)
	}
	if len(f.R3_SourceAcceptRate) == 0 {
		t.Error("R3: empty map")
	}
	if f.R4_RecentRejections < 0 || f.R4_RecentRejections > 5 {
		t.Errorf("R4: invalid: %.0f", f.R4_RecentRejections)
	}
	if f.R5_NeglectHours <= 0 {
		t.Errorf("R5: want >0, got %.2f", f.R5_NeglectHours)
	}
	if f.R6_DepthTrend < -1 || f.R6_DepthTrend > 1 {
		t.Errorf("R6: out of [-1,1]: %.3f", f.R6_DepthTrend)
	}
	if f.R7_UserInitiative24h < 2 {
		t.Errorf("R7: want >=2, got %.0f", f.R7_UserInitiative24h)
	}
	if f.R8_IntimacyTrend < -1 || f.R8_IntimacyTrend > 1 {
		t.Errorf("R8: out of [-1,1]: %.3f", f.R8_IntimacyTrend)
	}

	// ============ Task Context ============
	if f.T5_TodayActivityCount < 3 {
		t.Errorf("T5: want >=3, got %.0f", f.T5_TodayActivityCount)
	}

	// ============ Tier 1 sanity ============
	if f.A1_Affection <= 0 || f.A6_DailyActionCount != 3 || f.E4_QuotaRemaining != 17 {
		t.Error("Tier 1 values wrong")
	}

	t.Logf("OK: U1=%q U2=%q U4=%.0fmin U5=%.0f U7=%.3f U8=%.1fs U10=%.2f U15=%.1fh U16=%.3f",
		f.U1_AppCategory, f.U2_WindowSubtype, f.U4_ContinuousWorkMins, f.U5_AppSwitchCount,
		f.U7_LengthTrend, f.U8_ResponseDelayEMA, f.U10_TimeWindowPref, f.U15_FatigueMentionHrs, f.U16_PrefDiversity)
	t.Logf("OK: A7=%v A10=%.0f A13=%.0f R1=%.2f(n=%.0f) R4=%.0f R5=%.1fh R6=%.3f R7=%.0f T5=%.0f",
		f.A7_ActionSuccessRate, f.A10_ActiveGoals, f.A13_NewFacts24h,
		f.R1_OverallAcceptRate, f.R1_SampleCount, f.R4_RecentRejections,
		f.R5_NeglectHours, f.R6_DepthTrend, f.R7_UserInitiative24h, f.T5_TodayActivityCount)
}

// ---- Seed Helpers ----

func seedAS(t *testing.T, db *sql.DB, now time.Time) {
	ts := func(s int) int64 { return now.Add(time.Duration(s) * time.Second).Unix() }
	exec := func(q string, args ...interface{}) { _, _ = db.Exec(q, args...) }

	// 4 apps: 3 within last 30min → 2 switches for U5.
	// Total working time ~107min for U4.
	exec(`INSERT INTO activity_sessions (app_name, window_title, is_working, start_time, end_time) VALUES (?,?,?,?,?)`,
		"Code", "main.go - VS Code", 1, ts(-8000), ts(-3000))
	exec(`INSERT INTO activity_sessions (app_name, window_title, is_working, start_time, end_time) VALUES (?,?,?,?,?)`,
		"Terminal", "/bin/zsh - Terminal", 1, ts(-1500), ts(-1001))
	exec(`INSERT INTO activity_sessions (app_name, window_title, is_working, start_time, end_time) VALUES (?,?,?,?,?)`,
		"Slack", "Slack - #general", 1, ts(-1000), ts(-501))
	exec(`INSERT INTO activity_sessions (app_name, window_title, is_working, start_time, end_time) VALUES (?,?,?,?,?)`,
		"Code", "fix bug in auth.go", 1, ts(-500), ts(-100))
	// Older non-working break for U4 boundary.
	exec(`INSERT INTO activity_sessions (app_name, window_title, is_working, start_time, end_time) VALUES (?,?,?,?,?)`,
		"Bilibili", "Bilibili - 猫娘视频", 0, ts(-12000), ts(-8100))
}

func seedAO(t *testing.T, db *sql.DB, now time.Time) {
	exec := func(q string, args ...interface{}) { _, _ = db.Exec(q, args...) }
	hr := now.Hour()
	dow := int(now.Weekday())
	base := now.Unix()

	ins := func(src, typ string, outcome, delay, h int) {
		exec(`INSERT INTO action_outcomes (action_source, action_type, hour_of_day, day_of_week, app_context, emotion_bucket, escalation_lvl, outcome, response_delay, created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			src, typ, h, dow, "Code", "neutral", 1, outcome, delay, base-int64((now.Hour()-h)*3600))
	}
	ins("care", "rest", 1, 30, hr)
	ins("care", "rest", -1, 0, hr)
	ins("care", "meal", 1, 45, hr)
	ins("care", "meal", 2, 20, hr)
	ins("casual", "social", 0, 0, hr)
	ins("casual", "social", 1, 60, hr)
	ins("knowledge_gap", "encourage", 1, 15, hr)
	ins("care", "rest", 1, 10, (hr-1+24)%24)
	ins("care", "hydration", -1, 0, hr)
	ins("casual", "social", 1, 90, hr)
}

func seedCH(t *testing.T, db *sql.DB, now time.Time) {
	exec := func(q string, args ...interface{}) { _, _ = db.Exec(q, args...) }
	ts := func(s int) int64 { return now.Add(time.Duration(s) * time.Second).Unix() }

	// 20 long user messages + 5 short = strong negative length trend.
	for i := 0; i < 20; i++ {
		exec(`INSERT INTO chat_history (role, content, created_at) VALUES (?,?,?)`,
			"user", "这是一个很长的用户消息用来模拟正常对话深度这是第条消息包含较多文本内容用于测试", ts(-3600-i*60))
		exec(`INSERT INTO chat_history (role, content, created_at) VALUES (?,?,?)`,
			"assistant", "喵~知道了主人", ts(-3600-i*60-5))
	}
	for i := 0; i < 5; i++ {
		exec(`INSERT INTO chat_history (role, content, created_at) VALUES (?,?,?)`,
			"user", "短", ts(-300-i*30))
	}
}

func seedFacts(t *testing.T, db *sql.DB, now time.Time) {
	exec := func(q string, args ...interface{}) { _, _ = db.Exec(q, args...) }
	ts := func(s int) int64 { return now.Add(time.Duration(s) * time.Second).Unix() }

	exec(`INSERT INTO facts (content, importance, created_at) VALUES (?,?,?)`,
		"主人说好累想休息一下", 0.6, ts(-3600)) // U15: ~1h ago
	exec(`INSERT INTO facts (content, importance, created_at) VALUES (?,?,?)`,
		"主人使用Go语言开发后端", 0.7, ts(-3600)) // within 24h
	exec(`INSERT INTO facts (content, importance, created_at) VALUES (?,?,?)`,
		"主人喜欢深色主题编辑器", 0.5, ts(-7200)) // within 24h
	exec(`INSERT INTO facts (content, importance, created_at) VALUES (?,?,?)`,
		"主人去年去过日本旅行", 0.4, ts(-100000)) // outside 24h
}

func seedIdent(t *testing.T, db *sql.DB, now time.Time) {
	exec := func(q string, args ...interface{}) { _, _ = db.Exec(q, args...) }
	ts := func(s int) int64 { return now.Add(time.Duration(s) * time.Second).Unix() }

	// 3 preference nodes: 2 in one cluster (Go+Rust), 1 in another (coffee).
	exec(`INSERT INTO identity_nodes (node_type, content, confidence, embedding, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		"preference", "喜欢Go语言", 0.9, encodeTestVec(1, 0, 0), 1, ts(-3600), ts(-3600))
	exec(`INSERT INTO identity_nodes (node_type, content, confidence, embedding, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		"preference", "喜欢Rust语言", 0.8, encodeTestVec(0.95, 0.05, 0), 1, ts(-3600), ts(-3600))
	exec(`INSERT INTO identity_nodes (node_type, content, confidence, embedding, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		"preference", "喜欢喝咖啡", 0.7, encodeTestVec(0, 1, 0), 1, ts(-3600), ts(-3600))
}

func seedThreads(t *testing.T, db *sql.DB, now time.Time) {
	exec := func(q string, args ...interface{}) { _, _ = db.Exec(q, args...) }
	ts := func(s int) int64 { return now.Add(time.Duration(s) * time.Second).Unix() }

	exec(`INSERT INTO conversation_threads (type, goal, status, priority, created_at, last_touched_at) VALUES (?,?,?,?,?,?)`,
		"follow_up", "跟进Rust学习进展", "active", 0.8, ts(-86400), ts(-3600))
	exec(`INSERT INTO conversation_threads (type, goal, status, priority, created_at, last_touched_at) VALUES (?,?,?,?,?,?)`,
		"care", "关注主人作息规律", "active", 0.6, ts(-86400), ts(-7200))
	exec(`INSERT INTO conversation_threads (type, goal, status, priority, created_at, last_touched_at) VALUES (?,?,?,?,?,?)`,
		"exploration", "已完成的探索线程", "resolved", 0.5, ts(-86400), ts(-172800))
}

func encodeTestVec(vals ...float32) []byte {
	out := make([]byte, len(vals)*4)
	for i, f := range vals {
		bits := math.Float32bits(f)
		out[i*4] = byte(bits)
		out[i*4+1] = byte(bits >> 8)
		out[i*4+2] = byte(bits >> 16)
		out[i*4+3] = byte(bits >> 24)
	}
	return out
}
