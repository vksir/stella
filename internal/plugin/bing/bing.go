package bing

import (
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"qq-bot-go/internal/common/config"
	"qq-bot-go/internal/common/logging"
	"qq-bot-go/internal/plugin/event"
	"regexp"
	"strings"
	"time"
)

var sess *Session
var cfg = config.GetConfig()
var log = logging.GetSugaredLogger()

type Handler struct {
}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) Name() string {
	return "Bing"
}

func (h *Handler) Help() string {
	return "@Me"

}

func (h *Handler) Parse(text string) []string {
	return []string{text}
}

func (h *Handler) Handle(req *event.Event) *event.Event {
	text := req.Text()
	args := h.Parse(text)
	if len(args) == 0 {
		return nil
	} else {
		var resp event.Event
		if reply, err := ask(args[0]); err != nil {
			resp.Chains = append(resp.Chains, event.Chain{
				Type: event.ChainPlain,
				Text: fmt.Sprintf("不知道！！(%s)", err),
			})
		} else {
			pattern := regexp.MustCompile(`必应|Bing`)
			reply = pattern.ReplaceAllString(reply, "Stella")
			reply = strings.Trim(reply, "\n")
			resp.Chains = append(resp.Chains, event.Chain{
				Type: event.ChainPlain,
				Text: reply,
			})
		}
		return &resp
	}
}

type Session struct {
	Id   string
	Time time.Time
}

func ask(prompt string) (string, error) {
	if sess == nil || time.Since(sess.Time) > time.Minute*5 {
		sess = &Session{Time: time.Now()}
	}
	resp, err := resty.New().
		SetTimeout(time.Second*60).
		R().
		SetQueryParam("session_id", sess.Id).
		SetQueryParam("prompt", prompt).
		Get(cfg.BingUrl)
	if err != nil {
		return "", err
	}
	log.Info("bing reply:", resp.String())
	var data struct {
		SessionId string `json:"session_id"`
		Reply     string `json:"reply"`
	}
	if err := json.Unmarshal(resp.Body(), &data); err != nil {
		return "", err
	}
	sess.Id = data.SessionId
	sess.Time = time.Now()
	return data.Reply, nil
}
