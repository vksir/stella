// Package server 提供 HTTP 与 WebSocket 服务。
package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"

	"github.com/vksir/stella/internal/config"
)

// Server 是 HTTP 服务器。
type Server struct {
	httpServer *http.Server
}

// New 创建 HTTP 服务器并注册路由。
func New(cfg config.ServerConfig) *Server {
	r := chi.NewRouter()
	r.Get(cfg.WSPath, handleWS)

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

// handleWS 处理 OneBot 适配器的 WebSocket 连接，读取消息直到连接关闭。
func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // 适配器连接不携带 Origin
	})
	if err != nil {
		slog.Warn("accept websocket failed", "err", err)
		return
	}
	defer conn.CloseNow()
	slog.Info("websocket connected", "remote", r.RemoteAddr)

	ctx := conn.CloseRead(r.Context())
	for {
		_, _, err := conn.Read(ctx)
		if err == nil {
			continue
		}
		if websocket.CloseStatus(err) != -1 {
			slog.Info("websocket closed", "remote", r.RemoteAddr, "err", err)
		} else {
			slog.Warn("read websocket message failed", "err", err)
		}
		return
	}
}
