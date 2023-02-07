package luxun

import (
	"qq-bot-go/internal/plugin/event"
	"regexp"
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

func (h *Handler) Handle(req *event.Event) *event.Event {
	text := req.Text()
	args := h.Parse(text)
	if len(args) == 0 {
		return nil
	} else {
		var resp event.Event
		if data, err := getImgBase64(args[1]); err != nil {
			resp.Chains = append(resp.Chains, event.Chain{
				Type: event.ChainImage,
				Text: "太长惹，鲁迅说不完",
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
