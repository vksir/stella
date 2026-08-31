//go:build windows

package tool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

func configureShellProcess(cmd *exec.Cmd) {}

func killProcessTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	if err := exec.Command(filepath.Join(root, "System32", "taskkill.exe"), "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run(); err != nil {
		_ = cmd.Process.Kill()
	}
}
