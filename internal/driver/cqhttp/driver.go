package cqhttp

import (
	"github.com/go-resty/resty/v2"
	"github.com/spf13/viper"
	"net/http"
	"net/url"
	"stella/internal/comm"
)

var log = comm.GetSugaredLogger()

const (
	MessageTypePrivate = "private"
	MessageTypeGroup   = "group"
)

func SendMsg(messageType string, id int64, msg string) error {
	b := SendMsgBody{Message: msg}
	switch messageType {
	case MessageTypePrivate:
		b.UserId = id
	case MessageTypeGroup:
		b.GroupId = id
	}

	resp, err := resty.New().R().SetBody(b).Post(getUrl("send_msg"))
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusOK {
		return comm.NewRespErr(resp)
	}
	return nil
}

func getUrl(endpoint string) string {
	u, _ := url.JoinPath(viper.GetString("cqhttp.addr"), endpoint)
	return u
}
