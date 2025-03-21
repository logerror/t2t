package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/gorilla/websocket"
	"github.com/logerror/t2t/pkg/constants/svcconstants"
	"github.com/logerror/t2t/pkg/util/commonutil"
	"github.com/logerror/t2t/pkg/util/versionutil"
	"golang.org/x/crypto/ssh/terminal"
	xwebsocket "golang.org/x/net/websocket"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

type Client struct {
	ws          *websocket.Conn
	hostTag     string
	clientId    string
	oldState    *terminal.State
	exitChan    chan struct{}
	reconnectCh chan struct{}
	wsLock      sync.Mutex  // 添加锁来保护 WebSocket 操作
	isConnected atomic.Bool // 添加连接状态标志
}

func NewClient(hostTag, clientId string) *Client {
	return &Client{
		hostTag:     hostTag,
		clientId:    clientId,
		exitChan:    make(chan struct{}),
		reconnectCh: make(chan struct{}, 1),
	}
}

func (c *Client) connect() error {
	c.wsLock.Lock()
	defer c.wsLock.Unlock()

	dialer := websocket.Dialer{
		ReadBufferSize:  32 * 1024,
		WriteBufferSize: 32 * 1024,
	}

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("获取当前用户失败: %v", err)
	}

	headers := http.Header{}
	headers.Set(svcconstants.XWsT2TClientUserHeader, currentUser.Username)
	headers.Set(svcconstants.XWsT2TClientVersionHeader, versionutil.GetCurrentClientVersion())

	url := fmt.Sprintf("%s://%s/v2/attach/%s/%s",
		svcconstants.AgentServerWsSchema,
		svcconstants.AgentServerHost,
		c.hostTag,
		c.clientId)

	ws, _, err := dialer.Dial(url, headers)
	if err != nil {
		c.isConnected.Store(false)
		return fmt.Errorf("连接服务器失败: %v", err)
	}

	c.ws = ws
	c.isConnected.Store(true)
	return nil
}

// 设置终端
func (c *Client) setupTerminal() error {
	if !terminal.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("stdin is not a terminal")
	}

	fd := int(os.Stdin.Fd())
	oldState, err := terminal.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("设置终端失败: %v", err)
	}
	c.oldState = oldState

	termios, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return fmt.Errorf("获取终端属性失败: %v", err)
	}

	termios.Iflag |= unix.ICRNL  // 将 CR 转换为 NL
	termios.Oflag |= unix.ONLCR  // 输出时将 NL 转换为 CR-NL
	termios.Lflag |= unix.IEXTEN // 启用输入处理
	termios.Oflag |= unix.OPOST  // 启用输出处理
	termios.Oflag |= unix.ONLRET // 在回车时不输出回车符

	// 设置自动回绕
	termios.Lflag |= unix.ECHOE // 擦除字符时擦除
	termios.Lflag |= unix.ECHOK // 删除行时输出换行

	termios.Cc[unix.VMIN] = 1
	termios.Cc[unix.VTIME] = 0

	if err := unix.IoctlSetTermios(fd, unix.TIOCSETA, termios); err != nil {
		return fmt.Errorf("设置终端属性失败: %v", err)
	}

	// 获取并发送初始终端大小
	if width, height, err := terminal.GetSize(fd); err == nil {
		msg := fmt.Sprintf("\x1b[8;%d;%dt", height, width)
		c.ws.WriteMessage(websocket.TextMessage, []byte(msg))
	}

	return nil
}

// 恢复终端
func (c *Client) restoreTerminal() {
	if c.oldState != nil && terminal.IsTerminal(int(os.Stdin.Fd())) {
		terminal.Restore(int(os.Stdin.Fd()), c.oldState)
	}
}

// 处理终端大小变化
func (c *Client) handleTerminalResize() {
	// 检查是否在终端环境中
	if !terminal.IsTerminal(int(os.Stdin.Fd())) {
		return
	}

	width, height, err := terminal.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return
	}
	msg := fmt.Sprintf("\x1b[8;%d;%dt", height, width)
	c.ws.WriteMessage(websocket.TextMessage, []byte(msg))
}

// 处理特殊按键
func (c *Client) handleSpecialKeys() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGWINCH)

	go func() {
		ctrlCCount := 0
		ctrlCTimer := time.NewTimer(0)
		<-ctrlCTimer.C // 立即消耗初始计时

		for {
			select {
			case sig := <-sigChan:
				switch sig {
				case syscall.SIGINT:
					ctrlCCount++
					if ctrlCCount == 1 {
						fmt.Printf("\r\n[提示] 再次按 Ctrl+C 断开连接\r\n")
						ctrlCTimer = time.NewTimer(3 * time.Second)
						go func() {
							<-ctrlCTimer.C
							ctrlCCount = 0
						}()
					} else if ctrlCCount == 2 {
						fmt.Printf("\r\n[提示] 断开连接\r\n")
						close(c.exitChan)
						return
					}
				case syscall.SIGWINCH:
					c.handleTerminalResize()
				case syscall.SIGTERM:
					close(c.exitChan)
					return
				}
			case <-c.exitChan:
				return
			}
		}
	}()
}

// 添加一个新的方法来处理特殊命令
func (c *Client) handleSpecialCommand(data []byte) bool {
	// 检查是否是 "exit" 命令
	if bytes.Equal(bytes.TrimSpace(data), []byte("bye!")) {
		fmt.Printf("\r\n[提示] 断开连接\r\n")
		//close(c.exitChan)
		return true
	}
	return false
}

// 处理数据转发
func (c *Client) handleDataTransfer() {
	// 创建一个 WaitGroup 来等待所有 goroutine 完成
	var wg sync.WaitGroup
	wg.Add(2)

	// 从 WebSocket 读取数据写入标准输出
	go func() {
		defer wg.Done()
		for {
			select {
			case <-c.exitChan:
				return
			default:
				if !c.isConnected.Load() {
					time.Sleep(time.Second)
					continue
				}

				c.wsLock.Lock()
				ws := c.ws
				c.wsLock.Unlock()

				if ws == nil {
					continue
				}

				messageType, data, err := ws.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err,
						websocket.CloseNormalClosure,
						websocket.CloseGoingAway) {
						select {
						case <-c.exitChan:
							return
						default:
							fmt.Printf("\r\n[错误] 连接断开: %v\r\n", err)
							c.closeWs()
							c.reconnectCh <- struct{}{}
						}
					}
					return
				}

				if messageType == websocket.BinaryMessage || messageType == websocket.TextMessage {
					if _, err = os.Stdout.Write(data); err != nil {
						fmt.Printf("\r\n[错误] 写入输出失败: %v\r\n", err)
					}
				}
			}
		}
	}()

	// 从标准输入读取数据写入 WebSocket
	go func() {
		defer wg.Done()
		for {
			select {
			case <-c.exitChan:
				return
			default:
				if !c.isConnected.Load() {
					time.Sleep(time.Second)
					continue
				}

				buf := make([]byte, 32*1024)
				n, err := os.Stdin.Read(buf)
				if err != nil {
					if err != io.EOF {
						fmt.Printf("\r\n[错误] 读取输入失败: %v\r\n", err)
					}
					return
				}

				data := make([]byte, n)
				copy(data, buf[:n])

				if c.handleSpecialCommand(data) {
					return
				}

				c.wsLock.Lock()
				ws := c.ws
				c.wsLock.Unlock()

				if ws == nil {
					continue
				}

				if err := ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
					select {
					case <-c.exitChan:
						return
					default:
						fmt.Printf("\r\n[错误] 发送数据失败: %v\r\n", err)
						c.closeWs()
						c.reconnectCh <- struct{}{}
					}
					continue
				}
			}
		}
	}()

	// 等待所有 goroutine 完成
	go func() {
		wg.Wait()
		close(c.exitChan)
	}()
}

// 添加安全的关闭方法
func (c *Client) closeWs() {
	c.wsLock.Lock()
	defer c.wsLock.Unlock()

	if c.ws != nil && c.isConnected.Load() {
		// 先标记连接状态为断开，避免其他 goroutine 继续写入
		c.isConnected.Store(false)
		// 使用带超时的写入来发送关闭消息
		c.ws.SetWriteDeadline(time.Now().Add(time.Second))
		c.ws.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.ws.Close()
		c.ws = nil
	}
}

// 启动客户端
func (c *Client) Start() error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	fmt.Println("当前用户")
	fmt.Printf("Gid %s\n", u.Gid)
	fmt.Printf("Uid %s\n", u.Uid)
	fmt.Printf("Username %s\n", u.Username)
	fmt.Println("")

	// 初始连接
	if err = c.connect(); err != nil {
		return err
	}

	// 设置终端
	if err := c.setupTerminal(); err != nil {
		fmt.Printf("Warning: %v, continuing in non-terminal mode\n", err)
	} else {
		defer c.restoreTerminal()
		// 立即发送初始终端大小
		c.handleTerminalResize()
	}

	// 处理特殊按键
	c.handleSpecialKeys()

	// 处理数据转发
	c.handleDataTransfer()

	// 重试计数器和状态跟踪
	var (
		retryCount    int
		maxRetries    = 3
		lastErrorTime time.Time
		errorWindow   = 30 * time.Second // 错误窗口期
	)

	// 等待退出或重连
	for {
		select {
		case <-c.exitChan:
			c.closeWs()
			return nil
		case <-c.reconnectCh:
			// 检查是否在错误窗口期内
			if time.Since(lastErrorTime) > errorWindow {
				// 超过窗口期，重置计数
				retryCount = 0
			}
			lastErrorTime = time.Now()

			// 增加重试计数
			retryCount++
			if retryCount > maxRetries {
				fmt.Printf("\r\n[错误] 重试次数已达上限 (%d次), 程序退出\r\n", maxRetries)
				c.closeWs()
				close(c.exitChan)
				return fmt.Errorf("maximum retry attempts (%d) exceeded", maxRetries)
			}

			fmt.Printf("\r\n[提示] 正在重新连接... (尝试 %d/%d)\r\n", retryCount, maxRetries)

			if err = c.connect(); err != nil {
				fmt.Printf("\r\n[错误] 重连失败: %v\r\n", err)
				if retryCount >= maxRetries {
					continue // 触发最大重试检查
				}
				// 使用指数退避策略
				backoffDuration := time.Duration(1<<uint(retryCount)) * time.Second
				fmt.Printf("\r\n[提示] %d 秒后进行下一次重试...\r\n", 1<<uint(retryCount))
				time.Sleep(backoffDuration)
				c.reconnectCh <- struct{}{} // 触发下一次重试
			} else {
				// 检查连接是否真正可用
				if err = c.checkConnection(); err != nil {
					fmt.Printf("\r\n[错误] 连接检查失败: %v\r\n", err)
					c.closeWs()
					if retryCount >= maxRetries {
						continue // 触发最大重试检查
					}
					c.reconnectCh <- struct{}{} // 触发下一次重试
					continue
				}

				fmt.Printf("\r\n[提示] 重连成功\r\n")
				c.handleDataTransfer()
			}
		}
	}
}

// 添加连接检查方法
func (c *Client) checkConnection() error {
	c.wsLock.Lock()
	defer c.wsLock.Unlock()

	if c.ws == nil {
		return fmt.Errorf("connection is nil")
	}

	// 设置写入超时
	c.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
	defer c.ws.SetWriteDeadline(time.Time{}) // 重置超时

	// 发送 ping
	if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
		return fmt.Errorf("ping failed: %v", err)
	}

	// 设置读取超时
	c.ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer c.ws.SetReadDeadline(time.Time{}) // 重置超时

	// 等待 pong
	for {
		messageType, _, err := c.ws.ReadMessage()
		if err != nil {
			return fmt.Errorf("failed to receive pong: %v", err)
		}
		if messageType == websocket.PongMessage {
			return nil
		}
	}
}

func printHelpInfo() {
	commonutil.PrintAgentFlag()
	currentVersion := versionutil.GetCurrentClientVersion()
	latestVersion, err := versionutil.GetLatestVersion()
	if err != nil {
		fmt.Printf("Error getting latest version: %v\n", err)
		//os.Exit(1)
	}

	helpUrl := fmt.Sprintf("%s://%s", svcconstants.AgentServerHttpSchema, svcconstants.AgentServerHost)
	fmt.Printf("当前版本: %s\n", currentVersion)
	fmt.Printf("最新版本: %s\n", latestVersion.Agent)
	if latestVersion != nil && currentVersion != latestVersion.Client {
		fmt.Printf("建议更新到最新版本后再运行此程序。参考: %s \n", helpUrl)
	} else {
		fmt.Printf("使用说明: %s \n", helpUrl)
	}
	fmt.Printf("#################################################################\n")
}

func main() {
	if len(os.Args) < 3 {
		fmt.Printf("Usage: %s <hostTag> <clientId>\n", os.Args[0])
		os.Exit(1)
	}
	printHelpInfo()
	hostTag := os.Args[1]
	clientId := os.Args[2]

	agentVersion, err := versionutil.GetAgentVersion(hostTag, clientId)
	if err != nil {
		fmt.Printf("Error getting agent version: %v\n", err)
		os.Exit(1)
	}

	if strings.HasPrefix(agentVersion, "2") {
		client := NewClient(hostTag, clientId)
		if err := client.Start(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		v1Attach(hostTag, clientId)
	}

}

func v1Attach(hostTag, clientId string) {
	url := fmt.Sprintf("%s://%s/attach/%s/%s", svcconstants.AgentServerWsSchema, svcconstants.AgentServerHost, hostTag, clientId)
	origin := fmt.Sprintf("%s://%s/", svcconstants.AgentServerHttpSchema, svcconstants.AgentServerHost)
	config, _ := xwebsocket.NewConfig(url, origin)
	u, err := user.Current()
	if err != nil {
		return
	}
	config.Header.Set(svcconstants.XWsT2TClientUserHeader, u.Username)
	config.Header.Set(svcconstants.XWsT2TClientVersionHeader, versionutil.GetCurrentClientVersion())
	ws, err := xwebsocket.DialConfig(config)

	if err != nil {
		log.Fatalf("Error connecting to server: %v", err)
	}
	defer ws.Close()

	setTerminalColumns(150)
	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("Error setting terminal to raw mode: %v", err)
	}
	defer terminal.Restore(int(os.Stdin.Fd()), oldState)

	// 捕捉终端调整信号
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGWINCH, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range signalChan {
			if sig == syscall.SIGWINCH {
				// 捕捉窗口调整信号的处理逻辑（可选）
				// width, height, err := terminal.GetSize(int(os.Stdin.Fd()))
				// if err != nil {
				// 	log.Printf("Error getting terminal size: %v", err)
				// 	continue
				// }
				// sizeMessage := fmt.Sprintf("%dSIGWINCH%d", width, height)
				// if err := websocket.Message.Send(ws, sizeMessage); err != nil {
				// 	log.Printf("Error sending terminal size: %v", err)
				// }
			} else if sig == syscall.SIGINT || sig == syscall.SIGTERM {
				// 捕捉退出信号时关闭 WebSocket 并退出
				fmt.Println("Received exit signal, closing connection...")
				ws.Close()
				os.Exit(0)
			}
		}
	}()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := ws.Read(buf)
			if err != nil {
				log.Printf("Error reading from connection with link agent: %v", err)
				break
			}
			os.Stdout.Write(buf[:n])
		}
	}()

	buf := make([]byte, 1024)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			log.Fatalf("Error reading from stdin: %v", err)
		}
		_, err = ws.Write(buf[:n])
		if err != nil {
			log.Printf("Error sending stdin to agent socket: %v", err)
			break
		}
	}
}
func setTerminalColumns(columns int) error {
	fd := int(os.Stdin.Fd())

	// 获取当前终端的大小
	_, height, err := term.GetSize(fd)
	if err != nil {
		return fmt.Errorf("error getting terminal size: %v", err)
	}

	ws := &unix.Winsize{
		Col: uint16(columns),
		Row: uint16(height),
	}

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(ws)),
	)
	if errno != 0 {
		return fmt.Errorf("ioctl error: %v", errno)
	}

	return nil
}
