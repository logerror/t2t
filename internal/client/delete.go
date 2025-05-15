package client

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/logerror/t2t/pkg/constants/svcconstants"
	"github.com/spf13/cobra"
)

var (
	host string
	code string
)

var deleteConnectionCmd = &cobra.Command{
	Use:   "delete",
	Short: "delete agent",
	Run: func(cmd *cobra.Command, args []string) {
		url := fmt.Sprintf("%s://%s/agent/%s/%s", svcconstants.AgentServerHttpSchema, svcconstants.AgentServerHost, host, code)
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
	},
}

func init() {
	rootCmd.AddCommand(deleteConnectionCmd)
	deleteConnectionCmd.Flags().StringVarP(&host, "tag", "t", "", "Host tag for the connection")
	deleteConnectionCmd.Flags().StringVarP(&code, "code", "c", "", "Client ID for the connection")
	deleteConnectionCmd.MarkFlagRequired("tag")
	deleteConnectionCmd.MarkFlagRequired("code")
}
