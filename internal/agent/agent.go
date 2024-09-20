package main

import (
	"fmt"
	"github.com/logerror/t2t/pkg/config"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"syscall"
	"time"

	"github.com/logerror/t2t/pkg/util/printutil"

	"github.com/creack/pty"
	"github.com/logerror/t2t/pkg/constants/svcconstants"
	"github.com/logerror/t2t/pkg/util/versionutil"
	"golang.org/x/net/websocket"
)

func main() {
	config.InitConfig()
	printHelpInfo()
	hostTag, clientId := getHostTagAndClientId()
	url := fmt.Sprintf("%s://%s/ws/%s/%s", svcconstants.AgentServerWsSchema, svcconstants.AgentServerHost, hostTag, clientId)
	origin := fmt.Sprintf("%s://%s/", svcconstants.AgentServerHttpSchema, svcconstants.AgentServerHost)
	wsConfig, _ := websocket.NewConfig(url, origin)
	wsConfig.Header.Set("X-T2T-Agent-Token", "ba8Eg6GQVNpRv6d0")
	ws, err := websocket.DialConfig(wsConfig)
	if err != nil {
		log.Fatalf("Error connecting to server: %v", err)
	}
	defer ws.Close()

	currentShell := "sh"
	err = exec.Command("bash").Run()
	if err == nil {
		currentShell = "bash"
	}

	cmd := exec.Command("sh", "-c", "exec "+currentShell)

	u, err := user.Current()
	if err == nil && u.HomeDir != "" {
		cmd.Dir = u.HomeDir
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		fmt.Printf("Error starting PTY: %v", err)
		os.Exit(1)
	}

	fmt.Println("↓↓↓ Send the following message to the user and use t2t-client to remote the machine ↓↓↓↓")
	fmt.Println("Host Info: " + hostTag)
	fmt.Println("Code:      " + clientId)

	// 处理系统退出信号
	exitCh := make(chan os.Signal, 1)
	signal.Notify(exitCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-exitCh
		log.Println("Exit signal received, cleanup in progress...")
		cleanUp(hostTag, clientId)
		os.Exit(0)
	}()

	defer func() { _ = ptmx.Close() }()

	// 处理终端窗口大小调整
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGWINCH)
		for range ch {
			if err = pty.InheritSize(os.Stdin, ptmx); err != nil {
				fmt.Printf("Error resizing pty: %v", err)
				break
			}
		}
	}()

	go func() {
		_, err = io.Copy(ws, ptmx)
		if err != nil {
			log.Printf("Error copying PTY output to connection: %v", err)
			exec.Command("exit").Run()
			os.Exit(1)
		}
	}()

	for {
		_, err = io.Copy(ptmx, ws)
		if err != nil {
			log.Printf("Error copying connection input to PTY: %v", err)
			exec.Command("exit").Run()
			os.Exit(1)
		}
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

func printHelpInfo() {
	printutil.PrintLogo()
	currentVersion := versionutil.GetCurrentAgentVersion()
	latestVersion, err := versionutil.GetLatestVersion()
	if err != nil {
		fmt.Printf("Error getting latest version: %v\n", err)
		//os.Exit(1)
	}

	helpUrl := fmt.Sprintf("%s://%s", svcconstants.AgentServerHttpSchema, svcconstants.AgentServerHost)
	fmt.Printf("Current Version: %s\n", currentVersion)
	fmt.Printf("Latest Version: %s\n", latestVersion.Agent)
	if latestVersion != nil && currentVersion != latestVersion.Agent {
		fmt.Printf("It is recommended to update to the latest version before running this program. Reference: %s \n", helpUrl)
	} else {
		fmt.Printf("Reference $ Usage: %s \n", helpUrl)
	}
	fmt.Println("###############################################################################################\n")
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
		log.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("read response failed: %v", err)
	}

	// 打印响应状态码和响应体
	fmt.Printf("res: %d\n", resp.StatusCode)
}
