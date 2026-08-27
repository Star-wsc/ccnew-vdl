//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"syscall"
	"unsafe"
)

var (
	procCreateMutex  = kernel32.NewProc("CreateMutexW")
	childPSPid       int
)

// acquireSingleInstance 尝试获取全局互斥锁，返回 false 表示已有实例在跑
func acquireSingleInstance() bool {
	name, _ := syscall.UTF16PtrFromString("Global\\ccnew-vdl-singleton")
	handle, _, err := procCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return true // 创建失败，放行
	}
	if err != nil && err.Error() == "The operation completed successfully." {
		return true // 新建成功，唯一实例
	}
	// ERROR_ALREADY_EXISTS = 183
	return false
}

// cleanupOrphans 杀掉残留的 WebView2 和由本项目启动的 PowerShell 进程
func cleanupOrphans() {
	selfPID := os.Getpid()

	// 杀掉残留 PowerShell（窗口标题含 CCNEW 或命令行含 ccnew-vdl-window）
	if out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq powershell.exe", "/FO", "CSV", "/NH").Output(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			cols := strings.Split(strings.TrimSpace(line), ",")
			if len(cols) < 2 { continue }
			pid := strings.Trim(cols[1], "\"")
			if pid == "" { continue }
			// 不杀自己
			if pid == fmt.Sprint(selfPID) { continue }
			// 检查命令行
			cmdOut, _ := exec.Command("wmic", "process", "where", fmt.Sprintf("ProcessId=%s", pid), "get", "CommandLine", "/FORMAT:VALUE").Output()
			if strings.Contains(strings.ToLower(string(cmdOut)), "ccnew-vdl") {
				log.Printf("[清理] 杀掉残留PowerShell PID=%s", pid)
				exec.Command("taskkill", "/F", "/PID", pid).Run()
			}
		}
	}

	// 杀掉残留 WebView2（属于本项目临时目录的）
	if out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq msedgewebview2.exe", "/FO", "CSV", "/NH").Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 0 {
			log.Printf("[清理] 发现 %d 个 WebView2 进程，尝试清理", len(lines))
			for _, line := range lines {
				cols := strings.Split(strings.TrimSpace(line), ",")
				if len(cols) < 2 { continue }
				pid := strings.Trim(cols[1], "\"")
				exec.Command("taskkill", "/F", "/PID", pid).Run()
			}
		}
	}
}

// killChildProcess 退出时清理子进程
func killChildProcess() {
	if childPSPid > 0 {
		log.Printf("[退出] 杀掉前端窗口 PID=%d", childPSPid)
		exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprint(childPSPid)).Run()
	}
	// 也清理可能残留的 WebView2
	exec.Command("taskkill", "/F", "/IM", "msedgewebview2.exe").Run()
}

func launchDesktopWindow(port string, quit chan os.Signal) {
	// 单实例检查
	if !acquireSingleInstance() {
		log.Println("已有实例在运行，退出")
		os.Exit(0)
	}

	// 清理残留进程
	cleanupOrphans()

	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	ps1Path := filepath.Join(exeDir, "ccnew-vdl-window.ps1")

	// 生成PS1脚本
	if err := generatePS1(ps1Path, port); err != nil {
		log.Printf("生成PS1脚本失败: %v", err)
		showError(fmt.Sprintf("生成启动脚本失败: %v", err))
		return
	}

	// 检查webview2 DLL是否存在
	wv2Dll := filepath.Join(exeDir, "webview2", "Microsoft.Web.WebView2.Wpf.dll")
	if _, err := os.Stat(wv2Dll); os.IsNotExist(err) {
		log.Printf("WebView2 DLL不存在: %s", wv2Dll)
		showError("WebView2 SDK 未找到，请确保 webview2 目录存在")
		return
	}

	// 启动PowerShell脚本（隐藏窗口）
	cmd := exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-File", ps1Path)
	cmd.Dir = exeDir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}

	if err := cmd.Start(); err != nil {
		log.Printf("PowerShell启动失败: %v", err)
		showError(fmt.Sprintf("启动桌面窗口失败: %v", err))
		return
	}

	childPSPid = cmd.Process.Pid
	log.Printf("桌面窗口已启动 (PID: %d)", childPSPid)

	// 注册退出清理
	go func() {
		cmd.Wait()
		log.Printf("桌面窗口已关闭")
		quit <- syscall.SIGTERM
	}()
}

func showError(msg string) {
	log.Printf("错误: %s", msg)
	exec.Command("powershell", "-Command", fmt.Sprintf(
		"Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.MessageBox]::Show('%s', 'CCNEW Video Downloader', 'OK', 'Error')",
		msg)).Run()
}

func generatePS1(path string, port string) error {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	wv2Dir := filepath.Join(exeDir, "webview2")
	coreDll := filepath.Join(wv2Dir, "Microsoft.Web.WebView2.Core.dll")
	wpfdll := filepath.Join(wv2Dir, "Microsoft.Web.WebView2.Wpf.dll")

	ps1Content := fmt.Sprintf(`$ErrorActionPreference = 'Continue'

$logFile = '%s\ccnew-vdl-launch.log'
"Starting at $(Get-Date)" | Out-File $logFile

Add-Type -AssemblyName PresentationFramework
Add-Type -AssemblyName PresentationCore
Add-Type -AssemblyName WindowsBase

$coreDll = '%s'
$wpfDll = '%s'

if (-not (Test-Path $wpfDll)) {
    "WebView2 not found" | Out-File $logFile -Append
    exit 1
}

try {
    Add-Type -Path $coreDll
    Add-Type -Path $wpfDll
} catch {
    "Failed to load DLL: $_" | Out-File $logFile -Append
    exit 1
}

$csCode = @"
using System;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;
using Microsoft.Web.WebView2.Wpf;

public class WindowCreator {
    private const double AspectRatio = 1.5;

    public static Window Create(string url, string title) {
        var win = new Window();
        win.Title = title;
        win.Width = 1600;
        win.Height = 1067;
        win.MinWidth = 1200;
        win.MinHeight = 800;
        win.WindowStartupLocation = WindowStartupLocation.CenterScreen;
        win.Background = new SolidColorBrush(Color.FromRgb(26, 26, 46));

        var grid = new Grid();
        var webView = new WebView2();
        grid.Children.Add(webView);
        win.Content = grid;

        bool isResizing = false;
        win.SizeChanged += (s, e) => {
            if (isResizing) return;
            isResizing = true;
            try {
                if (e.WidthChanged) {
                    double h = e.NewSize.Width / AspectRatio;
                    if (h >= win.MinHeight) win.Height = h;
                } else {
                    double w = e.NewSize.Height * AspectRatio;
                    if (w >= win.MinWidth) win.Width = w;
                }
            } catch {}
            isResizing = false;
        };

        win.Loaded += async (s, e) => {
            try {
                var env = await Microsoft.Web.WebView2.Core.CoreWebView2Environment.CreateAsync(null, System.IO.Path.Combine(System.IO.Path.GetTempPath(), "ccnew-vdl-wv2"));
                await webView.EnsureCoreWebView2Async(env);
                webView.ZoomFactor = 1.0;
                webView.CoreWebView2.Navigate(url);
            } catch (Exception ex) {
                MessageBox.Show("WebView2 error: " + ex.Message, title, MessageBoxButton.OK, MessageBoxImage.Error);
            }
        };

        win.Closing += (s, e) => {
            try { webView.Dispose(); } catch {}
            Application.Current.Shutdown();
            Environment.Exit(0);
        };

        return win;
    }
}
"@

Add-Type -TypeDefinition $csCode -ReferencedAssemblies @(
    'PresentationFramework',
    'PresentationCore',
    'WindowsBase',
    'System.Xaml',
    $wpfDll,
    $coreDll
)

try {
    $window = [WindowCreator]::Create('http://127.0.0.1:%s/?t=%d', 'CCNEW Video Downloader')
    $app = New-Object System.Windows.Application
    $app.Run($window)
} catch {
    "Error: $_" | Out-File $logFile -Append
}
`, exeDir, coreDll, wpfdll, port, time.Now().Unix())

	bom := []byte{0xEF, 0xBB, 0xBF}
	content := append(bom, []byte(ps1Content)...)
	return os.WriteFile(path, content, 0644)
}