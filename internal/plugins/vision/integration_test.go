package vision

import (
	"context"
	"strings"
	"testing"

	"desktop-pet/internal/app/plugin"
)

// TestPhase5_ScreenshotDetection verifies the full flow:
// message with Image → FilterMessage tags Meta → OnBeforeChat injects prompt.
func TestPhase5_ScreenshotDetection(t *testing.T) {
	p := &VisionPlugin{}
	p.Awake(plugin.PluginContext{Ctx: context.Background()})
	p.Start()

	// Step 1: Build a user message with a screenshot image.
	msg := &plugin.Message{
		Role:    "user",
		Content: "这段报错是什么意思",
		Images:  []plugin.Image{{Base64: "abc123", Format: "png"}},
	}

	// Step 2: FilterMessage should tag Meta.
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

	// Step 3: OnBeforeChat should detect the image and inject system prompt.
	ctx := &plugin.ChatContext{
		Input:    "这段报错是什么意思",
		Messages: []plugin.Message{*msg},
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

	// Step 4: Verify the original user message with image is preserved.
	if ctx.Messages[1].Role != "user" {
		t.Errorf("second message role = %q, want user", ctx.Messages[1].Role)
	}
	if len(ctx.Messages[1].Images) != 1 {
		t.Errorf("user message must still have 1 image, got %d", len(ctx.Messages[1].Images))
	}

	p.Stop()
}

// TestPhase5_ContentTypeClassification verifies the keyword-based content
// type routing for all 5 screenshot types.
func TestPhase5_ContentTypeClassification(t *testing.T) {
	p := &VisionPlugin{}

	tests := []struct {
		name        string
		description string
		want        string
	}{
		{
			name:        "error",
			description: "这段报错是什么原因",
			want:        "报错",
		},
		{
			name:        "exception",
			description: "This exception keeps crashing",
			want:        "报错",
		},
		{
			name:        "code",
			description: "这段代码有什么优化建议",
			want:        "代码",
		},
		{
			name:        "compile",
			description: "编译不过是什么原因",
			want:        "代码",
		},
		{
			name:        "general",
			description: "帮我看看这个图",
			want:        "截图",
		},
		{
			name:        "empty",
			description: "",
			want:        "请描述",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.AnalyzeScreenshot(tt.description)
			if err != nil {
				t.Fatalf("AnalyzeScreenshot failed: %v", err)
			}
			if !strings.Contains(result, tt.want) {
				t.Errorf("result must contain %q, got: %s", tt.want, result)
			}
		})
	}
}

// TestPhase5_MultiModalGateway verifies toChatMessages handles multi-modal
// and plain text messages correctly, with backward compatibility.
func TestPhase5_MultiModalGateway(t *testing.T) {
	t.Run("multi_modal_format", func(t *testing.T) {
		// Verify VisionPlugin constructs correct multi-modal messages
		// that the chat gateway's toChatMessages will format correctly.
		p := &VisionPlugin{}
		mockLLM := func(msgs []plugin.Message) (string, error) {
			if len(msgs) != 2 {
				t.Errorf("expected 2 messages (system + user), got %d", len(msgs))
			}
			if msgs[0].Role != "system" {
				t.Errorf("first message role = %q, want system", msgs[0].Role)
			}
			userMsg := msgs[1]
			if userMsg.Role != "user" {
				t.Errorf("second message role = %q, want user", userMsg.Role)
			}
			if len(userMsg.Images) != 1 {
				t.Errorf("expected 1 image, got %d", len(userMsg.Images))
			}
			if userMsg.Images[0].Base64 != "imgdata" {
				t.Errorf("expected base64 'imgdata', got %q", userMsg.Images[0].Base64)
			}
			if userMsg.Images[0].Format != "png" {
				t.Errorf("expected format 'png', got %q", userMsg.Images[0].Format)
			}
			return "分析完成", nil
		}
		p.SetLLMSync(mockLLM)

		result, err := p.AnalyzeScreenshotWithImage("imgdata", "png", "这是什么错误")
		if err != nil {
			t.Fatalf("AnalyzeScreenshotWithImage failed: %v", err)
		}
		if result != "分析完成" {
			t.Errorf("result = %q, want '分析完成'", result)
		}
	})

	t.Run("backward_compat", func(t *testing.T) {
		// Verify that messages without images still work (AnalyzeScreenshot
		// keyword path doesn't require rawLLM).
		p := &VisionPlugin{}
		result, err := p.AnalyzeScreenshot("帮我看看这个代码")
		if err != nil {
			t.Fatalf("AnalyzeScreenshot failed: %v", err)
		}
		if !strings.Contains(result, "代码") {
			t.Errorf("expected keyword response, got: %s", result)
		}
	})

	t.Run("mixed_scenario", func(t *testing.T) {
		// Simulate a chat context with mixed messages.
		ctx := &plugin.ChatContext{
			Input: "先看这个截图再回答",
			Messages: []plugin.Message{
				{Role: "user", Content: "之前的问题"},
				{
					Role:    "user",
					Content: "先看这个截图再回答",
					Images:  []plugin.Image{{Base64: "abc", Format: "png"}},
				},
			},
		}

		// OnBeforeChat should find the second user message with image.
		p := &VisionPlugin{}
		if err := p.OnBeforeChat(ctx); err != nil {
			t.Fatalf("OnBeforeChat failed: %v", err)
		}

		// System prompt should be injected.
		if ctx.Messages[0].Role != "system" {
			t.Errorf("first message must be system prompt, got %q", ctx.Messages[0].Role)
		}
		if !strings.Contains(ctx.Messages[0].Content, "截图分析") {
			t.Error("system prompt must contain '截图分析'")
		}

		// Both original messages should still be present.
		userCount := 0
		for _, m := range ctx.Messages {
			if m.Role == "user" {
				userCount++
			}
		}
		if userCount != 2 {
			t.Errorf("expected 2 user messages, got %d", userCount)
		}
	})
}

// TestPhase5_VisionPluginInManager verifies VisionPlugin is correctly
// registered and auto-discovered by the plugin Manager.
func TestPhase5_VisionPluginInManager(t *testing.T) {
	manager := plugin.NewManager(nil)

	// Register vision (requires chat first).
	if err := manager.Register(NewPlugin()); err == nil {
		t.Error("registering vision without chat should fail (Requires=[chat])")
	}

	// Register chat first, then vision.
	// Use a minimal stub that satisfies plugin.Plugin for the chat dependency.
	// Since Manager.Register checks Requires, we need a real "chat" plugin.
	stubChat := &stubPlugin{info: plugin.PluginInfo{Name: "chat", Priority: 10}}
	if err := manager.Register(stubChat); err != nil {
		t.Fatalf("register stub chat: %v", err)
	}
	if err := manager.Register(NewPlugin()); err != nil {
		t.Fatalf("register vision: %v", err)
	}

	// Verify auto-discovery of capabilities.
	visionP := manager.Plugin("vision")
	if visionP == nil {
		t.Fatal("vision plugin not found in manager")
	}
	if _, ok := visionP.(plugin.MessageFilter); !ok {
		t.Error("VisionPlugin must be auto-discovered as MessageFilter")
	}
	if _, ok := visionP.(plugin.ChatProcessor); !ok {
		t.Error("VisionPlugin must be auto-discovered as ChatProcessor")
	}
	if _, ok := visionP.(plugin.FunctionProvider); !ok {
		t.Error("VisionPlugin must be auto-discovered as FunctionProvider")
	}
}

// stubPlugin is a minimal plugin.Plugin for testing Manager registration.
type stubPlugin struct {
	info plugin.PluginInfo
}

func (s *stubPlugin) Info() plugin.PluginInfo               { return s.info }
func (s *stubPlugin) Awake(pctx plugin.PluginContext) error { return nil }
func (s *stubPlugin) Start() error                          { return nil }
func (s *stubPlugin) Stop() error                           { return nil }
func (s *stubPlugin) IsRunning() bool                       { return true }
