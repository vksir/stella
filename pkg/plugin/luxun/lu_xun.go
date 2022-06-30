package luxun

import (
	"log"
	"qq-bot-go/pkg/event"
	"regexp"
)

const Title = "lu_xun"

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) Title() string {
	return "鲁迅说"
}

func (h *Handler) Help() string {
	return "鲁迅说<要说的话>"
}

func (h *Handler) Parse(text string) []string {
	re := regexp.MustCompile(`鲁迅说(.*)`)
	return re.FindStringSubmatch(text)
}

func (h *Handler) Handle(r event.Receive) *event.Send {
	text := r.GetAllPlainText()
	args := h.Parse(text)
	if len(args) == 0 {
		return nil
	} else {
		var send event.Send
		if data, err := GetBase64(args[1]); err != nil {
			log.Println(err)
			send.AppendChain(event.TypePlain, "太长惹，鲁迅说不完", "", "")
		} else {
			send.AppendChain(event.TypeImage, "", "", data)
		}
		return &send
	}
}
