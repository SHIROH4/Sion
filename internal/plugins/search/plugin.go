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
		`搜索互联网获取最新的实时信息。在以下情况下调用：
- 用户明确要求搜索、查找、查询最新信息
- 问题涉及实时信息（"今天"、"最新"、"最近"、"现在"）
- 具体技术问题你不确定（版本号、API变更、新特性、库的最新用法）
- 需要验证的事实性问题
不应在以下情况调用：常识问题、纯逻辑推理、数学计算、闲聊、你确定知道的内容`,
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
