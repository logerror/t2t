package agent

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/logerror/t2t/pkg/constants/svcconstants"

	"github.com/gorilla/websocket"
	"github.com/logerror/t2t/pkg/util/versionutil"
)

const (
	maxRetries    = 3
	retryInterval = 5 * time.Second
)

type Agent struct {
	ws          *websocket.Conn
	terminal    Terminal
	hostTag     string
	clientId    string
	isConnected bool
	exitChan    chan struct{}
	mutex       sync.Mutex

	// 命令监控
	shellMonitor *ShellMonitor
}

func NewAgent(ws *websocket.Conn, terminal Terminal, hostTag, clientId string) *Agent {
	return &Agent{
		ws:       ws,
		terminal: terminal,
		hostTag:  hostTag,
		clientId: clientId,
		exitChan: make(chan struct{}),
	}
}

func (a *Agent) Start(ctx context.Context) error {
	// 初始化命令监控器
	a.shellMonitor = NewShellMonitor(a.hostTag, a.clientId, "unknown")

	// 启动命令监控
	a.shellMonitor.Start()
	if err := a.terminal.StartShell(); err != nil {
		return fmt.Errorf("shell 启动失败: %v", err)
	}

	// 版本检查
	if err := a.checkForUpdates(); err != nil {
		log.Printf("版本检查失败: %v", err)
	}
	a.isConnected = true
	go a.ptyToWs(ctx)
	go a.wsToPty(ctx)
	return nil
}

// 版本检查
func (a *Agent) checkForUpdates() error {
	currentVersion := versionutil.GetCurrentAgentVersion()
	latestVersion, err := versionutil.GetLatestVersion()
	if err != nil {
		return err
	}
	if latestVersion.Agent != currentVersion {
		log.Printf("发现新版本: %s，建议升级", latestVersion.Agent)
	}
	return nil
}

// 支持重连
func (a *Agent) reconnect() error {
	log.Printf("开始重连...")
	for i := 0; i < maxRetries; i++ {
		if err := a.connect(); err != nil {
			log.Printf("重连尝试 %d 失败: %v", i+1, err)
			time.Sleep(retryInterval)
			continue
		}
		log.Printf("重连成功")
		return nil
	}
	return fmt.Errorf("重连失败，已达到最大重试次数")
}

// 重新建立 websocket 连接（可根据实际情况调整参数）
func (a *Agent) connect() error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	dialer := websocket.Dialer{
		ReadBufferSize:  32 * 1024,
		WriteBufferSize: 32 * 1024,
	}

	headers := http.Header{}
	headers.Set(svcconstants.WsT2TAgentTokenHeader, "ba8Eg6GQVNpRv6d0") // 如有 token 可替换
	headers.Set(svcconstants.WsT2TAgentArchHeader, runtime.GOARCH)
	headers.Set(svcconstants.WsT2TAgentVersionHeader, versionutil.GetCurrentAgentVersion())

	url := fmt.Sprintf("%s://%s/v2/ws/%s/%s",
		svcconstants.AgentServerWsSchema,
		svcconstants.AgentServerHost,
		a.hostTag,
		a.clientId,
	)

	ws, _, err := dialer.Dial(url, headers)
	if err != nil {
		return fmt.Errorf("连接服务器失败: %v", err)
	}

	a.ws = ws
	a.isConnected = true
	return nil
}

func (a *Agent) ptyToWs(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.exitChan:
			return
		default:
			buf := make([]byte, 32*1024)
			n, err := a.terminal.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("从 PTY 读取失败: %v", err)
				}
				// shell 退出，自动重启
				if err := a.restartShell(); err != nil {
					log.Printf("重启 shell 失败: %v", err)
					return
				}
				continue
			}
			data := buf[:n]
			a.mutex.Lock()
			ws := a.ws
			a.mutex.Unlock()
			if ws == nil {
				continue
			}
			if err := ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
				log.Printf("写入 WebSocket 失败: %v", err)
				// 连接断开，尝试重连
				if err := a.reconnect(); err != nil {
					log.Printf("重连失败: %v", err)
					return
				}
				continue
			}
		}
	}
}

func (a *Agent) wsToPty(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.exitChan:
			return
		default:
			ws := a.ws
			if ws == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			messageType, data, err := ws.ReadMessage()
			if err != nil {
				log.Printf("从 WebSocket 读取失败: %v", err)
				// 连接断开，尝试重连
				if err := a.reconnect(); err != nil {
					log.Printf("重连失败: %v", err)
					return
				}
				continue
			}
			if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
				continue
			}
			if a.handleSpecialCommand(data) {
				continue
			}
			if strings.HasPrefix(string(data), "\x1b[8;") && strings.HasSuffix(string(data), "t") {
				var rows, cols int
				fmt.Sscanf(string(data), "\x1b[8;%d;%dt", &rows, &cols)
				a.terminal.Resize(rows, cols)
				continue
			}
			if _, err := a.terminal.Write(data); err != nil {
				log.Printf("写入 PTY 失败: %v", err)
				// shell 退出，自动重启
				if err := a.restartShell(); err != nil {
					log.Printf("重启 shell 失败: %v", err)
					return
				}
				continue
			}
		}
	}
}

// shell 自动重启
func (a *Agent) restartShell() error {
	log.Printf("Shell 退出，尝试自动重启...")
	return a.terminal.StartShell()
}

func (a *Agent) handleSpecialCommand(input []byte) bool {
	if strings.TrimSpace(string(input)) == "exit" {
		msg := "\r\n[提示] 使用 Ctrl+C 断开连接，或使用 exit! 强制退出 shell\r\n"
		if err := a.ws.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			log.Printf("发送提示消息失败: %v", err)
		}
		return true
	}
	return false
}

func (a *Agent) Close() {
	close(a.exitChan)

	// 停止命令监控
	if a.shellMonitor != nil {
		a.shellMonitor.Stop()
	}

	a.terminal.Close()
	if a.ws != nil {
		a.ws.Close()
	}
}
