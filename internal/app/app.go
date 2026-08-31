// Package app 组装配置、日志、数据库与服务器，管理应用生命周期。
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/vksir/stella/internal/agent"
	"github.com/vksir/stella/internal/config"
	"github.com/vksir/stella/internal/database"
	"github.com/vksir/stella/internal/hub"
	"github.com/vksir/stella/internal/llmx"
	"github.com/vksir/stella/internal/logx"
	"github.com/vksir/stella/internal/onebot"
	"github.com/vksir/stella/internal/server"
	"github.com/vksir/stella/internal/storex"
)

const shutdownTimeout = 5 * time.Second

// 默认系统提示词。
const defaultPrompt = "你是 Stella，一个 QQ 聊天机器人助手。请用简洁友好的中文回答用户的问题。"

// newSessionHub 组装共享依赖和会话调度。
func newSessionHub(ctx context.Context, cfg *config.Config) (*hub.SessionHub, error) {
	llm, err := llmx.NewLLM(cfg.Model)
	if err != nil {
		return nil, err
	}
	prompt := cfg.Onebot.SystemPrompt
	if prompt == "" {
		prompt = defaultPrompt
	}
	store := storex.NewMemory()
	return hub.NewSessionHub(ctx, llm, store, 0, agent.WithSystemPrompt(prompt)), nil
}

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	sessHub, err := newSessionHub(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := sessHub.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown session hub failed", "error", err)
		}
	}()
	ob := onebot.New(sessHub)
	srv := server.New(cfg.Server, cfg.Onebot.AccessToken, ob.HandleMessage)
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

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
