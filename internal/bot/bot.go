package bot

import (
	"context"
	"qq-bot-go/internal/bot/mirai"
	"qq-bot-go/internal/listener/terrariarun"
)

func Run(terrariaRunListener *terrariarun.Listener) {
	bot := mirai.NewMirai()

	handler := mirai.NewHandler(bot.SendChannel())
	bot.RegisterRecvChannel(handler.RecvChannel())
	terrariaRunListener.RegisterChannel(handler.TerrariaRunReportChannel)

	if err := bot.Start(); err != nil {
		panic(err)
	}
	if err := handler.Start(context.Background()); err != nil {
		panic(err)
	}
}
