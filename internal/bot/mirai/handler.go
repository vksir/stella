package mirai

import (
	"context"
	"fmt"
	"qq-bot-go/internal/common/config"
	"qq-bot-go/internal/listener/terrariarun"
	"qq-bot-go/internal/plugin"
	"qq-bot-go/internal/plugin/bing"
	"strconv"
)

const (
	bufSize = 32
)

var cfg = config.GetConfig()

type Handler struct {
	recvChannel chan *Receive
	sendChannel chan *Send

	TerrariaRunReportChannel chan *terrariarun.Event
}

func NewHandler(sendChannel chan *Send) *Handler {
	h := Handler{
		recvChannel:              make(chan *Receive, bufSize),
		sendChannel:              sendChannel,
		TerrariaRunReportChannel: make(chan *terrariarun.Event, bufSize),
	}
	return &h
}

func (h *Handler) RecvChannel() chan *Receive {
	return h.recvChannel
}

func (h *Handler) Start(ctx context.Context) error {
	log.Info("Begin handler")
	go h.watchRecv(ctx)
	go h.watchTerrariaRunReport(ctx)
	return nil
}

func (h *Handler) watchRecv(ctx context.Context) {
	for {
		select {
		case e := <-h.recvChannel:
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
}

func (h *Handler) handleFriendAndGroupMessage(recv *Receive) {
	events := plugin.HandleDefault(recv.ToEvent())
	if len(events) == 0 {
		if recv.Data.Type == TypeGroupMessage {
			meId, err := strconv.Atoi(cfg.Bot.Mirai.Me)
			if err != nil {
				log.Error("invalid me id:", err)
				return
			}
			if !recv.IsAt(meId) {
				return
			}
		}
		events = plugin.Handle(recv.ToEvent(), []plugin.Interface{bing.New()})
	}

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

func (h *Handler) watchTerrariaRunReport(ctx context.Context) {
	for {
		select {
		case e := <-h.TerrariaRunReportChannel:
			for _, f := range cfg.Bot.Mirai.Report.Friend {
				id, err := strconv.Atoi(f.Id)
				if err != nil {
					log.Error("invalid id:", err)
					continue
				}
				send := Send{
					SyncId:  "",
					Command: CommandSendFriendMessage,
					Content: Content{
						Target: id,
						MessageChains: []MessageChain{
							{
								Type: ChainPlain,
								Text: fmt.Sprintf("Terraria: %s", e.Msg),
							},
						},
					},
				}
				h.sendChannel <- &send
			}
		case <-ctx.Done():
			return
		}
	}
}
