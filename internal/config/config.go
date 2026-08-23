// Package config 负责加载和校验 stella.toml 配置。
package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// Config 是 stella.toml 的整体配置。
type Config struct {
	Server   ServerConfig   `toml:"server"`
	Model    ModelConfig    `toml:"model"`
	Log      LogConfig      `toml:"log"`
	Database DatabaseConfig `toml:"database"`
}

// ServerConfig 是 HTTP 服务器配置。
type ServerConfig struct {
	Listen string `toml:"listen"`
	WSPath string `toml:"ws_path"`
}

// ModelConfig 是模型服务配置。
type ModelConfig struct {
	Provider string `toml:"provider"`
	APIKey   string `toml:"api_key"`
	Model    string `toml:"model"`
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
	if c.Model.Provider == "" {
		return fmt.Errorf("model.provider is required")
	}
	if c.Model.APIKey == "" {
		return fmt.Errorf("model.api_key is required")
	}
	if c.Model.Model == "" {
		return fmt.Errorf("model.model is required")
	}
	return nil
}
