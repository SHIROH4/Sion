package chat

import (
	"fmt"
	"regexp"
	"strings"

	"desktop-pet/internal/domain"
)

// ---- regex vars ----

var (
	memorizeRe = regexp.MustCompile(`(?s)\[MEMORIZE\](.*?)\[/MEMORIZE\]`)
	recallRe   = regexp.MustCompile(`\[RECALL\s+([A-Za-z0-9\-]+)\]`)
)

// ---- MEMORIZE extractor ----

// ExtractMemorize scans messages for [MEMORIZE] markers and calls memorizeFn.
func ExtractMemorize(ctx *domain.ChatContext, memorizeFn func(content string)) {
	for i := len(ctx.Messages) - 1; i >= 0; i-- {
		m := ctx.Messages[i]
		if m.Role != "assistant" && m.Role != "user" {
			continue
		}
		matches := memorizeRe.FindAllStringSubmatch(m.Content, -1)
		for _, match := range matches {
			if len(match) > 1 {
				memorizeFn(strings.TrimSpace(match[1]))
			}
		}
		if len(matches) > 0 {
			ctx.Messages[i].Content = memorizeRe.ReplaceAllString(m.Content, "")
		}
	}
}

// ---- RECALL extractor ----

// ExtractRecall scans messages for [RECALL] markers and calls recallFn.
func ExtractRecall(ctx *domain.ChatContext, recallFn func(index string) (string, error)) {
	for i := len(ctx.Messages) - 1; i >= 0; i-- {
		m := ctx.Messages[i]
		if m.Role != "assistant" {
			continue
		}
		matches := recallRe.FindAllStringSubmatch(m.Content, -1)
		for _, match := range matches {
			if len(match) > 1 {
				index := strings.TrimSpace(match[1])
				content, err := recallFn(index)
				if err != nil {
					content = fmt.Sprintf("(未找到存档 %s)", index)
				}
				ctx.Messages = append(ctx.Messages[:i+1],
					append([]domain.Message{
						{Role: "system", Content: fmt.Sprintf("[存档回溯 %s]\n%s", index, content)},
					}, ctx.Messages[i+1:]...)...)
			}
		}
		if len(matches) > 0 {
			ctx.Messages[i].Content = recallRe.ReplaceAllString(m.Content, "")
		}
	}
}

// ---- Confirmation pattern extraction ----

var confirmPatterns = []struct {
	re      *regexp.Regexp
	factFmt string
}{
	{regexp.MustCompile(`生日(?:是|在|为)?\s*(\d{1,2}\s*[月\-.\/]\s*\d{1,2})[日号]?`), "主人生日是%s"},
	{regexp.MustCompile(`(?:叫|名字是?|称呼是?)\s*([^\s，。！？,.!?\n]{1,10})`), "主人叫%s"},
	{regexp.MustCompile(`用\s*([A-Za-z+#.\-]+(?:\s*(?:和|、|,|，)\s*[A-Za-z+#.\-]+)*)\s*(?:开发|编程|语言|写)`), "主人常用%s"},
	{regexp.MustCompile(`喜欢(?:玩|打|听|看|吃|喝|用)?\s*([^\s，。！？,.!?\n]{2,30})`), "主人喜欢%s"},
	{regexp.MustCompile(`爱(?:玩|打|听|看|吃|喝|用)?\s*([^\s，。！？,.!?\n]{2,30})`), "主人爱%s"},
	{regexp.MustCompile(`讨厌\s*([^\s，。！？,.!?\n]{2,30})`), "主人讨厌%s"},
	{regexp.MustCompile(`经常(?:玩|打|听|看|用)?\s*([^\s，。！？,.!?\n]{2,30})`), "主人经常%s"},
	{regexp.MustCompile(`平时(?:喜欢|用|在)?\s*([^\s，。！？,.!?\n]{2,30})`), "主人平时%s"},
}

var questionWords = []string{"什么", "谁", "哪", "怎么", "吗", "呢", "吧", "嘛", "如何", "为何", "多少", "几"}

func isGarbageValue(val string) bool {
	if val == "" {
		return true
	}
	for _, q := range questionWords {
		if strings.Contains(val, q) {
			return true
		}
	}
	if strings.ContainsAny(val, "?？！()（）…。、，*#") {
		return true
	}
	if strings.Contains(val, "的记录") || strings.Contains(val, "的信息") || strings.Contains(val, "别的称呼") {
		return true
	}
	return false
}

// ExtractConfirmations scans messages for confirmation patterns and calls memorizeFn.
func ExtractConfirmations(ctx *domain.ChatContext, store domain.MemoryStore, memorizeFn func(content string)) {
	if store == nil {
		return
	}
	seen := map[string]bool{}
	for _, f := range store.LoadFacts() {
		seen[strings.TrimSpace(f)] = true
	}
	for _, m := range ctx.Messages {
		if m.Role != "assistant" && m.Role != "user" {
			continue
		}
		if m.Role == "assistant" && (strings.Contains(m.Content, "？") || strings.Contains(m.Content, "?")) {
			continue
		}
		for _, cp := range confirmPatterns {
			matches := cp.re.FindAllStringSubmatch(m.Content, -1)
			for _, match := range matches {
				if len(match) > 1 {
					val := strings.TrimSpace(match[1])
					if isGarbageValue(val) {
						continue
					}
					fact := fmt.Sprintf(cp.factFmt, val)
					if !seen[fact] {
						seen[fact] = true
						memorizeFn(fact)
					}
				}
			}
		}
	}
}

// ---- Profile extraction ----

var (
	nameRe      = regexp.MustCompile(`(?:我叫|我是|称呼我?|叫我)\s*([^\s，。！？,.!?\n]{1,10})`)
	techStackRe = regexp.MustCompile(`(?:主要?用?|技术栈[是]?|做)\s*([A-Za-z+#.\-/]+(?:\s*[、,，和]\s*[A-Za-z+#.\-/]+)*)\s*(?:开发|编程)?`)
)

// ExtractProfile scans messages for user profile information.
func ExtractProfile(ctx *domain.ChatContext, store domain.MemoryStore, profile *domain.UserProfile) {
	if store == nil {
		return
	}
	for _, m := range ctx.Messages {
		if m.Role != "user" {
			continue
		}
		if match := nameRe.FindStringSubmatch(m.Content); len(match) > 1 {
			name := strings.TrimSpace(match[1])
			if name != "" && profile.Name == "" {
				store.SaveProfileValue("name", name)
				profile.Name = name
			}
		}
		if match := techStackRe.FindStringSubmatch(m.Content); len(match) > 1 {
			stack := parseStack(match[1])
			if len(stack) > 0 {
				existing := make(map[string]bool)
				for _, s := range profile.TechStack {
					existing[s] = true
				}
				for _, s := range stack {
					if !existing[s] {
						profile.TechStack = append(profile.TechStack, s)
					}
				}
				store.SaveProfileValue("tech_stack", strings.Join(profile.TechStack, ","))
			}
		}
	}
}

func parseStack(raw string) []string {
	parts := regexp.MustCompile(`[、,，和]\s*`).Split(raw, -1)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
