package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/logerror/easylog"
	"github.com/logerror/t2t/internal/server/web"
	"go.uber.org/zap"
)

var tUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
}

// TerminalMessage 定义终端消息结构
type TerminalMessage struct {
	Type string `json:"type"`
	Rows int    `json:"rows,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Data string `json:"data,omitempty"`
}

// ServeTerminal 提供终端页面
func ServeTerminal(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFS(web.TemplateFiles, "templates/terminal.html"))
	tmpl.Execute(w, nil)
}

// HandleTerminalWS 处理Web终端的WebSocket连接
func HandleTerminalWS(w http.ResponseWriter, r *http.Request) {
	hostTag := r.URL.Query().Get("hostTag")
	clientId := r.URL.Query().Get("clientId")

	if hostTag == "" || clientId == "" {
		http.Error(w, "Missing hostTag or clientId", http.StatusBadRequest)
		return
	}

	// 升级HTTP连接为WebSocket
	webConn, err := tUpgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Could not upgrade connection", http.StatusInternalServerError)
		return
	}
	defer webConn.Close()

	// 获取对应的agent连接
	tag := fmt.Sprintf("%s-%s", hostTag, clientId)

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// 如果解析失败，直接使用 RemoteAddr
		ip = r.RemoteAddr
	}
	c, exist := connManager.clients[tag]
	if exist {
		c.ws = webConn
		c.clientUser = ip
		c.clientVersion = "Web Terminal"
	} else {
		connManager.clients[tag] = &ClientConnection{
			ws:            webConn,
			hostTag:       hostTag,
			clientId:      clientId,
			clientUser:    ip,
			clientVersion: "web",
		}
	}

	agentConn, exists := connManager.agents[tag]
	if !exists || agentConn == nil {
		webConn.WriteMessage(websocket.TextMessage, []byte("\x1b[31mAgent connection not found\x1b[0m\r\n"))
		return
	}

	easylog.Info("Web terminal connection established",
		zap.String("hostTag", hostTag),
		zap.String("clientId", clientId))

	// 设置WebSocket配置
	webConn.SetReadDeadline(time.Time{})  // 清除读取超时
	webConn.SetWriteDeadline(time.Time{}) // 清除写入超时

	// 设置ping处理
	webConn.SetPingHandler(func(appData string) error {
		return webConn.WriteControl(websocket.PongMessage, []byte{}, time.Now().Add(time.Second))
	})

	// 创建双向数据转发
	var wg sync.WaitGroup
	wg.Add(2)

	// 创建关闭通道
	done := make(chan struct{})
	defer close(done)

	// Web -> Agent
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				messageType, message, err := webConn.ReadMessage()
				if err != nil {
					easylog.Error("Error reading from web terminal",
						zap.String("tag", tag),
						zap.Error(err))
					return
				}

				// 检查是否是调整大小的消息
				if messageType == websocket.TextMessage {
					var termMsg TerminalMessage
					if err := json.Unmarshal(message, &termMsg); err == nil && termMsg.Type == "resize" {
						sizeMessage := []byte(fmt.Sprintf("\x1b[8;%d;%dt", termMsg.Rows, termMsg.Cols))
						if err := agentConn.ws.WriteMessage(websocket.BinaryMessage, sizeMessage); err != nil {
							easylog.Error("Error sending resize message",
								zap.String("tag", tag),
								zap.Error(err))
							return
						}
						continue
					}
				}

				// 转发消息到agent
				if err := agentConn.ws.WriteMessage(messageType, message); err != nil {
					easylog.Error("Error forwarding message to agent",
						zap.String("tag", tag),
						zap.Error(err))
					return
				}
			}
		}
	}()

	// Agent -> Web
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				messageType, message, err := agentConn.ws.ReadMessage()
				if err != nil {
					easylog.Error("Error reading from agent",
						zap.String("tag", tag),
						zap.Error(err))
					return
				}

				if err := webConn.WriteMessage(messageType, message); err != nil {
					easylog.Error("Error forwarding message to web terminal",
						zap.String("tag", tag),
						zap.Error(err))
					return
				}
			}
		}
	}()

	// 等待连接结束
	wg.Wait()
	easylog.Info("Web terminal connection closed",
		zap.String("hostTag", hostTag),
		zap.String("clientId", clientId))
}
