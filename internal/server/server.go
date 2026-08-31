// Package server 提供 HTTP 与 WebSocket 服务。
package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/vksir/stella/internal/config"
	"github.com/vksir/stella/internal/onebot"
)

// Server 是 HTTP 服务器。
type Server struct {
	httpServer *http.Server
}

// New 创建 HTTP 服务器并注册 OneBot 反向 WebSocket 路由。
func New(cfg config.ServerConfig, accessToken string, handler onebot.MessageHandler) *Server {
	r := chi.NewRouter()
	r.Get(cfg.WSPath, handleWS(accessToken, handler))

	return &Server{
		httpServer: &http.Server{
			Addr:    cfg.Listen,
			Handler: r,
		},
	}
}

// ListenAndServe 启动服务器，返回服务器停止时的错误。
func (s *Server) ListenAndServe() error {
	slog.Info("http server listening", "addr", s.httpServer.Addr)
	err := s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown 在超时时间内优雅停止服务器。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// handleWS 处理 OneBot 适配器的反向 WebSocket 连接。
func handleWS(accessToken string, handler onebot.MessageHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if accessToken != "" && r.Header.Get("Authorization") != "Bearer "+accessToken {
			slog.Warn("websocket auth failed", "remote", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true, // 适配器连接不携带 Origin
		})
		if err != nil {
			slog.Warn("accept websocket failed", "err", err)
			return
		}
		defer conn.CloseNow()
		slog.Info("websocket connected", "remote", r.RemoteAddr)

		// 连接关闭时取消收发，不影响共享会话运行。
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		var mu sync.Mutex
		send := func(ctx context.Context, payload []byte) error {
			mu.Lock()
			defer mu.Unlock()
			return conn.Write(ctx, websocket.MessageText, payload)
		}

		for {
			typ, data, err := conn.Read(ctx)
			if err != nil {
				if websocket.CloseStatus(err) != -1 {
					slog.Info("websocket closed", "remote", r.RemoteAddr, "err", err)
				} else {
					slog.Warn("read websocket message failed", "err", err)
				}
				return
			}
			if typ != websocket.MessageText {
				continue
			}
			if err := handler(ctx, data, send); err != nil {
				slog.Warn("handle onebot message failed", "err", err)
			}
		}
	}
}
