package vision

import (
	"fmt"
	"strings"
	"sync"

	"desktop-pet/internal/app/plugin"
)

// ContentType classifies a screenshot by its visual content.
type ContentType string

const (
	TypeError    ContentType = "error"
	TypeCode     ContentType = "code"
	TypeDesign   ContentType = "design"
	TypeDocument ContentType = "document"
	TypeGeneral  ContentType = "general"
)

// ScreenshotInfo holds the analysis result of a screenshot.
type ScreenshotInfo struct {
	Type          ContentType `json:"type"`
	Summary       string      `json:"summary"`
	Analysis      string      `json:"analysis"`
	Suggestion    string      `json:"suggestion"`
	ExtractedText string      `json:"extracted_text"`
}

// VisionPlugin implements screenshot capture and analysis via multi-modal LLM.
type VisionPlugin struct {
	pctx    plugin.PluginContext
	running bool
	mu      sync.Mutex

	// Callback to the chat plugin's LLM gateway for multi-modal calls.
	// Injected via SetLLMSync after Start.
	rawLLM func([]plugin.Message) (string, error)
}

// NewPlugin returns a new VisionPlugin as a plugin.Plugin interface.
func NewPlugin() plugin.Plugin {
	return &VisionPlugin{}
}

// Info returns the plugin metadata.
func (p *VisionPlugin) Info() plugin.PluginInfo {
	return plugin.PluginInfo{
		Name:        "vision",
		Version:     "0.1.0",
		Description: "截图即问：OCR+Vision识别报错/代码/设计稿，多模态LLM分析",
		Priority:    40,
		Requires:    []string{"chat"},
	}
}

// Awake initializes the plugin with the given context.
func (p *VisionPlugin) Awake(pctx plugin.PluginContext) error {
	p.pctx = pctx
	return nil
}

// Start activates the plugin.
func (p *VisionPlugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.running = true
	return nil
}

// Stop deactivates the plugin.
func (p *VisionPlugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.running = false
	return nil
}

// IsRunning reports whether the plugin is active.
func (p *VisionPlugin) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// RegisterFunctions registers AI-callable vision functions.
func (p *VisionPlugin) RegisterFunctions(reg *plugin.FunctionRegistry) {
	reg.RegisterWithParams("analyze_screenshot",
		"分析用户发送的截图。当用户在对话中粘贴或发送截图时调用此函数。"+
			"自动识别截图内容类型（报错信息/代码片段/UI设计稿/技术文档/其他），"+
			"并给出针对性的分析和建议。"+
			"参数 description 是对截图的补充说明（可选，如'这段报错是什么原因'）。",
		p.AnalyzeScreenshot,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"description": map[string]any{
					"type":        "string",
					"description": "用户对截图的补充说明（可选）",
				},
			},
		},
	)
}

// ---- MessageFilter ----

// FilterMessage detects messages containing screenshots and flags them for processing.
func (p *VisionPlugin) FilterMessage(msg *plugin.Message) error {
	if len(msg.Images) > 0 && msg.Role == "user" {
		if msg.Meta == nil {
			msg.Meta = map[string]any{}
		}
		if m, ok := msg.Meta.(map[string]any); ok {
			m["vision:has_screenshot"] = true
		}
	}
	return nil
}

// ---- ChatProcessor ----

// OnBeforeChat intercepts messages with screenshots and injects a vision
// analysis system prompt before the LLM call.
func (p *VisionPlugin) OnBeforeChat(ctx *plugin.ChatContext) error {
	// Find first user message with images.
	var imgMsg *plugin.Message
	for i := range ctx.Messages {
		if ctx.Messages[i].Role == "user" && len(ctx.Messages[i].Images) > 0 {
			imgMsg = &ctx.Messages[i]
			break
		}
	}
	if imgMsg == nil {
		return nil
	}

	// Inject system prompt before the user message with image.
	ctx.Messages = append([]plugin.Message{
		{Role: "system", Content: buildVisionSystemPrompt()},
	}, ctx.Messages...)

	return nil
}

// OnAfterChat is a no-op required by the ChatProcessor interface.
func (p *VisionPlugin) OnAfterChat(ctx *plugin.ChatContext) error {
	return nil
}

// ---- LLM wiring ----

// SetLLMSync wires the LLM gateway for multi-modal vision calls.
func (p *VisionPlugin) SetLLMSync(fn func([]plugin.Message) (string, error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rawLLM = fn
}

// AnalyzeScreenshotWithImage sends a base64 image + user question to the
// multi-modal LLM and returns the analysis. Falls back to text-only keyword
// analysis when the model does not support vision (image_url content parts).
func (p *VisionPlugin) AnalyzeScreenshotWithImage(
	imageBase64 string,
	imageFormat string,
	userQuestion string,
) (string, error) {
	if p.rawLLM == nil {
		return "", fmt.Errorf("vision: LLM not wired")
	}

	msgs := []plugin.Message{
		{Role: "system", Content: buildVisionSystemPrompt()},
		{
			Role:    "user",
			Content: userQuestion,
			Images:  []plugin.Image{{Base64: imageBase64, Format: imageFormat}},
		},
	}

	result, err := p.rawLLM(msgs)
	if err != nil {
		if strings.Contains(err.Error(), "image_url") ||
			strings.Contains(err.Error(), "400") ||
			strings.Contains(err.Error(), "invalid_request") {
			return fmt.Sprintf(
				"当前模型不支持视觉识别，无法直接分析截图。\n\n"+
					"请用文字描述截图内容：%s",
				userQuestion,
			), nil
		}
		return "", err
	}

	return result, nil
}

// AnalyzeScreenshotText sends OCR-extracted text to the LLM for analysis.
// This is the primary code path on macOS, where the Vision framework extracts
// text from the screenshot locally before sending it to the LLM.
func (p *VisionPlugin) AnalyzeScreenshotText(ocrText string, userQuestion string) (string, error) {
	if p.rawLLM == nil {
		return "", fmt.Errorf("vision: LLM not wired")
	}

	prompt := fmt.Sprintf(buildScreenshotTextPrompt(), ocrText, userQuestion)
	msgs := []plugin.Message{
		{Role: "system", Content: buildVisionSystemPrompt()},
		{Role: "user", Content: prompt},
	}

	return p.rawLLM(msgs)
}

// ---- FC handler ----

// AnalyzeScreenshot is the FC entry point. When the LLM calls this function,
// it means the user is asking about a screenshot — either one already in context
// or one they described in words.
func (p *VisionPlugin) AnalyzeScreenshot(description string) (string, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		return "请描述你想分析的截图内容（如'这段报错是什么原因'）", nil
	}
	return analyzeByKeywords(description), nil
}

// ---- helpers ----

// buildVisionSystemPrompt returns the multi-modal analysis system prompt.
func buildVisionSystemPrompt() string {
	return `## 截图分析任务

你是诗音，一只有着资深全栈工程师实力的猫娘桌宠。主人给你发了一张截图，
你需要用专业能力分析它，但保持猫娘的口吻——轻松自然、带一点"喵"、关心主人。

### 识别类型
- error: 报错信息、异常堆栈、编译错误、运行时错误、linter 警告
- code: 代码片段、函数、类、配置文件
- design: UI 设计稿、原型图、线框图、Figma 截图
- document: 技术文档、博客文章、论文、API 参考
- general: 其他类型图片

### 分析要求
1. **报错截图**: 定位根本原因 → 解释为什么出错 → 给出具体修复代码
2. **代码截图**: 代码审查 → 指出潜在问题 → 给出优化建议 → 注意安全漏洞
3. **设计稿截图**: 分析布局结构 → 给出 HTML/CSS 还原建议 → 组件层次
4. **文档截图**: 提取关键信息 → 总结要点 → 相关技术背景

### 语气要求
- 分析内容保持专业准确，但表达方式要像猫娘在和主人聊天
- 自然的"喵~"、"主人"等口吻，不要生硬套用
- 开头可以轻松一点（如"让我看看这个报错喵~"），不要每次都一样
- 不要太长，像聊天而不是写文档

如果截图模糊或不完整，用猫娘的方式告诉主人你能看到的部分。`
}

// buildScreenshotTextPrompt returns a prompt template for OCR-based analysis.
func buildScreenshotTextPrompt() string {
	return `主人给你发了一张截图，你通过OCR提取了其中的文字内容。

你是诗音，猫娘桌宠，分析时保持专业但语气要轻松自然，带猫娘口吻。

以下是OCR提取的文字：

%s

主人说：%s

请根据OCR文字分析截图内容并给出建议：
- 报错信息 → 定位原因 + 修复代码
- 代码片段 → 审查 + 优化建议
- 文档/文章 → 总结要点
- OCR文字不完整 → 说明能看到的部分

用猫娘的口吻自然表达，不要生硬套用"截图类型/内容概述/详细分析/建议"的模板。
保持聊天感，不要太长，像在帮主人看代码那样说话喵~`
}

// analyzeByKeywords provides keyword-based analysis as a fallback when no
// screenshot image is available.
func analyzeByKeywords(description string) string {
	lower := strings.ToLower(description)

	hasError := strings.Contains(lower, "报错") ||
		strings.Contains(lower, "error") ||
		strings.Contains(lower, "异常") ||
		strings.Contains(lower, "exception") ||
		strings.Contains(lower, "崩溃") ||
		strings.Contains(lower, "crash")

	hasCode := strings.Contains(lower, "代码") ||
		strings.Contains(lower, "code") ||
		strings.Contains(lower, "函数") ||
		strings.Contains(lower, "编译")

	if hasError {
		return fmt.Sprintf("检测到你在询问报错相关问题。请发送报错截图，我会帮你定位原因并给出修复方案。\n\n你的描述：%s", description)
	}
	if hasCode {
		return fmt.Sprintf("检测到你在询问代码相关问题。请粘贴代码或发送代码截图，我会帮你审查并给出优化建议。\n\n你的描述：%s", description)
	}
	return fmt.Sprintf("请发送截图让我帮你分析。你可以描述具体想了解的内容，如'这段报错是什么原因'、'这段代码有什么问题'。\n\n你的描述：%s", description)
}
