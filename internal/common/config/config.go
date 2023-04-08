package config

import (
	"encoding/json"
	"os"
	"qq-bot-go/assets"
	"qq-bot-go/internal/common/constant"
)

var CFG *Config

func GetConfig() *Config {
	if CFG == nil {
		Read()
	}
	return CFG
}

func Read() {
	if _, err := os.Stat(constant.ConfigPath); os.IsNotExist(err) {
		panic(err)
	}
	bytes, err := os.ReadFile(constant.ConfigPath)
	if err != nil {
		panic(err)
	}
	CFG = &Config{}
	if err := json.Unmarshal(bytes, &CFG); err != nil {
		panic(err)
	}
}

func Write() {
	bytes, err := json.MarshalIndent(CFG, "", "    ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(constant.ConfigPath, bytes, 0644); err != nil {
		panic(err)
	}
}

func init() {
	setDefault()
}

func setDefault() {
	if f, _ := os.Stat(constant.ConfigPath); f != nil {
		return
	}
	bytes, err := assets.FS.ReadFile("assets/config.json")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(constant.ConfigPath, bytes, 0644); err != nil {
		panic(err)
	}
}
