package config

// DefaultPluginConfigs returns the default configuration for all plugins.
func DefaultPluginConfigs() map[string]map[string]interface{} {
	return map[string]map[string]interface{}{
		"chat": {
			"enabled": true,
		},
		"memory": {
			"enabled":               true,
			"emotion_cloud_enabled": false,
			"compressor": map[string]interface{}{
				"enabled":      true,
				"l0_threshold": 20,
			},
			"diary": map[string]interface{}{
				"merge_enabled":    true,
				"merge_similarity": 0.85,
			},
			"learning": map[string]interface{}{
				"enabled":        true,
				"qa_max_per_day": 3,
				"qa_max_pending": 5,
			},
			"dream": map[string]interface{}{
				"enabled": true,
			},
			"forget": map[string]interface{}{
				"history_days":    30,
				"diary_days":      90,
				"decay_threshold": 0.1,
			},
			"self_update": map[string]interface{}{
				"interval_days": 7,
			},
			"background": map[string]interface{}{
				"interval_sec": 300,
			},
		},
		"vision": {
			"enabled":              true,
			"screen_cool_down_sec": 1800,
			"ocr_enabled":          true,
			"cloud_enabled":        true,
		},
		"qq": {
			"enabled":    false,
			"app_id":     "",
			"app_secret": "",
		},
		"care": {
			"enabled": true,
			"hydration": map[string]interface{}{
				"interval_min": 45,
				"cooldown_min": 30,
			},
			"meal": map[string]interface{}{
				"interval_min": 240,
				"lunch_hour":   12,
				"dinner_hour":  19,
			},
			"rest": map[string]interface{}{
				"continuous_work_min": 90,
				"break_min":           5,
			},
			"encourage": map[string]interface{}{
				"enabled": true,
			},
			"max_daily": 20,
			"escalation": map[string]interface{}{
				"enabled": true,
			},
		},
		"scheduler": {
			"enabled":              true,
			"interval_min":         15,
			"emotion_weight":       0.4,
			"context_weight":       0.3,
			"loneliness_threshold": 0.6,
			"worry_threshold":      0.5,
			"curiosity_threshold":  0.4,
			"cooldown_min":         10,
			"dedup_window_min":     60,
		},
	}
}

// GetPluginConfig returns the config for a named plugin. If no config exists,
// it returns the default.
func (c *GlobalConfig) GetPluginConfig(name string) map[string]interface{} {
	if c.PluginsConfig != nil {
		if cfg, ok := c.PluginsConfig[name]; ok {
			return cfg
		}
	}
	defs := DefaultPluginConfigs()
	if cfg, ok := defs[name]; ok {
		return cfg
	}
	return nil
}

// SetPluginConfig sets the config for a named plugin.
func (c *GlobalConfig) SetPluginConfig(name string, cfg map[string]interface{}) {
	if c.PluginsConfig == nil {
		c.PluginsConfig = make(map[string]map[string]interface{})
	}
	c.PluginsConfig[name] = cfg
}
