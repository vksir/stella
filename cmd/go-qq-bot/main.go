package main

import (
	"qq-bot-go/common"
	"qq-bot-go/confs"
	"qq-bot-go/internal/agent"
	"qq-bot-go/internal/server"
	"qq-bot-go/pkg/plugin"
)

func main() {

	fp := common.NewFilePath()
	fp.InitPath()
	confs.NewConf(fp)
	plugin.LoadPlugins()
	agent.LoadAgents()
	server.Run()
}
