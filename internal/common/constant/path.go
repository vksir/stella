package constant

import (
	"os"
	"path/filepath"
)

func Home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return h
}

func Workspace() string {
	p := filepath.Join(Home(), ".qq-bot")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		if err := os.Mkdir(p, 0755); err != nil {
			panic(err)
		}
	}
	return p
}

var ConfigPath = filepath.Join(Workspace(), "config.json")
var LogPath = filepath.Join(Workspace(), "qq-bot.log")
