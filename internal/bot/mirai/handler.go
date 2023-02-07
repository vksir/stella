package mirai

import (
	"context"
	"qq-bot-go/internal/plugin"
)

type Handler struct {
	channel     chan *Receive
	sendChannel chan *Send
}

func NewHandler(sendChannel chan *Send) *Handler {
	h := Handler{
		channel:     make(chan *Receive, 32),
		sendChannel: sendChannel,
	}
	return &h
}

func (h *Handler) Channel() chan *Receive {
	return h.channel
}

func (h *Handler) Start(ctx context.Context) error {
	log.Info("Begin handler")
	go func() {
		for {
			select {
			case e := <-h.channel:
				if e.SyncId == "-1" || e.SyncId == "" {
					switch e.Data.Type {
					case TypeFriendMessage, TypeGroupMessage:
						h.handleFriendAndGroupMessage(e)
					}
				} else {
					// TODO handle response
				}
			case <-ctx.Done():
				log.Info("Handler stopped")
				return
			}
		}
	}()
	return nil
}

func (h *Handler) handleFriendAndGroupMessage(recv *Receive) {
	events := plugin.Handle(recv.ToEvent())
	for _, e := range events {
		// TODO rand SyncId
		send := NewSend(e.RespEvent)
		switch recv.Data.Type {
		case TypeFriendMessage:
			send.Command = CommandSendFriendMessage
			send.Content.Target = recv.Data.Sender.Id
		case TypeGroupMessage:
			send.Command = CommandSendGroupMessage
			send.Content.Target = recv.Data.Sender.Group.Id
		}
		h.sendChannel <- send
	}
}
