package main

import (
	"fmt"

	"github.com/logerror/easylog"
	"github.com/logerror/easylog/pkg/option"
	"github.com/logerror/t2t/internal/server/http"
	"github.com/logerror/t2t/pkg/config"
	"github.com/logerror/t2t/pkg/util/printutil"
	"github.com/logerror/t2t/pkg/util/signalutil"
	"github.com/logerror/t2t/pkg/util/versionutil"
	"go.uber.org/zap"
)

func main() {
	printutil.PrintLogo()
	config.InitConfig()
	easylog.InitGlobalLogger(
		option.WithLogLevel("info"),
		option.WithConsole(true),
		option.WithCallerSkip(2),
	)
	fmt.Println("Current Server Version:" + versionutil.GetCurrentServerVersion())

	srv := http.NewServer(config.Configuration)
	ch := make(chan struct{})
	go signalutil.SignalHandler(ch, srv.Shutdown)

	if err := srv.ListenAndServe(); err != nil {
		easylog.Error("Failed to start agent server", zap.Error(err))
	}

	<-ch
	easylog.Info("agent server terminated")

}
