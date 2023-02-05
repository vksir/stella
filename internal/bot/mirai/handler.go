package mirai

import (
	"context"
	"encoding/json"
	"qq-bot-go/internal/plugin"
	"qq-bot-go/pkg/event"
	"time"
)

type Handler struct {
	mirai   *Mirai
	respQue *responseQueues
}

func NewHandler() *Handler {
	rq := make(responseQueues)
	h := Handler{
		mirai:   NewMirai(),
		respQue: &rq,
	}
	return &h
}

func (h *Handler) Start() error {
	h.mirai.Connect()
	go h.loop()
	return nil
}

func (h *Handler) loop() {
	for {
		recv, err := h.mirai.Read()
		if err != nil {
			log.Error("Read failed: ", err)
			continue
		}
		log.Debug("Recv: ", string(recv))
		receiveEvent := EventReceive{}
		if err := json.Unmarshal(recv, &receiveEvent); err != nil {
			log.Info("json unmarshal failed: ", err)
		}
		go h.handle(&receiveEvent)
	}
}

func (h *Handler) handle(receiveEvent *EventReceive) {
	if receiveEvent.SyncId == "-1" || receiveEvent.SyncId == "" {
		h.handlePushMessage(receiveEvent)
	} else {
		h.handleResponseMessage(receiveEvent)
	}
}

func (h *Handler) handleResponseMessage(receiveEvent *EventReceive) {
	h.respQue.putResponse(receiveEvent)
}

func (h *Handler) handlePushMessage(receiveEvent *EventReceive) {
	switch receiveEvent.Data.Type {
	case TypeFriendMessage, TypeGroupMessage:
		h.handleFriendOrGroupMessage(receiveEvent)
	}
}

func (h *Handler) handleFriendOrGroupMessage(receiveEvent *EventReceive) {
	receive := receiveEvent.transformToStandardReceive()
	for _, p := range plugin.Plugins {
		if send := p.Handle(receive); send != nil {
			h.sendFriendOrGroupMessage(send, receiveEvent)
		}
	}
}

func (h *Handler) sendFriendOrGroupMessage(send *event.Send, receiveEvent *EventReceive) {
	sendEvent := newSendEvent(send)
	switch receiveEvent.Data.Type {
	case TypeFriendMessage:
		_, err := h.sendFriendMessage(sendEvent, receiveEvent.Data.Sender.Id)
		if err != nil {
			log.Info("sendFriendMessage failed: ", err)
		}
	case TypeGroupMessage:
		h.sendGroupMessage(sendEvent, receiveEvent.Data.Sender.Group.Id)
	}
}

func (h *Handler) sendFriendMessage(sendEvent *EventSend, senderId int) (int, error) {
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

func (h *Handler) sendGroupMessage(sendEvent *EventSend, senderId int) {
	h.sendMessage(sendEvent, "", CommandSendGroupMessage, senderId)
}

func (h *Handler) sendMessage(sendEvent *EventSend, syncId string, command string, target int) {
	sendEvent.SyncId = syncId
	sendEvent.Command = command
	sendEvent.Content.Target = target
	sendBytes, _ := json.Marshal(sendEvent)
	log.Info("mirai send: ", sendEvent.log())
	if err := h.mirai.Write(sendBytes); err != nil {
		log.Error("Write failed: ", err)
	}
}
