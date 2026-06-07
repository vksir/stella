// internal/plugin/luxun/lu_xun.go
package luxun

import (
	"regexp"
	"stella/entity"
)

const Title = "lu_xun"

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) Name() string {
	return "鲁迅说"
}

func (h *Handler) Help() string {
	return "鲁迅说<要说的话>"
}

func (h *Handler) Parse(text string) []string {
	re := regexp.MustCompile(`鲁迅说(.*)`)
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
			Text: "太长惹，鲁迅说不完",
		})
	} else {
		resp.Chains = append(resp.Chains, entity.Chain{
			Type: entity.ChainTypeImage,
			Data: []byte(data),
		})
	}
	return &resp
}
