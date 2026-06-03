$ErrorActionPreference = 'Continue'

$logFile = 'D:\coderom\CCNEW-VideoDownloader\ccnew-vdl-launch.log'
"Starting at $(Get-Date)" | Out-File $logFile

Add-Type -AssemblyName PresentationFramework
Add-Type -AssemblyName PresentationCore
Add-Type -AssemblyName WindowsBase

$coreDll = 'D:\coderom\CCNEW-VideoDownloader\webview2\Microsoft.Web.WebView2.Core.dll'
$wpfDll = 'D:\coderom\CCNEW-VideoDownloader\webview2\Microsoft.Web.WebView2.Wpf.dll'
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
        win.Width = 1200;
        win.Height = 800;
        win.MinWidth = 900;
        win.MinHeight = 600;
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
    $window = [WindowCreator]::Create('http://127.0.0.1:18000', 'CCNEW Video Downloader')
    "Window created" | Out-File $logFile -Append

    $app = New-Object System.Windows.Application
    "Running app..." | Out-File $logFile -Append
    $app.Run($window)
} catch {
    "Error: $_" | Out-File $logFile -Append
    [System.Windows.MessageBox]::Show("启动失败: $_", 'CCNEW Video Downloader', 'OK', 'Error')
}
%!(EXTRA string=18000)