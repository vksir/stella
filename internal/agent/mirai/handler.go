package mirai

import (
	"context"
	"encoding/json"
	"log"
	"qq-bot-go/pkg/event"
	"qq-bot-go/pkg/plugin"
	"time"
)

type handler struct {
	mirai   *mirai
	respQue *responseQueues
}

func (h *handler) loop() {
	for {
		recv := h.read()
		var receiveEvent EventReceive
		if err := json.Unmarshal(recv, &receiveEvent); err != nil {
			log.Println("json unmarshal failed: ", err)
		} else {
			go h.handle(&receiveEvent)
		}
	}
}

func (h *handler) handle(receiveEvent *EventReceive) {
	if receiveEvent.SyncId == "-1" || receiveEvent.SyncId == "" {
		h.handlePushMessage(receiveEvent)
	} else {
		h.handleResponseMessage(receiveEvent)
	}
}

func (h *handler) handleResponseMessage(receiveEvent *EventReceive) {
	h.respQue.putResponse(receiveEvent)
}

func (h *handler) handlePushMessage(receiveEvent *EventReceive) {
	switch receiveEvent.Data.Type {
	case TypeFriendMessage, TypeGroupMessage:
		h.handleFriendOrGroupMessage(receiveEvent)
	}
}

func (h *handler) handleFriendOrGroupMessage(receiveEvent *EventReceive) {
	receive := receiveEvent.transformToStandardReceive()
	for _, p := range plugin.Plugins {
		if send := p.Handle(receive); send != nil {
			h.sendFriendOrGroupMessage(send, receiveEvent)
		}
	}
}

func (h *handler) sendFriendOrGroupMessage(send *event.Send, receiveEvent *EventReceive) {
	sendEvent := newSendEvent(send)
	switch receiveEvent.Data.Type {
	case TypeFriendMessage:
		_, err := h.sendFriendMessage(sendEvent, receiveEvent.Data.Sender.Id)
		if err != nil {
			log.Println("sendFriendMessage failed: ", err)
		}
	case TypeGroupMessage:
		h.sendGroupMessage(sendEvent, receiveEvent.Data.Sender.Group.Id)
	}
}

func (h *handler) sendFriendMessage(sendEvent *EventSend, senderId int) (int, error) {
	syncId, q := h.respQue.register()
	defer h.respQue.unRegister(syncId)
	h.sendMessage(sendEvent, syncId, CommandSendFriendMessage, senderId)
	return getResponseEvent(q)
}

func getResponseEvent(q chan *EventReceive) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	select {
	case responseEvent := <-q:
		return responseEvent.Data.MessageId, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (h *handler) sendGroupMessage(sendEvent *EventSend, senderId int) {
	h.sendMessage(sendEvent, "", CommandSendGroupMessage, senderId)
}

func (h *handler) sendMessage(sendEvent *EventSend, syncId string, command string, target int) {
	sendEvent.SyncId = syncId
	sendEvent.Command = command
	sendEvent.Content.Target = target
	sendBytes, _ := json.Marshal(sendEvent)
	log.Println("mirai send: ", sendEvent.log())
	h.write(sendBytes)
}

func (h *handler) read() []byte {
	for {
		if recv, err := h.mirai.read(); err != nil {
			log.Println("mirai read failed: ", err)
			h.mirai.connect()
		} else {
			log.Println("mirai read: ", string(recv))
			return recv
		}
	}
}

func (h *handler) write(msg []byte) {
	if err := h.mirai.write(msg); err != nil {
		log.Println("mirai write failed: ", err)
	}
}

func Load() {
	log.Println("LoadAgent: mirai")
	var m mirai
	rq := make(responseQueues)
	h := handler{&m, &rq}
	h.mirai.connect()
	go h.loop()
}
