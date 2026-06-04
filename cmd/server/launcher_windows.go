package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func launchDesktopWindow(port string, quit chan os.Signal) {
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
	// 设置进程不显示窗口
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}

	if err := cmd.Start(); err != nil {
		log.Printf("PowerShell启动失败: %v", err)
		showError(fmt.Sprintf("启动桌面窗口失败: %v", err))
		return
	}

	log.Printf("桌面窗口已启动 (PID: %d)", cmd.Process.Pid)

	// 窗口关闭时退出程序
	go func() {
		cmd.Wait()
		log.Printf("桌面窗口已关闭")
		quit <- syscall.SIGTERM
	}()
}

func showError(msg string) {
	log.Printf("错误: %s", msg)
	exec.Command("powershell", "-Command", fmt.Sprintf(`
		Add-Type -AssemblyName System.Windows.Forms
		[System.Windows.Forms.MessageBox]::Show('%s', 'CCNEW Video Downloader', 'OK', 'Error')
	`, msg)).Run()
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
"CoreDll: $coreDll (exists: $(Test-Path $coreDll))" | Out-File $logFile -Append
"WpfDll: $wpfDll (exists: $(Test-Path $wpfDll))" | Out-File $logFile -Append

if (-not (Test-Path $wpfDll)) {
    "WebView2 not found" | Out-File $logFile -Append
    exit 1
}

try {
    Add-Type -Path $coreDll
    "Loaded core DLL" | Out-File $logFile -Append
} catch {
    "Failed to load core DLL: $_" | Out-File $logFile -Append
    exit 1
}

try {
    Add-Type -Path $wpfDll
    "Loaded wpf DLL" | Out-File $logFile -Append
} catch {
    "Failed to load wpf DLL: $_" | Out-File $logFile -Append
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
    "Creating window..." | Out-File $logFile -Append
    $window = [WindowCreator]::Create('http://127.0.0.1:%s', 'CCNEW Video Downloader')
    "Window created" | Out-File $logFile -Append

    $app = New-Object System.Windows.Application
    "Running app..." | Out-File $logFile -Append
    $app.Run($window)
} catch {
    "Error: $_" | Out-File $logFile -Append
    [System.Windows.MessageBox]::Show("启动失败: $_", 'CCNEW Video Downloader', 'OK', 'Error')
}
`, exeDir, coreDll, wpfdll, port, port)

	// Write with UTF-8 BOM for Chinese character support
	bom := []byte{0xEF, 0xBB, 0xBF}
	content := append(bom, []byte(ps1Content)...)
	return os.WriteFile(path, content, 0644)
}
