// internal/plugin/pixiv/pixiv.go
package pixiv

import (
	"regexp"
	"stella/entity"
)

const (
	Title = "pixiv"
)

func New() *Handler {
	return &Handler{}
}

type Handler struct{}

func (h *Handler) Name() string {
	return "涩图"
}

func (h *Handler) Help() string {
	return "<中文量词>份涩图"
}

func (h *Handler) Parse(text string) []string {
	re := regexp.MustCompile(`([来一二两三四五六七八九十])份涩图`)
	return re.FindStringSubmatch(text)
}

var numMap = map[string]int{
	"来": 1, "一": 1, "二": 2, "两": 2, "三": 3,
	"四": 4, "五": 5, "六": 6, "七": 7,
	"八": 8, "九": 9, "十": 10,
}

func (h *Handler) Handle(req *entity.PluginMessage) *entity.PluginMessage {
	text := req.Text()
	args := h.Parse(text)
	if len(args) == 0 {
		return nil
	}

	num := numMap[args[1]]
	var resp entity.PluginMessage
	imageUrls := getPixivImageUrls(num)
	for _, u := range imageUrls {
		resp.Chains = append(resp.Chains, entity.Chain{
			Type: entity.ChainTypeImage,
			Data: []byte(u),
		})
	}
	return &resp
}
