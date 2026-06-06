package memory

import (
	"desktop-pet/internal/domain"; "desktop-pet/internal/infra"
	"encoding/json"
	"fmt"
	"strings"
)

// ExtractMemCells analyses a conversation turn via LLM and extracts MemCells.
// Returns nil + nil when no noteworthy information is found.
// Importance uses dual-track scoring: LLM judgment + deterministic rules take max (G4).
func ExtractMemCells(
	llmSync func([]domain.Message) (string, error),
	turnMessages []domain.Message,
	emotion domain.EmotionState,
) ([]domain.MemCell, error) {
	if llmSync == nil {
		return nil, nil
	}

	prompt := buildMemCellPrompt(turnMessages, emotion)

	result, err := llmSync([]domain.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return nil, nil
	}

	cells, err := parseMemCellJSON(result)
	if err != nil {
		return nil, nil
	}

	var filtered []domain.MemCell
	for _, c := range cells {
		// Dual-track: emotion boost × LLM score, then take max with deterministic rules.
		llmScore := c.Importance * (1 + emotion.Intensity*0.3)
		if llmScore > 1 {
			llmScore = 1
		}
		c.Importance = max(llmScore, DeterministicImportance(c.Content, emotion))
		if c.Importance >= 0.5 {
			filtered = append(filtered, c)
		}
	}

	return filtered, nil
}

// deterministicImportance applies rule-based importance scoring that does NOT
// depend on LLM judgment. This catches preferences and personal info that the
// LLM might undervalue (G4).
func DeterministicImportance(content string, emotion domain.EmotionState) float64 {
	score := 0.0

	// High-emotion moments: always important.
	if emotion.Intensity > 0.7 {
		score = max(score, 0.8)
	}

	// Personal identity markers: very important.
	if matchAny(content,
		"我是", "我叫", "我的名字", "我住在", "我来自",
		"我的生日", "我的电话", "我的邮箱", "我的GitHub",
	) {
		score = max(score, 0.85)
	}

	// Preferences: should always be remembered.
	if matchAny(content,
		"我喜欢", "我爱", "我讨厌", "我不喜欢",
		"我经常", "我习惯", "我一般", "我平时",
		"我最爱", "我最喜欢", "我最讨厌",
	) {
		score = max(score, 0.6)
	}

	// Specific facts with concrete data.
	if containsDigit(content) || containsDatePattern(content) {
		score = max(score, 0.7)
	}

	// Skills and abilities.
	if matchAny(content, "我会", "我擅长", "我在学", "我可以", "我能") {
		score = max(score, 0.65)
	}

	return score
}

// --- small helpers (no external deps) ---

func matchAny(s string, patterns ...string) bool {
	for _, p := range patterns {
		if len(s) >= len(p) {
			for i := 0; i <= len(s)-len(p); i++ {
				if s[i:i+len(p)] == p {
					return true
				}
			}
		}
	}
	return false
}

func containsDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func containsDatePattern(s string) bool {
	return matchAny(s, "月", "日", "年", "号", "点")
}

// buildMemCellPrompt builds the LLM prompt for memory cell extraction.
func buildMemCellPrompt(turnMessages []domain.Message, emotion domain.EmotionState) string {
	var sb strings.Builder
	for _, m := range turnMessages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, m.Content))
	}

	return fmt.Sprintf(memCellPromptTemplate, emotion.Valence, emotion.Arousal, sb.String())
}

// parseMemCellJSON parses the LLM JSON response into domain.MemCell slice.
func parseMemCellJSON(raw string) ([]domain.MemCell, error) {
	raw = infra.CleanJSON(raw)
	var cells []domain.MemCell
	if err := json.Unmarshal([]byte(raw), &cells); err != nil {
		return nil, err
	}
	return cells, nil
}

const memCellPromptTemplate = `## 记忆提取

分析以下对话，判断是否有值得记录的信息。不是每条消息都值得记——
日常寒暄、无信息量的对话可以直接跳过。

### 当前情绪
愉悦度:%.2f 唤醒度:%.2f

### 对话内容
%s

### 判断标准
值得记录的信息类型：
- fact: 具体事实（"生日是X月X日"、"在X公司工作"）
- prefer: 偏好（"喜欢VSCode"、"爱玩RPG游戏"、"讨厌开会"）← 主人提到的任何偏好都必须记录！
- event: 有意义的事件（"完成了第一个PR"、"熬夜debug成功"）
- emotion: 情绪时刻（"因为bug崩溃大哭"）
- skill: 技能/知识（"会Rust"、"在学习K8s"）
- relation: 关系变化（"第一次主动关心主人"）

**重要：主人提到的任何关于自己的信息——哪怕看起来是闲聊——都应该被记录。**
特别是：
- 喜欢/不喜欢什么（游戏类型、食物、音乐、编辑器...）
- 做过什么（玩过什么游戏、去过哪里、经历过什么...）
- 习惯和日常（作息、工作方式、休闲方式...）
这些信息构成主人的画像，即使 importance 不高也要记录（importance >= 0.5）。

**内容质量要求：**
- content 必须是完整的独立陈述句，读者不需要看上下文就能理解
- 例如"主人叫白羽"是好的，"主人叫嘛"、"主人叫和用的"是坏的——信息不完整
- 不要记录对话片段或推断出来的猜测
- 如果信息不完整或模糊，宁可跳过不要记录

**去重检查：**
- 如果这条信息之前已经记录过，不要重复记录
- 例如之前已有"主人叫白羽"→ 不要再输出"主人名字叫白羽"、"用户名叫白羽"的同义变体
- 更新性的事件例外（如完成了特定任务、产生了新想法）

不值得记录的：
- 纯日常寒暄（"晚安"、"今天天气真好"）
- 无信息的技术问答（纯代码问题，无个人信息）
- 与之前记录完全重复的信息

### 输出格式
如果没有值得记录的信息，输出空的 JSON 数组: []
否则输出:
[{"type":"prefer","content":"主人喜欢玩RPG游戏","importance":0.6}]

importance 值:
- 0.9+: 核心个人信息（生日、姓名、重要约定）
- 0.7-0.8: 有价值的偏好或事件
- 0.5-0.6: 一般偏好和日常信息（如游戏喜好、食物偏好）← 大部分偏好在这里

只输出 JSON 数组。`
