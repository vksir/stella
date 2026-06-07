// internal/plugin/repeater/repeater.go
package repeater

import (
	"regexp"
	"stella/entity"
)

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) Name() string {
	return "复读姬"
}

func (h *Handler) Help() string {
	return "复读姬<复读内容>"
}

func (h *Handler) Parse(text string) []string {
	re := regexp.MustCompile(`^复读[姬机](.*)`)
	return re.FindStringSubmatch(text)
}

func (h *Handler) Handle(req *entity.PluginMessage) *entity.PluginMessage {
	text := req.Text()
	args := h.Parse(text)
	if len(args) == 0 {
		return nil
	}
	var resp entity.PluginMessage
	resp.Chains = append(resp.Chains, entity.Chain{
		Type: entity.ChainTypeText,
		Text: args[1],
	})
	return &resp
}
