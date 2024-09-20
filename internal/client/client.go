package main

import (
	"fmt"
	"github.com/logerror/t2t/pkg/config"
	"log"
	"os"
	"os/signal"
	"os/user"
	"syscall"

	"github.com/logerror/t2t/pkg/util/printutil"

	"github.com/logerror/t2t/pkg/constants/svcconstants"
	"github.com/logerror/t2t/pkg/util/versionutil"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/net/websocket"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatalf("Usage: sudo %s <Agent Host> <Code>", os.Args[0])
	}
	config.InitConfig()
	printHelpInfo()

	u, err := user.Current()
	if err != nil {
		return
	}

	fmt.Println("Current User Info:")
	fmt.Printf("Gid %s\n", u.Gid)
	fmt.Printf("Uid %s\n", u.Uid)
	fmt.Printf("Username %s\n", u.Username)
	fmt.Println("")

	hostTag := os.Args[1]
	clientId := os.Args[2]

	url := fmt.Sprintf("%s://%s/attach/%s/%s", svcconstants.AgentServerWsSchema, svcconstants.AgentServerHost, hostTag, clientId)
	origin := fmt.Sprintf("%s://%s/", svcconstants.AgentServerHttpSchema, svcconstants.AgentServerHost)
	wsConfig, _ := websocket.NewConfig(url, origin)
	wsConfig.Header.Set("X-T2T-Client-User", u.Username)
	ws, err := websocket.DialConfig(wsConfig)

	if err != nil {
		log.Fatalf("Error connecting to server: %v", err)
	}
	defer ws.Close()

	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("Error setting terminal to raw mode: %v", err)
	}
	defer terminal.Restore(int(os.Stdin.Fd()), oldState)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGWINCH, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range signalChan {
			if sig == syscall.SIGWINCH {
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
				log.Printf("Error reading from connection: %v", err)
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
			log.Printf("Error sending to WebSocket: %v", err)
			break
		}
	}
}

func printHelpInfo() {
	printutil.PrintLogo()
	currentVersion := versionutil.GetCurrentClientVersion()
	latestVersion, err := versionutil.GetLatestVersion()
	if err != nil {
		fmt.Printf("Error getting latest version: %v\n", err)
	}

	helpUrl := fmt.Sprintf("%s://%s", svcconstants.AgentServerHttpSchema, svcconstants.AgentServerHost)
	fmt.Printf("Current Version: %s\n", currentVersion)
	fmt.Printf("Latest Version: %s\n", latestVersion.Agent)
	if latestVersion != nil && currentVersion != latestVersion.Client {
		fmt.Printf("It is recommended to update to the latest version before running this program. Reference: %s \n", helpUrl)
	} else {
		fmt.Printf("使用说明: %s \n", helpUrl)
	}
	fmt.Printf("#################################################################\n")
}
