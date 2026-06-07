// internal/plugin/plugin.go
package plugin

import (
	"stella/entity"
	"stella/internal/plugin/fanzhiyi"
	"stella/internal/plugin/girl"
	"stella/internal/plugin/luxun"
	"stella/internal/plugin/pixiv"
	"stella/internal/plugin/repeater"
)

var plugins []Interface

type Interface interface {
	Name() string
	Help() string
	Parse(string) []string
	Handle(*entity.PluginMessage) *entity.PluginMessage
}

type Result struct {
	PluginName string
	Message    *entity.PluginMessage
}

func HandleByAllPlugins(req *entity.PluginMessage) []*Result {
	return Handle(req, plugins)
}

func Handle(req *entity.PluginMessage, plugins []Interface) []*Result {
	var results []*Result
	for _, p := range plugins {
		if resp := p.Handle(req); resp != nil {
			results = append(results, &Result{
				PluginName: p.Name(),
				Message:    resp,
			})
		}
	}
	return results
}

func Load() {
	register(NewHelp())
	register(repeater.New())
	register(luxun.New())
	register(pixiv.New())
	register(girl.New())
	register(fanzhiyi.New())
}

func register(h Interface) {
	plugins = append(plugins, h)
}
