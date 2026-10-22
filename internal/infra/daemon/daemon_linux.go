//go:build linux

package daemon

import (
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/kardianos/service"
)

const unitPath = "/etc/systemd/system/" + Name + ".service"

// setAutostartOption systemd 无需启动类型选项（Install 时统一 enable）。
func setAutostartOption(sc *service.Config, autostart bool) {}

// afterInstall 关闭自启时撤销 kardianos 安装时的 enable（Restart=always 为模板默认）。
func afterInstall(autostart bool) error {
	if autostart {
		return nil
	}
	if err := exec.Command("systemctl", "disable", Name).Run(); err != nil {
		return err
	}
	return nil
}

// registeredConfigPath 从 unit 文件 ExecStart 解析 --config 参数。
func registeredConfigPath() string {
	data, err := os.ReadFile(unitPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "ExecStart=") {
			continue
		}
		args := tokenize(strings.TrimPrefix(line, "ExecStart="))
		if len(args) == 0 {
			return ""
		}
		return configFromArgs(args[1:])
	}
	return ""
}

// queryAutostart 经 systemctl is-enabled 查询自启状态。
func queryAutostart() bool {
	out, err := exec.Command("systemctl", "is-enabled", Name).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "enabled"
}

// queryPID 经 systemctl show 查询主进程 PID。
func queryPID() int {
	out, err := exec.Command("systemctl", "show", Name, "--property=MainPID", "--value").Output()
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return pid
}
