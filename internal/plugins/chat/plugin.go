package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"desktop-pet/internal/app/plugin"
	"desktop-pet/internal/infra/config"
	"desktop-pet/internal/infra/llm"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ChatPlugin is the AI chat plugin implementing Plugin, ChatProcessor, and FunctionProvider.
type ChatPlugin struct {
	pctx          plugin.PluginContext
	running       bool
	gateway       *llm.Gateway
	funcReg       *plugin.FunctionRegistry
	tools         []llm.Tool
	mu            sync.Mutex
	currentCancel context.CancelFunc
	emitFn        func(ctx context.Context, eventName string, data interface{})
	pipeline      plugin.ChatPipeline
}

// NewPlugin returns a new ChatPlugin as a plugin.Plugin interface.
func NewPlugin() plugin.Plugin {
	return &ChatPlugin{}
}

// Info returns the plugin metadata.
func (p *ChatPlugin) Info() plugin.PluginInfo {
	return plugin.PluginInfo{
		Name:        "chat",
		Version:     "0.1.0",
		Description: "AI 对话插件，LLM 网关 + 对话编排 + Poke 缓冲",
		Priority:    10,
	}
}

// Awake initializes the plugin with the given context.
func (p *ChatPlugin) Awake(pctx plugin.PluginContext) error {
	p.pctx = pctx
	p.pipeline = pctx.Pipeline
	cfg := pctx.Config.(*config.GlobalConfig)
	p.gateway = llm.NewGateway(cfg)
	p.funcReg = &plugin.FunctionRegistry{}
	p.emitFn = func(ctx context.Context, eventName string, data interface{}) {
		defer func() { recover() }()
		if ctx == nil || ctx.Err() != nil {
			return // context cancelled or invalid (e.g., settings process without Wails window)
		}
		runtime.EventsEmit(ctx, eventName, data)
	}

	pctx.PokeBuffer.SetSink(func(merged string) error {
		msgs := []plugin.Message{
			{Role: "system", Content: BuildSystemPrompt(cfg)},
			{Role: "user", Content: merged},
		}
		reply, err := p.gateway.ChatSync(context.Background(), msgs)
		if err != nil {
			return err
		}
		p.emitFn(pctx.Ctx, "chat:stream", reply)
		return nil
	})

	return nil
}

// Start activates the plugin and begins listening for chat events.
func (p *ChatPlugin) Start() error {
	p.running = true

	p.pctx.EventBus.On("frontend:chat:send", func(payload any) {
		msg, ok := payload.(string)
		if !ok {
			return
		}
		p.handleChat(msg)
	})

	return nil
}

// Stop deactivates the plugin and cancels any in-flight chat.
func (p *ChatPlugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.running = false
	if p.currentCancel != nil {
		p.currentCancel()
		p.currentCancel = nil
	}
	return nil
}

// IsRunning reports whether the plugin is active.
func (p *ChatPlugin) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// OnBeforeChat prepends the system prompt to the chat context.
func (p *ChatPlugin) OnBeforeChat(ctx *plugin.ChatContext) error {
	cfg := p.pctx.Config.(*config.GlobalConfig)
	sysMsg := plugin.Message{Role: "system", Content: BuildSystemPrompt(cfg)}
	ctx.Messages = append([]plugin.Message{sysMsg}, ctx.Messages...)
	return nil
}

// OnAfterChat is a no-op in Phase 2 (history saving comes in Phase 3).
func (p *ChatPlugin) OnAfterChat(ctx *plugin.ChatContext) error {
	return nil
}

// LLMSync returns a synchronous LLM call function for use by other plugins
// (e.g. the memory compressor). It bridges the gateway's ChatSync into the
// signature expected by memory.Compressor.
func (p *ChatPlugin) LLMSync() func([]plugin.Message) (string, error) {
	return func(msgs []plugin.Message) (string, error) {
		return p.gateway.ChatSync(context.Background(), msgs)
	}
}

// RegisterFunctions registers AI-callable functions (no-op in Phase 2).
func (p *ChatPlugin) RegisterFunctions(reg *plugin.FunctionRegistry) {
	// Functions will be registered in a later phase.
}

// handleChat manages the lifecycle of a single chat interaction with cancellation support.
func (p *ChatPlugin) handleChat(userMsg string) {
	p.mu.Lock()
	if p.currentCancel != nil {
		p.currentCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.currentCancel = cancel
	p.mu.Unlock()

	defer cancel()

	if p.pipeline == nil {
		p.fallbackChat(ctx, userMsg)
		return
	}

	tools := p.tools        // snapshot
	const maxToolRounds = 3 // P1-3: reduced from 5 to prevent runaway tool loops

	llmCall := func(messages []plugin.Message, onChunk func(string) error) error {
		if len(tools) == 0 {
			return p.gateway.ChatStream(ctx, messages, func(chunk string) error {
				p.emitFn(p.pctx.Ctx, "chat:stream", chunk)
				return onChunk(chunk)
			})
		}

		msgs := messages
		for round := 0; round < maxToolRounds; round++ {
			var toolCalls []plugin.ToolCall
			err := p.gateway.ChatStreamWithTools(ctx, msgs, tools,
				func(chunk string) error {
					p.emitFn(p.pctx.Ctx, "chat:stream", chunk)
					return onChunk(chunk)
				},
				func(calls []plugin.ToolCall) error {
					toolCalls = calls
					return nil
				},
			)
			if err != nil {
				return err
			}

			if len(toolCalls) == 0 {
				return nil
			}

			// Append assistant message with tool_calls (required by OpenAI API).
			msgs = append(msgs, plugin.Message{
				Role:      "assistant",
				Content:   "",
				ToolCalls: toolCalls,
			})

			// Execute tools and append results for next LLM round.
			for _, tc := range toolCalls {
				result := p.executeSingleTool(tc)
				msgs = append(msgs, plugin.Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
				})
			}
		}
		return nil
	}

	chatCtx, err := p.pipeline.ProcessChat(userMsg, llmCall)
	if err != nil {
		if p.pctx.Logger != nil {
			p.pctx.Logger.Error("chat stream failed", "err", err)
		}
		p.emitFn(p.pctx.Ctx, "chat:stream", "[错误] "+err.Error())
		return
	}

	p.emitFn(p.pctx.Ctx, "chat:sent", map[string]string{
		"input":  userMsg,
		"output": chatCtx.Output,
	})
}

// SetFunctionRegistry receives the combined function registry from Manager
// after all plugins have registered their functions.
func (p *ChatPlugin) SetFunctionRegistry(reg *plugin.FunctionRegistry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.funcReg = reg
	p.tools = llm.BuildTools(reg.Entries())
}

// EmbeddingFunc returns a function that embeds text using the gateway.
func (p *ChatPlugin) EmbeddingFunc() func(string) ([]float32, error) {
	return func(text string) ([]float32, error) {
		return p.gateway.GetEmbedding(context.Background(), text)
	}
}

// executeSingleTool invokes one registered tool by name and returns its result.
func (p *ChatPlugin) executeSingleTool(tc plugin.ToolCall) string {
	entries := p.funcReg.Entries()
	for _, e := range entries {
		if e.Name == tc.Function.Name {
			return p.invokeHandler(e.Handler, tc.Function.Arguments)
		}
	}
	return fmt.Sprintf("未找到工具: %s", tc.Function.Name)
}

// invokeHandler calls the registered handler with the JSON arguments string.
// Handlers are expected to be func(string) (string, error).
func (p *ChatPlugin) invokeHandler(handler interface{}, argsJSON string) string {
	h, ok := handler.(func(string) (string, error))
	if !ok {
		return "工具调用失败：处理器类型不匹配"
	}

	// Extract the first string value from the JSON arguments.
	var args map[string]interface{}
	var arg string
	if err := json.Unmarshal([]byte(argsJSON), &args); err == nil {
		for _, v := range args {
			if s, ok := v.(string); ok {
				arg = s
				break
			}
		}
	}

	result, err := h(arg)
	if err != nil {
		return fmt.Sprintf("工具调用出错: %v", err)
	}
	return result
}

// fallbackChat is the pre-Phase-3 direct LLM path, used only when the
// pipeline is not wired (e.g. in tests that don't go through Manager).
func (p *ChatPlugin) fallbackChat(ctx context.Context, userMsg string) {
	cfg := p.pctx.Config.(*config.GlobalConfig)
	messages := []plugin.Message{
		{Role: "system", Content: BuildSystemPrompt(cfg)},
		{Role: "user", Content: userMsg},
	}

	var fullReply string
	err := p.gateway.ChatStream(ctx, messages, func(chunk string) error {
		fullReply += chunk
		p.emitFn(p.pctx.Ctx, "chat:stream", chunk)
		return nil
	})
	if err != nil {
		if p.pctx.Logger != nil {
			p.pctx.Logger.Error("chat stream failed", "err", err)
		}
		p.emitFn(p.pctx.Ctx, "chat:stream", "[错误] "+err.Error())
		return
	}

	p.emitFn(p.pctx.Ctx, "chat:sent", map[string]string{
		"input":  userMsg,
		"output": fullReply,
	})
}
