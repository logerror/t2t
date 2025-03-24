package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/logerror/t2t/pkg/constants/svcconstants"
	"github.com/logerror/t2t/pkg/util/commonutil"
	"github.com/logerror/t2t/pkg/util/versionutil"
	"golang.org/x/sys/unix"
)

// 定义重连相关常量
const (
	maxRetries    = 3
	retryInterval = 5 * time.Second
)

// Agent 结构体
type Agent struct {
	hostTag     string
	clientId    string
	ws          *websocket.Conn
	shellCmd    *exec.Cmd
	pty         *os.File
	mutex       sync.Mutex
	shellMutex  sync.Mutex
	isConnected bool
	exitChan    chan struct{}
	termType    string
}

// 创建新的 Agent
func NewAgent(hostTag, clientId string) *Agent {
	return &Agent{
		hostTag:  hostTag,
		clientId: clientId,
		exitChan: make(chan struct{}),
	}
}

// 建立 WebSocket 连接
func (a *Agent) connect() error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	dialer := websocket.Dialer{
		ReadBufferSize:  32 * 1024,
		WriteBufferSize: 32 * 1024,
	}

	headers := http.Header{}
	headers.Set(svcconstants.WsT2TAgentTokenHeader, "ba8Eg6GQVNpRv6d0")
	headers.Set(svcconstants.WsT2TAgentArchHeader, runtime.GOARCH)
	headers.Set(svcconstants.WsT2TAgentVersionHeader, versionutil.GetCurrentAgentVersion())

	a.termType = "xterm-256color"

	url := fmt.Sprintf("%s://%s/v2/ws/%s/%s",
		svcconstants.AgentServerWsSchema,
		svcconstants.AgentServerHost,
		a.hostTag,
		a.clientId)

	ws, _, err := dialer.Dial(url, headers)
	if err != nil {
		return fmt.Errorf("连接服务器失败: %v", err)
	}

	a.ws = ws
	a.isConnected = true
	return nil
}

// 重连方法
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

// 处理特殊命令
func (a *Agent) handleSpecialCommand(input []byte) bool {
	if bytes.Equal(bytes.TrimSpace(input), []byte("exit")) {
		msg := "\r\n[提示] 使用 Ctrl+C 断开连接，或使用 exit! 强制退出 shell\r\n"
		if err := a.ws.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			log.Printf("发送提示消息失败: %v", err)
		}
		return true
	}
	return false
}

// 重启 shell
func (a *Agent) restartShell() error {
	a.shellMutex.Lock()
	defer a.shellMutex.Unlock()

	// 关闭旧的 shell
	if a.shellCmd != nil && a.shellCmd.Process != nil {
		a.shellCmd.Process.Kill()
	}
	if a.pty != nil {
		a.pty.Close()
	}

	// 启动新的 shell
	return a.setupShell()
}

// 设置 Shell
func (a *Agent) setupShell() error {
	currentShell := "/bin/bash"
	if _, err := os.Stat(currentShell); err != nil {
		currentShell = "/bin/sh"
	}

	cmd := exec.Command(currentShell)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("TERM=%s", a.termType),
		"COLORTERM=truecolor",
	)
	u, err := user.Current()
	if err == nil && u.HomeDir != "" {
		cmd.Dir = u.HomeDir
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("启动 PTY 失败: %v", err)
	}

	// 添加：设置初始窗口大小
	if err := pty.Setsize(ptmx, &pty.Winsize{
		Rows: 24,
		Cols: 80,
		X:    0,
		Y:    0,
	}); err != nil {
		return fmt.Errorf("设置 PTY 大小失败: %v", err)
	}

	// 设置 PTY 的终端属性
	if err := a.setupPtyAttr(ptmx); err != nil {
		return fmt.Errorf("设置 PTY 属性失败: %v", err)
	}

	a.shellCmd = cmd
	a.pty = ptmx
	return nil
}

// 检查更新
func checkForUpdates() (bool, string) {
	currentVersion := versionutil.GetCurrentAgentVersion()
	latestVersion, err := versionutil.GetLatestVersion()
	if err != nil {
		log.Printf("获取最新版本失败: %v", err)
		return false, ""
	}

	if latestVersion.Agent != currentVersion {
		return true, latestVersion.Agent
	}
	return false, ""
}

// 下载更新
func downloadUpdate(version string) error {
	url := fmt.Sprintf("%s://%s/public/agent/t2t-agent-%s-%s-%s", svcconstants.AgentServerHttpSchema, svcconstants.AgentServerHost, runtime.GOOS, runtime.GOARCH, version)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("下载更新失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("更新服务器返回错误: %s", resp.Status)
	}

	out, err := os.Create("t2t-agent-updated")
	if err != nil {
		return fmt.Errorf("创建更新文件失败: %v", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("写入更新文件失败: %v", err)
	}

	// 设置下载的文件为可执行文件
	if err := os.Chmod("t2t-agent-updated", 0755); err != nil {
		return fmt.Errorf("设置文件权限失败: %v", err)
	}

	return nil
}

// 替换当前执行文件并重启
func replaceCurrentExecutable() error {
	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取当前执行文件路径失败: %v", err)
	}

	err = os.Rename("t2t-agent-updated", currentPath)
	if err != nil {
		return fmt.Errorf("替换当前执行文件失败: %v", err)
	}

	// 重启程序
	cmd := exec.Command(currentPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		return err
	}

	return nil
}

// 启动 Agent
func (a *Agent) Start() error {
	// 检查更新
	//needsUpdate, newVersion := checkForUpdates()
	//if needsUpdate {
	//	log.Printf("发现新版本: %s，开始下载更新...", newVersion)
	//	if err := downloadUpdate(newVersion); err != nil {
	//		log.Printf("更新下载失败: %v", err)
	//		return err
	//	}
	//	log.Printf("更新下载完成，替换当前执行文件...")
	//	if err := replaceCurrentExecutable(); err != nil {
	//		log.Printf("更新替换失败: %v", err)
	//		return err
	//	}
	//	log.Printf("更新完成，请重启程序以应用新版本.")
	//	os.Exit(0) // 退出以便用户手动重启
	//}

	// 继续启动逻辑...
	if err := a.connect(); err != nil {
		return err
	}

	// 创建并启动 shell
	if err := a.setupShell(); err != nil {
		return err
	}

	// 启动数据转发
	go a.handlePtyToWs()
	go a.handleWsToPty()

	return nil
}

// 处理从 PTY 到 WebSocket 的数据转发
func (a *Agent) handlePtyToWs() {
	for {
		select {
		case <-a.exitChan:
			return
		default:
			if !a.isConnected {
				time.Sleep(time.Second)
				continue
			}

			// 为每次读取创建新的缓冲区
			buf := make([]byte, 32*1024)
			n, err := a.pty.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("从 PTY 读取失败: %v", err)
				}
				if strings.Contains(err.Error(), "input/output error") {
					log.Printf("Shell 可能已退出，尝试重启")
					if err := a.restartShell(); err != nil {
						log.Printf("重启 shell 失败: %v", err)
						return
					}
					continue
				}
				return
			}

			// 创建新的切片来存储实际数据
			data := make([]byte, n)
			copy(data, buf[:n])

			a.mutex.Lock()
			ws := a.ws
			a.mutex.Unlock()

			if ws == nil {
				continue
			}

			if err := ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
				log.Printf("写入 WebSocket 失败: %v", err)
				//if err := a.reconnect(); err != nil {
				//	log.Printf("重连失败: %v", err)
				//	return
				//}
				continue
			}
		}
	}
}

func (a *Agent) setupPtyAttr(ptmx *os.File) error {
	termios, err := unix.IoctlGetTermios(int(ptmx.Fd()), unix.TCGETS)
	if err != nil {
		return err
	}

	// 修改终端属性
	termios.Iflag |= unix.ICRNL  // 将 CR 转换为 NL
	termios.Oflag |= unix.ONLCR  // 将 NL 转换为 CR-NL
	termios.Lflag |= unix.ICANON // 启用规范模式
	termios.Lflag |= unix.ECHO   // 启用回显
	termios.Oflag |= unix.OPOST  // 启用输出处理
	termios.Oflag |= unix.ONLRET // 在回车时不输出回车符

	// 设置自动回绕
	termios.Lflag |= unix.ECHOE // 擦除字符时擦除
	termios.Lflag |= unix.ECHOK // 删除行时输出换行

	// 设置输入缓冲区大小
	termios.Cc[unix.VMIN] = 1
	termios.Cc[unix.VTIME] = 0

	return unix.IoctlSetTermios(int(ptmx.Fd()), unix.TCSETS, termios)
}

// 处理从 WebSocket 到 PTY 的数据转发
func (a *Agent) handleWsToPty() {
	for {
		select {
		case <-a.exitChan:
			return
		default:
			if !a.isConnected {
				time.Sleep(time.Second)
				continue
			}
			ws := a.ws
			if ws == nil {
				continue
			}

			messageType, data, err := ws.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err,
					websocket.CloseNormalClosure,
					websocket.CloseGoingAway) {
					log.Printf("从 WebSocket 读取失败: %v", err)
					if err = a.reconnect(); err != nil {
						log.Printf("重连失败: %v", err)
						return
					}
				}
				continue
			}
			fmt.Printf("Received input type : %d\n", messageType)
			fmt.Printf("Received input data : %v\n", data)

			// 只处理文本和二进制消息
			if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
				continue
			}

			if a.handleSpecialCommand(data) {
				continue
			}

			// 检查是否是终端大小变化消息
			if messageType == websocket.TextMessage && len(data) > 0 &&
				strings.HasPrefix(string(data), "\x1b[8;") &&
				strings.HasSuffix(string(data), "t") {
				a.handleWindowResize(data)
				continue
			}

			// 其他数据正常写入 PTY
			a.shellMutex.Lock()
			_, err = a.pty.Write(data)
			a.shellMutex.Unlock()

			if err != nil {
				if err.Error() == "input/output error" {
					log.Printf("Shell 可能已退出，尝试重启")
					if err := a.restartShell(); err != nil {
						log.Printf("重启 shell 失败: %v", err)
						return
					}
					continue
				}
				log.Printf("写入 PTY 失败: %v", err)
				return
			}
		}
	}
}

// 添加：处理终端大小变化
func (a *Agent) handleWindowResize(data []byte) {
	// 解析客户端发送的终端大小信息
	var rows, cols uint16
	fmt.Sscanf(string(data), "\x1b[8;%d;%dt", &rows, &cols)

	if a.pty != nil {
		pty.Setsize(a.pty, &pty.Winsize{
			Rows: rows,
			Cols: cols,
			X:    0,
			Y:    0,
		})
	}
}

func main() {
	printHelpInfo()
	hostTag, clientId := getHostTagAndClientId()

	fmt.Println("将以下信息发送给需要远程该机器的用户")
	fmt.Println("Host信息: " + hostTag)
	fmt.Println("授权码:   " + clientId)
	agent := NewAgent(hostTag, clientId)
	if err := agent.Start(); err != nil {
		log.Fatalf("启动 Agent 失败: %v", err)
	}

	// 等待信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// 清理资源
	cleanUp(hostTag, clientId)
	if agent.ws != nil {
		agent.ws.Close()
	}
	if agent.pty != nil {
		agent.pty.Close()
	}
	if agent.shellCmd != nil && agent.shellCmd.Process != nil {
		agent.shellCmd.Process.Kill()
	}
}

func getHostTagAndClientId() (string, string) {
	hostTag := "default"
	hostName, err := os.Hostname()
	if err != nil {
		fmt.Printf("Error getting hostname: %v\n", err)
	} else {
		hostTag = hostName
	}

	rand.Seed(time.Now().UnixNano())
	digits := "0123456789"
	length := 10
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = digits[rand.Intn(len(digits))]
	}

	clientId := string(result)
	return hostTag, clientId
}

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func printHelpInfo() {
	commonutil.PrintAgentFlag()
	currentVersion := versionutil.GetCurrentAgentVersion()
	latestVersion, err := versionutil.GetLatestVersion()
	if err != nil {
		fmt.Printf("Error getting latest version: %v\n", err)
		//os.Exit(1)
	}

	helpUrl := fmt.Sprintf("%s://%s", svcconstants.AgentServerHttpSchema, svcconstants.AgentServerHost)
	fmt.Printf("当前版本: %s\n", currentVersion)
	fmt.Printf("最新版本: %s\n", latestVersion.Agent)
	if latestVersion != nil && currentVersion != latestVersion.Agent {
		fmt.Printf("建议更新到最新版本后再运行此程序。参考: %s \n", helpUrl)
	} else {
		fmt.Printf("使用说明: %s \n", helpUrl)
	}
	fmt.Printf("#################################################################\n")
}

func cleanUp(hostTag, clientId string) {
	url := fmt.Sprintf("%s://%s/agent/%s/%s", svcconstants.AgentServerHttpSchema, svcconstants.AgentServerHost, hostTag, clientId)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		log.Fatalf("创建请求失败: %v", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("读取响应失败: %v", err)
	}

	// 打印响应状态码和响应体
	fmt.Printf("res: %d\n", resp.StatusCode)
}
