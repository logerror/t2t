package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/logerror/t2t/internal/agent"
	"github.com/logerror/t2t/pkg/constants/svcconstants"
	"github.com/logerror/t2t/pkg/util/commonutil"
	"github.com/logerror/t2t/pkg/util/versionutil"
)

func main() {
	printHelpInfo()
	hostTag, clientId := getHostTagAndClientId()

	fmt.Println("将以下信息发送给需要远程该机器的用户")
	fmt.Println("Host信息: " + hostTag)
	fmt.Println("授权码:   " + clientId)

	url := fmt.Sprintf("%s://%s/v2/ws/%s/%s",
		svcconstants.AgentServerWsSchema,
		svcconstants.AgentServerHost,
		hostTag,
		clientId)

	// 设置WebSocket连接的header
	headers := http.Header{}
	headers.Set(svcconstants.WsT2TAgentTokenHeader, "ba8Eg6GQVNpRv6d0")
	headers.Set(svcconstants.WsT2TAgentArchHeader, runtime.GOARCH)
	headers.Set(svcconstants.WsT2TAgentVersionHeader, versionutil.GetCurrentAgentVersion())

	ws, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		log.Fatalf("WebSocket 连接失败: %v", err)
	}

	term := agent.NewTerminal("xterm-256color")
	if err := term.StartShell(); err != nil {
		log.Fatalf("Shell 启动失败: %v", err)
	}

	ag := agent.NewAgent(ws, term, hostTag, clientId)
	ctx := context.Background()
	if err := ag.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Agent 运行异常: %v\n", err)
		os.Exit(1)
	}

	// 等待信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// 清理资源
	cleanUp(hostTag, clientId)
	ag.Close()
}

func getHostTagAndClientId() (string, string) {
	carIDPath := "/tmp/t2t/host_tag"
	hostTag, err := readFile(carIDPath)
	if err != nil {
		hostTag = os.Getenv("T2T_HOST_TAG")
		if hostTag == "" {
			hostName, err := os.Hostname()
			if err != nil {
				fmt.Printf("Error getting hostname: %v\n", err)
				hostTag = "default"
			} else {
				hostTag = hostName
			}
		}
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
	fmt.Printf("res: %d\n", resp.StatusCode)
}
