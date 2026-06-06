package memory

import (
	"encoding/json"
	"fmt"
	"strings"

	"desktop-pet/internal/domain"
)

// MemorizeFact saves a permanent memory fact.
func MemorizeFact(store domain.MemoryStore, content string) (string, error) {
	if store == nil {
		return "", fmt.Errorf("store not initialised")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return "", nil
	}
	if err := store.SaveFact(content, "chat"); err != nil {
		return "", err
	}
	return content, nil
}

// RecallArchive retrieves the full original messages behind a compressed archive.
func RecallArchive(store domain.MemoryStore, index string) (string, error) {
	if store == nil {
		return "", fmt.Errorf("store not initialised")
	}
	raw, err := store.FindArchiveByName(index)
	if err != nil {
		return "", fmt.Errorf("archive %s not found", index)
	}
	var msgs []domain.Message
	if err := json.Unmarshal([]byte(raw), &msgs); err != nil {
		return raw, nil
	}
	var sb strings.Builder
	for _, m := range msgs {
		sb.WriteString(fmt.Sprintf("[%s] %s\n", m.Role, m.Content))
	}
	return sb.String(), nil
}

// SearchMemory looks up archives and facts by keyword and returns formatted results.
func SearchMemory(store domain.MemoryStore, keyword string) (string, error) {
	if store == nil {
		return "", fmt.Errorf("store not initialised")
	}
	results, err := store.SearchArchives(keyword, 5)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return fmt.Sprintf("未找到与 '%s' 相关的记忆。", keyword), nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("搜索 '%s' 的结果：\n", keyword))
	for i, r := range results {
		prefix := ""
		if r.Source == "fact" {
			prefix = "[核心记忆]"
		} else {
			prefix = fmt.Sprintf("[L%d]", r.Level)
		}
		sb.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, prefix, r.Summary))
	}
	return sb.String(), nil
}
