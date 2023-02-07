package bot

import (
	"context"
	mirai2 "qq-bot-go/internal/bot/mirai"
)

type Bot interface {
	Name() string
	Start() error
	listenMsg()
	handleEvent()
}

func LoadAgents() {
	bot := mirai2.NewMirai()
	handler := mirai2.NewHandler(bot.SendChannel())
	bot.RegisterRecvChannel(handler.Channel())
	if err := bot.Start(); err != nil {
		panic(err)
	}
	if err := handler.Start(context.Background()); err != nil {
		panic(err)
	}
}
