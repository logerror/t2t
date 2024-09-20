package http

import (
	"fmt"
	"net/http"

	"github.com/logerror/easylog"

	"github.com/logerror/t2t/internal/server/handler"
	"github.com/logerror/t2t/pkg/config"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"golang.org/x/net/websocket"
)

type Server struct {
	cfg    *config.Config
	server *http.Server
}

func NewServer(cfg *config.Config) *Server {
	return &Server{
		cfg: cfg,
	}
}

func (s *Server) Shutdown() {
	easylog.Info("Shutting down agent server ...")
	if err := s.server.Shutdown(context.TODO()); err != nil {
		easylog.Error("Shutdown agent server error", zap.Error(err))
	}
}

func (s *Server) ListenAndServe() error {

	mux := http.NewServeMux()

	mux.HandleFunc("/", handler.IndexHelper)
	mux.HandleFunc("/agents", handler.ListAgents)
	mux.HandleFunc("/agent/", handler.AgentOption)
	mux.HandleFunc("/version", handler.StableVersion)

	fileServer := http.FileServer(http.Dir("/tmp/server_cache/public"))
	mux.Handle("/public/", http.StripPrefix("/public", fileServer))
	mux.Handle("/ws/", websocket.Handler(handler.HandleWebSocket))
	mux.Handle("/attach/", websocket.Handler(handler.HandleAttach))

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.cfg.Server.Port),
		Handler: mux,
	}

	easylog.Info("Server started on " + fmt.Sprintf(":%d", s.cfg.Server.Port))
	return s.server.ListenAndServe()
}
