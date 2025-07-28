package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/logerror/t2t/pkg/constants/svcconstants"
	"github.com/logerror/t2t/pkg/util/versionutil"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
)

type Connection struct {
	HostTag       string `json:"hostTag"`
	ClientId      string `json:"clientId"`
	AgentVersion  string `json:"agentVersion"`
	AgentArch     string `json:"agentArch"`
	ClientVersion string `json:"clientVersion"`
	ClientUser    string `json:"clientUser"`
}

var (
	hostTag        string
	clientId       string
	useWebTerminal bool
)

var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect to a remote terminal",
	Long: `Connect to a remote terminal using host tag and client ID.
Example: t2t-client connect --tag <hostTag> --code <clientId>`,
	Run: runConnect,
}

func init() {
	rootCmd.AddCommand(connectCmd)
	connectCmd.Flags().StringVarP(&hostTag, "tag", "t", "", "Host tag for the connection")
	connectCmd.Flags().StringVarP(&clientId, "code", "c", "", "Client ID for the connection")
	connectCmd.MarkFlagRequired("tag")
	connectCmd.MarkFlagRequired("code")
	connectCmd.Flags().AddFlagSet(rootCmd.PersistentFlags())
}

func openDefaultBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	default:
		return nil
	}

	return exec.Command(cmd, args...).Start()
}

func runConnect(cmd *cobra.Command, args []string) {
	connect(hostTag, clientId, useWebTerminal)
}

func connect(hostTag, clientId string, useWebTerminal bool) {
	//_, claim := authutil.GetToken()
	//loginName := claim.Subject
	if useWebTerminal {
		if err := openDefaultBrowser(fmt.Sprintf("http://localhost:9002/terminal?hostTag=%s&clientId=%s", hostTag, clientId)); err != nil {
			fmt.Printf("Error opening browser: %v\n", err)
			return
		}
		return
	}
	//agentVersion, err := versionutil.GetAgentVersion(hostTag, clientId)
	//if err != nil {
	//	fmt.Printf("Error getting agent version: %v\n", err)
	//	return
	//}
	//fmt.Printf("Agent version: %s\n", agentVersion)
	printHelpInfo()

	term := NewTerminal("xterm-256color")
	if err := term.StartShell(); err != nil {
		fmt.Printf("Shell 启动失败: %v\n", err)
		return
	}

	client := NewClient(hostTag, clientId, "", term)
	if err := client.Start(); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

}

type Client struct {
	ws          *websocket.Conn
	currentUser string
	hostTag     string
	clientId    string
	terminal    Terminal
	oldState    *terminal.State
	exitChan    chan struct{}
	reconnectCh chan struct{}
	wsLock      sync.Mutex
	isConnected atomic.Bool
	exitOnce    sync.Once // 新增
}

func NewClient(hostTag, clientId, loginName string, terminal Terminal) *Client {
	return &Client{
		currentUser: loginName,
		hostTag:     hostTag,
		clientId:    clientId,
		terminal:    terminal,
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

	headers := make(map[string][]string)
	headers[svcconstants.XWsT2TClientUserHeader] = []string{currentUser.Username}
	headers[svcconstants.XWsT2TClientVersionHeader] = []string{versionutil.GetCurrentClientVersion()}

	//token, _ := authutil.GetToken()
	//if token != "" {
	//	headers["Authorization"] = []string{token}
	//}

	url := fmt.Sprintf("%s://%s/v2/attach/%s/%s",
		svcconstants.AgentServerWsSchema,
		svcconstants.AgentServerHost,
		c.hostTag,
		c.clientId)

	ws, _, err := dialer.Dial(url, http.Header(headers))
	if err != nil {
		c.isConnected.Store(false)
		return fmt.Errorf("连接服务器失败: %v", err)
	}

	c.ws = ws
	c.isConnected.Store(true)
	return nil
}

func (c *Client) setupTerminal() error {
	if !terminal.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("stdin is not a terminal")
	}
	// 交给 Terminal 实现
	if setupper, ok := c.terminal.(interface{ SetupLocalTerminal() error }); ok {
		return setupper.SetupLocalTerminal()
	}
	return nil
}

func (c *Client) restoreTerminal() {
	if restorer, ok := c.terminal.(interface{ RestoreLocalTerminal() }); ok {
		restorer.RestoreLocalTerminal()
	}
}

func (c *Client) handleTerminalResize() {
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

func (c *Client) handleSpecialKeys() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGWINCH)

	go func() {
		for {
			select {
			case sig := <-sigChan:
				// 只处理 SIGTERM 和 SIGWINCH
				switch sig {
				case syscall.SIGWINCH:
					c.handleTerminalResize()
				case syscall.SIGTERM:
					c.safeCloseExitChan()
					return
				}
			case <-c.exitChan:
				return
			}
		}
	}()
}

func (c *Client) handleSpecialCommand(data []byte) bool {
	cmd := string(bytes.TrimSpace(data))
	switch cmd {
	case "exit", "quit", "bye", "bye!":
		fmt.Printf("\r\n[提示] 断开连接\r\n")
		c.safeCloseExitChan()
		return true
	}
	return false
}

func (c *Client) handleDataTransfer() {
	var wg sync.WaitGroup
	wg.Add(2)

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

	go func() {
		defer wg.Done()
		ctrlCCount := 0
		var ctrlCTimer *time.Timer
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
				// 检查 Ctrl+C (\x03)
				if n == 1 && buf[0] == 0x14 {
					ctrlCCount++
					if ctrlCCount == 1 {
						fmt.Printf("\r\n[提示] 再次按 Ctrl+T 断开连接\r\n")
						if ctrlCTimer != nil {
							ctrlCTimer.Stop()
						}
						ctrlCTimer = time.AfterFunc(3*time.Second, func() {
							ctrlCCount = 0
						})
					} else if ctrlCCount == 2 {
						fmt.Printf("\r\n[提示] 断开连接\r\n")
						c.safeCloseExitChan()
						return
					}
					continue
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

	go func() {
		wg.Wait()
		c.safeCloseExitChan()
	}()
}

func (c *Client) closeWs() {
	c.wsLock.Lock()
	defer c.wsLock.Unlock()
	if c.ws != nil && c.isConnected.Load() {
		c.isConnected.Store(false)
		c.ws.SetWriteDeadline(time.Now().Add(time.Second))
		c.ws.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.ws.Close()
		c.ws = nil
	}
}

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

	if err = c.connect(); err != nil {
		return err
	}

	if err := c.setupTerminal(); err != nil {
		fmt.Printf("Warning: %v, continuing in non-terminal mode\n", err)
	} else {
		defer c.restoreTerminal()
		c.handleTerminalResize()
	}

	c.handleSpecialKeys()
	c.handleDataTransfer()

	for {
		select {
		case <-c.exitChan:
			c.closeWs()
			return nil
		case <-c.reconnectCh:
			return fmt.Errorf("exit and reconnect with cmd: t2t-client %s %s", c.hostTag, c.clientId)
		}
	}
}

// 新增安全关闭方法
func (c *Client) safeCloseExitChan() {
	c.exitOnce.Do(func() {
		close(c.exitChan)
	})
}
