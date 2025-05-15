package client

import (
	"fmt"
	"github.com/logerror/t2t/pkg/constants/svcconstants"
	"os/exec"
	"runtime"
	"strings"

	"github.com/logerror/t2t/pkg/util/versionutil"
	"github.com/spf13/cobra"
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
Example: t2t connect --tag <hostTag> --code <clientId>`,
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
		// macOS 使用 open 命令
		cmd = "open"
		args = []string{url}
	case "linux":
		// Linux 使用 xdg-open
		cmd = "xdg-open"
		args = []string{url}
	default:
		return nil
	}

	return exec.Command(cmd, args...).Start()
}

func runConnect(cmd *cobra.Command, args []string) {
	printHelpInfo()
	connect(hostTag, clientId, useWebTerminal)
}

func connect(hostTag, clientId string, useWebTerminal bool) {
	if useWebTerminal {
		if err := openDefaultBrowser(fmt.Sprintf("%s://%s/terminal?hostTag=%s&clientId=%s", svcconstants.AgentServerHttpSchema, svcconstants.AgentServerHost, hostTag, clientId)); err != nil {
			fmt.Printf("Error opening browser: %v\n", err)
			return
		}
		return
	}
	agentVersion, err := versionutil.GetAgentVersion(hostTag, clientId)
	if err != nil {
		fmt.Printf("Error getting agent version: %v\n", err)
		return
	}
	fmt.Printf("Agent version: %s\n", agentVersion)

	if strings.HasPrefix(agentVersion, "2") {
		client := NewClient(hostTag, clientId)
		if err := client.Start(); err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
	} else {
		v1Attach(hostTag, clientId)
	}
}
