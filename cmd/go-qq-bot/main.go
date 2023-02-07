package main

import (
	"context"
	"qq-bot-go/internal/bot"
	"qq-bot-go/internal/common/config"
	"qq-bot-go/internal/common/logging"
	"qq-bot-go/internal/listener/terrariarun"
	"qq-bot-go/internal/plugin"
	"qq-bot-go/internal/server"
)

var log = logging.SugaredLogger()

func main() {
	log.Info("Hello stella ^_^")
	config.Read()
	plugin.Load()
	terrariaRunListener := terrariarun.NewListener()
	bot.Run(terrariaRunListener)
	if err := terrariaRunListener.Start(context.Background()); err != nil {
		panic(err)
	}
	server.Run()
}
