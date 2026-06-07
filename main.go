package main

import (
	"context"
	"os"
	"path/filepath"
	"stella/api"
	"stella/api/handler"
	"stella/entity"
	"stella/internal/agent"
	"stella/internal/platform/qq"
	"stella/pkg/cache"
	"stella/pkg/cfg"
	"stella/pkg/database"
	"stella/pkg/workspace"

	"github.com/urfave/cli/v3"
	"github.com/vksir/vkiss-lib/pkg/log"
	"github.com/vksir/vkiss-lib/pkg/util/errutil"
	"github.com/vksir/vkiss-lib/pkg/util/fileutil"
	"github.com/vksir/vkiss-lib/pkg/util/installutil"
	"github.com/vksir/vkiss-lib/thirdpkg/systemctl"
)

// @title Stella AI Agent API
// @version 1.0
// @description Stella AI Agent 项目的 HTTP API
// @host localhost:5801
// @BasePath /
func main() {
	cmd := &cli.Command{
		Name: "stella",
		Commands: []*cli.Command{
			{
				Name: "install",
				Action: func(ctx context.Context, command *cli.Command) error {
					log.Init("", "debug")
					svc := &systemctl.Service{
						Name:             "stella",
						Description:      "stella",
						ExecStart:        "/opt/stella/stella -c /opt/stella/config.toml",
						RestartOnFailure: true,
					}
					return installutil.InstallService(svc, "/opt/stella/stella")
				},
			},
			{
				Name: "serve",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "config",
						Aliases: []string{"c"},
						Usage:   "config file",
					},
					&cli.StringFlag{
						Name:    "workspace",
						Aliases: []string{"w"},
						Value:   filepath.Dir(fileutil.Executable),
						Usage:   "workspace directory",
					},
				},
				Action: serve,
			},
		},
	}
	errutil.Check(cmd.Run(context.Background(), os.Args))
}

func serve(ctx context.Context, cmd *cli.Command) error {
	cfgFile := cmd.String("config")
	ws := cmd.String("workspace")

	// Init Config
	if cfgFile == "" {
		cfgFile = filepath.Join(ws, "config.toml")
	}
	err := cfg.Init(cfgFile, cfg.DefaultConfig)
	if err != nil {
		return errutil.Wrap(err)
	}

	// Init Workspace
	workspace.Init(ws)

	// Init Log
	log.Init(workspace.LogPath(), cfg.G.LogLevel)
	log.Warn("init cfg", "path", cfgFile)
	log.Warn("init workspace", "path", ws)

	// Init Others
	cache.Init(workspace.CachePath())
	database.Init(workspace.DBPath())

	// Init Agent
	logger := log.NewLogger("agent")
	agentInstance, err := agent.New(logger)
	if err != nil {
		return errutil.Wrap(err)
	}
	handler.Agent = agentInstance

	// Init QQ Adapter
	qqAdapter := qq.New()
	qqAdapter.SetChatFunc(func(ctx context.Context, userID string, evt *entity.Event) error {
		return agentInstance.Chat(ctx, userID, evt)
	})
	err = qqAdapter.Start(ctx)
	if err != nil {
		return errutil.Wrap(err)
	}

	// Start HTTP server with QQ WebSocket endpoint
	r := api.SetupRouter(qqAdapter)
	log.Info("Starting server on :5801")
	if err := r.Run(":5801"); err != nil {
		return errutil.Wrap(err)
	}

	return nil
}
