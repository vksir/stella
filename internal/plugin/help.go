package plugin

import (
	"fmt"
	"qq-bot-go/internal/event"
	"regexp"
	"strings"
)

type Help struct{}

func NewHelp() *Help {
	return &Help{}
}

func (h *Help) Name() string {
	return "帮助"
}

func (h *Help) Help() string {
	return "帮助"
}

func (h *Help) Parse(text string) []string {
	re := regexp.MustCompile(`帮助`)
	return re.FindStringSubmatch(text)
}

func (h *Help) Handle(req *event.Event) *event.Event {
	text := req.GetAllPlainText()
	args := h.Parse(text)
	if len(args) == 0 {
		return nil
	} else {
		var resp event.Event
		var textSlice []string
		for i, p := range plugins {
			textSlice = append(textSlice, fmt.Sprintf("[%d] %s: %s", i, p.Name(), p.Help()))
		}
		resp.Chains = append(resp.Chains, event.Chain{
			Type: event.ChainPlain,
			Text: strings.Join(textSlice, "\n"),
		})
		return &resp
	}
}
