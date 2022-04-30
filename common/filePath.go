package common

import (
	"log"
	"os"
	"path/filepath"
)

type FilePath struct {
	Home       string
	ConfigDir  string
	ConfigPath string
	LogPath    string
}

func NewFilePath() *FilePath {
	f := FilePath{}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalln(err)
	}
	f.Home = home
	f.ConfigDir = filepath.Join(f.Home, ".qq_bot")
	f.ConfigPath = filepath.Join(f.ConfigDir, "config.yaml")
	f.LogPath = filepath.Join(f.ConfigDir, "qq_bot.log")
	return &f
}

func (f *FilePath) InitPath() {
	makeDir(f.ConfigDir)
}

func makeDir(path string) {
	err := os.MkdirAll(path, 655)
	if err != nil {
		log.Fatalln(err)
	}
}
