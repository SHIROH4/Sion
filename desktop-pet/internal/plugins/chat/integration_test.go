package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"desktop-pet/internal/app/plugin"
	"desktop-pet/internal/infra/config"
	"desktop-pet/internal/infra/llm"
)

// TestIntegration_ChatStream_RealAPI verifies streaming chat with the real LLM.
func TestIntegration_ChatStream_RealAPI(t *testing.T) {
	cfg := config.Load()
	if cfg.LLMAPIKey == "" {
		t.Skip("no LLM_API_KEY configured, skipping integration test")
	}

	gw := llm.NewGateway(cfg)

	// Test 1: Character personality - should respond with 喵 mannerisms
	t.Run("personality", func(t *testing.T) {
		messages := []plugin.Message{
			{Role: "system", Content: BuildSystemPrompt(cfg)},
			{Role: "user", Content: "你好，介绍一下你自己"},
		}

		var reply string
		err := gw.ChatStream(context.Background(), messages, func(chunk string) error {
			reply += chunk
			return nil
		})
		if err != nil {
			t.Fatalf("ChatStream failed: %v", err)
		}
		if reply == "" {
			t.Error("expected non-empty reply")
		}
		t.Logf("Personality reply: %s", reply)
	})

	// Test 2: Technical question - should be professional and accurate
	t.Run("technical", func(t *testing.T) {
		messages := []plugin.Message{
			{Role: "system", Content: BuildSystemPrompt(cfg)},
			{Role: "user", Content: "Go里面slice和array的区别是什么"},
		}

		var reply string
		err := gw.ChatStream(context.Background(), messages, func(chunk string) error {
			reply += chunk
			return nil
		})
		if err != nil {
			t.Fatalf("ChatStream failed: %v", err)
		}
		if reply == "" {
			t.Error("expected non-empty reply")
		}
		t.Logf("Technical reply: %s", reply)
	})

	// Test 3: ChatSync (non-streaming) works
	t.Run("sync", func(t *testing.T) {
		messages := []plugin.Message{
			{Role: "system", Content: BuildSystemPrompt(cfg)},
			{Role: "user", Content: "说一个喵"},
		}
		reply, err := gw.ChatSync(context.Background(), messages)
		if err != nil {
			t.Fatalf("ChatSync failed: %v", err)
		}
		if !strings.Contains(reply, "喵") {
			t.Errorf("expected 喵 in reply, got: %s", reply)
		}
		t.Logf("Sync reply: %s", reply)
	})
}

// TestIntegration_ChatPlugin_PokeBuffer verifies the poke buffer sink works.
func TestIntegration_ChatPlugin_PokeBuffer(t *testing.T) {
	cfg := config.Load()
	if cfg.LLMAPIKey == "" {
		t.Skip("no LLM_API_KEY configured, skipping integration test")
	}

	eb := plugin.NewEventBus()
	pb := plugin.NewPokeBuffer()
	pctx := plugin.PluginContext{
		Ctx:        context.Background(),
		EventBus:   eb,
		PokeBuffer: pb,
		Config:     cfg,
	}

	p := &ChatPlugin{}
	if err := p.Awake(pctx); err != nil {
		t.Fatalf("Awake: %v", err)
	}
	p.emitFn = func(ctx context.Context, eventName string, data interface{}) {
		eb.Emit(eventName, data)
	}

	// Collect streamed reply
	var reply string
	done := make(chan struct{})
	eb.On("chat:stream", func(payload any) {
		if s, ok := payload.(string); ok {
			reply += s
		}
		// Close on first chunk for test (avoid blocking on stream completion)
		select {
		case <-done:
		default:
			close(done)
		}
	})

	// Manually trigger the poke sink
	pb.Poke("打个招呼，超简短")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go pb.FlushLoop(ctx)

	select {
	case <-done:
		t.Logf("Poke reply: %s", reply)
	case <-time.After(15 * time.Second):
		t.Error("timeout waiting for poke response")
	}
}


