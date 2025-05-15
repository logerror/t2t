package client

import (
	"fmt"

	"github.com/spf13/cobra"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "upgrade t2t(WIP)",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("正在赶来的路上...")
	},
}

func init() {
	//rootCmd.AddCommand(upgradeCmd)
}
