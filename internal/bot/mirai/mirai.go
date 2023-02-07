package mirai

import (
	"context"
	"encoding/json"
	"fmt"
	"nhooyr.io/websocket"
	"qq-bot-go/internal/common/config"
	"qq-bot-go/internal/common/logging"
	"sync"
	"time"
)

var log = logging.SugaredLogger()

type Mirai struct {
	url               string
	conn              *websocket.Conn
	ctx               context.Context
	stopKeepConnected context.CancelFunc
	wg                *sync.WaitGroup
	sendChannel       chan *Send
	recvChannels      []chan *Receive
}

func NewMirai() *Mirai {
	m := Mirai{
		url:         fmt.Sprintf("ws://%s:%d/all", config.CFG.Bot.Mirai.Host, config.CFG.Bot.Mirai.Port),
		wg:          &sync.WaitGroup{},
		sendChannel: make(chan *Send, 32),
	}
	return &m
}

func (m *Mirai) Start() error {
	m.ctx, m.stopKeepConnected = context.WithCancel(context.Background())
	m.wg.Add(3)
	go m.keepConnected(m.ctx)
	go m.watchSendChannel(m.ctx)
	go m.watchRecvChannels(m.ctx)
	return nil
}

func (m *Mirai) Close() error {
	m.stopKeepConnected()
	m.wg.Wait()
	if m.conn == nil {
		return nil
	}
	return m.conn.Close(websocket.StatusInternalError, "")
}

func (m *Mirai) SendChannel() chan *Send {
	return m.sendChannel
}

func (m *Mirai) RegisterRecvChannel(c chan *Receive) {
	m.recvChannels = append(m.recvChannels, c)
}

func (m *Mirai) watchRecvChannels(ctx context.Context) {
	log.Info("Begin watch recv")
	defer m.wg.Done()
	for {
		select {
		case e := <-m.sendChannel:
			bytes, err := json.Marshal(e)
			if err != nil {
				log.Error("Json marshal failed: ", err)
				break
			}
			log.Debug("Begin write: ", e.String())
			if err := m.write(bytes); err != nil {
				log.Error("Write failed: ", err)
			}
		case <-ctx.Done():
			log.Info("Watch recv stopped")
			return
		}
	}
}

func (m *Mirai) watchSendChannel(ctx context.Context) {
	log.Info("Begin watch send")
	defer m.wg.Done()
	for {
		select {
		case <-ctx.Done():
			log.Info("Watch send stopped")
			return
		default:
			bytes, err := m.read()
			if err != nil {
				log.Error("Read failed: ", err)
				time.Sleep(5 * time.Second)
				break
			}
			log.Debug("Recv: ", string(bytes))
			var e Receive
			if err := json.Unmarshal(bytes, &e); err != nil {
				log.Error("Json unmarshal failed: ", err)
				break
			}
			for _, c := range m.recvChannels {
				c <- &e
			}
		}
	}
}

func (m *Mirai) keepConnected(ctx context.Context) {
	log.Info("Begin keep connected")
	defer m.wg.Done()
	for {
		select {
		case <-ctx.Done():
			log.Info("Keep connected stopped")
			return
		default:
			if m.conn != nil {
				if err := m.conn.Ping(ctx); err == nil {
					break
				}
			}
			c, _, err := websocket.Dial(ctx, m.url, nil)
			if err != nil {
				log.Error("Connect failed: ", err)
				break
			}
			m.conn = c
		}
		time.Sleep(5 * time.Second)
	}
}

func (m *Mirai) read() ([]byte, error) {
	if m.conn == nil {
		return nil, fmt.Errorf("no connection")
	}
	_, recv, err := m.conn.Read(context.Background())
	return recv, err
}

func (m *Mirai) write(msg []byte) error {
	if m.conn == nil {
		return fmt.Errorf("no connection")
	}
	return m.conn.Write(context.Background(), websocket.MessageText, msg)
}
