package client

import (
	"fmt"

	"github.com/logerror/t2t/pkg/constants/svcconstants"
	"github.com/logerror/t2t/pkg/util/commonutil"
	"github.com/logerror/t2t/pkg/util/versionutil"
)

func printHelpInfo() {
	commonutil.PrintAgentFlag()
	currentVersion := versionutil.GetCurrentClientVersion()
	latestVersion, err := versionutil.GetLatestVersion()
	if err != nil {
		fmt.Printf("Error getting latest version: %v\n", err)
	}

	helpUrl := fmt.Sprintf("%s://%s", svcconstants.AgentServerHttpSchema, svcconstants.AgentServerHost)
	fmt.Printf("当前版本: %s\n", currentVersion)
	fmt.Printf("最新版本: %s\n", latestVersion.Client)
	if currentVersion != latestVersion.Client {
		fmt.Printf("建议更新到最新版本后再运行此程序。参考: %s \n", helpUrl)
	} else {
		fmt.Printf("使用说明: %s \n", helpUrl)
	}
	fmt.Printf("#################################################################\n")
}
