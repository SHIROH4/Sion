package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg := Load()
	if cfg.LLMProvider != "deepseek" {
		t.Errorf("default LLMProvider = %q, want %q", cfg.LLMProvider, "deepseek")
	}
	if cfg.LLMModel != "deepseek-chat" {
		t.Errorf("default LLMModel = %q, want %q", cfg.LLMModel, "deepseek-chat")
	}
	if cfg.LLMBaseURL != "https://api.deepseek.com" {
		t.Errorf("default LLMBaseURL = %q, want %q", cfg.LLMBaseURL, "https://api.deepseek.com")
	}
	if cfg.UserName != "主人" {
		t.Errorf("default UserName = %q, want %q", cfg.UserName, "主人")
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgContent := []byte(`
llm_provider: claude
llm_api_key: sk-test-key
llm_model: claude-opus-4-8
llm_base_url: https://api.anthropic.com
user_name: "测试用户"
user_tech_stack:
  - Go
  - React
  - Python
`)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), cfgContent, 0644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cfg := Load()
	if cfg.LLMProvider != "claude" {
		t.Errorf("LLMProvider = %q, want %q", cfg.LLMProvider, "claude")
	}
	if cfg.LLMAPIKey != "sk-test-key" {
		t.Errorf("LLMAPIKey = %q, want %q", cfg.LLMAPIKey, "sk-test-key")
	}
	if cfg.LLMModel != "claude-opus-4-8" {
		t.Errorf("LLMModel = %q, want %q", cfg.LLMModel, "claude-opus-4-8")
	}
	if cfg.LLMBaseURL != "https://api.anthropic.com" {
		t.Errorf("LLMBaseURL = %q, want %q", cfg.LLMBaseURL, "https://api.anthropic.com")
	}
	if cfg.UserName != "测试用户" {
		t.Errorf("UserName = %q, want %q", cfg.UserName, "测试用户")
	}
	if len(cfg.UserTechStack) != 3 {
		t.Fatalf("UserTechStack len = %d, want 3", len(cfg.UserTechStack))
	}
	if cfg.UserTechStack[0] != "Go" || cfg.UserTechStack[1] != "React" || cfg.UserTechStack[2] != "Python" {
		t.Errorf("UserTechStack = %v, want [Go React Python]", cfg.UserTechStack)
	}
}

func TestLoadPartialFile(t *testing.T) {
	dir := t.TempDir()
	cfgContent := []byte(`
llm_provider: openai
user_name: "partial"
`)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), cfgContent, 0644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cfg := Load()
	if cfg.LLMProvider != "openai" {
		t.Errorf("LLMProvider = %q, want %q", cfg.LLMProvider, "openai")
	}
	if cfg.UserName != "partial" {
		t.Errorf("UserName = %q, want %q", cfg.UserName, "partial")
	}
	if cfg.LLMModel != "deepseek-chat" {
		t.Errorf("LLMModel default = %q, want %q", cfg.LLMModel, "deepseek-chat")
	}
	if cfg.LLMBaseURL != "https://api.deepseek.com" {
		t.Errorf("LLMBaseURL default = %q, want %q", cfg.LLMBaseURL, "https://api.deepseek.com")
	}
	if cfg.UserTechStack != nil && len(cfg.UserTechStack) != 0 {
		t.Errorf("UserTechStack should be empty, got %v", cfg.UserTechStack)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	os.Setenv("LLM_API_KEY", "env-key-123")
	defer os.Unsetenv("LLM_API_KEY")

	cfg := Load()
	if cfg.LLMAPIKey != "env-key-123" {
		t.Errorf("LLMAPIKey = %q, want %q (env override)", cfg.LLMAPIKey, "env-key-123")
	}
}
