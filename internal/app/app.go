// Package app 组装配置、日志、数据库与服务器，管理应用生命周期。
package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/vksir/stella/internal/adapter/llm"
	"github.com/vksir/stella/internal/adapter/store"
	"github.com/vksir/stella/internal/adapter/tool"
	bookkeeping "github.com/vksir/stella/internal/extension/bookkeeping"
	"github.com/vksir/stella/internal/core/agent"
	"github.com/vksir/stella/internal/core/hub"
	"github.com/vksir/stella/internal/infra/config"
	"github.com/vksir/stella/internal/infra/database"
	"github.com/vksir/stella/internal/infra/logx"
	"github.com/vksir/stella/internal/transport/onebot"
	"github.com/vksir/stella/internal/transport/server"
)

const shutdownTimeout = 5 * time.Second

func newAgentOptions(cfg *config.Config, sessionStore agent.Store) ([]agent.Option, error) {
	llm, err := llm.NewLLM(cfg.Model)
	if err != nil {
		return nil, err
	}
	opts := []agent.Option{
		agent.WithLLM(llm),
		agent.WithStore(sessionStore),
		agent.WithTools(tool.NewRead(), tool.NewWrite(), tool.NewBash(), tool.NewEdit()),
	}
	return opts, nil
}

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

	return run(cfg)
}

func run(cfg *config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var sessionStore agent.Store
	var db *sql.DB
	var err error
	switch cfg.Store.Type {
	case "memory":
		sessionStore = store.NewMemory()
		// 记账扩展使用内存 SQLite，随进程消亡
		db, err = sql.Open("sqlite", ":memory:")
		if err != nil {
			return fmt.Errorf("open memory sqlite: %w", err)
		}
		db.SetMaxOpenConns(1)
	case "database":
		db, err = database.Open(cfg.Database)
		if err != nil {
			return err
		}
		sessionStore, err = store.NewDatabase(ctx, db)
		if err != nil {
			return err
		}
		slog.Info("database opened", "path", cfg.Database.Path)
	default:
		return fmt.Errorf("unsupported store type %q", cfg.Store.Type)
	}
	defer db.Close()

	bkStore, err := bookkeeping.NewStore(ctx, db)
	if err != nil {
		return err
	}
	// 记账扩展按私聊用户构造，发图函数由 onebot 提供
	extensionFactory := func(userID int64, sendImage func([]byte)) []agent.Extension {
		return []agent.Extension{bookkeeping.New(bkStore, userID, sendImage)}
	}

	opts, err := newAgentOptions(cfg, sessionStore)
	if err != nil {
		return err
	}
	stopOnebot := startOnebot(ctx, cfg, opts, extensionFactory)
	defer stopOnebot()
	return runHTTPServer(ctx, cfg)
}

func startOnebot(ctx context.Context, cfg *config.Config, opts []agent.Option, extensionFactory onebot.ExtensionFactory) func() {
	agentHub := hub.NewAgentHub(0)
	wsCtx, cancelWS := context.WithCancel(ctx)
	wsDone := make(chan struct{})
	go func() {
		defer close(wsDone)
		onebot.New(agentHub, cfg.Onebot.URL, cfg.Onebot.AccessToken, extensionFactory, opts...).Run(wsCtx)
	}()
	return func() {
		cancelWS()
		<-wsDone
	}
}

func runHTTPServer(ctx context.Context, cfg *config.Config) error {
	mux := http.NewServeMux()
	srv := server.New(cfg.Server, mux)
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
