package plugin

import (
	"qq-bot-go/pkg/event"
	"qq-bot-go/pkg/plugin/fanzhiyi"
	"qq-bot-go/pkg/plugin/girl"
	"qq-bot-go/pkg/plugin/luxun"
	"qq-bot-go/pkg/plugin/pixiv"
)

var Plugins []Handler

func LoadPlugins() {
	Register(Handler(New()))
	Register(Handler(luxun.New()))
	Register(Handler(pixiv.New()))
	Register(Handler(girl.New()))
	Register(fanzhiyi.New())
}

type Handler interface {
	Title() string
	Help() string
	Parse(text string) []string
	Handle(r event.Receive) *event.Send
}

func Register(h Handler) {
	Plugins = append(Plugins, h)
}
