package chat

import (
	"strings"
	"testing"

	"desktop-pet/internal/infra/config"
)

func TestBuildSystemPrompt(t *testing.T) {
	cfg := &config.GlobalConfig{UserName: "测试用户", UserTechStack: []string{"Go", "React"}}
	result := BuildSystemPrompt(cfg)

	if !strings.Contains(result, "测试用户") {
		t.Error("should contain user name")
	}
	if !strings.Contains(result, "Go") {
		t.Error("should contain tech stack item Go")
	}
	if !strings.Contains(result, "React") {
		t.Error("should contain tech stack item React")
	}
	if !strings.Contains(result, "诗音") {
		t.Error("should contain character name")
	}
	if !strings.Contains(result, "猫娘") {
		t.Error("should contain cat-ear character trait")
	}
}

func TestBuildSystemPrompt_DefaultName(t *testing.T) {
	cfg := &config.GlobalConfig{UserName: "主人", UserTechStack: nil}
	result := BuildSystemPrompt(cfg)

	if !strings.Contains(result, "主人") {
		t.Error("should contain default user name")
	}
}

func TestBuildSystemPrompt_BehaviorRules(t *testing.T) {
	cfg := &config.GlobalConfig{UserName: "test"}
	result := BuildSystemPrompt(cfg)

	if !strings.Contains(result, "<tool_rules>") {
		t.Error("should contain tool rules section")
	}
	if !strings.Contains(result, "web_search") {
		t.Error("should contain web_search rule")
	}
	if !strings.Contains(result, "get_memory") {
		t.Error("should contain get_memory rule")
	}
	if !strings.Contains(result, "Memorize") {
		t.Error("should contain Memorize rule")
	}
	if !strings.Contains(result, "必须调用") {
		t.Error("should contain mandatory calling language")
	}
}
