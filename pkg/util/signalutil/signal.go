package signalutil

import (
	"os"
	"os/signal"
	"syscall"
)

// SignalHandler 处理CTRL+C等中断信号
func SignalHandler(ch chan<- struct{}, fn func()) {
	c := make(chan os.Signal, 5)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	for {
		sig := <-c
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM:
			signal.Stop(c)
			fn()
			ch <- struct{}{}
			return
		}
	}
}
