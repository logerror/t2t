package client

import (
	"fmt"

	"github.com/logerror/t2t/pkg/util/versionutil"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "version of t2t",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("t2t version:", versionutil.GetCurrentClientVersion())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
