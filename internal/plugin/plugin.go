package plugin

import (
	"qq-bot-go/internal/event"
	"qq-bot-go/internal/plugin/fanzhiyi"
	"qq-bot-go/internal/plugin/girl"
	"qq-bot-go/internal/plugin/luxun"
	"qq-bot-go/internal/plugin/pixiv"
)

var plugins []Interface

type Interface interface {
	Name() string
	Help() string
	Parse(string) []string
	Handle(*event.Event) *event.Event
}

type Event struct {
	PluginName string
	RespEvent  *event.Event
}

func Handle(req *event.Event) []*Event {
	var events []*Event
	for _, p := range plugins {
		if resp := p.Handle(req); resp != nil {
			events = append(events, &Event{
				PluginName: p.Name(),
				RespEvent:  resp,
			})
		}
	}
	return events
}

func LoadPlugins() {
	register(NewHelp())
	register(luxun.New())
	register(pixiv.New())
	register(girl.New())
	register(fanzhiyi.New())
}

func register(h Interface) {
	plugins = append(plugins, h)
}
