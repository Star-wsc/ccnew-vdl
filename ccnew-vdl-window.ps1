$ErrorActionPreference = 'Continue'

$logFile = 'D:\coderom\CCNEW-VideoDownloader\ccnew-vdl-launch.log'
"Starting at $(Get-Date)" | Out-File $logFile

Add-Type -AssemblyName PresentationFramework
Add-Type -AssemblyName PresentationCore
Add-Type -AssemblyName WindowsBase

$coreDll = 'D:\coderom\CCNEW-VideoDownloader\webview2\Microsoft.Web.WebView2.Core.dll'
$wpfDll = 'D:\coderom\CCNEW-VideoDownloader\webview2\Microsoft.Web.WebView2.Wpf.dll'

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
    $window = [WindowCreator]::Create('http://127.0.0.1:18000/?t=1788243102', 'CCNEW Video Downloader')
    $app = New-Object System.Windows.Application
    $app.Run($window)
} catch {
    "Error: $_" | Out-File $logFile -Append
}
