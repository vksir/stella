package cfg

import (
	_ "embed"

	"github.com/BurntSushi/toml"
	"github.com/vksir/vkiss-lib/pkg/util/errutil"
	"github.com/vksir/vkiss-lib/pkg/util/fileutil"
)

const (
	ModelTypeOpenAI = "openai"

	ModelUseChat      = "chat"
	ModelUseChatFlash = "chat_flash"
	ModelUseEmbed     = "embed"
)

//go:embed config.toml
var DefaultConfig string

type ConfigModel struct {
	Name    string `toml:"name"`
	Type    string `toml:"type"`
	Use     string `toml:"use"`
	BaseUrl string `toml:"base_url"`
	ApiKey  string `toml:"api_key"`
}

type ConfigMcp struct {
	Name     string `toml:"name"`
	BaseUrl  string `toml:"base_url"`
	Protocol string `toml:"protocol"`
}

type Config struct {
	Listen   string        `toml:"listen"`
	Host     string        `toml:"host"`
	LogLevel string        `toml:"log_level"`
	Model    []ConfigModel `toml:"model"`
	Mcp      []ConfigMcp   `toml:"mcp"`
}

var G *Config

func Init(path string, defaultConfig string) error {
	G = &Config{
		LogLevel: "warn",
	}

	if !fileutil.Exist(path) {
		err := fileutil.Write(path, []byte(defaultConfig))
		if err != nil {
			return errutil.Wrap(err)
		}
	}

	_, err := toml.DecodeFile(path, G)
	if err != nil {
		return errutil.Wrap(err)
	}
	return nil
}
