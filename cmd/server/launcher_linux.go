//go:build !windows

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func launchDesktopWindow(port string, quit chan os.Signal) {
	log.Println("桌面窗口模式仅支持 Windows，以纯服务器模式运行")
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	quit <- syscall.SIGTERM
}

func showError(msg string) {
	log.Printf("错误: %s", msg)
}

func killChildProcess() {}

func acquireSingleInstance() bool { return true }