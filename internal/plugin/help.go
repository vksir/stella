package plugin

import (
	"fmt"
	"qq-bot-go/pkg/event"
	"regexp"
	"strings"
)

type HelpHandler struct{}

func New() *HelpHandler {
	return &HelpHandler{}
}

func (h *HelpHandler) Title() string {
	return "帮助"
}

func (h *HelpHandler) Help() string {
	return "帮助"
}

func (h *HelpHandler) Parse(text string) []string {
	re := regexp.MustCompile(`帮助`)
	return re.FindStringSubmatch(text)
}

func (h *HelpHandler) Handle(r event.Receive) *event.Send {
	text := r.GetAllPlainText()
	args := h.Parse(text)
	if len(args) == 0 {
		return nil
	} else {
		var send event.Send
		var textSlice []string
		for i, p := range Plugins {
			textSlice = append(textSlice, fmt.Sprintf("[%d] %s: %s", i, p.Title(), p.Help()))
		}
		send.AppendChain(event.TypePlain, strings.Join(textSlice, "\n"), "", "")
		return &send
	}
}
