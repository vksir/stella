// internal/plugin/fanzhiyi/fan_zhi_yi.go
package fanzhiyi

import (
	"regexp"
	"stella/entity"
)

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) Name() string {
	return "范志毅说"
}

func (h *Handler) Help() string {
	return "范志毅说<要说的话>"
}

func (h *Handler) Parse(text string) []string {
	re := regexp.MustCompile(`范志毅说(.*)`)
	return re.FindStringSubmatch(text)
}

func (h *Handler) Handle(req *entity.PluginMessage) *entity.PluginMessage {
	text := req.Text()
	args := h.Parse(text)
	if len(args) == 0 {
		return nil
	}

	var resp entity.PluginMessage
	if data, err := getImgBase64(args[1]); err != nil {
		resp.Chains = append(resp.Chains, entity.Chain{
			Type: entity.ChainTypeText,
			Text: "范志毅说不完 没这个能力知道吗？",
		})
	} else {
		resp.Chains = append(resp.Chains, entity.Chain{
			Type: entity.ChainTypeImage,
			Data: []byte(data),
		})
	}
	return &resp
}
