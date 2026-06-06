package background

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"desktop-pet/internal/domain"
)

// ReflectDecision is one LLM decision from the reflection prompt.
type reflectDecision struct {
	Action       string  `json:"action"`
	ID           int64   `json:"id,omitempty"`
	KeepID       int64   `json:"keep_id,omitempty"`
	DuplicateIDs []int64 `json:"duplicate_ids,omitempty"`
	OldID        int64   `json:"old_id,omitempty"`
	NewID        int64   `json:"new_id,omitempty"`
}

// ReflectAndForget runs LLM reflection on recent facts to merge duplicates,
// resolve contradictions, and archive stale entries. lastReflectAt is updated in-place.
func ReflectAndForget(
	store domain.MemoryStore,
	rawLLM func([]domain.Message) (string, error),
	lastReflectAt *time.Time,
	logFn func(msg string, args ...any),
) {
	if rawLLM == nil || store == nil {
		return
	}

	now := time.Now().Unix()
	since := lastReflectAt.Unix()
	if since == 0 {
		since = now - 30*86400
	}

	facts := store.GetRecentFacts(since)
	if len(facts) < 10 {
		return
	}

	var factsText strings.Builder
	for _, f := range facts {
		factsText.WriteString(fmt.Sprintf(
			"[id:%d] [role:%s] [src:%s] [imp:%.1f] [recall:%d] %s\n",
			f.ID, f.FactRole, f.Source, f.Importance, f.RecallCount, f.Content,
		))
	}

	result, err := rawLLM([]domain.Message{{Role: "user", Content: fmt.Sprintf(reflectAndForgetPrompt, len(facts), factsText.String(), "")}})
	if err != nil {
		return
	}

	type decision struct {
		Action       string  `json:"action"`
		ID           int64   `json:"id,omitempty"`
		KeepID       int64   `json:"keep_id,omitempty"`
		DuplicateIDs []int64 `json:"duplicate_ids,omitempty"`
		OldID        int64   `json:"old_id,omitempty"`
		NewID        int64   `json:"new_id,omitempty"`
	}

	var decisions []decision
	raw := strings.TrimSpace(result)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	if err := json.Unmarshal([]byte(raw), &decisions); err != nil {
		return
	}

	merged, corrected, staled := 0, 0, 0
	for _, d := range decisions {
		switch d.Action {
		case "merge":
			for _, dupID := range d.DuplicateIDs {
				if dupID != d.KeepID {
					store.ArchiveFact(dupID)
					merged++
				}
			}
		case "correct":
			store.ReplaceFact(d.OldID, d.NewID)
			corrected++
		case "stale":
			store.ArchiveFact(d.ID)
			staled++
		}
	}

	*lastReflectAt = time.Now()

	if logFn != nil && (merged+corrected+staled) > 0 {
		logFn("background: reflect done", "merged", merged, "corrected", corrected, "staled", staled)
	}
}

const reflectAndForgetPrompt = `## 记忆反思与清理

你是诗音的记忆管理模块。现在审查最近新增的 %d 条事实，清理不需要保留的内容。

### 审查规则
1. 合并 (merge): 多条事实表达同一个意思 → 保留 importance 最高的，归档其余
   - "白羽生日是10月14日" 和 "主人生日10月14" → 合并
2. 纠正 (correct): 新事实推翻了旧事实 → 归档旧事实
   - "主人改用Zig了" 推翻 "主人常用Go" → 归档旧事实
   - 注意: "主人也会Rust" 不推翻 "主人常用Go"，两者共存
3. 过时 (stale): temporal 类型且明显已过期 → 归档
   - "主人正在改bug" (3天前) → 过时
   - "主人凌晨写代码后睡觉很晚" (长期习惯) → 保留
4. 保留: 以上都不适用 → 不输出

### 最近事实
%s
%s

### 输出格式
JSON 数组，只输出需要处理的条目:
[
  {"action": "merge", "keep_id": 100, "duplicate_ids": [101, 102]},
  {"action": "correct", "old_id": 50, "new_id": 200},
  {"action": "stale", "id": 30}
]

只输出 JSON 数组。`
