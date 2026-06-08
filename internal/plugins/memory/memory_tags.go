package memory

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"desktop-pet/internal/app/plugin"
	"desktop-pet/internal/domain"
	svcmemory "desktop-pet/internal/service/memory"
)

var (
	MemorizeFact  = svcmemory.MemorizeFact
	RecallArchive = svcmemory.RecallArchive
)

// Search looks up archives and facts by keyword.
func (p *MemoryPlugin) Search(keyword string) (string, error) {
	return svcmemory.SearchMemory(p.store, keyword)
}

// Memorize saves a permanent memory fact.
// Accepts raw JSON args like {"content": "..."} and parses them.
func (p *MemoryPlugin) Memorize(argsJSON string) (string, error) {
	var args struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err == nil && args.Content != "" {
		return svcmemory.MemorizeFact(p.store, args.Content)
	}
	return svcmemory.MemorizeFact(p.store, argsJSON)
}

// Recall retrieves the full original messages behind a compressed archive.
func (p *MemoryPlugin) Recall(index string) (string, error) {
	return svcmemory.RecallArchive(p.store, index)
}

// UpdateSelfProfile invokes the LLM to update the self-model.
func (p *MemoryPlugin) UpdateSelfProfile(llmSync func([]plugin.Message) (string, error)) error {
	if p.selfModel == nil || p.sessionBuf == nil {
		return nil
	}
	recent := p.sessionBuf.Recent(10)
	if len(recent) == 0 {
		return nil
	}
	var turns []string
	for _, m := range recent {
		turns = append(turns, fmt.Sprintf("[%s]: %s", m.Role, m.Content))
	}
	recentTurns := strings.Join(turns, "\n")
	var newFacts string
	if p.store != nil {
		facts := p.store.ListActiveFacts(ActiveThreshold)
		if len(facts) > 0 {
			var lines []string
			for i, f := range facts {
				if i >= 5 {
					break
				}
				lines = append(lines, "- "+f.Content)
			}
			newFacts = strings.Join(lines, "\n")
		}
	}
	prompt := BuildSelfUpdatePrompt(recentTurns, p.selfModel.Current(), newFacts)
	msgs := []plugin.Message{{Role: "user", Content: prompt}}
	result, err := llmSync(msgs)
	if err != nil {
		return err
	}
	return p.selfModel.Save(strings.TrimSpace(result))
}

// RegisterFunctions registers AI-callable memory functions.
func (p *MemoryPlugin) RegisterFunctions(reg *plugin.FunctionRegistry) {
	reg.RegisterWithParams("get_memory",
		"Search long-term memory for past conversations, events, or user info. "+
			"Call when the user references past discussions (\"之前\"/\"上次\"/\"还记得\"), "+
			"asks you to recall something, or tests your memory. "+
			"Do NOT call for general knowledge questions — use web_search for those.",
		p.GetMemory,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"description": map[string]any{
					"type":        "string",
					"description": "要检索的内容描述",
				},
			},
			"required": []string{"description"},
		},
	)
	reg.RegisterWithParams("Memorize",
		"Permanently store important user information. "+
			"Call when the user explicitly shares facts about themselves "+
			"(birthday, name, preferences, plans, habits). "+
			"Do NOT call for casual statements or opinions.",
		p.Memorize,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": "要记住的内容",
				},
			},
			"required": []string{"content"},
		},
	)
}

// GetMemory is the LLM-callable memory retrieval function.
// Accepts raw JSON args like {"description": "..."} and parses them.
func (p *MemoryPlugin) GetMemory(argsJSON string) (string, error) {
	var args struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err == nil && args.Description != "" {
		argsJSON = args.Description // extract the real query
	}
	description := strings.TrimSpace(argsJSON)
	if description == "" {
		return "请提供要检索的内容描述", nil
	}
	var sb strings.Builder
	if p.sessionBuf != nil && p.sessionBuf.Len() > 0 {
		recent := p.sessionBuf.Recent(20)
		sb.WriteString(fmt.Sprintf("### 最近对话记录（共%d条消息）\n", p.sessionBuf.Len()))
		for _, m := range recent {
			role := "主人"
			if m.Role == "assistant" {
				role = "诗音"
			}
			sb.WriteString(fmt.Sprintf("[%s]: %s\n", role, m.Content))
		}
		sb.WriteString("\n")
	}
	useVector := false
	if p.embSvc != nil && p.store != nil {
		queryVec, err := p.embSvc.Vectorize(description)
		if err == nil {
			useVector = true
			candidates, _ := p.store.UnifiedSearch(queryVec, description, 10)
			topResults := p.rerankWithLLM(candidates, description, 5)
			if len(topResults) > 0 {
				sb.WriteString("### 相关记忆\n")
				for _, r := range topResults {
					sourceTag := map[string]string{"fact": "[事实]", "diary": "[日记]"}[r.Source]
					sb.WriteString(fmt.Sprintf("- %s%s %s\n", sourceTag, relativeTimeTag(r), r.Content))
				}
			}
		}
	}
	if !useVector {
		if p.store != nil {
			factResults, _ := p.store.SearchArchives(description, 5)
			for _, r := range factResults {
				if r.Source == "fact" {
					sb.WriteString(fmt.Sprintf("- [核心记忆] %s\n", r.Summary))
				}
			}
		}
		if p.store != nil {
			cells := p.store.ListMemCells("", 10)
			keyword := strings.ToLower(description)
			for _, c := range cells {
				if strings.Contains(strings.ToLower(c.Content), keyword) ||
					strings.Contains(strings.ToLower(string(c.Type)), keyword) {
					typeTag := svcmemory.MemCellTypeTag(domain.MemCellType(c.Type))
					sb.WriteString(fmt.Sprintf("- [%s] %s\n", typeTag, c.Content))
				}
			}
		}
		if p.diaryStore != nil {
			vec, _ := p.diaryStore.Vectorize(description)
			if vec != nil {
				entries, _ := p.diaryStore.Search(vec, 5)
				for i, e := range entries {
					timeStr := time.Unix(e.CreatedAt, 0).Format("2006-01-02 15:04")
					sb.WriteString(fmt.Sprintf("%d. [%s] %s %s\n   %s\n",
						i+1, timeStr, EmotionTag(e.EmotionValence, e.EmotionArousal), e.Title, e.Summary))
				}
			}
		}
	}
	if sb.Len() == 0 {
		return "没有找到相关的记忆", nil
	}
	return sb.String(), nil
}
