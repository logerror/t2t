package http

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/logerror/easylog"

	"github.com/logerror/t2t/internal/server/handler"
	"github.com/logerror/t2t/internal/server/web"
	"github.com/logerror/t2t/pkg/config"
	"github.com/logerror/t2t/pkg/util/authutil"
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

	mux.HandleFunc("/login", handler.ServeLoginPage)
	mux.HandleFunc("/", AuthMiddleware(handler.ServeIndexPage))
	mux.HandleFunc("/api/login", handler.Login)
	mux.HandleFunc("/help", handler.IndexHelper)
	mux.HandleFunc("/agents", handler.ListAgents)
	mux.HandleFunc("/agent/", handler.AgentOption)
	mux.HandleFunc("/version", handler.StableVersion)
	//mux.HandleFunc("/check", AuthMiddleware(handler.CheckConnections))
	mux.HandleFunc("/check", handler.CheckConnections)
	mux.HandleFunc("/check/", handler.CheckConnection)

	// 静态文件服务
	fs := http.FileServer(http.FS(web.StaticFiles))
	mux.Handle("/static/", http.StripPrefix("/", fs))

	// Web终端页面和WebSocket处理
	mux.HandleFunc("/terminal", AuthMiddleware(handler.ServeTerminal))
	mux.HandleFunc("/terminal/ws", AuthMiddleware(handler.HandleTerminalWS))

	//mux.Handle("/ws/", xwebsocket.Handler(handler.HandleWebSocket))
	//mux.Handle("/attach/", xwebsocket.Handler(handler.HandleAttach))

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

func isPageRequest(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html") || strings.Contains(accept, "application/xhtml+xml")
}

func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			cookie, err := r.Cookie("token")
			if err == nil {
				token = cookie.Value
			}
		}
		if token == "" {
			if isPageRequest(r) {
				http.Redirect(w, r, "/login", http.StatusFound)
			} else {
				http.Error(w, `{"error": "missing token"}`, http.StatusUnauthorized)
			}
			return
		}
		claims, err := authutil.VerifyToken(token)
		if err != nil || claims == nil {
			if isPageRequest(r) {
				http.Redirect(w, r, "/login", http.StatusFound)
			} else {
				http.Error(w, `{"error": "invalid token"}`, http.StatusUnauthorized)
			}
			return
		}
		next.ServeHTTP(w, r)
	}
}
