package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"desktop-pet/internal/app/plugin"
	"desktop-pet/internal/service/tools"
)

// SearchPlugin implements Plugin and FunctionProvider, registering web_search
// as an LLM-callable function via the Bocha Search API.
type SearchPlugin struct {
	pctx     plugin.PluginContext
	bochaKey string
}

// NewPlugin returns a SearchPlugin as a plugin.Plugin interface.
func NewPlugin(bochaAPIKey string) plugin.Plugin {
	return &SearchPlugin{bochaKey: bochaAPIKey}
}

// Info returns plugin metadata.
func (p *SearchPlugin) Info() plugin.PluginInfo {
	return plugin.PluginInfo{
		Name:        "search",
		Version:     "0.1.0",
		Description: "联网搜索插件，支持博查 Web Search API，为 LLM 提供实时互联网搜索能力",
		Priority:    15,
		Requires:    []string{"chat"},
	}
}

// Awake stores the plugin context.
func (p *SearchPlugin) Awake(pctx plugin.PluginContext) error {
	p.pctx = pctx
	return nil
}

// Start is a no-op — search functions are registered passively via FunctionProvider.
func (p *SearchPlugin) Start() error { return nil }

// Stop is a no-op.
func (p *SearchPlugin) Stop() error { return nil }

// IsRunning reports whether the plugin is active.
func (p *SearchPlugin) IsRunning() bool { return true }

// RegisterFunctions registers web_search in the shared FunctionRegistry.
func (p *SearchPlugin) RegisterFunctions(reg *plugin.FunctionRegistry) {
	reg.RegisterWithParams(
		"web_search",
		"Search the web for real-time information. "+
			"Call when the user explicitly asks to search/lookup/query, "+
			"or asks about latest/recent/current facts you are unsure about "+
			"(e.g. version numbers, API changes, new features). "+
			"Do NOT call for: casual chat, common knowledge, math, logic puzzles, "+
			"or things you already know from training data.",
		p.handleSearch,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "搜索关键词，使用简洁的关键词而非完整句子",
				},
			},
			"required": []string{"query"},
		},
	)
}

// handleSearch executes a web search and returns formatted results for the LLM.
func (p *SearchPlugin) handleSearch(argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("解析搜索参数失败: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", fmt.Errorf("搜索关键词不能为空")
	}

	if p.bochaKey == "" {
		return "搜索功能未配置 API Key，请告知用户配置 bocha_api_key", nil
	}

	results, err := tools.SearchBochaAPI(context.Background(), query, p.bochaKey)
	if err != nil {
		slog.Warn("search: Bocha API failed", "err", err)
		return "搜索服务暂时不可用（API 额度可能已用完）。请基于已有知识回答，或告知用户稍后再试。", nil
	}
	if len(results) == 0 {
		return fmt.Sprintf("搜索 %q 未找到结果，请基于已有知识回答或建议用户换个关键词", query), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("搜索结果（共%d条）：\n", len(results)))
	for i, r := range results {
		if i >= 5 {
			break
		}
		snippet := r.Snippet
		if len([]rune(snippet)) > 200 {
			snippet = string([]rune(snippet)[:200]) + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n   摘要: %s\n   链接: %s\n", i+1, r.Title, snippet, r.URL))
	}
	return sb.String(), nil
}
