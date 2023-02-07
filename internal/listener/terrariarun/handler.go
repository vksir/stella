package terrariarun

import (
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"qq-bot-go/internal/common/config"
)

type Client struct {
	host string
	port int
}

func NewClient() *Client {
	c := Client{
		host: config.CFG.Listener.TerrariaRun.Host,
		port: config.CFG.Listener.TerrariaRun.Port,
	}
	return &c
}

func (c *Client) GetGameEvents() (*Events, error) {
	url := fmt.Sprintf("http://%s:%d/game/events", c.host, c.port)
	resp, err := resty.New().R().Get(url)
	if err != nil {
		return nil, err
	}
	var events Events
	if err = json.Unmarshal(resp.Body(), &events); err != nil {
		return nil, err
	}
	return &events, nil
}
