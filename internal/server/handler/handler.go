package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"regexp"
	"sync"
	"syscall"
	"time"

	"github.com/logerror/easylog"
	"github.com/logerror/t2t/pkg/config"
	"github.com/logerror/t2t/pkg/data/version"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/net/websocket"
)

var (
	Clients   = make(map[string]*websocket.Conn)
	mutex     sync.Mutex
	closeChan = make(chan string)
)

func HandleWebSocket(ws *websocket.Conn) {
	urlPath := ws.Request().URL.Path
	easylog.Info("Handling connection", zap.String("urlPath", urlPath))

	pathPattern := regexp.MustCompile(`^/ws/([a-zA-Z0-9_-]+)/([0-9]+)$`)

	matches := pathPattern.FindStringSubmatch(urlPath)
	if matches == nil {
		easylog.Info("Invalid path", zap.String("path", urlPath))
		ws.Close()
		return
	}

	mutex.Lock()
	tag := fmt.Sprintf("%s-%s", matches[1], matches[2])
	Clients[tag] = ws
	mutex.Unlock()

	defer func() {
		mutex.Lock()
		delete(Clients, tag)
		mutex.Unlock()
		easylog.Info("Connection closed", zap.String("urlPath", urlPath))
	}()

	select {}
}

func HandleAttach(wsAttach *websocket.Conn) {
	clientUser := wsAttach.Request().Header.Get("X-T2T-Client-User")
	urlPath := wsAttach.Request().URL.Path

	pathPattern := regexp.MustCompile(`^/attach/([a-zA-Z0-9_-]+)/([0-9]+)$`)
	matches := pathPattern.FindStringSubmatch(urlPath)
	if matches == nil {
		easylog.Info("Invalid attach request", zap.String("path", urlPath), zap.String("clientUser", clientUser))
		wsAttach.Write([]byte("Invalid attach request"))
		wsAttach.Close()
		return
	}

	hostTag := matches[1]
	clientId := matches[2]
	socketTag := fmt.Sprintf("%s-%s", hostTag, clientId)
	easylog.Info("Handling attach", zap.String("clientUser", clientUser), zap.String("urlPath", urlPath))

	ws, ok := Clients[hostTag+"-"+clientId]
	if !ok {
		wsAttach.Write([]byte("Agent can not attach \\n"))
		wsAttach.Close()
		return
	}
	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("Error setting terminal to raw mode: %v", err)
	}
	defer terminal.Restore(int(os.Stdin.Fd()), oldState)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGWINCH)
	go func() {
		for range signalChan {
			// 获取当前终端的大小
			//width, height, err := terminal.GetSize(int(os.Stdin.Fd()))
			//if err != nil {
			//	log.Printf("Error getting terminal size: %v", err)
			//	continue
			//}

			//// 将终端大小通过 WebSocket 发送给客户端
			//sizeMessage := fmt.Sprintf("%dSIGWINCH%d", width, height)
			//if err := websocket.Message.Send(ws, sizeMessage); err != nil {
			//	log.Printf("Error sending terminal size: %v", err)
			//}
		}
	}()

	logPathDir := fmt.Sprintf("/tmp/server_cache/%s/%s", time.Now().Format("2006_01_02"), hostTag)
	err = os.MkdirAll(logPathDir, 0775)
	if err != nil {
		easylog.Error("Error creating directory:", zap.Error(err))
		return
	}
	file, err := os.OpenFile(path.Join(logPathDir, fmt.Sprintf("%s-%s", clientUser, clientId)), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		easylog.Error("failed to open log file", zap.Error(err))
		return
	}
	defer file.Close()

	tr := io.TeeReader(wsAttach, file)

	go func() {
		buf := make([]byte, 1024)
		for {
			n, readErr := tr.Read(buf)
			if readErr != nil {
				easylog.Error("read connection from client err",
					zap.String("clientUser", clientUser),
					zap.Error(readErr),
				)
				wsAttach.Close()
				break
			}
			_, err = ws.Write(buf[:n])
			if err != nil {
				return
			}
		}
	}()

	buf := make([]byte, 1024)
	for {
		n, readErr := ws.Read(buf)
		if readErr != nil {
			easylog.Error("read connection from agent err",
				zap.String("clientUser", clientUser),
				zap.String("hostTag", hostTag),
				zap.String("clientId", clientId),
				zap.Error(readErr),
			)
			delete(Clients, socketTag)
			break
		}
		_, writeErr := wsAttach.Write(buf[:n])
		if writeErr != nil {
			easylog.Error("Error sending data to client", zap.Error(writeErr))
			break
		}
	}
}

func ListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		jsonData, err := json.Marshal(Clients)
		if err != nil {
			http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func AgentOption(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		pathPattern := regexp.MustCompile(`^/agent/([a-zA-Z0-9_-]+)/([0-9]+)$`)
		matches := pathPattern.FindStringSubmatch(r.URL.Path)
		if matches == nil {
			easylog.Info("Invalid attach request", zap.String("path", r.URL.Path))
			return
		}

		hostTag := matches[1]
		clientId := matches[2]

		delete(Clients, hostTag+"-"+clientId)

		jsonData, err := json.Marshal(&Response{})
		if err != nil {
			http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func StableVersion(w http.ResponseWriter, r *http.Request) {
	jsonData, err := json.Marshal(&version.Version{
		Agent:  config.Configuration.Version.Agent,
		Client: config.Configuration.Version.Client,
		Server: config.Configuration.Version.Server,
	})
	if err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
		return
	}

	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func IndexHelper(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if r.URL.Path == "/" || r.URL.Path == "index" {
			http.Redirect(w, r, config.Configuration.HelpUrl, http.StatusFound)
		} else {
			http.Error(w, "Status not found", http.StatusNotFound)
		}
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
