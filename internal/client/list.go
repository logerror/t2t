package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/user"

	"github.com/logerror/t2t/pkg/constants/svcconstants"
	"github.com/logerror/t2t/pkg/util/versionutil"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available connections",
	Run: func(cmd *cobra.Command, args []string) {
		listConnections()
	},
}

func listConnections() {
	// 获取当前用户信息
	currentUser, err := user.Current()
	if err != nil {
		fmt.Printf("Error getting current user: %v\n", err)
		return
	}

	url := fmt.Sprintf("%s://%s/agents",
		svcconstants.AgentServerHttpSchema,
		svcconstants.AgentServerHost)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	// 添加请求头
	req.Header.Set(svcconstants.XWsT2TClientUserHeader, currentUser.Username)
	req.Header.Set(svcconstants.XWsT2TClientVersionHeader, versionutil.GetCurrentClientVersion())
	// 新增：带上token
	//token, _ := authutil.GetToken()
	//if token != "" {
	//	req.Header.Set("Authorization", token)
	//}

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error fetching connections: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error: server returned status code %d\n", resp.StatusCode)
		return
	}

	var connections []Connection
	if err := json.NewDecoder(resp.Body).Decode(&connections); err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Host Tag", "Client Id", "Agent Version", "Agent Arch", "Client Version", "Client User"})

	for _, v := range connections {
		table.Append([]string{v.HostTag, v.ClientId, v.AgentVersion, v.AgentArch, v.ClientVersion, v.ClientUser})
	}

	table.Render()
}

func init() {
	rootCmd.AddCommand(listCmd)
}
