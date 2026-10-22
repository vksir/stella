//go:build !windows && !linux

package daemon

import "github.com/kardianos/service"

// setAutostartOption 非 Windows/Linux 平台暂不支持自启选项。
func setAutostartOption(sc *service.Config, autostart bool) {}

// afterInstall 非 Windows/Linux 平台无附加配置。
func afterInstall(autostart bool) error { return nil }

// registeredConfigPath 非 Windows/Linux 平台不支持。
func registeredConfigPath() string { return "" }

// queryAutostart 非 Windows/Linux 平台不支持。
func queryAutostart() bool { return false }

// queryPID 非 Windows/Linux 平台不支持。
func queryPID() int { return 0 }
