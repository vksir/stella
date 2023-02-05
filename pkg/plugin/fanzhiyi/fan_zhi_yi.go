package fanzhiyi

import (
	"log"
	"qq-bot-go/pkg/event"
	"regexp"
)

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) Title() string {
	return "范志毅说"
}

func (h *Handler) Help() string {
	return "范志毅说<要说的话>"
}

func (h *Handler) Parse(text string) []string {
	re := regexp.MustCompile(`范志毅说(.*)`)
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
			send.AppendChain(event.TypePlain, "范志毅说不完 没这个能力知道吗？", "", "")
		} else {
			send.AppendChain(event.TypeImage, "", "", data)
		}
		return &send
	}
}
