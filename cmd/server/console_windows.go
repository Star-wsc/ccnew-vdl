package main

import (
	"syscall"
	"time"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	user32           = syscall.NewLazyDLL("user32.dll")
	getConsoleWindow = kernel32.NewProc("GetConsoleWindow")
	showWindow       = user32.NewProc("ShowWindow")
	isWindowVisible  = user32.NewProc("IsWindowVisible")
	freeConsole      = kernel32.NewProc("FreeConsole")
	allocConsole     = kernel32.NewProc("AllocConsole")
)

const (
	SW_HIDE = 0
	SW_SHOW = 5
)

var consoleHidden = true // 默认隐藏

// hideConsoleWindow 彻底隐藏控制台窗口
func hideConsoleWindow() {
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd != 0 {
		// 先隐藏窗口
		showWindow.Call(hwnd, SW_HIDE)
		// 设置窗口样式为工具窗口（不在任务栏显示）
		// 然后释放控制台
		freeConsole.Call()
		consoleHidden = true
	}
}

// showConsoleWindow 重新分配并显示控制台窗口
func showConsoleWindow() {
	if consoleHidden {
		// 重新分配控制台
		allocConsole.Call()
		consoleHidden = false
		// 等待控制台创建完成
		time.Sleep(100 * time.Millisecond)
	}
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd != 0 {
		showWindow.Call(hwnd, SW_SHOW)
	}
}

// isConsoleVisible 检查控制台是否可见
func isConsoleVisible() bool {
	return !consoleHidden
}
