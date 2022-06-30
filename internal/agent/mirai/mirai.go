package mirai

import (
	"context"
	"fmt"
	"log"
	"nhooyr.io/websocket"
	"qq-bot-go/confs"
	"time"
)

const (
	Title = "mirai"
)

type mirai struct {
	conn *websocket.Conn
}

func (m *mirai) connect() {
	for {
		url := fmt.Sprintf("ws://%s:%d/all", confs.CONF.Mirai.Host, confs.CONF.Mirai.Port)
		if c, _, err := websocket.Dial(context.Background(), url, nil); err != nil {
			log.Println("connect failed: ", err)
			time.Sleep(time.Second * 5)
		} else {
			log.Println("connected")
			m.conn = c
			return
		}
	}
}

func (m *mirai) close() error {
	return m.conn.Close(websocket.StatusInternalError, "")
}

func (m *mirai) read() ([]byte, error) {
	_, recv, err := m.conn.Read(context.Background())
	return recv, err
}

func (m *mirai) write(msg []byte) error {
	return m.conn.Write(context.Background(), websocket.MessageText, msg)
}
