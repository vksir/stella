package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/vksir/stella/internal/onebot"
)

// TestHandleWS 验证一帧消息送达 handler 后连接仍保持，可继续收发消息。
func TestHandleWS(t *testing.T) {
	var calls atomic.Int32
	handler := func(_ context.Context, _ []byte, _ onebot.SendFunc) error {
		calls.Add(1)
		return nil
	}
	srv := httptest.NewServer(handleWS("", handler))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()

	for i := 0; i < 2; i++ {
		if err := conn.Write(ctx, websocket.MessageText, []byte("{}")); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("handler called %d times, want 2 (connection was closed after first message)", got)
	}
}