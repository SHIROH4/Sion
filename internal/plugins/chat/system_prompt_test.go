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

	// Tools are now passed via API tools field, not in system prompt.
	// System prompt should only contain identity, user, time, self_and_emotion.
	if !strings.Contains(result, "<identity>") {
		t.Error("should contain identity section")
	}
	if !strings.Contains(result, "<user>") {
		t.Error("should contain user section")
	}
	if !strings.Contains(result, "<time>") {
		t.Error("should contain time section")
	}
	if !strings.Contains(result, "<self_and_emotion>") {
		t.Error("should contain self_and_emotion section")
	}
	// Verify tool_rules is NOT in the prompt.
	if strings.Contains(result, "<tool_rules>") {
		t.Error("tool_rules should NOT be in system prompt — tools are via API field")
	}
}
