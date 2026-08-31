// Package onebot 实现 OneBot 11 反向 WebSocket 适配器，处理事件并驱动 agent 回复。
package onebot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/vksir/stella/internal/agent"
	"github.com/vksir/stella/internal/hub"
)

// SendFunc 向连接发送一帧文本消息。
type SendFunc func(ctx context.Context, payload []byte) error

// MessageHandler 处理连接上收到的一帧消息。
type MessageHandler func(ctx context.Context, payload []byte, send SendFunc) error

// 运行时错误提示。
const errorMessage = "出错了，请稍后再试"

// OneBot 提交会话输入并串行回复，锁只保护回复队列和连接信息。
type OneBot struct {
	hub     *hub.SessionHub
	log     *slog.Logger
	mu      sync.Mutex
	replies map[string]*replyQueue
}

type replyQueue struct {
	ctx   context.Context
	event Event
	send  SendFunc
	runs  []*hub.Run
}

// New 创建 OneBot 适配器。
func New(sessHub *hub.SessionHub) *OneBot {
	o := &OneBot{
		hub:     sessHub,
		log:     slog.Default(),
		replies: make(map[string]*replyQueue),
	}
	o.log.Debug("onebot adapter created")
	return o
}

// HandleMessage 同步提交入站消息，后台等待回执并回复；API 响应仅记录失败。
func (o *OneBot) HandleMessage(ctx context.Context, payload []byte, send SendFunc) (err error) {
	defer func() {
		if err != nil {
			o.log.Error("handle onebot message failed", "error", err)
		}
	}()
	var head incoming
	if err := json.Unmarshal(payload, &head); err != nil {
		return fmt.Errorf("decode onebot payload: %w", err)
	}

	if head.RetCode != nil {
		if head.Status != "ok" || *head.RetCode != 0 {
			o.log.Warn("onebot api call failed", "echo", head.Echo, "status", head.Status, "retcode", *head.RetCode)
		}
		return nil
	}

	if head.PostType != "message" {
		o.log.Debug("onebot event ignored", "post_type", head.PostType)
		return nil
	}

	var evt Event
	if err := json.Unmarshal(payload, &evt); err != nil {
		return fmt.Errorf("decode onebot event: %w", err)
	}
	text, ok := trigger(evt)
	if !ok {
		o.log.Debug("onebot message skipped", "message_type", evt.MessageType, "user_id", evt.UserID, "group_id", evt.GroupID)
		return nil
	}
	o.log.Info("onebot message received", "message_type", evt.MessageType, "user_id", evt.UserID, "group_id", evt.GroupID, "message_id", evt.MessageID)

	key := chatKey(evt.MessageType, evt.UserID, evt.GroupID)
	o.mu.Lock()
	defer o.mu.Unlock()
	submission, err := o.hub.Submit(ctx, key, agent.Message{Role: agent.RoleUser, Content: text}, hub.SteerIfBusy)
	if err != nil {
		return fmt.Errorf("submit onebot message: %w", err)
	}
	queue := o.replies[key]
	startWorker := queue == nil
	if startWorker {
		if !submission.Started {
			return nil
		}
		queue = &replyQueue{}
		o.replies[key] = queue
	}
	queue.ctx, queue.event, queue.send = ctx, evt, send
	if submission.Started {
		queue.runs = append(queue.runs, submission.Run)
	}
	if startWorker {
		go o.reply(key, queue)
	}
	return nil
}

// reply 按运行顺序发送结果，等待回执不绑定连接生命周期。
func (o *OneBot) reply(key string, queue *replyQueue) {
	for {
		o.mu.Lock()
		if len(queue.runs) == 0 {
			delete(o.replies, key)
			o.mu.Unlock()
			o.log.Debug("onebot reply queue drained", "chat_key", key)
			return
		}
		run := queue.runs[0]
		queue.runs[0] = nil
		queue.runs = queue.runs[1:]
		o.mu.Unlock()

		result, err := run.Wait(context.Background())
		log := o.log.With("chat_key", key, "run_id", run.ID())
		text := errorMessage
		if err != nil {
			log.Error("onebot session run failed", "error", err)
		} else {
			var parts []string
			for _, message := range result.Messages {
				if message.Role == agent.RoleAssistant && message.Content != "" {
					parts = append(parts, message.Content)
				}
			}
			text = strings.Join(parts, "\n\n")
		}
		if text == "" {
			log.Debug("onebot reply skipped", "reason", "empty response")
			continue
		}

		o.mu.Lock()
		ctx, evt, send := queue.ctx, queue.event, queue.send
		o.mu.Unlock()
		if send == nil {
			log.Warn("onebot reply skipped", "reason", "connection unavailable")
			continue
		}
		if err := ctx.Err(); err != nil {
			log.Warn("onebot reply skipped", "error", err)
			continue
		}
		if err := o.sendText(ctx, evt, text, send); err != nil {
			log.Error("onebot reply failed", "error", err)
		} else {
			log.Info("onebot reply sent")
		}
	}
}

// sendText 将文本按长度分段发送。
func (o *OneBot) sendText(ctx context.Context, evt Event, text string, send SendFunc) error {
	if text == "" {
		return nil
	}
	for _, part := range splitText(text) {
		payload, err := buildSendMsg(evt.MessageType, evt.UserID, evt.GroupID, uuid.NewString(), part)
		if err != nil {
			return err
		}
		if err := send(ctx, payload); err != nil {
			return fmt.Errorf("send message: %w", err)
		}
		o.log.Debug("onebot message segment sent", "message_type", evt.MessageType, "user_id", evt.UserID, "group_id", evt.GroupID)
	}
	return nil
}
