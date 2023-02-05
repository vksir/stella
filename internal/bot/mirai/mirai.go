package mirai

import (
	"context"
	"fmt"
	"nhooyr.io/websocket"
	"qq-bot-go/confs"
	"qq-bot-go/internal/common/logging"
	"time"
)

var log = logging.SugaredLogger()

type Mirai struct {
	conn *websocket.Conn
}

func NewMirai() *Mirai {
	m := Mirai{}
	return &m
}

func (m *Mirai) Name() string {
	return "Mirai"
}

func (m *Mirai) Connect() {
	url := fmt.Sprintf("ws://%s:%d/all", confs.CONF.Mirai.Host, confs.CONF.Mirai.Port)
	m.conn = connect(url)
	go func() {
		for {
			if err := m.conn.Ping(context.Background()); err != nil {
				m.conn = connect(url)
			}
			time.Sleep(5 * time.Second)
		}
	}()
}

func (m *Mirai) Close() error {
	return m.conn.Close(websocket.StatusInternalError, "")
}

func (m *Mirai) Read() ([]byte, error) {
	_, recv, err := m.conn.Read(context.Background())
	return recv, err
}

func (m *Mirai) Write(msg []byte) error {
	return m.conn.Write(context.Background(), websocket.MessageText, msg)
}

func connect(url string) *websocket.Conn {
	log.Info("Begin connect: ", url)
	for {
		c, _, err := websocket.Dial(context.Background(), url, nil)
		if err != nil {
			log.Error("Connect failed, retry: ", err)
			time.Sleep(5 * time.Second)
			continue
		}
		log.Info("Connect success")
		return c
	}
}
