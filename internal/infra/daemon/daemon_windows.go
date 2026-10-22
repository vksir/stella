//go:build windows

package daemon

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/kardianos/service"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceRegistryPath = `SYSTEM\CurrentControlSet\Services\` + Name

// setAutostartOption 注册时设置启动类型（默认 automatic，仅 manual 需显式指定）。
func setAutostartOption(sc *service.Config, autostart bool) {
	if !autostart {
		sc.Option["StartType"] = "manual"
	}
}

// afterInstall 配置服务恢复策略：崩溃后自动重启。
func afterInstall(autostart bool) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect service manager: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(Name)
	if err != nil {
		return fmt.Errorf("open service %s: %w", Name, err)
	}
	defer s.Close()
	actions := []mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 10 * time.Second}}
	if err := s.SetRecoveryActions(actions, 86400); err != nil {
		return fmt.Errorf("set recovery actions: %w", err)
	}
	return nil
}

// registeredConfigPath 从注册表 ImagePath 解析 --config 参数。
func registeredConfigPath() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, serviceRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	imagePath, _, err := k.GetStringValue("ImagePath")
	if err != nil {
		return ""
	}
	args := tokenize(imagePath)
	if len(args) == 0 {
		return ""
	}
	return configFromArgs(args[1:])
}

// queryAutostart 注册表 StartType：2 = SERVICE_AUTO_START。
func queryAutostart() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, serviceRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	startType, _, err := k.GetIntegerValue("StartType")
	if err != nil {
		return false
	}
	return startType == 2
}

// queryPID 经 QueryServiceStatusEx 查询主进程 PID，普通权限即可。
func queryPID() int {
	mgrHandle, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return 0
	}
	defer windows.CloseServiceHandle(mgrHandle)

	namePtr, err := windows.UTF16PtrFromString(Name)
	if err != nil {
		return 0
	}
	svcHandle, err := windows.OpenService(mgrHandle, namePtr, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return 0
	}
	defer windows.CloseServiceHandle(svcHandle)

	var buf [unsafe.Sizeof(windows.SERVICE_STATUS_PROCESS{})]byte
	var needed uint32
	if err := windows.QueryServiceStatusEx(svcHandle, windows.SC_STATUS_PROCESS_INFO,
		&buf[0], uint32(len(buf)), &needed); err != nil {
		return 0
	}
ssp := (*windows.SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0]))
	return int(ssp.ProcessId)
}
