// Package config 负责加载和校验 stella.toml 配置。
package config

import (
	"fmt"
	"slices"

	"github.com/BurntSushi/toml"
)

// Config 是 stella.toml 的整体配置。
type Config struct {
	Server   ServerConfig   `toml:"server"`
	Model    ModelConfig    `toml:"model"`
	Onebot   OnebotConfig   `toml:"onebot"`
	Log      LogConfig      `toml:"log"`
	Database DatabaseConfig `toml:"database"`
}

// OnebotConfig 是 OneBot 适配器配置。
type OnebotConfig struct {
	AccessToken  string `toml:"access_token"`
	SystemPrompt string `toml:"system_prompt"`
}

// ServerConfig 是 HTTP 服务器配置。
type ServerConfig struct {
	Listen string `toml:"listen"`
	WSPath string `toml:"ws_path"`
}

// ModelConfig 是模型服务配置，支持多 provider 与多模型。
type ModelConfig struct {
	// Temperature 与 ReasoningEffort 是作用于所有模型的全局生成参数，模型级配置可覆盖。
	Temperature     *float64 `toml:"temperature"`
	ReasoningEffort string   `toml:"reasoning_effort"`
	// DefaultProvider 与 DefaultModel 指向默认使用的模型，仅一个可选项时可省略。
	DefaultProvider string                    `toml:"default_provider"`
	DefaultModel    string                    `toml:"default_model"`
	Provider        map[string]ProviderConfig `toml:"provider"`
}

// ProviderConfig 是单个 provider 的连接配置与可用模型。
type ProviderConfig struct {
	APIKey string       `toml:"api_key"`
	Models []ModelEntry `toml:"models"`
}

// ModelEntry 是单个模型定义，Temperature 与 ReasoningEffort 未设置时继承全局。
type ModelEntry struct {
	Model           string   `toml:"model"`
	Temperature     *float64 `toml:"temperature"`
	ReasoningEffort string   `toml:"reasoning_effort"`
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
	temperature := c.Model.Temperature
	if entry.Temperature != nil {
		temperature = entry.Temperature
	}
	reasoningEffort := c.Model.ReasoningEffort
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

// LogConfig 是日志配置。
type LogConfig struct {
	Level string `toml:"level"`
	File  string `toml:"file"`
}

// DatabaseConfig 是数据库配置。
type DatabaseConfig struct {
	Path string `toml:"path"`
}

// Load 读取并解析配置文件，返回带默认值的配置。
func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
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
	if c.Server.WSPath == "" {
		c.Server.WSPath = "/adapter/onebot"
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Database.Path == "" {
		c.Database.Path = "data/stella.db"
	}
}

func (c *Config) validate() error {
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
