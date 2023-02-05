package main

import (
	"qq-bot-go/confs"
	"qq-bot-go/internal/bot"
	"qq-bot-go/internal/common"
	"qq-bot-go/internal/common/logging"
	"qq-bot-go/internal/plugin"
	"qq-bot-go/internal/server"
)

var log = logging.SugaredLogger()

func main() {
	log.Info("Hello stella ^_^")
	fp := common.NewFilePath()
	fp.InitPath()
	confs.NewConf(fp)
	plugin.LoadPlugins()
	bot.LoadAgents()
	server.Run()
}
