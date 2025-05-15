package client

import (
	"fmt"

	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "start agent (WIP)",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("正在赶来的路上...")
	},
}

func init() {
	//rootCmd.AddCommand(agentCmd)
}
