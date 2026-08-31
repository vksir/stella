package onebot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestTrigger 覆盖消息触发判定：文本提取、@ 判定、空数组 fallback。
func TestTrigger(t *testing.T) {
	tests := []struct {
		name string
		evt  Event
		want string
		ok   bool
	}{
		{
			name: "private text",
			evt: Event{MessageType: msgTypePrivate, UserID: 100,
				Message: []Segment{{Type: "text", Data: map[string]any{"text": "你好"}}}},
			want: "你好", ok: true,
		},
		{
			name: "group at self",
			evt: Event{MessageType: msgTypeGroup, GroupID: 200, SelfID: 123,
				Message: []Segment{
					{Type: "at", Data: map[string]any{"qq": "123"}},
					{Type: "text", Data: map[string]any{"text": " 你好"}},
				}},
			want: " 你好", ok: true,
		},
		{
			name: "group at all not trigger",
			evt: Event{MessageType: msgTypeGroup, GroupID: 200, SelfID: 123,
				Message: []Segment{
					{Type: "at", Data: map[string]any{"qq": "all"}},
					{Type: "text", Data: map[string]any{"text": "大家好"}},
				}},
			ok: false,
		},
		{
			name: "group not at",
			evt: Event{MessageType: msgTypeGroup, GroupID: 200, SelfID: 123,
				Message: []Segment{{Type: "text", Data: map[string]any{"text": "你好"}}}},
			ok: false,
		},
		{
			name: "empty message fallback raw",
			evt:  Event{MessageType: msgTypePrivate, UserID: 100, RawMessage: "raw 文本"},
			want: "raw 文本", ok: true,
		},
		{
			name: "image only ignored",
			evt: Event{MessageType: msgTypePrivate, UserID: 100,
				Message: []Segment{{Type: "image", Data: map[string]any{"url": "http://x"}}}},
			ok: false,
		},
		{
			name: "group at self no text ignored",
			evt: Event{MessageType: msgTypeGroup, GroupID: 200, SelfID: 123,
				Message: []Segment{{Type: "at", Data: map[string]any{"qq": "123"}}}},
			ok: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := trigger(tt.evt)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("trigger() = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

// TestBuildSendMsg 覆盖 send_msg 请求构建。
func TestBuildSendMsg(t *testing.T) {
	payload, err := buildSendMsg(msgTypeGroup, 0, 200, "echo1", "hi")
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		Action string `json:"action"`
		Params struct {
			MessageType string `json:"message_type"`
			GroupID     int64  `json:"group_id"`
			Message     string `json:"message"`
		} `json:"params"`
		Echo string `json:"echo"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		t.Fatal(err)
	}
	if req.Action != "send_msg" || req.Params.MessageType != msgTypeGroup ||
		req.Params.GroupID != 200 || req.Params.Message != "hi" || req.Echo != "echo1" {
		t.Fatalf("unexpected request: %+v", req)
	}
	// 群聊不携带 user_id
	if strings.Contains(string(payload), "user_id") {
		t.Fatalf("group send_msg should not contain user_id: %s", payload)
	}
}

// TestSplitText 覆盖超长消息分段边界。
func TestSplitText(t *testing.T) {
	if got := splitText("短文本"); len(got) != 1 || got[0] != "短文本" {
		t.Fatalf("splitText(short) = %v", got)
	}

	text := strings.Repeat("中", maxMsgRunes+1)
	parts := splitText(text)
	if len(parts) != 2 {
		t.Fatalf("splitText parts = %d, want 2", len(parts))
	}
	if len([]rune(parts[0])) != maxMsgRunes || parts[1] != "中" {
		t.Fatalf("unexpected split: first %d runes, second %q", len([]rune(parts[0])), parts[1])
	}
}

// TestHandleMessage 覆盖入站帧识别，heartbeat 元事件的 status 为对象，不应报错。
func TestHandleMessage(t *testing.T) {
	ob := New(nil)
	// heartbeat 元事件（status 为对象）应被忽略且不报错
	if err := ob.HandleMessage(context.Background(), []byte(`{"post_type":"meta_event","meta_event_type":"heartbeat","status":{"online":true},"interval":5000}`), nil); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	// API 响应（status 为字符串）应被忽略且不报错
	if err := ob.HandleMessage(context.Background(), []byte(`{"status":"ok","retcode":0,"data":{"message_id":1},"echo":"e1"}`), nil); err != nil {
		t.Fatalf("api response: %v", err)
	}
	// API 失败响应应被忽略且不报错
	if err := ob.HandleMessage(context.Background(), []byte(`{"status":"failed","retcode":100,"echo":"e2"}`), nil); err != nil {
		t.Fatalf("api failed: %v", err)
	}
}

// TestChatKey 覆盖会话键构造，群临时会话按私聊归类。
func TestChatKey(t *testing.T) {
	if got := chatKey(msgTypePrivate, 100, 200); got != "private:100" {
		t.Fatalf("chatKey(private) = %s, want private:100", got)
	}
	if got := chatKey(msgTypeGroup, 0, 200); got != "group:200" {
		t.Fatalf("chatKey(group) = %s, want group:200", got)
	}
}
