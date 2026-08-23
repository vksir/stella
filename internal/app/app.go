// Package app 组装配置、日志、数据库与服务器，管理应用生命周期。
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/vksir/stella/internal/config"
	"github.com/vksir/stella/internal/database"
	"github.com/vksir/stella/internal/logx"
	"github.com/vksir/stella/internal/server"
)

const shutdownTimeout = 5 * time.Second

// Run 启动应用，收到中断信号后优雅退出。
func Run(cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	logger, err := logx.New(cfg.Log)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)

	db, err := database.Open(cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	slog.Info("database opened", "path", cfg.Database.Path)

	srv := server.New(cfg.Server)
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}
	slog.Info("server stopped")
	return nil
}
