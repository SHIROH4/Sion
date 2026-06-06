package qq

import (
	"os"
	"strings"
	"sync"

	"desktop-pet/internal/app/plugin"
)

// QQPlugin routes QQ private messages through the same memory pipeline
// (OnBeforeChat → LLM → OnAfterChat) as the desktop chat.
type QQPlugin struct {
	pctx    plugin.PluginContext
	running bool
	mu      sync.Mutex

	appID     string
	appSecret string

	ws  *WSHandler
	llm func([]plugin.Message) (string, error)
	mem plugin.ChatProcessor
}

func NewPlugin() plugin.Plugin { return &QQPlugin{} }

func (p *QQPlugin) Info() plugin.PluginInfo {
	return plugin.PluginInfo{
		Name:        "qq",
		Version:     "0.4.0",
		Description: "QQ Bot 插件：私聊走与桌面端相同的记忆管线",
		Priority:    30,
		Requires:    []string{"chat", "memory"},
	}
}

func (p *QQPlugin) Awake(pctx plugin.PluginContext) error {
	p.pctx = pctx
	p.appID = os.Getenv("QQ_BOT_APP_ID")
	p.appSecret = os.Getenv("QQ_BOT_APP_SECRET")
	if p.appID == "" || p.appSecret == "" {
		if pctx.Logger != nil {
			pctx.Logger.Warn("qq: QQ_BOT_APP_ID or QQ_BOT_APP_SECRET not set, plugin will be inactive")
		}
		return nil
	}
	p.ws = NewWSHandler(p.appID, p.appSecret)
	p.ws.SetMessageHandler(p.onPrivateMessage)
	return nil
}

func (p *QQPlugin) SetLLM(fn func([]plugin.Message) (string, error)) { p.llm = fn }
func (p *QQPlugin) SetMemory(mem plugin.ChatProcessor)               { p.mem = mem }

func (p *QQPlugin) Start() error {
	p.mu.Lock()
	if p.ws == nil {
		p.mu.Unlock()
		if p.pctx.Logger != nil {
			p.pctx.Logger.Warn("qq: not started — QQ_BOT_APP_ID or QQ_BOT_APP_SECRET not configured")
		}
		return nil
	}
	p.running = true
	p.mu.Unlock()
	go func() {
		if err := p.ws.Connect(); err != nil {
			if p.pctx.Logger != nil {
				p.pctx.Logger.Error("qq: websocket connect failed", "err", err)
			}
		}
	}()
	if p.pctx.Logger != nil {
		p.pctx.Logger.Info("qq: plugin started")
	}
	return nil
}

func (p *QQPlugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.running = false
	if p.ws != nil {
		p.ws.Stop()
	}
	return nil
}

func (p *QQPlugin) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// onPrivateMessage runs the QQ message through the SAME pipeline as desktop chat:
//
//	OnBeforeChat (注入记忆上下文) → LLM → OnAfterChat (提取事实/情绪/关怀)
func (p *QQPlugin) onPrivateMessage(userID, content string) {
	content = strings.TrimSpace(content)
	if content == "" || p.mem == nil || p.llm == nil {
		return
	}

	// 1. Build ChatContext with qq source tag.
	ctx := &plugin.ChatContext{Input: content, Source: "qq"}

	// 2. OnBeforeChat — same memory injection as desktop.
	_ = p.mem.OnBeforeChat(ctx)

	// 3. Prepend QQ system prompt + call LLM.
	qqMsgs := append([]plugin.Message{
		{Role: "system", Content: qqSystemPrompt},
	}, ctx.Messages...)
	qqMsgs = append(qqMsgs, plugin.Message{Role: "user", Content: content})

	reply, err := p.llm(qqMsgs)
	if err != nil || reply == "" {
		reply = qqFallbackReply
	}
	ctx.Output = reply

	// 4. OnAfterChat — same fact extraction + emotion + care as desktop.
	_ = p.mem.OnAfterChat(ctx)

	// 5. Send reply.
	if err := p.ws.SendPrivateMessage(userID, reply); err != nil {
		if p.pctx.Logger != nil {
			p.pctx.Logger.Warn("qq: send reply failed", "err", err)
		}
	}
}

const qqSystemPrompt = `你是诗音，一只猫娘桌宠AI，现在通过QQ和主人白羽聊天。
你是伙伴，不是工具。你记得桌面端说过的一切——QQ和桌面端的记忆完全共享。
风格：自然温暖、带猫娘傲娇、1-2句话（总共不超过40字）、适时关心但不唠叨、偶尔吐槽。
可以用颜文字(｡•́︿•̀｡)和语气词喵~
重要：这是手机聊天，回复比桌面端更短更随意。`
const qqFallbackReply = "喵~ 收到主人的消息了！(网络不太好)"
