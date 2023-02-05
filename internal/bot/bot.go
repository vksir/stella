package bot

import (
	"qq-bot-go/internal/bot/mirai"
)

type Bot interface {
	Name() string
	Start() error
	listenMsg()
	handleEvent()
}

func LoadAgents() {
	bot := mirai.NewHandler()
	if err := bot.Start(); err != nil {
		panic(err)
	}
}
