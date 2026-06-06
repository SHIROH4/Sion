package vision

import (
	"context"
	"strings"
	"testing"

	"desktop-pet/internal/app/plugin"
)

func TestVisionPlugin_Info(t *testing.T) {
	p := NewPlugin()
	info := p.Info()

	if info.Name != "vision" {
		t.Errorf("Name = %q, want %q", info.Name, "vision")
	}
	if info.Version != "0.1.0" {
		t.Errorf("Version = %q, want %q", info.Version, "0.1.0")
	}
	if info.Priority != 40 {
		t.Errorf("Priority = %d, want 40", info.Priority)
	}
	if len(info.Requires) != 1 || info.Requires[0] != "chat" {
		t.Errorf("Requires = %v, want [chat]", info.Requires)
	}
}

func TestVisionPlugin_ImplementsPlugin(t *testing.T) {
	p := NewPlugin()
	_, ok := p.(plugin.Plugin)
	if !ok {
		t.Error("VisionPlugin must implement plugin.Plugin")
	}
}

func TestVisionPlugin_ImplementsFunctionProvider(t *testing.T) {
	p := &VisionPlugin{}
	_, ok := interface{}(p).(plugin.FunctionProvider)
	if !ok {
		t.Error("VisionPlugin must implement plugin.FunctionProvider")
	}
}

func TestVisionPlugin_ImplementsMessageFilter(t *testing.T) {
	p := &VisionPlugin{}
	_, ok := interface{}(p).(plugin.MessageFilter)
	if !ok {
		t.Error("VisionPlugin must implement plugin.MessageFilter")
	}
}

func TestVisionPlugin_ImplementsChatProcessor(t *testing.T) {
	p := &VisionPlugin{}
	_, ok := interface{}(p).(plugin.ChatProcessor)
	if !ok {
		t.Error("VisionPlugin must implement plugin.ChatProcessor")
	}
}

func TestVisionPlugin_AwakeAndStart(t *testing.T) {
	p := &VisionPlugin{}
	pctx := plugin.PluginContext{
		Ctx: context.Background(),
	}

	if err := p.Awake(pctx); err != nil {
		t.Fatalf("Awake failed: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !p.IsRunning() {
		t.Error("IsRunning must return true after Start")
	}
}

func TestVisionPlugin_Stop(t *testing.T) {
	p := &VisionPlugin{}
	pctx := plugin.PluginContext{
		Ctx: context.Background(),
	}

	p.Awake(pctx)
	p.Start()

	if err := p.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if p.IsRunning() {
		t.Error("IsRunning must return false after Stop")
	}
}

func TestVisionPlugin_RegisterFunctions(t *testing.T) {
	p := &VisionPlugin{}
	reg := &plugin.FunctionRegistry{}
	p.RegisterFunctions(reg)

	entries := reg.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 function, got %d", len(entries))
	}

	e := entries[0]
	if e.Name != "analyze_screenshot" {
		t.Errorf("Name = %q, want analyze_screenshot", e.Name)
	}
	if e.Description == "" {
		t.Error("Description must not be empty")
	}
	if e.Parameters == nil {
		t.Error("Parameters must not be nil")
	}

	props, ok := e.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatal("Parameters must have 'properties' key")
	}
	descProp, ok := props["description"].(map[string]any)
	if !ok {
		t.Fatal("Parameters must have 'description' property")
	}
	if descProp["type"] != "string" {
		t.Errorf("description type = %v, want string", descProp["type"])
	}
}

// ---- Task 5.2 tests ----

func TestVisionPlugin_FilterMessage_HasScreenshot(t *testing.T) {
	p := &VisionPlugin{}
	msg := &plugin.Message{
		Role:    "user",
		Content: "这段报错是什么意思",
		Images:  []plugin.Image{{Base64: "abc123", Format: "png"}},
	}

	if err := p.FilterMessage(msg); err != nil {
		t.Fatalf("FilterMessage failed: %v", err)
	}

	meta, ok := msg.Meta.(map[string]any)
	if !ok {
		t.Fatal("Meta must be map[string]any after FilterMessage")
	}
	if v, ok := meta["vision:has_screenshot"]; !ok || v != true {
		t.Error("Meta must have vision:has_screenshot = true")
	}
}

func TestVisionPlugin_FilterMessage_NoScreenshot(t *testing.T) {
	p := &VisionPlugin{}
	msg := &plugin.Message{
		Role:    "user",
		Content: "hello",
	}

	if err := p.FilterMessage(msg); err != nil {
		t.Fatalf("FilterMessage failed: %v", err)
	}

	if msg.Meta != nil {
		t.Error("Meta must remain nil when no images")
	}
}

func TestVisionPlugin_FilterMessage_NonUserRole(t *testing.T) {
	p := &VisionPlugin{}
	msg := &plugin.Message{
		Role:    "assistant",
		Content: "some reply",
		Images:  []plugin.Image{{Base64: "abc123", Format: "png"}},
	}

	if err := p.FilterMessage(msg); err != nil {
		t.Fatalf("FilterMessage failed: %v", err)
	}

	if msg.Meta != nil {
		t.Error("Meta must remain nil for non-user messages even with images")
	}
}

func TestVisionPlugin_OnBeforeChat_InjectsPrompt(t *testing.T) {
	p := &VisionPlugin{}
	ctx := &plugin.ChatContext{
		Input: "这段报错是什么意思",
		Messages: []plugin.Message{
			{
				Role:    "user",
				Content: "这段报错是什么意思",
				Images:  []plugin.Image{{Base64: "abc123", Format: "png"}},
				Meta: map[string]any{
					"vision:has_screenshot": true,
				},
			},
		},
	}

	if err := p.OnBeforeChat(ctx); err != nil {
		t.Fatalf("OnBeforeChat failed: %v", err)
	}

	if len(ctx.Messages) < 2 {
		t.Fatal("expected at least 2 messages after injection")
	}
	if ctx.Messages[0].Role != "system" {
		t.Errorf("first message role = %q, want system", ctx.Messages[0].Role)
	}
	if !strings.Contains(ctx.Messages[0].Content, "截图分析") {
		t.Error("system prompt must contain '截图分析'")
	}
}

func TestVisionPlugin_OnBeforeChat_NoScreenshot(t *testing.T) {
	p := &VisionPlugin{}
	original := []plugin.Message{
		{Role: "user", Content: "hello"},
	}
	ctx := &plugin.ChatContext{
		Input:    "hello",
		Messages: append([]plugin.Message{}, original...),
	}

	if err := p.OnBeforeChat(ctx); err != nil {
		t.Fatalf("OnBeforeChat failed: %v", err)
	}

	if len(ctx.Messages) != len(original) {
		t.Errorf("messages count = %d, want %d (no injection)", len(ctx.Messages), len(original))
	}
}

func TestVisionPlugin_AnalyzeScreenshot_Empty(t *testing.T) {
	p := &VisionPlugin{}
	result, err := p.AnalyzeScreenshot("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "请描述") {
		t.Errorf("empty description must return guidance, got: %s", result)
	}
}

func TestVisionPlugin_AnalyzeScreenshot_ErrorKeyword(t *testing.T) {
	p := &VisionPlugin{}
	result, err := p.AnalyzeScreenshot("这段报错是什么原因")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "报错") {
		t.Errorf("error keyword result must mention '报错', got: %s", result)
	}
}

func TestVisionPlugin_AnalyzeScreenshot_CodeKeyword(t *testing.T) {
	p := &VisionPlugin{}
	result, err := p.AnalyzeScreenshot("这段代码有什么问题")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "代码") {
		t.Errorf("code keyword result must mention '代码', got: %s", result)
	}
}

func TestVisionPlugin_AnalyzeScreenshot_General(t *testing.T) {
	p := &VisionPlugin{}
	result, err := p.AnalyzeScreenshot("帮我看看这个")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "截图") {
		t.Errorf("general result must mention '截图', got: %s", result)
	}
}

func TestVisionPlugin_BuildSystemPrompt(t *testing.T) {
	prompt := buildVisionSystemPrompt()

	keywords := []string{"error", "code", "design", "document", "general"}
	for _, kw := range keywords {
		if !strings.Contains(prompt, kw) {
			t.Errorf("prompt must contain content type %q", kw)
		}
	}
	if !strings.Contains(prompt, "截图分析") {
		t.Error("prompt must contain '截图分析'")
	}
}

func TestVisionPlugin_OnAfterChat(t *testing.T) {
	p := &VisionPlugin{}
	if err := p.OnAfterChat(&plugin.ChatContext{}); err != nil {
		t.Errorf("OnAfterChat should be no-op: %v", err)
	}
}

// ---- Task 5.3 tests ----

func TestVisionPlugin_SetLLMSync(t *testing.T) {
	p := &VisionPlugin{}
	mockLLM := func(msgs []plugin.Message) (string, error) {
		return "分析结果", nil
	}
	p.SetLLMSync(mockLLM)

	if p.rawLLM == nil {
		t.Error("rawLLM must be non-nil after SetLLMSync")
	}
}

func TestVisionPlugin_AnalyzeScreenshotWithImage_NoLLM(t *testing.T) {
	p := &VisionPlugin{}
	_, err := p.AnalyzeScreenshotWithImage("abc123", "png", "这是什么报错")
	if err == nil {
		t.Error("expected error when rawLLM is nil")
	}
	if err.Error() != "vision: LLM not wired" {
		t.Errorf("error = %q, want 'vision: LLM not wired'", err.Error())
	}
}

func TestVisionPlugin_AnalyzeScreenshotWithImage_Success(t *testing.T) {
	p := &VisionPlugin{}
	mockLLM := func(msgs []plugin.Message) (string, error) {
		// Verify system prompt and image are passed through.
		if len(msgs) != 2 {
			t.Errorf("expected 2 messages, got %d", len(msgs))
		}
		if msgs[0].Role != "system" {
			t.Errorf("first message role = %q, want system", msgs[0].Role)
		}
		if msgs[1].Role != "user" {
			t.Errorf("second message role = %q, want user", msgs[1].Role)
		}
		if len(msgs[1].Images) != 1 {
			t.Errorf("expected 1 image, got %d", len(msgs[1].Images))
		}
		return "这是一段TypeScript类型错误...", nil
	}
	p.SetLLMSync(mockLLM)

	result, err := p.AnalyzeScreenshotWithImage("abc123", "png", "这是什么报错")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "这是一段TypeScript类型错误..." {
		t.Errorf("result = %q", result)
	}
}

func TestVisionPlugin_AnalyzeScreenshotText_Success(t *testing.T) {
	p := &VisionPlugin{}
	var capturedContent string
	mockLLM := func(msgs []plugin.Message) (string, error) {
		if len(msgs) != 2 {
			t.Errorf("expected 2 messages, got %d", len(msgs))
		}
		if msgs[0].Role != "system" {
			t.Errorf("first message role = %q, want system", msgs[0].Role)
		}
		if msgs[1].Role != "user" {
			t.Errorf("second message role = %q, want user", msgs[1].Role)
		}
		capturedContent = msgs[1].Content
		return "分析：这是一个TypeError，原因是...", nil
	}
	p.SetLLMSync(mockLLM)

	result, err := p.AnalyzeScreenshotText("TypeError at line 42", "这是什么报错")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "分析：这是一个TypeError，原因是..." {
		t.Errorf("result = %q", result)
	}
	if !strings.Contains(capturedContent, "TypeError at line 42") {
		t.Errorf("prompt must contain OCR text, got: %s", capturedContent)
	}
	if !strings.Contains(capturedContent, "这是什么报错") {
		t.Errorf("prompt must contain user question, got: %s", capturedContent)
	}
}

func TestVisionPlugin_AnalyzeScreenshotText_NoLLM(t *testing.T) {
	p := &VisionPlugin{}
	_, err := p.AnalyzeScreenshotText("some text", "问题")
	if err == nil {
		t.Error("expected error when rawLLM is nil")
	}
	if err.Error() != "vision: LLM not wired" {
		t.Errorf("error = %q, want 'vision: LLM not wired'", err.Error())
	}
}

func TestVisionPlugin_BuildScreenshotTextPrompt(t *testing.T) {
	prompt := buildScreenshotTextPrompt()

	if !strings.Contains(prompt, "OCR") {
		t.Error("prompt must mention OCR")
	}
	if !strings.Contains(prompt, "%s") {
		t.Error("prompt must have placeholders for OCR text and user question")
	}
	if !strings.Contains(prompt, "诗音") {
		t.Error("prompt must mention character name 诗音")
	}
	if !strings.Contains(prompt, "猫娘") {
		t.Error("prompt must include character voice")
	}
	if !strings.Contains(prompt, "喵") {
		t.Error("prompt must include 喵 mannerism")
	}
}

func TestVisionPlugin_OnBeforeChat_InjectsPromptForImage(t *testing.T) {
	p := &VisionPlugin{}
	ctx := &plugin.ChatContext{
		Input: "这段报错是什么意思",
		Messages: []plugin.Message{
			{
				Role:    "user",
				Content: "这段报错是什么意思",
				Images:  []plugin.Image{{Base64: "abc123", Format: "png"}},
			},
		},
	}

	if err := p.OnBeforeChat(ctx); err != nil {
		t.Fatalf("OnBeforeChat failed: %v", err)
	}

	if len(ctx.Messages) < 2 {
		t.Fatal("expected at least 2 messages after injection")
	}
	if ctx.Messages[0].Role != "system" {
		t.Errorf("first message role = %q, want system", ctx.Messages[0].Role)
	}
	if !strings.Contains(ctx.Messages[0].Content, "截图分析") {
		t.Error("system prompt must contain '截图分析'")
	}
}

func TestVisionPlugin_OnBeforeChat_NoImage(t *testing.T) {
	p := &VisionPlugin{}
	original := []plugin.Message{
		{Role: "user", Content: "hello"},
	}
	ctx := &plugin.ChatContext{
		Input:    "hello",
		Messages: append([]plugin.Message{}, original...),
	}

	if err := p.OnBeforeChat(ctx); err != nil {
		t.Fatalf("OnBeforeChat failed: %v", err)
	}

	if len(ctx.Messages) != len(original) {
		t.Errorf("messages count = %d, want %d (no injection)", len(ctx.Messages), len(original))
	}
}

func TestVisionPlugin_ContentTypeConstants(t *testing.T) {
	tests := []struct {
		ct   ContentType
		want string
	}{
		{TypeError, "error"},
		{TypeCode, "code"},
		{TypeDesign, "design"},
		{TypeDocument, "document"},
		{TypeGeneral, "general"},
	}
	for _, tt := range tests {
		if string(tt.ct) != tt.want {
			t.Errorf("ContentType %v = %q, want %q", tt.ct, string(tt.ct), tt.want)
		}
	}
}
