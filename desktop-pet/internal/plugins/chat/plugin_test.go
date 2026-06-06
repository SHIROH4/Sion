package chat

import (
	"context"
	"testing"
	"time"

	"desktop-pet/internal/app/plugin"
	"desktop-pet/internal/infra/config"
	infrallm "desktop-pet/internal/infra/llm"
)

func TestChatPlugin_Info(t *testing.T) {
	p := NewPlugin()
	info := p.Info()

	if info.Name != "chat" {
		t.Errorf("Name = %q, want %q", info.Name, "chat")
	}
	if info.Priority != 10 {
		t.Errorf("Priority = %d, want 10", info.Priority)
	}
	if info.Version != "0.1.0" {
		t.Errorf("Version = %q, want %q", info.Version, "0.1.0")
	}
}

func TestChatPlugin_ImplementsPlugin(t *testing.T) {
	p := NewPlugin()
	_, ok := p.(plugin.Plugin)
	if !ok {
		t.Error("ChatPlugin must implement plugin.Plugin")
	}
}

func TestChatPlugin_ImplementsChatProcessor(t *testing.T) {
	p := &ChatPlugin{}
	_, ok := interface{}(p).(plugin.ChatProcessor)
	if !ok {
		t.Error("ChatPlugin must implement plugin.ChatProcessor")
	}
}

func TestChatPlugin_ImplementsFunctionProvider(t *testing.T) {
	p := &ChatPlugin{}
	_, ok := interface{}(p).(plugin.FunctionProvider)
	if !ok {
		t.Error("ChatPlugin must implement plugin.FunctionProvider")
	}
}

func TestChatPlugin_Awake(t *testing.T) {
	p := &ChatPlugin{}
	eb := plugin.NewEventBus()
	pb := plugin.NewPokeBuffer()
	cfg := &config.GlobalConfig{
		LLMProvider: "deepseek",
		LLMModel:    "deepseek-chat",
		LLMBaseURL:  "https://api.deepseek.com",
		LLMAPIKey:   "test-key",
	}
	pctx := plugin.PluginContext{
		Ctx:        context.Background(),
		EventBus:   eb,
		PokeBuffer: pb,
		Config:     cfg,
	}

	if err := p.Awake(pctx); err != nil {
		t.Fatalf("Awake failed: %v", err)
	}
	if p.gateway == nil {
		t.Error("gateway must be initialized after Awake")
	}
	if p.funcReg == nil {
		t.Error("funcReg must be initialized after Awake")
	}
}

func TestChatPlugin_StartStop(t *testing.T) {
	p := &ChatPlugin{}
	eb := plugin.NewEventBus()
	pb := plugin.NewPokeBuffer()
	cfg := &config.GlobalConfig{LLMBaseURL: "https://api.test.com"}
	pctx := plugin.PluginContext{
		Ctx:        context.Background(),
		EventBus:   eb,
		PokeBuffer: pb,
		Config:     cfg,
	}

	p.Awake(pctx)

	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !p.IsRunning() {
		t.Error("IsRunning must return true after Start")
	}

	if err := p.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if p.IsRunning() {
		t.Error("IsRunning must return false after Stop")
	}
}

func TestChatPlugin_OnBeforeChat(t *testing.T) {
	p := &ChatPlugin{}
	cfg := &config.GlobalConfig{UserName: "测试"}
	p.pctx = plugin.PluginContext{Config: cfg}

	chatCtx := &plugin.ChatContext{
		Input:    "hello",
		Messages: []plugin.Message{{Role: "user", Content: "hello"}},
	}

	if err := p.OnBeforeChat(chatCtx); err != nil {
		t.Fatalf("OnBeforeChat failed: %v", err)
	}

	if len(chatCtx.Messages) < 2 {
		t.Fatal("expected at least 2 messages (system + user)")
	}
	if chatCtx.Messages[0].Role != "system" {
		t.Errorf("first message role = %q, want system", chatCtx.Messages[0].Role)
	}
	if chatCtx.Messages[1].Role != "user" {
		t.Errorf("second message role = %q, want user", chatCtx.Messages[1].Role)
	}
}

func TestChatPlugin_OnAfterChat(t *testing.T) {
	p := &ChatPlugin{}
	if err := p.OnAfterChat(&plugin.ChatContext{}); err != nil {
		t.Errorf("OnAfterChat should be no-op: %v", err)
	}
}

func TestChatPlugin_RegisterFunctions(t *testing.T) {
	p := &ChatPlugin{}
	reg := &plugin.FunctionRegistry{}
	p.RegisterFunctions(reg)
	if len(reg.Entries()) != 0 {
		t.Error("RegisterFunctions should be no-op in Phase 2")
	}
}

func TestChatPlugin_HandleChatCancellation(t *testing.T) {
	p := &ChatPlugin{}
	eb := plugin.NewEventBus()
	pb := plugin.NewPokeBuffer()
	cfg := &config.GlobalConfig{LLMBaseURL: "https://api.invalid.test"}
	pctx := plugin.PluginContext{
		Ctx:        context.Background(),
		EventBus:   eb,
		PokeBuffer: pb,
		Config:     cfg,
	}
	p.Awake(pctx)
	// Override emitFn for tests — runtime.EventsEmit panics without a real Wails context.
	p.emitFn = func(ctx context.Context, eventName string, data interface{}) {}

	// Start a chat (will fail quickly due to invalid endpoint)
	done := make(chan struct{})
	go func() {
		p.handleChat("first message")
		close(done)
	}()

	// Send a second message while first is in-flight (triggers cancellation).
	time.Sleep(10 * time.Millisecond)
	go p.handleChat("second message")

	select {
	case <-done:
		// First handleChat completed (likely errored due to invalid URL).
	case <-time.After(2 * time.Second):
		t.Error("first handleChat should not block indefinitely after cancellation")
	}
}

func TestChatPlugin_ExecuteSingleTool(t *testing.T) {
	p := &ChatPlugin{}
	p.funcReg = &plugin.FunctionRegistry{}

	called := false
	var receivedArg string
	p.funcReg.RegisterWithParams("get_memory", "搜索记忆", func(desc string) (string, error) {
		called = true
		receivedArg = desc
		return "找到了3条相关记忆", nil
	}, nil)

	p.tools = infrallm.BuildTools(p.funcReg.Entries())

	result := p.executeSingleTool(plugin.ToolCall{
		ID:   "call_001",
		Type: "function",
		Function: plugin.ToolCallFunction{
			Name:      "get_memory",
			Arguments: `{"description": "用户的生日"}`,
		},
	})

	if !called {
		t.Error("handler should have been called")
	}
	if receivedArg != "用户的生日" {
		t.Errorf("expected arg '用户的生日', got %q", receivedArg)
	}
	if result != "找到了3条相关记忆" {
		t.Errorf("expected '找到了3条相关记忆', got %q", result)
	}
}

func TestChatPlugin_ExecuteSingleTool_NotFound(t *testing.T) {
	p := &ChatPlugin{}
	p.funcReg = &plugin.FunctionRegistry{}

	result := p.executeSingleTool(plugin.ToolCall{
		ID:   "call_002",
		Type: "function",
		Function: plugin.ToolCallFunction{
			Name:      "nonexistent",
			Arguments: `{}`,
		},
	})

	if result != "未找到工具: nonexistent" {
		t.Errorf("expected not-found message, got %q", result)
	}
}

func TestChatPlugin_InvokeHandler(t *testing.T) {
	p := &ChatPlugin{}

	handler := func(desc string) (string, error) {
		return "结果: " + desc, nil
	}

	result := p.invokeHandler(handler, `{"description": "Rust编译错误"}`)
	if result != "结果: Rust编译错误" {
		t.Errorf("expected '结果: Rust编译错误', got %q", result)
	}
}

func TestChatPlugin_InvokeHandler_BadType(t *testing.T) {
	p := &ChatPlugin{}
	result := p.invokeHandler("not a function", `{}`)
	if result != "工具调用失败：处理器类型不匹配" {
		t.Errorf("expected type mismatch message, got %q", result)
	}
}

func TestChatPlugin_SetFunctionRegistry(t *testing.T) {
	p := &ChatPlugin{}
	reg := &plugin.FunctionRegistry{}
	reg.RegisterWithParams("get_memory", "搜索记忆", func(s string) (string, error) { return "ok", nil },
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"description": map[string]any{"type": "string", "description": "查询描述"},
			},
			"required": []string{"description"},
		},
	)

	p.SetFunctionRegistry(reg)

	if len(p.tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(p.tools))
	}
	if p.tools[0].Function.Name != "get_memory" {
		t.Errorf("expected tool name 'get_memory', got %q", p.tools[0].Function.Name)
	}
	if p.tools[0].Type != "function" {
		t.Errorf("expected tool type 'function', got %q", p.tools[0].Type)
	}
}
