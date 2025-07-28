package client

import (
	"fmt"
	"os"

	"github.com/logerror/easylog"
	"github.com/logerror/easylog/pkg/option"
	"github.com/logerror/t2t/pkg/util/commonutil"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "t2t",
	Short: "t2t is a CLI tool for managing remote host",
	Args:  cobra.ArbitraryArgs,
	Long:  commonutil.T2TFlag,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		easylog.InitGlobalLogger(
			option.WithLogLevel("info"),
			option.WithConsole(false),
			option.WithCallerSkip(2),
		)

		// 登录校验，排除 login/version/help/list 命令
		//if cmd.Name() != "login" && cmd.Name() != "version" && cmd.Name() != "help" && cmd.Name() != "list" {
		//	defer func() {
		//		if r := recover(); r != nil {
		//			fmt.Println("请先使用 t2t login 登录后再执行该命令")
		//			os.Exit(1)
		//		}
		//	}()
		//	_, _ = authutil.GetToken()
		//}
	},
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			_ = cmd.Help()
			return
		}
		firstArg := args[0]
		if firstArg != "list" && firstArg != "version" && firstArg != "connect" && firstArg != "help" {
			if len(args) >= 2 {
				connect(args[0], args[1], useWebTerminal)
			} else {
				fmt.Println("connect command requires 2 arguments")
			}
		}
	},
}

func Execute() {
	rootCmd.PersistentFlags().BoolVarP(&useWebTerminal, "wt", "w", false, "Open a web terminal")
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
