$ErrorActionPreference = 'Continue'

try {
Add-Type -AssemblyName PresentationFramework
Add-Type -AssemblyName PresentationCore
Add-Type -AssemblyName WindowsBase

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$coreDll = Join-Path $scriptDir 'webview2\Microsoft.Web.WebView2.Core.dll'
$wpfDll = Join-Path $scriptDir 'webview2\Microsoft.Web.WebView2.Wpf.dll'

if (-not (Test-Path $wpfDll)) {
    [System.Windows.MessageBox]::Show('WebView2 SDK not found.', 'Video Downloader', 'OK', 'Error')
    exit 1
}

Add-Type -Path $coreDll
Add-Type -Path $wpfDll

$csCode = @'
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
                }
            }
        } catch {}
        win.Width = 1200;
        win.Height = 800;
        win.MinWidth = 800;
        win.MinHeight = 600;
        win.WindowStartupLocation = WindowStartupLocation.CenterScreen;
        win.Background = new System.Windows.Media.SolidColorBrush(System.Windows.Media.Color.FromRgb(15, 15, 35));

        var grid = new Grid();
        var webView = new WebView2();
        grid.Children.Add(webView);
        win.Content = grid;

        win.Loaded += async (s, e) => {
            try {
                var env = await Microsoft.Web.WebView2.Core.CoreWebView2Environment.CreateAsync(null, System.IO.Path.Combine(System.IO.Path.GetTempPath(), "vdownloader-wv2"));
                await webView.EnsureCoreWebView2Async(env);
                webView.CoreWebView2.Settings.AreDevToolsEnabled = true;
                webView.CoreWebView2.Settings.AreDefaultContextMenusEnabled = true;
                webView.CoreWebView2.Settings.IsStatusBarEnabled = false;
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
'@

Add-Type -TypeDefinition $csCode -ReferencedAssemblies @(
    'PresentationFramework',
    'PresentationCore',
    'WindowsBase',
    'System.Xaml',
    'System.Drawing',
    $wpfDll,
    $coreDll
) -ErrorAction Stop

$exePath = Join-Path $scriptDir 'server.exe'
$port = "18000"
$window = [WindowCreator]::Create("http://127.0.0.1:$port", 'Video Downloader', $exePath)

$app = New-Object System.Windows.Application
$app.Run($window)
} catch {
    Write-Host "Error: $_"
    pause
}
