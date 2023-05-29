package comm

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

var StellaHome = filepath.Join(Home(), "stella")
var ConfigPath = filepath.Join(StellaHome, "stella.toml")
var LogPath = filepath.Join(StellaHome, "stella.log")

func initPath() {
	if err := MkDir(StellaHome); err != nil {
		panic(err)
	}
}
