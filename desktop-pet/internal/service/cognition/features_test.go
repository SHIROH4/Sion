package cognition

import (
	"math"
	"os"
	"testing"
	"time"

	"desktop-pet/internal/domain"
	"desktop-pet/internal/infra/storage"
)

// ---- Test Helpers ----

func testEmotionVec() domain.EmotionVector {
	return domain.EmotionVector{
		Affection: 0.6, Worry: 0.3, Curiosity: 0.5, Sleepiness: 0.2,
		Playfulness: 0.4, Loneliness: 0.3, Confidence: 0.7, Annoyance: 0.1,
	}
}

func testEmotionState() domain.EmotionState {
	return domain.EmotionState{Primary: "neutral", Intensity: 0.4, Valence: 0.2}
}

func testFeatureComputer() *FeatureComputer {
	return NewFeatureComputer(nil, nil)
}

// fastCompute calls ComputeFast with sensible defaults for all 17 params.
func fastCompute(fc *FeatureComputer, now time.Time) *domain.QuantifiedFeatures {
	return fc.ComputeFast(
		now, testEmotionVec(), testEmotionState(),
		0, false, 0,
		0, 20, 0,
		0, 0,
		0, 0, 0,
		0.5, 0.5, 0.5,
	)
}

// ---- ComputeFast: Sanity ----

func TestComputeFast_NonNull(t *testing.T) {
	if f := fastCompute(testFeatureComputer(), time.Now()); f == nil {
		t.Fatal("expected non-nil")
	}
}

// ---- ComputeFast: User Factors ----

func TestComputeFast_IsWorking(t *testing.T) {
	fc := testFeatureComputer()
	f := fc.ComputeFast(
		time.Now(), testEmotionVec(), testEmotionState(),
		0, true, 90,
		5, 20, 1,
		0, 0, 0, 0, 0,
		0.5, 0.5, 0.5,
	)
	if f.U3_IsWorking != 1.0 {
		t.Errorf("U3 = %.2f, want 1.0", f.U3_IsWorking)
	}
	if f.U4_ContinuousWorkMins != 90 {
		t.Errorf("U4 mins = %.2f, want 90", f.U4_ContinuousWorkMins)
	}
	if f.U4_ContinuousWorkNorm <= 0 || f.U4_ContinuousWorkNorm > 1 {
		t.Errorf("U4 norm out of range: %.3f", f.U4_ContinuousWorkNorm)
	}
}

func TestComputeFast_TimeSinceChat(t *testing.T) {
	fc := testFeatureComputer()
	f := fc.ComputeFast(
		time.Now(), testEmotionVec(), testEmotionState(),
		25*time.Minute, false, 0,
		0, 20, 0, 0, 0, 0, 0, 0,
		0.5, 0.5, 0.5,
	)
	if f.U14_TimeSinceChatMins < 24 || f.U14_TimeSinceChatMins > 26 {
		t.Errorf("U14 = %.2f, want ~25", f.U14_TimeSinceChatMins)
	}
}

func TestComputeFast_MealTime(t *testing.T) {
	fc := testFeatureComputer()
	if f := fastCompute(fc, time.Date(2026, 6, 5, 12, 30, 0, 0, time.Local)); f.U11_MealTime != 0.5 {
		t.Error("12:30 should be meal time")
	}
	if f := fastCompute(fc, time.Date(2026, 6, 5, 15, 0, 0, 0, time.Local)); f.U11_MealTime != 0 {
		t.Error("15:00 should not be meal time")
	}
	if f := fastCompute(fc, time.Date(2026, 6, 5, 19, 0, 0, 0, time.Local)); f.U11_MealTime != 0.5 {
		t.Error("19:00 should be meal time")
	}
}

func TestComputeFast_NightTime(t *testing.T) {
	fc := testFeatureComputer()
	if f := fastCompute(fc, time.Date(2026, 6, 5, 1, 0, 0, 0, time.Local)); f.U12_NightTime != 0.6 {
		t.Error("1am should be night")
	}
	if f := fastCompute(fc, time.Date(2026, 6, 5, 10, 0, 0, 0, time.Local)); f.U12_NightTime != 0 {
		t.Error("10am should not be night")
	}
}

func TestComputeFast_Weekend(t *testing.T) {
	fc := testFeatureComputer()
	sat := time.Date(2026, 6, 6, 12, 0, 0, 0, time.Local) // Saturday
	if f := fastCompute(fc, sat); f.U13_IsWeekend != 1.0 {
		t.Errorf("Sat U13 = %.2f, want 1.0", f.U13_IsWeekend)
	}
	mon := time.Date(2026, 6, 8, 12, 0, 0, 0, time.Local) // Monday
	if f := fastCompute(fc, mon); f.U13_IsWeekend != 0.0 {
		t.Errorf("Mon U13 = %.2f, want 0.0", f.U13_IsWeekend)
	}
}

// ---- ComputeFast: Agent Factors ----

func TestComputeFast_AgentEmotion(t *testing.T) {
	fc := testFeatureComputer()
	vec := domain.EmotionVector{
		Affection: 0.7, Worry: 0.6, Curiosity: 0.8, Sleepiness: 0.3,
		Playfulness: 0.5, Loneliness: 0.4, Confidence: 0.9, Annoyance: 0.2,
	}
	state := domain.EmotionState{Primary: "joy", Intensity: 0.75, Valence: 0.6}
	f := fc.ComputeFast(
		time.Now(), vec, state,
		0, false, 0, 0, 20, 0, 0, 0, 0, 0, 0,
		0.5, 0.5, 0.5,
	)
	if f.A1_Affection != 0.7 || f.A1_Worry != 0.6 || f.A1_Curiosity != 0.8 {
		t.Error("A1 emotion vector mismatch")
	}
	if f.A2_PrimaryEmotion != "joy" {
		t.Errorf("A2 = %q", f.A2_PrimaryEmotion)
	}
	if f.A3_Intensity != 0.75 {
		t.Errorf("A3 = %.2f", f.A3_Intensity)
	}
}

func TestComputeFast_AgentPersonality(t *testing.T) {
	fc := testFeatureComputer()
	f := fc.ComputeFast(
		time.Now(), testEmotionVec(), testEmotionState(),
		0, false, 0, 0, 20, 0, 0, 0, 0, 0, 0,
		0.3, 0.7, 0.5,
	)
	if f.A5_AnnoySensitivity != 0.3 || f.A5_AffectWarmth != 0.7 || f.A5_WorryTendency != 0.5 {
		t.Error("A5 personality mismatch")
	}
}

func TestComputeFast_AgentCounts(t *testing.T) {
	fc := testFeatureComputer()
	f := fc.ComputeFast(
		time.Now(), testEmotionVec(), testEmotionState(),
		0, false, 0,
		5, 20, 2,
		3, 1,
		4, 2, 6,
		0.5, 0.5, 0.5,
	)
	if f.A6_DailyActionCount != 5 {
		t.Errorf("A6 = %.0f", f.A6_DailyActionCount)
	}
	if f.A11_ActiveInquiries != 3 {
		t.Errorf("A11 = %.0f", f.A11_ActiveInquiries)
	}
	if f.A12_KnowledgeGaps != 1 {
		t.Errorf("A12 = %.0f", f.A12_KnowledgeGaps)
	}
	if f.A14_ConsecutiveCount != 2 {
		t.Errorf("A14 = %.0f", f.A14_ConsecutiveCount)
	}
}

// ---- ComputeFast: Environment Factors ----

func TestComputeFast_Environment(t *testing.T) {
	fc := testFeatureComputer()
	now := time.Now()
	fc.NoteAction()
	fc.NoteDecision()
	fc.NoteReflection(now.Add(-20 * time.Hour))

	later := now.Add(10 * time.Minute)
	f := fc.ComputeFast(
		later, testEmotionVec(), testEmotionState(),
		0, false, 0,
		3, 20, 1,
		0, 0, 0, 0, 3,
		0.5, 0.5, 0.5,
	)
	if f.E1_Hour != float64(later.Hour()) {
		t.Errorf("E1 = %.0f, want %d", f.E1_Hour, later.Hour())
	}
	if f.E2_DayOfWeek != float64(later.Weekday()) {
		t.Errorf("E2 = %.0f, want %d", f.E2_DayOfWeek, later.Weekday())
	}
	cyc := f.E2_DOWSin*f.E2_DOWSin + f.E2_DOWCos*f.E2_DOWCos
	if cyc < 0.99 || cyc > 1.01 {
		t.Errorf("E2 cyclical = %.4f", cyc)
	}
	if f.E3_MinsSinceAction < 9 || f.E3_MinsSinceAction > 11 {
		t.Errorf("E3 mins = %.2f", f.E3_MinsSinceAction)
	}
	if f.E4_QuotaRemaining != 17 {
		t.Errorf("E4 = %.0f", f.E4_QuotaRemaining)
	}
	if f.E7_HoursSinceReflection < 19.5 || f.E7_HoursSinceReflection > 20.5 {
		t.Errorf("E7 hrs = %.2f", f.E7_HoursSinceReflection)
	}
}

// ---- ComputeFast: Task Context ----

func TestComputeFast_TaskContext(t *testing.T) {
	fc := testFeatureComputer()
	f := fc.ComputeFast(
		time.Now(), testEmotionVec(), testEmotionState(),
		0, false, 0, 0, 20, 0, 0, 0,
		8, 4, 12,
		0.5, 0.5, 0.5,
	)
	if f.T1_PrincipleCount != 8 || f.T2_PatternCount != 4 || f.T3_ReflexionLogCount != 12 {
		t.Error("task context mismatch")
	}
}

// ---- ComputeFast: All In Range ----

func TestComputeFast_AllInRange(t *testing.T) {
	fc := testFeatureComputer()
	base := time.Now()
	for i := 0; i < 8; i++ {
		tm := base.Add(time.Duration(i*3) * time.Hour)
		f := fc.ComputeFast(
			tm, testEmotionVec(), testEmotionState(),
			30*time.Minute, true, 60,
			5, 20, 2,
			3, 2, 5, 2, 4,
			0.5, 0.5, 0.5,
		)
		// Check factors that are always in [0,1] regardless of state.
		for name, v := range map[string]float64{
			"U3": f.U3_IsWorking, "U4n": f.U4_ContinuousWorkNorm,
			"U11": f.U11_MealTime, "U13": f.U13_IsWeekend,
			"A3": f.A3_Intensity,
		} {
			if v < 0 || v > 1 {
				t.Errorf("%s=%.3f out of [0,1]", name, v)
			}
		}
	}
}

// ---- Ring Buffer ----

func TestRingBuffer_PushAll(t *testing.T) {
	rb := newRingBuffer(4)
	now := time.Now()
	rb.Push(emotionSnapshot{valence: 0.1, at: now})
	rb.Push(emotionSnapshot{valence: 0.2, at: now.Add(time.Minute)})
	rb.Push(emotionSnapshot{valence: 0.3, at: now.Add(2 * time.Minute)})
	all := rb.All()
	if len(all) != 3 || all[0].valence != 0.1 || all[2].valence != 0.3 {
		t.Error("push/all order broken")
	}
}

func TestRingBuffer_Wrap(t *testing.T) {
	rb := newRingBuffer(3)
	now := time.Now()
	for i := 0; i < 5; i++ {
		rb.Push(emotionSnapshot{valence: float64(i), at: now.Add(time.Duration(i) * time.Minute)})
	}
	all := rb.All()
	if len(all) != 3 || all[0].valence != 2 || all[2].valence != 4 {
		t.Errorf("wrap broken: len=%d first=%.0f last=%.0f", len(all), all[0].valence, all[2].valence)
	}
}

func TestRingBuffer_Snapshot(t *testing.T) {
	rb := newRingBuffer(4)
	if rb.SnapshotNHoursAgo(1) != nil {
		t.Error("empty→nil")
	}
	now := time.Now()
	rb.Push(emotionSnapshot{valence: 0.2, at: now.Add(-61 * time.Minute)})
	rb.Push(emotionSnapshot{valence: 0.3, at: now.Add(-59 * time.Minute)})
	if snap := rb.SnapshotNHoursAgo(1); snap == nil {
		t.Error("should find ~1h ago")
	}
}

// ---- Helpers ----

func TestSaturateNorm(t *testing.T) {
	if saturateNorm(0, 180) > 0.01 {
		t.Error("0→~0")
	}
	if v := saturateNorm(180, 180); v < 0.95 || v > 0.97 {
		t.Errorf("180→[0.95,0.97], got %.4f", v)
	}
	prev := -1.0
	for x := 0.0; x <= 400; x += 20 {
		v := saturateNorm(x, 180)
		if v < prev {
			t.Errorf("not monotonic at x=%.0f", x)
		}
		prev = v
	}
}

func TestBoolToFloat(t *testing.T) {
	if boolToFloat(true) != 1.0 || boolToFloat(false) != 0.0 {
		t.Error("boolToFloat")
	}
}

func TestVecDistance(t *testing.T) {
	a := domain.EmotionVector{Affection: 0.5, Worry: 0.5, Curiosity: 0.5, Sleepiness: 0.5,
		Playfulness: 0.5, Loneliness: 0.5, Confidence: 0.5, Annoyance: 0.5}
	if vecDistance(a, a) != 0 {
		t.Error("self≠0")
	}
	d := vecDistance(a, domain.EmotionVector{})
	if math.Abs(d-math.Sqrt(2.0)) > 0.001 {
		t.Errorf("dist=%.4f want √2", d)
	}
}

func TestClamps(t *testing.T) {
	if clampNeg1_1(1.5) != 1 || clampNeg1_1(-1.5) != -1 {
		t.Error("clampNeg1_1")
	}
	if storage.Clamp01(1.5) != 1 || storage.Clamp01(-0.5) != 0 {
		t.Error("clamp01")
	}
}

func TestCosineSimilarity(t *testing.T) {
	if s := storage.CosineSimilarity([]float32{1, 0}, []float32{1, 0}); math.Abs(s-1) > 0.001 {
		t.Error("identical≠1")
	}
	if s := storage.CosineSimilarity([]float32{1, 0}, []float32{0, 1}); math.Abs(s-0) > 0.001 {
		t.Error("ortho≠0")
	}
	if storage.CosineSimilarity([]float32{1, 0}, []float32{1}) != 0 {
		t.Error("diff len≠0")
	}
}

func TestAverages(t *testing.T) {
	if average(nil) != 0 || average([]float64{1, 2, 3}) != 2 {
		t.Error("average")
	}
	if averageInt(nil) != 0 || averageInt([]int{2, 4, 6}) != 4 {
		t.Error("averageInt")
	}
}

func TestDecodeVectorBlob(t *testing.T) {
	if storage.DecodeVector(nil) != nil || storage.DecodeVector([]byte{1, 2, 3}) != nil {
		t.Error("bad input→nil")
	}
	bits := math.Float32bits(0.5)
	blob := []byte{byte(bits), byte(bits >> 8), byte(bits >> 16), byte(bits >> 24)}
	bits = math.Float32bits(-0.25)
	blob = append(blob, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24))
	dec := storage.DecodeVector(blob)
	if len(dec) != 2 || math.Abs(float64(dec[0]-0.5)) > 0.001 || math.Abs(float64(dec[1]+0.25)) > 0.001 {
		t.Errorf("round-trip: %v", dec)
	}
}

// ---- App & Window ----

func TestCategorizeApp(t *testing.T) {
	for _, tt := range []struct{ app, want string }{
		{"Code", "work"}, {"Visual Studio Code", "work"}, {"Terminal", "work"},
		{"Bilibili", "play"}, {"YouTube", "play"},
		{"WeChat", "social"}, {"Discord", "social"}, {"微信", "social"},
	} {
		if categorizeApp(tt.app) != tt.want {
			t.Errorf("%q→%q want %q", tt.app, categorizeApp(tt.app), tt.want)
		}
	}
	if categorizeApp("RandomApp") != "idle" || categorizeApp("") != "idle" {
		t.Error("unknown→idle")
	}
}

func TestMatchWindowSubtype(t *testing.T) {
	for _, tt := range []struct{ title, want string }{
		{"fix bug in auth.go", "debugging"},
		{"PR review: refactor", "code_review"},
		{"Zoom meeting - sprint", "meeting"},
		{"Inbox (32)", "email"},
		{"Notion - Docs", "writing"},
		{"Bilibili - 视频", "watching"},
		{"原神 - 任务", "gaming"},
		{"淘宝 - 购物车", "shopping"},
		{"main.go - VS Code", "coding"},
		{"/bin/zsh - Terminal", "terminal"},
	} {
		if matchWindowSubtype(tt.title) != tt.want {
			t.Errorf("%q→%q want %q", tt.title, matchWindowSubtype(tt.title), tt.want)
		}
	}
	if matchWindowSubtype("xyz") != "" || matchWindowSubtype("") != "" {
		t.Error("unknown→empty")
	}
}

// ---- Clustering ----

func TestClusterBySimilarity(t *testing.T) {
	if clusterBySimilarity(nil, 0.7) != nil {
		t.Error("nil→nil")
	}
	nodes := []prefNode{
		{content: "Go", vec: []float32{1, 0, 0}},
		{content: "Rust", vec: []float32{0.95, 0.05, 0}},
		{content: "咖啡", vec: []float32{0, 1, 0}},
		{content: "跑步", vec: []float32{0, 0, 1}},
	}
	c := clusterBySimilarity(nodes, 0.7)
	if len(c) != 3 {
		t.Errorf("expected 3 clusters, got %d", len(c))
	}
}

func TestClusterCentroid(t *testing.T) {
	nodes := []prefNode{{vec: []float32{1, 0}}, {vec: []float32{0, 1}}}
	c := clusterCentroid(nodes)
	if len(c) != 2 || math.Abs(float64(c[0]-0.5)) > 0.001 {
		t.Errorf("centroid=%v", c)
	}
}

func TestCopyVec(t *testing.T) {
	orig := []float32{1, 2, 3}
	cp := copyVec(orig)
	cp[0] = 99
	if orig[0] != 1 {
		t.Error("not deep copy")
	}
}

// ---- State Setters ----

func TestStateSetters(t *testing.T) {
	fc := testFeatureComputer()
	before := time.Now()

	fc.NoteAction()
	f := fc.ComputeFast(before.Add(5*time.Minute), testEmotionVec(), testEmotionState(),
		0, false, 0, 0, 20, 0, 0, 0, 0, 0, 0, 0.5, 0.5, 0.5)
	if f.E3_MinsSinceAction < 4 || f.E3_MinsSinceAction > 6 {
		t.Errorf("NoteAction: E3=%.2f", f.E3_MinsSinceAction)
	}

	fc.NoteDecision()
	f = fc.ComputeFast(before.Add(3*time.Minute), testEmotionVec(), testEmotionState(),
		0, false, 0, 0, 20, 0, 0, 0, 0, 0, 0, 0.5, 0.5, 0.5)
	if f.E5_MinsSinceDecision < 2.5 || f.E5_MinsSinceDecision > 3.5 {
		t.Errorf("NoteDecision: E5=%.2f", f.E5_MinsSinceDecision)
	}

	fc.NoteReflection(before.Add(-12 * time.Hour))
	f = fc.ComputeFast(before, testEmotionVec(), testEmotionState(),
		0, false, 0, 0, 20, 0, 0, 0, 0, 0, 0, 0.5, 0.5, 0.5)
	if f.E7_HoursSinceReflection < 11.5 || f.E7_HoursSinceReflection > 12.5 {
		t.Errorf("NoteReflection: E7=%.2f", f.E7_HoursSinceReflection)
	}

	fc.SetLLMAvailable(false)
	fc.SetVisionAvailable(false)
	f = fastCompute(fc, before)
	if f.E6_LLMAvailable || f.E6_VisionAvailable {
		t.Error("E6 should be false")
	}
}

// ---- Emotion History Persistence ----

func TestEmotionHistory_PersistAndLoad(t *testing.T) {
	tmp := t.TempDir() + "/emotion_history.json"

	fc1 := testFeatureComputer()
	fc1.SetEmotionHistoryPath(tmp)
	fc1.PushEmotionSnapshot(0.3, domain.EmotionVector{Affection: 0.5})
	fc1.PushEmotionSnapshot(0.5, domain.EmotionVector{Affection: 0.7})
	if _, err := os.Stat(tmp); os.IsNotExist(err) {
		t.Fatal("file not created")
	}

	fc2 := testFeatureComputer()
	fc2.SetEmotionHistoryPath(tmp)
	if fc2.emotionHistory.size != 2 {
		t.Fatalf("loaded %d, want 2", fc2.emotionHistory.size)
	}
}

func TestEmotionHistory_Nonexistent(t *testing.T) {
	fc := testFeatureComputer()
	fc.SetEmotionHistoryPath("/tmp/definitely_does_not_exist_emotion_hist.json")
	if fc.emotionHistory.size != 0 {
		t.Error("should be empty")
	}
}

// ---- A4 trend via PushEmotionSnapshot ----

func TestA4Trend(t *testing.T) {
	fc := testFeatureComputer()
	now := time.Now()
	fc.emotionHistory.Push(emotionSnapshot{
		valence: 0.0,
		vec:     domain.EmotionVector{Affection: 0.3, Worry: 0.3, Curiosity: 0.3, Sleepiness: 0.3, Playfulness: 0.3, Loneliness: 0.3, Confidence: 0.3, Annoyance: 0.3},
		at:      now.Add(-61 * time.Minute),
	})
	vec := domain.EmotionVector{Affection: 0.5, Worry: 0.5, Curiosity: 0.5, Sleepiness: 0.5, Playfulness: 0.5, Loneliness: 0.5, Confidence: 0.5, Annoyance: 0.5}
	f := fc.ComputeFast(now, vec, domain.EmotionState{Valence: 0.5},
		0, false, 0, 0, 20, 0, 0, 0, 0, 0, 0, 0.5, 0.5, 0.5)
	if math.Abs(f.A4_ValenceTrend-0.5) > 0.01 || f.A4_VecDelta <= 0 {
		t.Error("A4 trend not computed")
	}
}

// ---- ComputeFull nil DB ----

func TestComputeFull_NilDB(t *testing.T) {
	fc := testFeatureComputer()
	f := fc.ComputeFull(
		time.Now(), testEmotionVec(), testEmotionState(),
		10*time.Minute, true,
		3, 20, 1, 2, 1, 4, 2, 5,
		0.5, 0.4, 0.6,
	)
	if f == nil {
		t.Fatal("ComputeFull nil db: nil result")
	}
	if f.A1_Affection <= 0 {
		t.Errorf("A1_Affection=%.2f, want >0", f.A1_Affection)
	}
	if f.R1_OverallAcceptRate != 0.5 {
		t.Errorf("R1=%.3f, want 0.5", f.R1_OverallAcceptRate)
	}
}

func TestNewFeatureComputer_NilDB(t *testing.T) {
	if fc := NewFeatureComputer(nil, nil); fc == nil {
		t.Fatal("nil")
	}
}

// ---- LLM / Vectorizer nil handling ----

func TestLLM_Nil(t *testing.T) {
	fc := testFeatureComputer()
	fc.SetLLM(nil)
	if fc.learnAppCategory("X") != "" {
		t.Error("nil LLM→empty")
	}
	fc.SetLLM(func(string) (string, error) { return "work", nil })
	if fc.learnAppCategory("X") != "work" {
		t.Error("mock LLM→work")
	}
	fc.SetLLM(func(string) (string, error) { return "garbage", nil })
	if fc.learnAppCategory("X") != "" {
		t.Error("invalid→empty")
	}
}

func TestVectorizer_Nil(t *testing.T) {
	fc := testFeatureComputer()
	fc.SetVectorizer(nil)
	if _, ok := fc.searchFatigueByEmbedding(); ok {
		t.Error("nil vectorizer→false")
	}
}

// ---- TTL cache with nil DB ----

func TestCache_NilDB(t *testing.T) {
	fc := testFeatureComputer()
	called := false
	v := fc.getCachedFloat("x", 3600, func() (float64, int) { called = true; return 0.75, 10 })
	if !called || v != 0.75 {
		t.Error("getCachedFloat nil db")
	}
	called = false
	s := fc.getCachedString("x", 3600, func() string { called = true; return "hi" })
	if !called || s != "hi" {
		t.Error("getCachedString nil db")
	}
	fc.saveToCache("x", 0.5, 1.0, 5, 3600) // no panic
	if _, ok := fc.loadFromCache("x", 3600); ok {
		t.Error("loadFromCache nil db→false")
	}
}

// ---- Default app categories ----

func TestDefaultAppCategories(t *testing.T) {
	cats := defaultAppCategories()
	if len(cats) < 20 {
		t.Errorf("only %d categories", len(cats))
	}
	for app, cat := range cats {
		switch cat {
		case "work", "play", "social", "idle":
		default:
			t.Errorf("%q→%q invalid", app, cat)
		}
	}
}
