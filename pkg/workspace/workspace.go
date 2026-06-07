package workspace

import (
	"os"
	"path/filepath"

	"github.com/vksir/vkiss-lib/pkg/util/errutil"
)

var ws string

func Ws() string {
	return ws
}

func LogDir() string {
	return filepath.Join(ws, "log")
}

func CachePath() string {
	return filepath.Join(ws, "cache.json")
}

func DBPath() string {
	return filepath.Join(ws, "stella.db")
}

func LogPath() string {
	return filepath.Join(LogDir(), "stella.log")
}

func Init(workspace string) {
	ws = workspace
	dirs := []string{ws, LogDir()}
	for _, d := range dirs {
		err := os.MkdirAll(d, 0o755)
		errutil.Check(err)
	}
}
