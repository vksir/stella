package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/vksir/stella/internal/infra/config"
)

type Server struct {
	server *http.Server
}

func New(cfg config.ServerConfig, handler http.Handler) *Server {
	return &Server{
		server: &http.Server{
			Addr:    cfg.Listen,
			Handler: handler,
		},
	}
}

func (s *Server) ListenAndServe() error {
	slog.Info("http server listening", "addr", s.server.Addr)
	err := s.server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
