// internal/platform/qq/qq.go
package qq

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"stella/entity"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/vksir/vkiss-lib/pkg/log"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for napcat
	},
}

// onebotMessage represents a Onebot v11 message event from napcat
type onebotMessage struct {
	PostType    string `json:"post_type"`
	MessageType string `json:"message_type"`
	UserID      int64  `json:"user_id"`
	GroupID     int64  `json:"group_id"`
	RawMessage  string `json:"raw_message"`
	MessageID   int64  `json:"message_id"`
	Sender      struct {
		Nickname string `json:"nickname"`
		Card     string `json:"card"`
		UserID   int64  `json:"user_id"`
	} `json:"sender"`
}

type Adapter struct {
	mu       sync.Mutex
	conn     *websocket.Conn
	evtChan  chan entity.Event
	chatFunc func(ctx context.Context, userID string, evt *entity.Event) error
}

func New() *Adapter {
	return &Adapter{
		evtChan: make(chan entity.Event, 100),
	}
}

// SetChatFunc sets the callback function for handling chat messages
func (q *Adapter) SetChatFunc(f func(ctx context.Context, userID string, evt *entity.Event) error) {
	q.chatFunc = f
}

func (q *Adapter) Start(ctx context.Context) error {
	log.InfoC(ctx, "QQ Adapter ready, waiting for napcat WebSocket connection on /ws/qq")
	return nil
}

func (q *Adapter) Chan() <-chan entity.Event {
	return q.evtChan
}

// Send sends an event response back to QQ via Onebot send_msg action
func (q *Adapter) Send(ctx context.Context, evt entity.Event) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.conn == nil {
		return fmt.Errorf("no websocket connection")
	}

	ansText := evt.AnsText()
	if ansText == "" {
		return nil
	}

	action := map[string]interface{}{
		"action": "send_msg",
		"params": map[string]interface{}{
			"message_type": evt.Type,
			"message":      ansText,
		},
	}

	if evt.Type == entity.EvtTypeGroup {
		action["params"].(map[string]interface{})["group_id"] = evt.SessionID
	} else {
		action["params"].(map[string]interface{})["user_id"] = evt.UserID
	}

	return q.conn.WriteJSON(action)
}

// HandleWS handles napcat WebSocket connections
func (q *Adapter) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.ErrorC(r.Context(), "ws upgrade failed", "err", err)
		return
	}

	q.mu.Lock()
	q.conn = conn
	q.mu.Unlock()

	log.InfoC(r.Context(), "napcat connected via WebSocket")

	defer func() {
		q.mu.Lock()
		q.conn = nil
		q.mu.Unlock()
		conn.Close()
		log.InfoC(context.Background(), "napcat disconnected")
	}()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			log.ErrorC(context.Background(), "ws read error", "err", err)
			return
		}

		var msg onebotMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.WarnC(context.Background(), "parse onebot message failed", "raw", string(raw), "err", err)
			continue
		}

		if msg.PostType != "message" {
			continue
		}

		evt := q.convertEvent(msg)

		if q.chatFunc != nil {
			go func() {
				err := q.chatFunc(context.Background(), evt.UserID, &evt)
				if err != nil {
					log.ErrorC(context.Background(), "agent chat error", "err", err)
					return
				}
				if err := q.Send(context.Background(), evt); err != nil {
					log.ErrorC(context.Background(), "send response error", "err", err)
				}
			}()
		}
	}
}

// convertEvent converts a Onebot message to an entity.Event
func (q *Adapter) convertEvent(msg onebotMessage) entity.Event {
	userID := fmt.Sprintf("%d", msg.Sender.UserID)
	userName := msg.Sender.Nickname
	if msg.Sender.Card != "" {
		userName = msg.Sender.Card
	}

	var evtType, sessionID string
	if msg.MessageType == "group" {
		evtType = entity.EvtTypeGroup
		sessionID = fmt.Sprintf("%d", msg.GroupID)
	} else {
		evtType = entity.EvtTypePrivate
		sessionID = userID
	}

	return entity.Event{
		Type:      evtType,
		UserID:    userID,
		UserName:  userName,
		SessionID: sessionID,
		Ask: []entity.Chain{
			{
				Role: entity.ChainRoleUser,
				Type: entity.ChainTypeText,
				Text: msg.RawMessage,
			},
		},
	}
}
