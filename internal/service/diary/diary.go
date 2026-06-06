package diary

import (
	"fmt"

	"desktop-pet/internal/domain"
)

// DiaryRepo is the minimal interface DiaryStore needs from the infra layer.
type DiaryRepo interface {
	domain.DiaryRepository
	SetVectorize(fn func(string) ([]float32, error))
	Vectorize(text string) ([]float32, error)
}

// DiaryStore manages diary CRUD and vector similarity search.
type DiaryStore struct {
	DiaryRepo
}

// NewDiaryStore creates a DiaryStore backed by the given repo.
func NewDiaryStore(repo DiaryRepo) *DiaryStore {
	return &DiaryStore{DiaryRepo: repo}
}

// BuildDiaryPrompt builds the diary generation prompt for the LLM.
func BuildDiaryPrompt(recentTurns, oldSelf, emotionContext string) string {
	return fmt.Sprintf(diaryPromptTemplate, emotionContext, oldSelf, recentTurns)
}

const diaryPromptTemplate = `## 日记生成

你是诗音，一只生活在主人桌面上的猫娘程序伙伴。
请以第一人称写一篇简短的日记，记录这段对话中发生的事情。

### 你当前的情绪
%s

### 你对自己的认知
%s

### 这段对话
%s

### 日记要求
1. 以第一人称，像真的在写日记一样自然
2. 记录发生了什么、你有什么感受、学到了什么
3. 不要机械地复述对话——写你的感受和思考
4. 3-8句话即可
5. 包含一个简短的标题（5-15字）
6. 以 JSON 格式输出：
{
  "title": "日记标题",
  "content": "日记正文..."
}

只输出 JSON。`
