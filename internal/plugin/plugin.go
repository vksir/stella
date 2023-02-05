package plugin

import (
	"qq-bot-go/internal/plugin/fanzhiyi"
	"qq-bot-go/internal/plugin/girl"
	"qq-bot-go/internal/plugin/luxun"
	"qq-bot-go/internal/plugin/pixiv"
	"qq-bot-go/pkg/event"
)

var Plugins []Handler

func LoadPlugins() {
	Register(New())
	Register(luxun.New())
	Register(pixiv.New())
	Register(girl.New())
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
