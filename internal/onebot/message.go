package onebot

import (
	"encoding/json"
	"strconv"
	"strings"
)

// 消息类型。
const (
	msgTypePrivate = "private"
	msgTypeGroup   = "group"
)

// 单条消息最大长度（rune），超过则分段发送。
const maxMsgRunes = 4000

// trigger 判定事件是否触发回复，返回提取的纯文本。
func trigger(evt Event) (string, bool) {
	text := extractText(evt.Message)
	if len(evt.Message) == 0 && evt.RawMessage != "" {
		text = evt.RawMessage
	}
	if text == "" {
		return "", false
	}
	if evt.MessageType == msgTypeGroup && !atSelf(evt.Message, evt.SelfID) {
		return "", false
	}
	return text, true
}

// extractText 拼接所有 text 段的文本。
func extractText(segs []Segment) string {
	var b strings.Builder
	for _, s := range segs {
		if s.Type != "text" {
			continue
		}
		if t, ok := s.Data["text"].(string); ok {
			b.WriteString(t)
		}
	}
	return b.String()
}

// atSelf 判断消息段中是否 @ 了 selfID，@全体不算。
func atSelf(segs []Segment, selfID int64) bool {
	for _, s := range segs {
		if s.Type != "at" {
			continue
		}
		qq, ok := s.Data["qq"]
		if !ok {
			continue
		}
		switch v := qq.(type) {
		case string:
			if v == "all" {
				continue
			}
			id, err := strconv.ParseInt(v, 10, 64)
			if err == nil && id == selfID {
				return true
			}
		case float64:
			if int64(v) == selfID {
				return true
			}
		}
	}
	return false
}

// chatKey 构造会话键：私聊按用户，群聊按群。
func chatKey(messageType string, userID, groupID int64) string {
	if messageType == msgTypeGroup {
		return "group:" + strconv.FormatInt(groupID, 10)
	}
	return "private:" + strconv.FormatInt(userID, 10)
}

// buildSendMsg 构建 send_msg API 请求。
func buildSendMsg(messageType string, userID, groupID int64, echo, text string) ([]byte, error) {
	return json.Marshal(apiRequest{
		Action: "send_msg",
		Params: sendMsgParams{
			MessageType: messageType,
			UserID:      userID,
			GroupID:     groupID,
			Message:     text,
		},
		Echo: echo,
	})
}

// splitText 按 maxMsgRunes 分段，保证不截断多字节字符。
func splitText(text string) []string {
	runes := []rune(text)
	if len(runes) <= maxMsgRunes {
		return []string{text}
	}
	parts := make([]string, 0, len(runes)/maxMsgRunes+1)
	for i := 0; i < len(runes); i += maxMsgRunes {
		parts = append(parts, string(runes[i:min(i+maxMsgRunes, len(runes))]))
	}
	return parts
}
