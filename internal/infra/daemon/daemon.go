// Package daemon 将 stella 注册为 OS 服务（Windows Service / systemd）后台托管。
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kardianos/service"

	"github.com/vksir/stella/internal/app"
	"github.com/vksir/stella/internal/infra/config"
)

const (
	// Name 服务名，全平台统一。
	Name = "stella"
	// stopTimeout 等待应用退出的上限。
	stopTimeout = 15 * time.Second
	// defaultLogFile daemon 模式下日志兜底路径（相对配置目录）。
	defaultLogFile = "data/stella.log"
)

// program 把服务启停回调映射到应用生命周期。
type program struct {
	cfgPath string
	cancel  context.CancelFunc
	done    chan struct{}
}

func (p *program) Start(service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})
	go func() {
		defer close(p.done)
		if err := app.RunService(ctx, p.cfgPath); err != nil {
			slog.Error("daemon exited with error", "err", err)
		}
	}()
	return nil
}

func (p *program) Stop(service.Service) error {
	if p.cancel == nil {
		return nil
	}
	p.cancel()
	select {
	case <-p.done:
	case <-time.After(stopTimeout):
		slog.Warn("daemon shutdown timed out")
	}
	return nil
}

func newService(cfgPath string, autostart bool) (service.Service, error) {
	abs, err := filepath.Abs(cfgPath)
	if err != nil {
		return nil, err
	}
	sc := &service.Config{
		Name:        Name,
		DisplayName: "Stella Agent",
		Description: "Stella LLM chat agent daemon",
		Arguments:   []string{"daemon", "run", "--config", abs},
	}
	setAutostartOption(sc, autostart)
	return service.New(&program{cfgPath: abs}, sc)
}

// hintPrivilege 为权限不足类错误附加提权提示。
func hintPrivilege(err error) error {
	if err == nil {
		return nil
	}
	if !strings.Contains(strings.ToLower(err.Error()), "denied") {
		return err
	}
	if runtime.GOOS == "windows" {
		return fmt.Errorf("%w (administrator privileges required)", err)
	}
	return fmt.Errorf("%w (root privileges required, run with sudo)", err)
}

// Install 显式注册服务；已安装时报错。autostart 仅在注册时生效。
func Install(cfgPath string, autostart bool) error {
	svc, err := newService(cfgPath, autostart)
	if err != nil {
		return err
	}
	if err := svc.Install(); err != nil {
		return hintPrivilege(err)
	}
	return afterInstall(autostart)
}

// Start 幂等启动：已运行则无事；已安装未运行则启动；未安装则先注册再启动。
func Start(cfgPath string, autostart bool) error {
	svc, err := newService(cfgPath, autostart)
	if err != nil {
		return err
	}
	status, serr := svc.Status()
	if serr == nil {
		switch status {
		case service.StatusRunning:
			return nil
		case service.StatusStopped:
			return hintPrivilege(svc.Start())
		}
	}
	// 未安装或 failed 状态：安装（容忍已存在）后启动
	if err := svc.Install(); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return hintPrivilege(err)
		}
	} else if err := afterInstall(autostart); err != nil {
		return err
	}
	return hintPrivilege(svc.Start())
}

// Stop 停止服务。
func Stop() error {
	svc, err := newService("", true)
	if err != nil {
		return err
	}
	return hintPrivilege(svc.Stop())
}

// Restart 重启服务。
func Restart() error {
	svc, err := newService("", true)
	if err != nil {
		return err
	}
	return hintPrivilege(svc.Restart())
}

// Uninstall 先停止再注销服务。
func Uninstall() error {
	svc, err := newService("", true)
	if err != nil {
		return err
	}
	_ = svc.Stop()
	if err := svc.Uninstall(); err != nil {
		return hintPrivilege(err)
	}
	return nil
}

// Run 服务模式入口，由 SCM/systemd 调起。
func Run(cfgPath string) error {
	abs, err := filepath.Abs(cfgPath)
	if err != nil {
		return err
	}
	// 相对路径（db/log）与前台运行保持一致
	if err := os.Chdir(filepath.Dir(abs)); err != nil {
		return err
	}
	svc, err := newService(abs, true)
	if err != nil {
		return err
	}
	return svc.Run()
}

// Status 服务详情。
type Status struct {
	Installed   bool
	State       string // running / stopped / not installed
	PID         int    // 0 表示未知
	Autostart   bool
	ConfigPath  string
	LogPath     string
}

// QueryStatus 汇总服务状态、注册参数与日志路径。
func QueryStatus() (Status, error) {
	svc, err := newService("", true)
	if err != nil {
		return Status{}, err
	}
	st := Status{}
	status, serr := svc.Status()
	switch {
	case serr == nil:
		st.Installed = true
		if status == service.StatusRunning {
			st.State = "running"
		} else {
			st.State = "stopped"
		}
	case errors.Is(serr, service.ErrNotInstalled):
		st.State = "not installed"
		return st, nil
	default:
		return Status{Installed: true, State: "unknown"}, serr
	}

	st.ConfigPath = registeredConfigPath()
	st.Autostart = queryAutostart()
	st.PID = queryPID()
	if st.ConfigPath != "" {
		if cfg, err := config.Load(st.ConfigPath); err == nil {
			logFile := cfg.Log.File
			if logFile == "" {
				logFile = defaultLogFile
			}
			if !filepath.IsAbs(logFile) {
				logFile = filepath.Join(filepath.Dir(st.ConfigPath), logFile)
			}
			st.LogPath = logFile
		}
	}
	return st, nil
}

// tokenize 按空白切分，支持双引号包裹的段（ImagePath / ExecStart）。
func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case !inQuote && (r == ' ' || r == '\t'):
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// configFromArgs 从命令参数中提取 --config 值。
func configFromArgs(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--config" || args[i] == "-c" {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
		if v := strings.TrimPrefix(args[i], "--config="); v != args[i] {
			return v
		}
		if v := strings.TrimPrefix(args[i], "-c="); v != args[i] {
			return v
		}
	}
	return ""
}
