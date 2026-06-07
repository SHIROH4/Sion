package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// WarmStartConfig holds pre-seeded data for a better first-run experience.
type WarmStartConfig struct {
	Personality PersonalityWarmStart `mapstructure:"personality" json:"personality"`
	KnownFacts  []string             `mapstructure:"known_facts"  json:"known_facts"`
}

// PersonalityWarmStart allows pre-configuring the cat's personality.
type PersonalityWarmStart struct {
	AnnoyanceSensitivity float64 `mapstructure:"annoyance_sensitivity" json:"annoyance_sensitivity"`
	AffectionWarmth      float64 `mapstructure:"affection_warmth"      json:"affection_warmth"`
	WorryTendency        float64 `mapstructure:"worry_tendency"        json:"worry_tendency"`
}

// GlobalConfig holds all user-facing configuration for the desktop pet.
type GlobalConfig struct {
	LLMProvider    string                            `mapstructure:"llm_provider"   json:"llm_provider"`
	LLMAPIKey      string                            `mapstructure:"llm_api_key"    json:"llm_api_key"`
	LLMModel       string                            `mapstructure:"llm_model"       json:"llm_model"`
	LLMBaseURL     string                            `mapstructure:"llm_base_url"    json:"llm_base_url"`
	VisionModel    string                            `mapstructure:"vision_model"    json:"vision_model"`
	VisionAPIKey   string                            `mapstructure:"vision_api_key"   json:"vision_api_key"`
	VisionBaseURL  string                            `mapstructure:"vision_base_url"  json:"vision_base_url"`
	EmotionModel   string                            `mapstructure:"emotion_model"    json:"emotion_model"`
	EmotionAPIKey  string                            `mapstructure:"emotion_api_key"  json:"emotion_api_key"`
	EmotionBaseURL string                            `mapstructure:"emotion_base_url" json:"emotion_base_url"`
	EmbeddingModel string                            `mapstructure:"embedding_model"  json:"embedding_model"`
	UserName       string                            `mapstructure:"user_name"       json:"user_name"`
	UserTechStack  []string                          `mapstructure:"user_tech_stack" json:"user_tech_stack"`
	PluginsConfig  map[string]map[string]interface{} `mapstructure:"plugins"         json:"plugins"`
	WarmStart         WarmStartConfig                   `mapstructure:"warm_start"            json:"warm_start"`
	BingSearchAPIKey  string                            `mapstructure:"bing_search_api_key"    json:"bing_search_api_key"`
	BochaAPIKey       string                            `mapstructure:"bocha_api_key"          json:"bocha_api_key"`

	configPath string // resolved config file path, set by Load
}

// Load reads config.yaml from ~/.desktop-pet/ or the current directory.
// Missing files are silently ignored and defaults are used instead.
func Load() *GlobalConfig {
	v := viper.New()

	v.SetConfigName("config")
	v.SetConfigType("yaml")

	v.AddConfigPath(".")
	home, err := os.UserHomeDir()
	if err == nil {
		v.AddConfigPath(filepath.Join(home, ".desktop-pet"))
	}

	v.SetDefault("llm_provider", "deepseek")
	v.SetDefault("llm_model", "deepseek-chat")
	v.SetDefault("llm_base_url", "https://api.deepseek.com")
	v.SetDefault("vision_model", "")
	v.SetDefault("vision_api_key", "")
	v.SetDefault("vision_base_url", "")
	v.SetDefault("emotion_model", "")
	v.SetDefault("emotion_api_key", "")
	v.SetDefault("emotion_base_url", "")
	v.SetDefault("user_name", "主人")

	_ = v.BindEnv("llm_api_key")
	_ = v.BindEnv("llm_provider")
	_ = v.BindEnv("llm_model")
	_ = v.BindEnv("llm_base_url")
	_ = v.BindEnv("vision_model")
	_ = v.BindEnv("vision_api_key")
	_ = v.BindEnv("vision_base_url")
	_ = v.BindEnv("emotion_model")
	_ = v.BindEnv("emotion_api_key")
	_ = v.BindEnv("emotion_base_url")

	if err := v.ReadInConfig(); err != nil {
		log.Println("config: no config file found, using defaults")
	}

	var cfg GlobalConfig
	if err := v.Unmarshal(&cfg); err != nil {
		log.Println("config: parse error, using defaults")
	}

	// Store the config file path for Save.
	cfg.configPath = v.ConfigFileUsed()

	return &cfg
}

// configPath stores the resolved config file path so Save knows where to write.
func (c *GlobalConfig) configFilePath() string {
	if c.configPath != "" {
		return c.configPath
	}
	home, err := os.UserHomeDir()
	if err == nil {
		return filepath.Join(home, ".desktop-pet", "config.yaml")
	}
	return "config.yaml"
}

// Save writes the current GlobalConfig back to the config file as YAML.
func Save(cfg *GlobalConfig) error {
	v := viper.New()
	v.SetConfigFile(cfg.configFilePath())

	v.Set("llm_provider", cfg.LLMProvider)
	v.Set("llm_api_key", cfg.LLMAPIKey)
	v.Set("llm_model", cfg.LLMModel)
	v.Set("llm_base_url", cfg.LLMBaseURL)
	v.Set("vision_model", cfg.VisionModel)
	v.Set("vision_api_key", cfg.VisionAPIKey)
	v.Set("vision_base_url", cfg.VisionBaseURL)
	v.Set("emotion_model", cfg.EmotionModel)
	v.Set("emotion_api_key", cfg.EmotionAPIKey)
	v.Set("emotion_base_url", cfg.EmotionBaseURL)
	v.Set("user_name", cfg.UserName)
	v.Set("user_tech_stack", cfg.UserTechStack)
	v.Set("embedding_model", cfg.EmbeddingModel)
	v.Set("bing_search_api_key", cfg.BingSearchAPIKey)
	v.Set("bocha_api_key", cfg.BochaAPIKey)
	v.Set("warm_start", cfg.WarmStart)
	if cfg.PluginsConfig != nil {
		v.Set("plugins", cfg.PluginsConfig)
	}

	return v.WriteConfig()
}
