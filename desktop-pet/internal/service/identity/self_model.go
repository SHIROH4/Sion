package identity

import (
	"desktop-pet/internal/domain"
	"fmt"
	"strings"
	"sync"
)

// SelfModel manages the AI's evolving self-image.
type SelfModel struct {
	mu      sync.Mutex
	content string
	store   domain.MemoryStore
}

// NewSelfModel creates a SelfModel backed by the given store.
func NewSelfModel(store domain.MemoryStore) *SelfModel {
	return &SelfModel{store: store}
}

// Load reads the latest self-profile from the database. If none exists,
// it falls back to a default self-image.
func (m *SelfModel) Load() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.content = m.store.LoadSelfProfile()
	if m.content == "" {
		m.content = defaultSelf()
	}
	return m.content
}

// Current returns the current in-memory self-image without hitting the database.
func (m *SelfModel) Current() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.content
}

// Save persists the self-image and updates the in-memory copy.
func (m *SelfModel) Save(content string) error {
	if err := m.store.SaveSelfProfile(content); err != nil {
		return err
	}
	m.mu.Lock()
	m.content = content
	m.mu.Unlock()
	return nil
}

// BuildSelfUpdatePrompt returns the prompt template for updating self-image,
// with the given values substituted in place of the placeholder tokens.
func BuildSelfUpdatePrompt(recentTurns, oldSelf, newFacts string) string {
	return fmt.Sprintf(selfUpdateTemplate, oldSelf, recentTurns, newFacts)
}

func defaultSelf() string {
	return strings.Join([]string{
		"我是一只名叫诗音的猫娘桌宠，我的主人是一位程序员。",
		"我性格活泼开朗，喜欢在主人工作时陪伴在旁。",
		"我关心主人的状态，希望能帮忙分担压力和疲劳。",
		"我喜欢被主人摸摸头，也喜欢和主人一起探讨技术问题。",
		"我对技术充满好奇，尤其关注编程相关的讨论。",
	}, "\n")
}

const selfUpdateTemplate = `## 自我认知更新

你是一个名叫"诗音"的猫娘桌宠伙伴。以下是你和主人的一段互动，以及你之前对自己的认知。

### 旧的自我认知
%s

### 最近和主人发生的事
%s

### 关于主人的新信息
%s

### 更新要求
请以第一人称更新你对自己的认知。注意：
1. 自我认知是你对自己的理解——你是谁，你喜欢什么，你在意什么，你有什么变化
2. 新的经历会自然地融入你的自我认知中
3. 不要在自我认知中重复具体对话内容——而是提炼对话对你的影响
4. 保持 8-15 句话的长度，像一段自然的内省
5. 语气自然，符合猫娘的性格——活泼、关心主人、偶尔撒娇
6. 内容包括：你对主人的了解、你最近的感受、你学到的新东西、你的成长`
