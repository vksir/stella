// Package onebot 实现 OneBot 11 正向 WebSocket 适配器，处理事件并驱动 agent 回复。
package onebot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
	"uuid"

	"github.com/coder/websocket"
	"github.com/vksir/stella/internal/core/agent"
	"github.com/vksir/stella/internal/core/hub"
)

const errorMessage = "出错了，请稍后再试"

// scenarioPrompt onebot 场景的专属提示词。
const scenarioPrompt = "The user is chatting with you via OneBot on QQ. Keep replies short and plain; avoid heavy formatting such as tables, which render poorly in chat clients."

const (
	dialTimeout    = 10 * time.Second
	writeTimeout   = 10 * time.Second
	reconnectDelay = 3 * time.Second
)

// ExtensionFactory 按私聊用户构造扩展，sendImage 直发图片到该会话。
type ExtensionFactory func(userID int64, sendImage func([]byte)) []agent.Extension

// OneBot 驱动 agent，通过当前 WebSocket 连接发送回复。
type OneBot struct {
	hub              *hub.AgentHub
	url              string
	accessToken      string
	extensionFactory ExtensionFactory
	opts             []agent.Option
	log              *slog.Logger
	mu               sync.RWMutex
	writeMu          sync.Mutex
	conn             *websocket.Conn
}

func New(agentHub *hub.AgentHub, url, accessToken string, extensionFactory ExtensionFactory, opts ...agent.Option) *OneBot {
	o := &OneBot{
		hub:              agentHub,
		url:              url,
		accessToken:      accessToken,
		extensionFactory: extensionFactory,
		opts:             append([]agent.Option(nil), opts...),
		log:              slog.Default(),
	}
	return o
}

// Run 主动连接 OneBot，断线后重连，直到上下文取消。
func (o *OneBot) Run(ctx context.Context) {
	header := http.Header{}
	if o.accessToken != "" {
		header.Set("Authorization", "Bearer "+o.accessToken)
	}
	for {
		dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
		conn, _, err := websocket.Dial(dialCtx, o.url, &websocket.DialOptions{HTTPHeader: header})
		cancel()
		if err != nil {
			o.log.Warn("onebot websocket dial failed", "err", err)
		} else {
			o.log.Info("onebot websocket connected", "url", o.url)
			o.serve(ctx, conn)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectDelay):
		}
	}
}

func (o *OneBot) serve(ctx context.Context, conn *websocket.Conn) {
	o.mu.Lock()
	o.conn = conn
	o.mu.Unlock()
	defer func() {
		o.mu.Lock()
		o.conn = nil
		o.mu.Unlock()
		conn.CloseNow()
	}()

	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) != -1 {
				o.log.Info("onebot websocket closed", "err", err)
			} else {
				o.log.Warn("onebot websocket read failed", "err", err)
			}
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		if err := o.handleMessage(data); err != nil {
			o.log.Warn("handle onebot message failed", "err", err)
		}
	}
}

func (o *OneBot) handleMessage(payload []byte) error {
	var head incoming
	if err := json.Unmarshal(payload, &head); err != nil {
		return fmt.Errorf("decode onebot payload: %w", err)
	}

	if head.PostType != "message" {
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
	o.log.Info("onebot message received", "message_type", evt.MessageType, "user_id", evt.UserID, "group_id", evt.GroupID, "msg", text)

	key := chatKey(evt.MessageType, evt.UserID, evt.GroupID)
	toolCalls := make(map[int]*agent.ToolCallDelta)
	var toolCallOrder []int
	var toolCallsMu sync.Mutex
	onEvent := func(ctx context.Context, event agent.Event) {
		switch event.Type {
		case agent.EventToolCallStart:
			delta := event.ToolCallDelta
			toolCallsMu.Lock()
			if _, exists := toolCalls[delta.Index]; !exists {
				toolCalls[delta.Index] = &agent.ToolCallDelta{Index: delta.Index, ID: delta.ID, Name: delta.Name}
				toolCallOrder = append(toolCallOrder, delta.Index)
			}
			toolCallsMu.Unlock()
		case agent.EventToolCallDelta:
			delta := event.ToolCallDelta
			toolCallsMu.Lock()
			call, exists := toolCalls[delta.Index]
			if !exists {
				call = &agent.ToolCallDelta{Index: delta.Index}
				toolCalls[delta.Index] = call
				toolCallOrder = append(toolCallOrder, delta.Index)
			}
			if delta.ID != "" {
				call.ID = delta.ID
			}
			if delta.Name != "" {
				call.Name = delta.Name
			}
			call.Arguments += delta.Arguments
			toolCallsMu.Unlock()
		case agent.EventToolExecution:
			result := event.ToolCallResult
			toolCallsMu.Lock()
			var arguments string
			if result.Index >= 0 && result.Index < len(toolCallOrder) {
				index := toolCallOrder[result.Index]
				if call := toolCalls[index]; call != nil {
					arguments = call.Arguments
					delete(toolCalls, index)
				}
			}
			if len(toolCalls) == 0 {
				toolCallOrder = nil
			}
			toolCallsMu.Unlock()
			o.send(evt, formatToolExecution(result.Name, arguments))
		}
	}
	// 私聊装配扩展：闭包捕获用户与发图通道，群聊暂不装配
	chatOpts := append([]agent.Option{}, o.opts...)
	chatOpts = append(chatOpts, agent.WithNonblock(true), agent.WithOnEvent(onEvent), agent.WithScenarioPrompt(scenarioPrompt))
	if o.extensionFactory != nil && evt.MessageType == msgTypePrivate {
		sendImage := func(png []byte) {
			o.send(evt, "[CQ:image,file=base64://"+base64.StdEncoding.EncodeToString(png)+"]")
		}
		chatOpts = append(chatOpts, agent.WithExtensions(o.extensionFactory(evt.UserID, sendImage)...))
	}
	agt, err := o.hub.GetOrCreate(key, chatOpts...)
	if err != nil {
		return fmt.Errorf("get onebot agent: %w", err)
	}
	go o.reply(evt, agt, text)
	return nil
}

const toolEventMaxRunes = 1000

func formatToolExecution(name, arguments string) string {
	if arguments == "" {
		arguments = "(empty)"
	}
	return fmt.Sprintf("Executing tool: %s\nArguments:\n%s",
		name,
		truncateRunes(arguments, toolEventMaxRunes),
	)
}

func truncateRunes(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max]) + "... [truncated]"
}

// reply 将输入交给非阻塞 agent，并发送最终回复（文本与图片）。
func (o *OneBot) reply(evt Event, agt *agent.Agent, input string) {
	log := o.log.With("chat_key", chatKey(evt.MessageType, evt.UserID, evt.GroupID))
	result, err := agt.Run(context.Background(), []agent.Message{{Role: agent.RoleUser, Content: agent.TextContent(input)}})
	if errors.Is(err, agent.ErrMessageSteered) {
		return
	}
	if err != nil {
		log.Error("onebot agent run failed", "err", err)
		o.send(evt, errorMessage)
		return
	}
	msg := formatReply(result.Content)
	if msg == "" {
		log.Debug("onebot reply skipped", "reason", "empty response")
		return
	}
	o.log.Info("onebot message sent", "message_type", evt.MessageType, "user_id", evt.UserID, "group_id", evt.GroupID, "msg", result.Content.Text())
	o.send(evt, msg)
}

// send 向当前连接发送回复，失败仅记录。
func (o *OneBot) send(evt Event, text string) {
	o.mu.RLock()
	conn := o.conn
	o.mu.RUnlock()

	log := o.log.With("chat_key", chatKey(evt.MessageType, evt.UserID, evt.GroupID))
	if conn == nil {
		log.Warn("onebot reply skipped", "reason", "connection unavailable")
		return
	}
	payload, err := buildSendMsg(evt.MessageType, evt.UserID, evt.GroupID, uuid.New().String(), text)
	if err != nil {
		log.Error("onebot reply failed", "error", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	o.writeMu.Lock()
	defer o.writeMu.Unlock()
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
		log.Error("onebot reply failed", "error", err)
	}
}
