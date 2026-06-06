package chat

import (
	"strings"
	"testing"

	"desktop-pet/internal/domain"
	infrastorage "desktop-pet/internal/infra/storage"
)

func TestExtractMemorize(t *testing.T) {
	var memorized []string
	ctx := &domain.ChatContext{
		Messages: []domain.Message{
			{Role: "user", Content: "你好"},
			{Role: "assistant", Content: "好的。[MEMORIZE]主人叫小明，用Go开发[/MEMORIZE] 还需要什么？"},
		},
	}
	ExtractMemorize(ctx, func(c string) { memorized = append(memorized, c) })

	if len(memorized) != 1 {
		t.Fatalf("expected 1 memorized fact, got %d", len(memorized))
	}
	if memorized[0] != "主人叫小明，用Go开发" {
		t.Errorf("expected '主人叫小明，用Go开发', got %q", memorized[0])
	}
	if strings.Contains(ctx.Messages[1].Content, "[MEMORIZE]") {
		t.Error("MEMORIZE markers should be stripped from content")
	}
}

func TestExtractRecall(t *testing.T) {
	recallFn := func(index string) (string, error) {
		return "回忆内容：" + index, nil
	}
	ctx := &domain.ChatContext{
		Messages: []domain.Message{
			{Role: "assistant", Content: "让我查一下。[RECALL abc-123]"},
		},
	}
	ExtractRecall(ctx, recallFn)

	found := false
	for _, m := range ctx.Messages {
		if strings.Contains(m.Content, "存档回溯") && strings.Contains(m.Content, "abc-123") {
			found = true
		}
	}
	if !found {
		t.Error("expected a recall system message to be injected")
	}
	if strings.Contains(ctx.Messages[0].Content, "[RECALL") {
		t.Error("RECALL markers should be stripped")
	}
}

func TestExtractConfirmations(t *testing.T) {
	var memorized []string
	ctx := &domain.ChatContext{
		Messages: []domain.Message{
			{Role: "user", Content: "我喜欢喝咖啡"},
		},
	}
	dir := t.TempDir()
	db, _ := infrastorage.OpenDB(dir + "/memory.db")
	store := infrastorage.NewStore(db)
	defer store.Close()

	ExtractConfirmations(ctx, store, func(c string) { memorized = append(memorized, c) })

	if len(memorized) == 0 {
		t.Error("expected at least 1 confirmation fact from '我喜欢喝咖啡'")
	}
}

func TestExtractProfile_Name(t *testing.T) {
	dir := t.TempDir()
	db, _ := infrastorage.OpenDB(dir + "/memory.db")
	store := infrastorage.NewStore(db)
	defer store.Close()

	profile := &domain.UserProfile{}
	ctx := &domain.ChatContext{
		Messages: []domain.Message{
			{Role: "user", Content: "我叫小明"},
		},
	}
	ExtractProfile(ctx, store, profile)

	if profile.Name != "小明" {
		t.Errorf("expected name '小明', got %q", profile.Name)
	}
}

func TestFilterMessage(t *testing.T) {
	msg := &domain.Message{Role: "user", Content: "你好"}
	if err := FilterMessage(msg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg.Content, "你好") {
		t.Error("message should still contain original content")
	}
	if !strings.Contains(msg.Content, "[") {
		t.Error("message should have timestamp prefix")
	}
}

func TestExtractIdentityLine(t *testing.T) {
	if id := ExtractIdentityLine(""); !strings.Contains(id, "诗音") {
		t.Errorf("empty self should default to 诗音: %q", id)
	}
	if id := ExtractIdentityLine("我是诗音，一只猫娘桌宠。我喜欢陪伴主人。"); !strings.Contains(id, "诗音") {
		t.Errorf("expected identity line to contain 诗音: %q", id)
	}
}

func TestDescribeTopEmotions(t *testing.T) {
	v := domain.EmotionVector{Affection: 0.9, Worry: 0.3, Curiosity: 0.2, Sleepiness: 0.1, Playfulness: 0.8, Loneliness: 0.1, Confidence: 0.7, Annoyance: 0.05}
	e := domain.EmotionState{Primary: "joy", Intensity: 0.7}
	desc := DescribeTopEmotions(v, e)
	if !strings.Contains(desc, "joy") {
		t.Errorf("expected 'joy' in emotion description: %q", desc)
	}
	if !strings.Contains(desc, "亲近") {
		t.Errorf("expected '亲近' for high affection: %q", desc)
	}
}

func TestBuildConcernLine(t *testing.T) {
	if line := BuildConcernLine(nil); line != "" {
		t.Errorf("expected empty for nil snapshot: %q", line)
	}
	sn := domain.NewUserCareState()
	sn.ContinuousWork = 60
	sn.StressLevel = 0.3
	snapshot := func() domain.UserCareState { return sn.Snapshot() }
	line := BuildConcernLine(snapshot)
	// Normal state returns either 陪着主人 or late-night concern depending on hour.
	if line == "" {
		t.Error("expected non-empty concern line")
	}
}
