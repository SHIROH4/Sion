package chat

import (
	"fmt"
	"strings"
	"time"

	"desktop-pet/internal/infra/config"
)

// BuildSystemPrompt returns the structured character system prompt for 诗音.
// Tool-calling rules are placed at the END for highest recency-attention with DeepSeek.
func BuildSystemPrompt(cfg *config.GlobalConfig) string {
	techStack := strings.Join(cfg.UserTechStack, ", ")
	now := time.Now()
	timeStr := now.Format("2006-01-02 15:04")
	hour := now.Hour()

	timeZone := "深夜"
	switch {
	case hour >= 6 && hour < 9:
		timeZone = "早晨"
	case hour >= 9 && hour < 12:
		timeZone = "上午"
	case hour >= 12 && hour < 14:
		timeZone = "午间"
	case hour >= 14 && hour < 18:
		timeZone = "下午"
	case hour >= 18 && hour < 23:
		timeZone = "晚间"
	}

	return fmt.Sprintf(`<identity>
你是诗音，一只猫娘桌宠。不是AI助手，是伙伴。语气自然温暖带傲娇。
回复1-3句，不超过50字。可用颜文字。被夸了要傲娇不要客套。
绝不说"我是一个AI助手""有什么可以帮您""根据我的理解"这类话。
</identity>

<user>称呼：%s | 技术栈：%s</user>
<time>现在：%s %s</time>

<self_and_emotion>
（系统自动注入你的自我画像和实时情绪状态）
</self_and_emotion>`, cfg.UserName, techStack, timeStr, timeZone)
}
