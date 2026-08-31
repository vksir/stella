package config

import (
	"fmt"
	"net/url"
	"slices"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server   ServerConfig   `toml:"server"`
	Model    ModelConfig    `toml:"model"`
	Onebot   OnebotConfig   `toml:"onebot"`
	Log      LogConfig      `toml:"log"`
	Database DatabaseConfig `toml:"database"`
	Store    StoreConfig    `toml:"store"`
}

type OnebotConfig struct {
	URL         string `toml:"url"`
	AccessToken string `toml:"access_token"`
}

type ServerConfig struct {
	Listen string `toml:"listen"`
}

type ModelConfig struct {
	DefaultTemperature     *float64                  `toml:"default_temperature"`
	DefaultReasoningEffort string                    `toml:"default_reasoning_effort"`
	DefaultProvider        string                    `toml:"default_provider"`
	DefaultModel           string                    `toml:"default_model"`
	Provider               map[string]ProviderConfig `toml:"provider"`
}

type ProviderConfig struct {
	APIKey string       `toml:"api_key"`
	Models []ModelEntry `toml:"models"`
}

type ModelEntry struct {
	Model           string   `toml:"model"`
	Temperature     *float64 `toml:"temperature"`
	ReasoningEffort string   `toml:"reasoning_effort"`
	SupportsImages  bool     `toml:"supports_images"`
}

// DefaultModelInfo 是默认模型的解析结果，生成参数已合并全局默认值。
type DefaultModelInfo struct {
	Provider        string
	APIKey          string
	Model           string
	Temperature     *float64
	ReasoningEffort string
}

// DefaultModel 解析默认 provider 下的默认模型，融合模型级与全局生成参数。
func DefaultModel(c *Config) DefaultModelInfo {
	p := c.Model.Provider[c.Model.DefaultProvider]
	entry := &p.Models[0]
	for i := range p.Models {
		if p.Models[i].Model == c.Model.DefaultModel {
			entry = &p.Models[i]
			break
		}
	}
	temperature := c.Model.DefaultTemperature
	if entry.Temperature != nil {
		temperature = entry.Temperature
	}
	reasoningEffort := c.Model.DefaultReasoningEffort
	if entry.ReasoningEffort != "" {
		reasoningEffort = entry.ReasoningEffort
	}
	return DefaultModelInfo{
		Provider:        c.Model.DefaultProvider,
		APIKey:          p.APIKey,
		Model:           entry.Model,
		Temperature:     temperature,
		ReasoningEffort: reasoningEffort,
	}
}

type LogConfig struct {
	Level string `toml:"level"`
	File  string `toml:"file"`
}

type DatabaseConfig struct {
	Path string `toml:"path"`
}

type StoreConfig struct {
	Type string `toml:"type"`
}

func Load(path string) (*Config, error) {
	var cfg Config
	metadata, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	for _, key := range metadata.Undecoded() {
		if key.String() == "onebot.system_prompt" {
			return nil, fmt.Errorf("onebot.system_prompt is not supported")
		}
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Listen == "" {
		c.Server.Listen = ":5810"
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Store.Type == "" {
		c.Store.Type = "memory"
	}
	if c.Database.Path == "" {
		c.Database.Path = "data/stella.db"
	}
}

func (c *Config) validate() error {
	if c.Store.Type != "memory" && c.Store.Type != "database" {
		return fmt.Errorf("store.type must be memory or database, got %q", c.Store.Type)
	}
	u, err := url.Parse(c.Onebot.URL)
	if err != nil || u.Host == "" || (u.Scheme != "ws" && u.Scheme != "wss") {
		return fmt.Errorf("onebot.url must be a ws:// or wss:// URL with a host")
	}
	if len(c.Model.Provider) == 0 {
		return fmt.Errorf("model.provider is required")
	}
	for name, p := range c.Model.Provider {
		if p.APIKey == "" {
			return fmt.Errorf("model.provider.%s.api_key is required", name)
		}
		if len(p.Models) == 0 {
			return fmt.Errorf("model.provider.%s.models is required", name)
		}
		for _, m := range p.Models {
			if m.Model == "" {
				return fmt.Errorf("model.provider.%s.models[].model is required", name)
			}
		}
	}
	if c.Model.DefaultProvider == "" {
		if len(c.Model.Provider) > 1 {
			return fmt.Errorf("model.default_provider is required when multiple providers are configured")
		}
		for name := range c.Model.Provider {
			c.Model.DefaultProvider = name
		}
	}
	p, ok := c.Model.Provider[c.Model.DefaultProvider]
	if !ok {
		return fmt.Errorf("model.default_provider %q is not configured", c.Model.DefaultProvider)
	}
	names := make([]string, 0, len(p.Models))
	for _, m := range p.Models {
		names = append(names, m.Model)
	}
	switch {
	case c.Model.DefaultModel == "":
		if len(p.Models) > 1 {
			return fmt.Errorf("model.default_model is required when the default provider has multiple models")
		}
		c.Model.DefaultModel = p.Models[0].Model
	case !slices.Contains(names, c.Model.DefaultModel):
		return fmt.Errorf("model.default_model %q is not configured under provider %q", c.Model.DefaultModel, c.Model.DefaultProvider)
	}
	return nil
}
