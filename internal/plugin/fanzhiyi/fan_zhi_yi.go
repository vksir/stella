package fanzhiyi

import (
	"qq-bot-go/internal/event"
	"regexp"
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

func (h *Handler) Handle(req *event.Event) *event.Event {
	text := req.GetAllPlainText()
	args := h.Parse(text)
	if len(args) == 0 {
		return nil
	} else {
		var resp event.Event
		if data, err := getImgBase64(args[1]); err != nil {
			resp.Chains = append(resp.Chains, event.Chain{
				Type: event.ChainPlain,
				Text: "范志毅说不完 没这个能力知道吗？",
			})
		} else {
			resp.Chains = append(resp.Chains, event.Chain{
				Type:   event.ChainImage,
				Base64: data,
			})
		}
		return &resp
	}
}
