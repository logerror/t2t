package http

import (
	"fmt"
	"net/http"

	"github.com/logerror/t2t/internal/server/web"

	xwebsocket "golang.org/x/net/websocket"

	"github.com/logerror/easylog"
	"github.com/logerror/t2t/internal/server/handler"
	"github.com/logerror/t2t/pkg/config"
	"go.uber.org/zap"
	"golang.org/x/net/context"
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

	mux.HandleFunc("/", handler.ServeIndexPage)
	mux.HandleFunc("/help", handler.IndexHelper)
	mux.HandleFunc("/agents", handler.ListAgents)
	mux.HandleFunc("/agent/", handler.AgentOption)
	mux.HandleFunc("/version", handler.StableVersion)
	mux.HandleFunc("/check", handler.CheckConnections)
	mux.HandleFunc("/check/", handler.CheckConnection)

	// 添加静态文件服务
	fs := http.FileServer(http.FS(web.StaticFiles))
	mux.Handle("/static/", http.StripPrefix("/", fs))

	// Web终端页面和WebSocket处理
	mux.HandleFunc("/terminal", handler.ServeTerminal)
	mux.HandleFunc("/terminal/ws", handler.HandleTerminalWS)

	mux.Handle("/ws/", xwebsocket.Handler(handler.HandleWebSocket))
	mux.Handle("/attach/", xwebsocket.Handler(handler.HandleAttach))

	// for v2
	mux.HandleFunc("/v2/ws/", handler.HandleWebSocketV2)
	mux.HandleFunc("/v2/attach/", handler.HandleAttachV2)

	fileServer := http.FileServer(http.Dir("/tmp/server_cache/public"))
	mux.Handle("/public/", http.StripPrefix("/public", fileServer))

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.cfg.Server.Port),
		Handler: mux,
	}

	easylog.Info("Server started on " + fmt.Sprintf(":%d", s.cfg.Server.Port))
	return s.server.ListenAndServe()
}
