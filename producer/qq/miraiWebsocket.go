package qq

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"nhooyr.io/websocket"
	"qq-bot-go/confs"
	"qq-bot-go/messageQueue"
	"time"
)

type miraiWebsocket struct {
	url  string
	conn *websocket.Conn
}

func NewMiraiWebsocket(conf *confs.Conf) *miraiWebsocket {
	url := fmt.Sprintf("ws://%s:%d/all", conf.Mirai.Host, conf.Mirai.Port)
	return &miraiWebsocket{
		url: url,
	}
}

func (m *miraiWebsocket) Listen(mq *messageQueue.MsgQueue) {
	m.connect()
	go func() {
		defer m.close()
		for {
			for {
				recv, err := m.read()
				if err != nil {
					log.Println("websocket read failed: ", err)
					break
				}
				log.Println("websocket recv: ", string(recv))

				var e Event
				err = json.Unmarshal(recv, &e)
				if err != nil {
					log.Println("json unmarshal failed: ", err)
					continue
				}
				switch e.Data.Type {
				case "FriendMessage":
					content := e.Data.MessageChain.getContent()
					mq.Task <- messageQueue.Task{
						Type:    messageQueue.TypeFriend,
						Content: content,
						Sender: messageQueue.Sender{
							Id:       e.Data.Sender.Id,
							NickName: e.Data.Sender.Nickname,
							Remark:   e.Data.Sender.Remark,
						},
					}
				case "GroupMessage":
					content := e.Data.MessageChain.getContent()
					mq.Task <- messageQueue.Task{
						Type:    messageQueue.TypeGroup,
						Content: content,
						Sender: messageQueue.Sender{
							Id:         e.Data.Sender.Id,
							MemberName: e.Data.Sender.MemberName,
							Permission: e.Data.Sender.Permission,
						},
						Group: messageQueue.Group{
							Id:   e.Data.Sender.Group.Id,
							Name: e.Data.Sender.Group.Name,
						},
					}
				}
			}
			m.connect()
		}
	}()
}

func (m *miraiWebsocket) connect() {
	for {
		c, _, err := websocket.Dial(context.Background(), m.url, nil)
		if err == nil {
			log.Println("connected")
			m.conn = c
			return
		}
		log.Println("connect failed: ", err)
		time.Sleep(time.Second * 5)
	}
}

func (m *miraiWebsocket) close() {
	err := m.conn.Close(websocket.StatusInternalError, "")
	if err != nil {
		log.Println("close failed: ", err)
	}
}

func (m *miraiWebsocket) read() ([]byte, error) {
	_, recv, err := m.conn.Read(context.Background())
	return recv, err
}

func (m *miraiWebsocket) write(msg []byte) error {
	err := m.conn.Write(context.Background(), websocket.MessageText, msg)
	return err
}

func (m *miraiWebsocket) sendMsg(command string, target int, msg string) {
	data := Message{
		SyncId:  0,
		Command: command,
		Content: Content{
			Target: target,
			MessageChain: MessageChain{{
				Type: "Plain",
				Text: msg,
			}},
		},
	}
	dataBytes, err := json.Marshal(&data)
	if err != nil {
		log.Println("json marshal failed: ", err)
		return
	}
	err = m.write(dataBytes)
	if err != nil {
		log.Println("websocket write failed: ", err)
		return
	}
	log.Printf("send msg succeed: cmd=%s, target=%d, msg=%s", command, target, msg)
}

func (m *miraiWebsocket) SendFriendMsg(userId int, msg string) {
	m.sendMsg("sendFriendMessage", userId, msg)
}

func (m *miraiWebsocket) SendGroupMsg(groupId int, msg string) {
	m.sendMsg("sendGroupMessage", groupId, msg)
}

func (m *miraiWebsocket) SendMsg(userId int, groupId int, targetType string, msg string) {
	log.Println(targetType)
	switch targetType {
	case messageQueue.TypeFriend:
		m.SendFriendMsg(userId, msg)
	case messageQueue.TypeGroup:
		m.SendGroupMsg(groupId, msg)
	default:
		log.Println("invalid target type: ", targetType)
	}
}
