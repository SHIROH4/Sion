package memory

import (
	"desktop-pet/internal/domain"; "desktop-pet/internal/infra"
	"encoding/json"
	"fmt"
	"strings"
)

// ExtractAtomicFacts extracts independent, verifiable atomic facts from
// conversation turns via LLM. Returns nil + nil when no facts are found.
func ExtractAtomicFacts(
	llmSync func([]domain.Message) (string, error),
	turnMessages []domain.Message,
	existingFacts []domain.FactEntry,
) ([]domain.AtomicFactInput, error) {
	if llmSync == nil {
		return nil, nil
	}

	var existingLines strings.Builder
	if len(existingFacts) > 0 {
		existingLines.WriteString("### 已有事实（避免重复）\n")
		for _, f := range existingFacts {
			existingLines.WriteString(fmt.Sprintf("- %s\n", f.Content))
		}
	} else {
		existingLines.WriteString("（暂无已有事实）")
	}

	var convText strings.Builder
	for _, m := range turnMessages {
		convText.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, m.Content))
	}

	prompt := fmt.Sprintf(atomicFactExtractionPrompt, convText.String(), existingLines.String())

	result, err := llmSync([]domain.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return nil, nil
	}

	facts, err := parseAtomicFactJSON(result)
	if err != nil || len(facts) == 0 {
		return nil, nil
	}

	// Deduplicate against existing facts.
	facts = deduplicateFacts(facts, existingFacts)

	return facts, nil
}

// deduplicateFacts removes exact-match duplicates from a batch of new facts
// against existing facts.
func deduplicateFacts(newFacts []domain.AtomicFactInput, existing []domain.FactEntry) []domain.AtomicFactInput {
	// Build a set of existing contents for O(1) lookup.
	existingSet := make(map[string]bool, len(existing))
	for _, ef := range existing {
		existingSet[strings.TrimSpace(ef.Content)] = true
	}

	var result []domain.AtomicFactInput
	seen := make(map[string]bool, len(newFacts))
	for _, nf := range newFacts {
		key := strings.TrimSpace(nf.Content)
		if existingSet[key] || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, nf)
	}
	return result
}

// parseAtomicFactJSON parses the LLM JSON response into domain.AtomicFactInput slice.
func parseAtomicFactJSON(raw string) ([]domain.AtomicFactInput, error) {
	raw = infra.CleanJSON(raw)
	var facts []domain.AtomicFactInput
	if err := json.Unmarshal([]byte(raw), &facts); err != nil {
		return nil, err
	}
	return facts, nil
}

// noisePrefixes lists fact content prefixes that indicate noise rather than real facts.
var noisePrefixes = []string{
	"主人问", "主人询问", "主人叫", "主人说了", "主人对诗音说",
	"主人当前时间", "现在时间", "主人说了晚安", "主人打了招呼",
	"用户问题不完整", "无法提取", "[不完整]",
}

// isNoiseFact returns true if the fact content is noise that should not be stored.
func IsNoiseFact(content string) bool {
	content = strings.TrimSpace(content)
	if len([]rune(content)) < 5 {
		return true // too short, likely garbled
	}
	for _, p := range noisePrefixes {
		if strings.HasPrefix(content, p) {
			return true
		}
	}
	// Truncated/ending mid-word (common with LLM token limits).
	if strings.HasSuffix(content, "-s") || strings.HasSuffix(content, "-") {
		return true
	}
	return false
}

const atomicFactExtractionPrompt = `## 原子事实提取

你是诗音的记忆提取系统。从以下对话中提取**关于主人的、独立的、可验证的原子事实**。

### 🚫 绝对不要提取的内容

以下类型的事实**绝对不要提取**，直接跳过：

- **关于诗音（AI）的事实**: "诗音建议…"、"诗音用QQ头像"、"诗音想换形象" — 你不是在记录自己
- **问题本身**: "主人问了XXX"、"主人询问XXX"、"主人问XXX" — 问题是对话，不是事实
- **对话填充词**: "好啦好啦"、"嗯嗯"、"哦"、"好的" — 无信息量
- **纯寒暄**: "主人说了晚安"、"主人打了招呼" — 不计入记忆
- **临时问答**: "主人当前时间是X"、"现在时间是X" — 瞬时状态不存储
- **用户问题不完整**: "[不完整]"、"[无法提取]" — 这不是事实，是错误信息
- **对AI说的话而非事实**: "主人对诗音说…" 这种元描述

### ✅ 应该提取的事实

- **身份信息**: 姓名、年龄、职业、学校、所在地
- **偏好习惯**: "主人喜欢王者荣耀"、"主人用Go语言写后端"
- **技能擅长**: "主人会Rust"、"主人擅长debug"
- **近期事件**: "主人今天修好了一个并发bug"、"主人昨天吃了煲仔饭"
- **计划决定**: "主人计划下周发布项目"、"主人决定重构记忆系统"
- **情绪状态** (仅当明确表达): "主人今天修bug修到崩溃"

### 提取规则

1. **原子性**: 每条只包含一个可验证的事实
   - "主人喜欢王者荣耀" ✅
   - "主人喜欢王者荣耀和瓦罗兰特" ❌ — 拆成两条

2. **重要性分级** (0.1~1.0):
   - 1.0: 永久身份信息 (姓名、生日、GitHub)
   - 0.8: 稳定偏好和习惯 (技术栈、游戏偏好、作息)
   - 0.6: 近期事件和决定 (本周做了什么、计划)
   - 0.4: 临时状态和情绪 (今天累了)
   - **⚠️ importance < 0.3 不提取**
   - 身份标记≥0.85、偏好≥0.6、技能≥0.65、含数字/日期≥0.7

3. **内容格式**: 第三人称，主语明确，30字以内，不含代词

4. **去重**: 语义相同的事实不要重复提取

### 对话内容
%s

### 已有事实（避免重复）
%s

### 时间字段说明
- start_time / end_time 使用 Unix 时间戳（秒）。0 表示不适用。
- core / preference / skill / causal 事实: start_time=0, end_time=0
- temporal 事实 (今天/昨天/本周): start_time 为事件发生时间，end_time 为过期时间
  - "今天吃了煲仔饭" → start_time=今天中午, end_time=明天0点
  - "昨天修了bug" → start_time=昨天, end_time=今天0点
- plan 事实: start_time=计划创建时间, end_time=计划截止时间
- ⚠️ 日期范围约束:
  - 过去的日期必须在近一个月内（基于当前对话时间推算）
  - 节日/纪念日不要猜测具体年份，使用最近的一次
  - "今天是六一儿童节" → 用今天的日期，不是2024年

### 输出格式
JSON 数组，每条包含 confidence 字段表示你对"这确实是事实而非猜测"的确信度:
[{"content": "主人喜欢王者荣耀", "importance": 0.8, "confidence": 0.9, "fact_role": "core", "start_time": 0, "end_time": 0}]

confidence 评分指南:
- 0.9-1.0: 用户明确陈述的事实 ("我叫张三"、"我用Go")
- 0.7-0.8: 从对话中可推断的事实 (用户反复提到某个工具)
- 0.5-0.6: 单次提及、可能有歧义
- 0.3-0.4: 不确定、可能是玩笑或夸张
- <0.3: 不提取

如果没有值得提取的事实，输出空数组 []。

只输出 JSON 数组。`
