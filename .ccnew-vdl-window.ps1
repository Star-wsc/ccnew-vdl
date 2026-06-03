$ErrorActionPreference = 'Stop'

Add-Type -AssemblyName PresentationFramework
Add-Type -AssemblyName PresentationCore
Add-Type -AssemblyName WindowsBase

$coreDll = 'D:\coderom\CCNEW-VideoDownloader\webview2\Microsoft.Web.WebView2.Core.dll'
$wpfDll = 'D:\coderom\CCNEW-VideoDownloader\webview2\Microsoft.Web.WebView2.Wpf.dll'

if (-not (Test-Path $wpfDll)) {
    # 没有WebView2，用浏览器打开
    Start-Process 'http://127.0.0.1:18000'
    exit 0
}

Add-Type -Path $coreDll
Add-Type -Path $wpfDll

$csCode = @"
using System;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using Microsoft.Web.WebView2.Wpf;

public class WindowCreator {
    [System.Runtime.InteropServices.DllImport("user32.dll", SetLastError = true)]
    static extern IntPtr SendMessage(IntPtr hWnd, uint Msg, IntPtr wParam, IntPtr lParam);
    const uint WM_SETICON = 0x0080;
    const int ICON_SMALL = 0;
    const int ICON_BIG = 1;

    // 固定宽高比 3:2
    private const double AspectRatio = 3.0 / 2.0;

    public static Window Create(string url, string title, string exePath) {
        var win = new Window();
        win.Title = title;
        try {
            if (!string.IsNullOrEmpty(exePath) && System.IO.File.Exists(exePath)) {
                var icon = System.Drawing.Icon.ExtractAssociatedIcon(exePath);
                if (icon != null) {
                    var bmp = icon.ToBitmap();
                    var ms = new System.IO.MemoryStream();
                    bmp.Save(ms, System.Drawing.Imaging.ImageFormat.Png);
                    ms.Position = 0;
                    var bitmap = new System.Windows.Media.Imaging.BitmapImage();
                    bitmap.BeginInit();
                    bitmap.StreamSource = ms;
                    bitmap.CacheOption = System.Windows.Media.Imaging.BitmapCacheOption.OnLoad;
                    bitmap.EndInit();
                    bitmap.Freeze();
                    win.Icon = bitmap;
                    win.SourceInitialized += (s, e) => {
                        var helper = new System.Windows.Interop.WindowInteropHelper(win);
                        SendMessage(helper.Handle, WM_SETICON, (IntPtr)ICON_SMALL, icon.Handle);
                        SendMessage(helper.Handle, WM_SETICON, (IntPtr)ICON_BIG, icon.Handle);
                    };
                }
            }
        } catch {}

        // 默认尺寸 1200x800 (3:2)
        win.Width = 1200;
        win.Height = 800;
        win.MinWidth = 900;
        win.MinHeight = 600;
        win.WindowStartupLocation = WindowStartupLocation.CenterScreen;
        win.Background = new System.Windows.Media.SolidColorBrush(System.Windows.Media.Color.FromRgb(26, 26, 46));

        var grid = new Grid();
        var webView = new WebView2();
        grid.Children.Add(webView);
        win.Content = grid;

        // 锁定宽高比例缩放
        bool isResizing = false;
        win.SizeChanged += (s, e) => {
            if (isResizing) return;
            isResizing = true;
            try {
                if (e.WidthChanged) {
                    double newHeight = e.NewSize.Width / AspectRatio;
                    if (newHeight >= win.MinHeight && newHeight <= SystemParameters.MaximizedPrimaryScreenHeight) {
                        win.Height = newHeight;
                    }
                } else {
                    double newWidth = e.NewSize.Height * AspectRatio;
                    if (newWidth >= win.MinWidth && newWidth <= SystemParameters.MaximizedPrimaryScreenWidth) {
                        win.Width = newWidth;
                    }
                }
            } catch {}
            isResizing = false;
        };

        win.Loaded += async (s, e) => {
            try {
                var env = await Microsoft.Web.WebView2.Core.CoreWebView2Environment.CreateAsync(null, System.IO.Path.Combine(System.IO.Path.GetTempPath(), "ccnew-vdl-wv2"));
                await webView.EnsureCoreWebView2Async(env);
                webView.CoreWebView2.Settings.AreDevToolsEnabled = true;
                webView.CoreWebView2.Settings.AreDefaultContextMenusEnabled = true;
                webView.CoreWebView2.Settings.IsStatusBarEnabled = false;
                // 设置默认缩放为100%!
(string=D:\coderom\CCNEW-VideoDownloader\server.exe)                webView.ZoomFactor = 1.0;
                webView.CoreWebView2.Navigate(url);
            } catch (Exception ex) {
                MessageBox.Show("WebView2 init error: " + ex.Message, title, MessageBoxButton.OK, MessageBoxImage.Error);
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
    'System.Drawing',
    $wpfDll,
    $coreDll
)

$iconPath = '18000'
$window = [WindowCreator]::Create('http://127.0.0.1:%!s(MISSING)', 'CCNEW Video Downloader', $iconPath)

$app = New-Object System.Windows.Application
$app.Run($window)
