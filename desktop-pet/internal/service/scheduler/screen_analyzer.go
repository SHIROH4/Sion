package scheduler

import (
	"desktop-pet/internal/domain"
	"fmt"
	"strings"
	"sync"
	"time"
)

const l2AnalyzeCooldown = 30 * time.Minute

// ScreenAnalyzer performs L2 screen analysis by sending a screenshot to a
// cloud multimodal LLM and returning a natural language description.
type ScreenAnalyzer struct {
	mu            sync.Mutex
	llmCall       func([]domain.Message) (string, error)
	lastAnalyzeAt time.Time
}

// NewScreenAnalyzer creates a ScreenAnalyzer backed by the given LLM caller.
func NewScreenAnalyzer(llmCall func([]domain.Message) (string, error)) *ScreenAnalyzer {
	return &ScreenAnalyzer{llmCall: llmCall}
}

// ShouldAnalyze returns true if the L2 cooldown has elapsed.
func (a *ScreenAnalyzer) ShouldAnalyze() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return time.Since(a.lastAnalyzeAt) >= l2AnalyzeCooldown
}

// Analyze sends a screenshot (base64-encoded PNG) to the multimodal LLM and
// returns a short, natural Chinese description of what the user is doing.
func (a *ScreenAnalyzer) Analyze(imageBase64 string, appName, windowTitle string) (string, error) {
	a.mu.Lock()
	a.lastAnalyzeAt = time.Now()
	a.mu.Unlock()

	prompt := fmt.Sprintf(`这张截图显示了电脑屏幕上当前的内容。

当前活跃应用: %s
窗口标题: %s

请用1-2句中文自然描述：
1. 主人在用什么软件做什么
2. 主人在做什么类型的任务（编程/看视频/聊天/写文档...）
3. 如果主人在工作，描述一下工作内容和大致进度

回答要自然口语化，像朋友好奇地看了一眼屏幕后的感想。不要太正式，不要像报告。可以带一点猫娘的好奇心（你是猫娘诗音）。`, appName, windowTitle)

	msgs := []domain.Message{
		{
			Role:    "user",
			Content: prompt,
			Images: []domain.Image{
				{Base64: imageBase64, Format: "png"},
			},
		},
	}

	result, err := a.llmCall(msgs)
	if err != nil {
		return "", fmt.Errorf("screen analyzer: %w", err)
	}

	return strings.TrimSpace(result), nil
}

// LastAnalyzeAt returns the timestamp of the most recent L2 analysis.
func (a *ScreenAnalyzer) LastAnalyzeAt() time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastAnalyzeAt
}
