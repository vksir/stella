package plugin

import (
	"stella/internal/plugin/event"
	"stella/internal/plugin/fanzhiyi"
	"stella/internal/plugin/girl"
	"stella/internal/plugin/luxun"
	"stella/internal/plugin/pixiv"
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

func HandleDefault(req *event.Event) []*Event {
	return Handle(req, plugins)
}

func Handle(req *event.Event, plugins []Interface) []*Event {
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

func Load() {
	register(NewHelp())
	register(luxun.New())
	register(pixiv.New())
	register(girl.New())
	register(fanzhiyi.New())
}

func register(h Interface) {
	plugins = append(plugins, h)
}
