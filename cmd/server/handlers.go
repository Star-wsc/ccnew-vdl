package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Star-wsc/ccnew-vdl/internal/bilibili"
	"github.com/Star-wsc/ccnew-vdl/internal/config"
	"github.com/Star-wsc/ccnew-vdl/internal/douyin"
	"github.com/Star-wsc/ccnew-vdl/internal/download"
	"github.com/Star-wsc/ccnew-vdl/internal/fsutil"
	"github.com/gin-gonic/gin"
)

// ==================== Models ====================

type TaskStatus = download.TaskStatus

const (
	StatusPending     = download.StatusPending
	StatusParsing     = download.StatusParsing
	StatusDownloading = download.StatusDownloading
	StatusCompleted   = download.StatusCompleted
	StatusFailed      = download.StatusFailed
)

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	TaskID    string `json:"task_id"`
	Message   string `json:"message"`
}

type CollectionVideoInfo struct {
	BVID         string `json:"bvid,omitempty"`
	VideoID      string `json:"video_id"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	Author       string `json:"author"`
	CoverURL     string `json:"cover_url"`
	Duration     int    `json:"duration"`
	Page         int    `json:"page"`
	Status       string `json:"status"`   // pending/downloading/completed/failed
	Progress     int    `json:"progress"` // 0-100
	Speed        int64  `json:"speed"`
	FilePath     string `json:"file_path"`
	FileSize     int64  `json:"file_size"`
	ErrorMessage string `json:"error_message"`
}

type CollectionInfo struct {
	ID              string                 `json:"id"`
	URL             string                 `json:"url"`
	Title           string                 `json:"title"`
	Author          string                 `json:"author"`
	CoverURL        string                 `json:"cover_url"`
	TotalCount      int                    `json:"total_count"`
	Videos          []*CollectionVideoInfo `json:"videos"`
	Status          string                 `json:"status"`
	Quality         string                 `json:"quality"`
	Source          string                 `json:"source,omitempty"` // "app" 或 "" (web)
	CreatedAt       time.Time              `json:"created_at"`
	Subscribed      bool                   `json:"subscribed"`       // 是否订阅
	SelectedIndices []int                  `json:"selected_indices"` // 用户选中的视频索引
	LastRefresh     time.Time              `json:"last_refresh"`     // 上次刷新时间
	RefreshInterval int                    `json:"refresh_interval"` // 刷新间隔（分钟），默认60
}

// ==================== Handlers ====================

type Handlers struct {
	cfg                *config.Config
	mgr                *download.Manager
	douyinParser       *douyin.Parser
	bilibiliParser     *bilibili.Parser
	douyinCollection   *douyin.CollectionParser
	bilibiliCollection *bilibili.CollectionParser
	collections        map[string]*CollectionInfo
	collectionsMu      sync.RWMutex
	logs               []LogEntry
	logsMu             sync.RWMutex
}

func NewHandlers(cfg *config.Config, mgr *download.Manager) *Handlers {
	h := &Handlers{
		cfg:                cfg,
		mgr:                mgr,
		douyinParser:       douyin.NewParser(cfg.Proxy),
		bilibiliParser:     bilibili.NewParser(),
		douyinCollection:   douyin.NewCollectionParser(),
		bilibiliCollection: bilibili.NewCollectionParser(),
		collections:        make(map[string]*CollectionInfo),
		logs:               make([]LogEntry, 0),
	}
	if cfg.BilibiliCookie != "" {
		h.bilibiliParser.SetCookies(cfg.BilibiliCookie)
		h.bilibiliCollection.SetCookies(cfg.BilibiliCookie)
	}
	if cfg.DouyinCookie != "" {
		h.douyinParser.SetCookies(cfg.DouyinCookie)
		h.douyinCollection.SetCookies(cfg.DouyinCookie)
	}
	return h
}

func (h *Handlers) addLog(level, taskID, message string) {
	// 同时输出到控制台
	log.Printf("[%s] %s: %s", level, taskID, message)

	h.logsMu.Lock()
	h.logs = append(h.logs, LogEntry{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Level:     level,
		TaskID:    taskID,
		Message:   message,
	})
	if len(h.logs) > 1000 {
		h.logs = h.logs[len(h.logs)-1000:]
	}
	logs := make([]LogEntry, len(h.logs))
	copy(logs, h.logs)
	h.logsMu.Unlock()

	h.saveOperationLogs(logs)
}

// operationLogsPath 操作日志持久化文件
func (h *Handlers) operationLogsPath() string {
	return filepath.Join(h.cfg.LogDir, "operation.json")
}

// saveOperationLogs 操作日志落盘（重启不丢）
func (h *Handlers) saveOperationLogs(logs []LogEntry) {
	data, err := json.Marshal(logs)
	if err != nil {
		return
	}
	tmp := h.operationLogsPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	os.Rename(tmp, h.operationLogsPath())
}

// loadOperationLogs 启动时恢复操作日志
func (h *Handlers) loadOperationLogs() {
	data, err := os.ReadFile(h.operationLogsPath())
	if err != nil {
		return // 文件不存在则跳过
	}
	var logs []LogEntry
	if err := json.Unmarshal(data, &logs); err != nil {
		return
	}
	h.logsMu.Lock()
	h.logs = logs
	h.logsMu.Unlock()
	log.Printf("[INFO] 已恢复 %d 条操作日志", len(logs))
}

// ==================== Index ====================

func (h *Handlers) Index(c *gin.Context) {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = filepath.Join(exeDir, "static")
	} else {
		staticDir = filepath.Join(staticDir, "static")
	}
	c.File(filepath.Join(staticDir, "index-v2.html"))
}

// ==================== Config ====================

func (h *Handlers) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"download_dir": h.cfg.DownloadDir,
		"version":      Version,
		"first_run":    false,
		"total_tasks":  len(h.mgr.GetAllTasks()),
		"platform":     runtime.GOOS,
	})
}

// ==================== Console Window ====================

func (h *Handlers) ToggleConsoleWindow(c *gin.Context) {
	var req struct {
		Show bool `json:"show"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效请求"})
		return
	}

	if req.Show {
		showConsoleWindow()
		log.Println("[INFO] 控制台窗口已显示")
	} else {
		hideConsoleWindow()
		log.Println("[INFO] 控制台窗口已隐藏")
	}

	c.JSON(http.StatusOK, gin.H{
		"visible": req.Show,
	})
}

func (h *Handlers) GetConsoleVisible(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"visible": isConsoleVisible(),
	})
}

// ==================== Download Dir ====================

func (h *Handlers) GetDownloadDir(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"download_dir": h.cfg.DownloadDir})
}

func (h *Handlers) SetDownloadDir(c *gin.Context) {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "路径不能为空"})
		return
	}
	if err := os.MkdirAll(req.Path, 0755); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无法创建目录: " + err.Error()})
		return
	}
	h.cfg.DownloadDir = req.Path
	h.cfg.Save()
	c.JSON(http.StatusOK, gin.H{"message": "下载目录已更新", "download_dir": req.Path})
}

// ==================== Browse Folder ====================

func (h *Handlers) BrowseFolder(c *gin.Context) {
	if runtime.GOOS != "windows" {
		c.JSON(http.StatusNotImplemented, gin.H{"detail": "此功能仅支持Windows桌面版"})
		return
	}
	script := `
	Add-Type -AssemblyName System.Windows.Forms
	Add-Type -AssemblyName System.Drawing

	# 创建隐藏的 TopMost 窗口作为对话框 Owner，强制置顶
	$form = New-Object System.Windows.Forms.Form
	$form.StartPosition = [System.Windows.Forms.FormStartPosition]::Manual
	$form.Location = New-Object System.Drawing.Point(-10000, -10000)
	$form.Size = New-Object System.Drawing.Size(1, 1)
	$form.TopMost = $true
	$form.ShowInTaskbar = $false
	$form.Opacity = 0
	$form.Show()
	$form.BringToFront()

	$dialog = New-Object System.Windows.Forms.FolderBrowserDialog
	$dialog.Description = "选择下载目录"
	$dialog.ShowNewFolderButton = $true
	[void]$dialog.ShowDialog($form)

	$form.Close()
	$form.Dispose()

	if ($dialog.SelectedPath) {
	    Write-Output $dialog.SelectedPath
	} else {
	    Write-Output "CANCELLED"
	}`
	cmd := exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"cancelled": true})
		return
	}
	path := strings.TrimSpace(string(output))
	if path == "CANCELLED" || path == "" {
		c.JSON(http.StatusOK, gin.H{"cancelled": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": path})
}

// ==================== Bilibili Cookie ====================

func (h *Handlers) GetBilibiliCookie(c *gin.Context) {
	cookie := h.cfg.BilibiliCookie
	masked := ""
	if len(cookie) > 20 {
		masked = cookie[:10] + "..." + cookie[len(cookie)-10:]
	} else if len(cookie) > 0 {
		masked = cookie[:5] + "..."
	}
	c.JSON(http.StatusOK, gin.H{"cookie_masked": masked, "has_cookie": cookie != ""})
}

func (h *Handlers) SetBilibiliCookie(c *gin.Context) {
	var req struct {
		Cookie string `json:"cookie"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效请求"})
		return
	}
	h.cfg.BilibiliCookie = req.Cookie
	h.cfg.Save()
	h.bilibiliParser.SetCookies(req.Cookie)
	h.bilibiliCollection.SetCookies(req.Cookie)
	h.mgr.SetBilibiliCookie(req.Cookie) // 同步给下载管理器，立即生效
	c.JSON(http.StatusOK, gin.H{"message": "B站Cookie已保存"})
}

func (h *Handlers) LoginBilibili(c *gin.Context) {
	if runtime.GOOS != "windows" {
		c.JSON(http.StatusNotImplemented, gin.H{"detail": "扫码登录仅支持Windows桌面版，请手动粘贴Cookie"})
		return
	}
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	cookiePath := filepath.Join(os.TempDir(), "bilibili_cookies.txt")
	os.Remove(cookiePath)

	// 无论登录成功与否，请求结束后都清掉含凭据的临时文件
	defer os.Remove(cookiePath)

	wv2Core := filepath.Join(exeDir, "webview2", "Microsoft.Web.WebView2.Core.dll")
	wv2Wpf := filepath.Join(exeDir, "webview2", "Microsoft.Web.WebView2.Wpf.dll")

	if _, err := os.Stat(wv2Wpf); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "WebView2 SDK 未找到"})
		return
	}

	wv2Core = strings.ReplaceAll(wv2Core, "'", "''")
	wv2Wpf = strings.ReplaceAll(wv2Wpf, "'", "''")
	cookiePathEsc := strings.ReplaceAll(cookiePath, "'", "''")

	script := fmt.Sprintf(`$ErrorActionPreference='Continue'
$coreDll='%s'
$wpfDll='%s'
$cookiePath='%s'
Add-Type -Path $coreDll
Add-Type -Path $wpfDll
$code=@"
using System;using System.Threading.Tasks;using System.Windows;using Microsoft.Web.WebView2.Wpf;
public class BilibiliLoginWin{
    public static Window Create(string url,string cp){
        var w=new Window();w.Title="B站登录";w.Width=500;w.Height=700;w.WindowStartupLocation=WindowStartupLocation.CenterScreen;
        var v=new WebView2();w.Content=v;
        w.Loaded+=async(s,e)=>{
            try{
                var env=await Microsoft.Web.WebView2.Core.CoreWebView2Environment.CreateAsync(null,System.IO.Path.Combine(System.IO.Path.GetTempPath(),"vd-wv2-bilibili"));
                await v.EnsureCoreWebView2Async(env);
                v.CoreWebView2.Navigate(url);
                System.Windows.Threading.DispatcherTimer timer=null;
                timer=new System.Windows.Threading.DispatcherTimer();
                timer.Interval=TimeSpan.FromSeconds(2);
                timer.Tick+=async(sender,args)=>{
                    try{
                        var cookies=await v.CoreWebView2.CookieManager.GetCookiesAsync(url);
                        bool found=false;
                        foreach(var c in cookies){if(c.Name=="bili_jct"||c.Name=="DedeUserID"){found=true;break;}}
                        if(found){
                            var parts=new System.Collections.Generic.List<string>();
                            foreach(var c2 in cookies){parts.Add(c2.Name+"="+c2.Value);}
                            System.IO.File.WriteAllText(cp,string.Join("; ",parts));
                            timer.Stop();w.Close();
                        }
                    }catch{}
                };
                timer.Start();
            }catch(Exception ex){MessageBox.Show("Init error: "+ex.Message,"Error",MessageBoxButton.OK,MessageBoxImage.Error);}
        };
        return w;
    }
}
"@
Add-Type -TypeDefinition $code -ReferencedAssemblies 'PresentationFramework','PresentationCore','WindowsBase','System.Xaml',$wpfDll,$coreDll
$win=[BilibiliLoginWin]::Create('https://www.bilibili.com',$cookiePath)
$app=New-Object System.Windows.Application
$app.Run($win)`, wv2Core, wv2Wpf, cookiePathEsc)

	cmd := exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[ERROR] LoginBilibili PS error: %v, output: %s\n", err, string(output))
	}

	cookieBytes, readErr := os.ReadFile(cookiePath)
	if readErr != nil || len(cookieBytes) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "未获取到Cookie，可能未登录或超时"})
		return
	}

	cookieStr := string(cookieBytes)
	h.cfg.BilibiliCookie = cookieStr
	h.cfg.Save()
	h.bilibiliParser.SetCookies(cookieStr)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "登录成功", "cookie_length": len(cookieStr)})
}

func (h *Handlers) PollBilibiliLogin(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": false, "message": "请使用扫码登录功能"})
}

// ==================== Douyin Cookie ====================

// ==================== Proxy ====================

// ---- 反 SSRF 白名单闸门：仅允许代理两家平台的封面/媒体 CDN ----

var proxyAllowedHostSuffixes = []string{
	// Bilibili
	"hdslb.com",
	"biliimg.com",
	"bilibili.com",
	"bilivideo.com",
	"bilivideo.cn",
	// Douyin / ByteDance 媒体资源
	"douyinpic.com",
	"douyinvod.com",
	"douyinstatic.com",
	"byteimg.com",
	"zjcdn.com",
	"snssdk.com",
	"amemv.com",
}

// isProxyURLAllowed 判断目标是否为白名单内的平台资源地址。
func isProxyURLAllowed(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if net.ParseIP(host) != nil {
		return false // 禁止 IP 直连，防止直指内网
	}
	for _, sfx := range proxyAllowedHostSuffixes {
		if host == sfx || strings.HasSuffix(host, "."+sfx) {
			return true
		}
	}
	return false
}

func isBlockedProxyIP(ip net.IP) bool {
	return ip == nil || ip.IsUnspecified() || ip.IsLoopback() ||
		ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

// proxyDialControl 在建连前校验已解析的 IP，防 DNS rebinding 绕过域名白名单。
func proxyDialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if isBlockedProxyIP(net.ParseIP(host)) {
		return fmt.Errorf("proxy target resolved to blocked address %s", host)
	}
	return nil
}

// newProxiedClient 构造带重定向校验与建连层 IP 校验的代理专用客户端。
func newProxiedClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: proxyDialControl}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: dialer.DialContext,
			Proxy:       http.ProxyFromEnvironment,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if !isProxyURLAllowed(req.URL.String()) {
				return fmt.Errorf("redirect to non-whitelisted host blocked")
			}
			return nil
		},
	}
}
func (h *Handlers) ProxyImage(c *gin.Context) {
	imageURL := c.Query("url")
	if imageURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "缺少图片URL"})
		return
	}

	if !isProxyURLAllowed(imageURL) {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "不支持的代理目标"})
		return
	}

	client := newProxiedClient(15 * time.Second)
	req, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "创建请求失败"})
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")

	// 根据图片URL域名设置正确的Referer
	if strings.Contains(imageURL, "bilibili.com") || strings.Contains(imageURL, "hdslb.com") || strings.Contains(imageURL, "bilivideo.com") {
		req.Header.Set("Referer", "https://www.bilibili.com/")
	} else if strings.Contains(imageURL, "douyin.com") || strings.Contains(imageURL, "byteimg.com") || strings.Contains(imageURL, "bytecdn.cn") || strings.Contains(imageURL, "bytedance.com") || strings.Contains(imageURL, "snssdk.com") {
		req.Header.Set("Referer", "https://www.douyin.com/")
	} else {
		req.Header.Set("Referer", "https://www.google.com/")
	}

	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "获取图片失败"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "图片源返回错误"})
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=86400")
	io.Copy(c.Writer, resp.Body)
}

func (h *Handlers) ProxyFileDownload(c *gin.Context) {
	fileURL := c.Query("url")
	if fileURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "缺少文件URL"})
		return
	}

	if !isProxyURLAllowed(fileURL) {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "不支持的代理目标"})
		return
	}

	client := newProxiedClient(600 * time.Second)
	req, err := http.NewRequest("GET", fileURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "创建请求失败"})
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://www.douyin.com/")

	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "下载失败"})
		return
	}
	defer resp.Body.Close()

	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	c.Header("Content-Length", resp.Header.Get("Content-Length"))
	io.Copy(c.Writer, resp.Body)
}

func (h *Handlers) ProxyDownload(c *gin.Context) {
	taskID := c.Param("task_id")
	task := h.mgr.GetTask(taskID)
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "任务不存在"})
		return
	}
	c.JSON(http.StatusNotImplemented, gin.H{"detail": "暂不支持"})
}

// ==================== Stats ====================

func (h *Handlers) GetStats(c *gin.Context) {
	sourceFilter := c.Query("source")
	tasks := h.mgr.GetAllTasksFiltered(sourceFilter)
	total, pending, parsing, downloading, completed, failed := 0, 0, 0, 0, 0, 0
	for _, t := range tasks {
		total++
		switch t.Status {
		case StatusPending:
			pending++
		case StatusParsing:
			parsing++
		case StatusDownloading:
			downloading++
		case StatusCompleted:
			completed++
		case StatusFailed:
			failed++
		}
	}

	// 合集统计（每个合集算一个任务）
	h.collectionsMu.RLock()
	collectionsTotal := len(h.collections)
	collectionsDownloading := 0
	collectionsCompleted := 0
	collectionsFailed := 0
	var collectionSpeed int64
	for _, col := range h.collections {
		switch col.Status {
		case "downloading":
			collectionsDownloading++
		case "completed":
			collectionsCompleted++
		case "partial", "failed":
			collectionsFailed++
		}
		for _, video := range col.Videos {
			if video.Status == "downloading" {
				collectionSpeed += video.Speed
			}
		}
	}
	h.collectionsMu.RUnlock()

	// 计算全局下载速度
	var globalSpeed int64
	for _, t := range tasks {
		if t.Status == StatusDownloading {
			globalSpeed += t.Speed
		}
	}

	globalSpeed += collectionSpeed
	c.JSON(http.StatusOK, gin.H{
		"total":                   total + collectionsTotal,
		"pending":                 pending,
		"parsing":                 parsing,
		"downloading":             downloading + collectionsDownloading,
		"completed":               completed + collectionsCompleted,
		"failed":                  failed + collectionsFailed,
		"single_total":            total,
		"collections_total":       collectionsTotal,
		"collections_downloading": collectionsDownloading,
		"collections_completed":   collectionsCompleted,
		"collections_failed":      collectionsFailed,
		"global_speed":            globalSpeed,
	})
}

// ==================== Logs ====================

func (h *Handlers) GetLogs(c *gin.Context) {
	levelFilter := c.Query("level")
	taskFilter := c.Query("task_id")

	h.logsMu.RLock()
	defer h.logsMu.RUnlock()

	filtered := make([]LogEntry, 0)
	for _, entry := range h.logs {
		if levelFilter != "" && entry.Level != levelFilter {
			continue
		}
		if taskFilter != "" && entry.TaskID != taskFilter {
			continue
		}
		filtered = append(filtered, entry)
	}
	c.JSON(http.StatusOK, filtered)
}

func (h *Handlers) ClearLogs(c *gin.Context) {
	h.logsMu.Lock()
	h.logs = make([]LogEntry, 0)
	h.logsMu.Unlock()
	c.JSON(http.StatusOK, gin.H{"message": "日志已清空"})
}

// ==================== Tasks ====================

func (h *Handlers) CreateTask(c *gin.Context) {
	var req struct {
		URL     string `json:"url"`
		Quality string `json:"quality"`
		Source  string `json:"source"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效请求"})
		return
	}

	if req.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "请提供视频链接"})
		return
	}

	// 从文本中提取URL
	extractedURL := extractFirstURL(req.URL)
	if extractedURL == "" {
		extractedURL = req.URL
	}

	// 预览模式：解析视频信息并返回，不创建下载任务（带重试）
	if req.Quality == "preview" {
		// 先判断是否为明确的合集URL（非短链接）
		if douyin.IsDouyinCollectionURL(extractedURL) || bilibili.IsBilibiliCollectionURL(extractedURL) {
			colInfo, colErr := h.previewCollection(extractedURL)
			if colErr == nil {
				c.JSON(http.StatusOK, colInfo)
				return
			}
		}

		// 尝试单视频解析（最多重试5次，间隔3秒）
		var videoInfo map[string]interface{}
		var err error
		for attempt := 1; attempt <= 5; attempt++ {
			videoInfo, err = h.mgr.ParseVideo(extractedURL, "4k")
			if err == nil {
				break
			}
			if attempt < 5 {
				log.Printf("[预览重试] 解析失败(第%d次): %v，3秒后重试...", attempt, err)
				time.Sleep(3 * time.Second)
			}
		}
		if err != nil {
			// 单视频失败，尝试合集解析作为 fallback
			colInfo, colErr := h.previewCollection(extractedURL)
			if colErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("解析失败: %v", err)})
				return
			}
			c.JSON(http.StatusOK, colInfo)
			return
		}
		c.JSON(http.StatusOK, videoInfo)
		return
	}
	if req.Quality == "" {
		req.Quality = "1080p"
	}

	// 检查是否已有相同URL的任务
	existing := h.mgr.FindTaskByURL(extractedURL)
	if existing != nil {
		c.JSON(http.StatusOK, existing)
		return
	}

	task := h.mgr.CreateTaskWithSource(extractedURL, req.Quality, req.Source)
	go h.mgr.ExecuteTask(context.Background(), task.ID)

	h.addLog("INFO", task.ID, "任务创建: "+extractedURL)

	c.JSON(http.StatusOK, task)
}

func (h *Handlers) CreateTaskFromPreview(c *gin.Context) {
	var req struct {
		URL         string                 `json:"url"`
		Quality     string                 `json:"quality"`
		Source      string                 `json:"source"`
		PreviewData map[string]interface{} `json:"preview_data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效请求"})
		return
	}

	if req.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "请提供视频链接"})
		return
	}

	if req.Quality == "" {
		req.Quality = "1080p"
	}

	// 从分享文本中提取纯URL
	extractedURL := extractFirstURL(req.URL)
	if extractedURL == "" {
		extractedURL = req.URL
	}

	existing := h.mgr.FindTaskByURL(extractedURL)
	if existing != nil {
		c.JSON(http.StatusOK, existing)
		return
	}

	task := h.mgr.CreateTaskWithSource(extractedURL, req.Quality, req.Source)

	// 从预览数据中填充任务信息（避免重复解析）
	if req.PreviewData != nil {
		if title, ok := req.PreviewData["title"].(string); ok && title != "" {
			task.Title = title
		}
		if author, ok := req.PreviewData["author"].(string); ok && author != "" {
			task.Author = author
		}
		if coverURL, ok := req.PreviewData["cover_url"].(string); ok && coverURL != "" {
			task.CoverURL = coverURL
		}
		if videoURL, ok := req.PreviewData["video_url"].(string); ok && videoURL != "" {
			task.VideoURL = videoURL
		}
		if audioURL, ok := req.PreviewData["audio_url"].(string); ok && audioURL != "" {
			task.AudioURL = audioURL
		}
		if platform, ok := req.PreviewData["platform"].(string); ok && platform != "" {
			task.Platform = platform
		}
	}

	go h.mgr.ExecuteTask(context.Background(), task.ID)

	c.JSON(http.StatusOK, task)
}

func (h *Handlers) GetTasks(c *gin.Context) {
	statusFilter := c.Query("status")
	sourceFilter := c.Query("source") // "app" 或 "" (全部)
	tasks := h.mgr.GetAllTasksFiltered(sourceFilter)

	if statusFilter == "" {
		c.JSON(http.StatusOK, tasks)
		return
	}

	filtered := make([]*download.Task, 0)
	for _, t := range tasks {
		if string(t.Status) == statusFilter {
			filtered = append(filtered, t)
		}
	}
	c.JSON(http.StatusOK, filtered)
}

func (h *Handlers) GetTask(c *gin.Context) {
	taskID := c.Param("task_id")
	task := h.mgr.GetTask(taskID)
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "任务不存在"})
		return
	}
	c.JSON(http.StatusOK, task)
}

func (h *Handlers) DeleteTask(c *gin.Context) {
	taskID := c.Param("task_id")
	deleteFile := c.Query("deleteFile") == "true"
	if h.mgr.DeleteTask(taskID, deleteFile) {
		c.JSON(http.StatusOK, gin.H{"message": "任务已删除"})
	} else {
		c.JSON(http.StatusNotFound, gin.H{"detail": "任务不存在"})
	}
}

func (h *Handlers) RetryTask(c *gin.Context) {
	taskID := c.Param("task_id")
	task := h.mgr.GetTask(taskID)
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "任务不存在"})
		return
	}
	if task.Status != StatusFailed {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "只能重试失败的任务"})
		return
	}
	if !h.mgr.ResetTaskForRetry(taskID) {
		c.JSON(http.StatusConflict, gin.H{"detail": "任务状态已变化，无法重试"})
		return
	}

	task = h.mgr.GetTask(taskID)
	go h.mgr.ExecuteTask(context.Background(), task.ID)
	c.JSON(http.StatusOK, task)
}

func (h *Handlers) DownloadTaskFile(c *gin.Context) {
	taskID := c.Param("task_id")
	task := h.mgr.GetTask(taskID)
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "任务不存在"})
		return
	}
	if task.Status != StatusCompleted {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "任务未完成"})
		return
	}
	if task.FilePath == "" {
		c.JSON(http.StatusNotFound, gin.H{"detail": "文件不存在"})
		return
	}
	filename := filepath.Base(task.FilePath)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.File(task.FilePath)
}

// PlayTaskFile Web在线播放（内联输出，无attachment头，支持Range拖动）
func (h *Handlers) PlayTaskFile(c *gin.Context) {
	taskID := c.Param("task_id")
	task := h.mgr.GetTask(taskID)
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "任务不存在"})
		return
	}
	if task.Status != StatusCompleted {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "任务未完成"})
		return
	}
	if task.FilePath == "" {
		c.JSON(http.StatusNotFound, gin.H{"detail": "文件不存在"})
		return
	}
	if _, err := os.Stat(task.FilePath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "文件已被删除"})
		return
	}
	c.Header("Content-Type", "video/mp4")
	c.File(task.FilePath)
}

// StreamTaskToMobile 流式输出给手机客户端，传输完成后删除服务端文件。
// 手机下载的视频不保留在服务器上。
func (h *Handlers) StreamTaskToMobile(c *gin.Context) {
	taskID := c.Param("task_id")
	task := h.mgr.GetTask(taskID)
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "任务不存在"})
		return
	}
	if task.Status != StatusCompleted {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "任务未完成"})
		return
	}
	if task.FilePath == "" {
		c.JSON(http.StatusNotFound, gin.H{"detail": "文件不存在"})
		return
	}
	if _, err := os.Stat(task.FilePath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "文件已被删除"})
		return
	}

	filename := filepath.Base(task.FilePath)
	// ASCII 安全的 filename + RFC5987 编码的 filename*
	quoted := strings.ReplaceAll(filename, `"`, `_`)
	c.Header("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
			quoted, url.PathEscape(filename)))
	c.Header("Content-Length", fmt.Sprintf("%d", task.FileSize))
	c.File(task.FilePath)

	// 流式传输完成后删除服务端文件（手机下载的不保留）
	go func() {
		// 等待响应完成
		time.Sleep(2 * time.Second)
		if err := os.Remove(task.FilePath); err == nil {
			log.Printf("[移动端] 已流式传输并删除: %s", filename)
		}
	}()
}

// ==================== Collections ====================

func (h *Handlers) PreviewCollection(c *gin.Context) {
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效请求"})
		return
	}

	if req.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "请提供合集链接"})
		return
	}

	// 从分享文本中提取URL
	if extracted := extractFirstURL(req.URL); extracted != "" {
		req.URL = extracted
	}

	// 根据URL判断平台
	isDouyin := strings.Contains(req.URL, "douyin.com") || strings.Contains(req.URL, "iesdouyin.com") || strings.Contains(req.URL, "v.douyin.com")
	isBilibili := strings.Contains(req.URL, "bilibili.com") || strings.Contains(req.URL, "b23.tv")

	if isDouyin {
		// 短链接直接走单视频解析，不尝试合集
		if strings.Contains(req.URL, "v.douyin.com") {
			videoInfo, vErr := h.mgr.ParseVideo(req.URL, "4k")
			if vErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("解析失败: %v", vErr)})
				return
			}
			c.JSON(http.StatusOK, videoInfo)
			return
		}
		info, err := h.douyinCollection.ParseCollection(req.URL)
		if err != nil {
			// 合集解析失败，尝试单视频解析
			videoInfo, vErr := h.mgr.ParseVideo(req.URL, "4k")
			if vErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"detail": "解析失败: " + err.Error()})
				return
			}
			c.JSON(http.StatusOK, videoInfo)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"type":        "collection",
			"platform":    "douyin",
			"title":       info.Title,
			"author":      info.Author,
			"count":       info.TotalCount,
			"total_count": info.TotalCount,
			"videos":      info.Videos,
		})
	} else if isBilibili {
		info, err := h.bilibiliCollection.ParseCollection(req.URL)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": "解析B站合集失败: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"type":        "collection",
			"platform":    "bilibili",
			"title":       info.Title,
			"author":      info.Author,
			"count":       info.TotalCount,
			"total_count": info.TotalCount,
			"videos":      info.Videos,
		})
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "不支持的链接格式"})
	}
}

func (h *Handlers) CreateCollection(c *gin.Context) {
	var req struct {
		URL             string                   `json:"url"`
		Title           string                   `json:"title"`
		Videos          []map[string]interface{} `json:"videos"`
		Quality         string                   `json:"quality"`
		Source          string                   `json:"source"`
		AutoDownload    bool                     `json:"auto_download"`
		SelectedIndices []int                    `json:"selected_indices"` // 选中要下载的视频索引
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效请求"})
		return
	}

	colID := fmt.Sprintf("col_%d", time.Now().UnixNano())
	videos := make([]*CollectionVideoInfo, 0, len(req.Videos))
	for i, v := range req.Videos {
		videoID, _ := v["video_id"].(string)
		bvid, _ := v["bvid"].(string)
		url, _ := v["url"].(string)
		title, _ := v["title"].(string)
		author, _ := v["author"].(string)
		cover, _ := v["cover"].(string)
		coverURL, _ := v["cover_url"].(string)
		if coverURL != "" {
			cover = coverURL
		}
		duration := 0
		if d, ok := v["duration"].(float64); ok {
			duration = int(d)
		}
		videos = append(videos, &CollectionVideoInfo{
			BVID:     bvid,
			VideoID:  videoID,
			URL:      url,
			Title:    title,
			Author:   author,
			CoverURL: cover,
			Duration: duration,
			Page:     i + 1,
			Status:   "pending",
		})
	}

	// 从视频列表中获取作者信息
	author := ""
	if len(videos) > 0 && videos[0].Author != "" {
		author = videos[0].Author
	}

	col := &CollectionInfo{
		ID:              colID,
		URL:             req.URL,
		Title:           req.Title,
		Author:          author,
		TotalCount:      len(videos),
		Videos:          videos,
		Status:          "pending",
		Quality:         req.Quality,
		Source:          req.Source,
		CreatedAt:       time.Now(),
		SelectedIndices: req.SelectedIndices,
		RefreshInterval: 60, // 默认60分钟刷新间隔
	}

	h.collectionsMu.Lock()
	h.collections[colID] = col
	h.collectionsMu.Unlock()

	log.Printf("[INFO] %s: 合集已创建: %s, AutoDownload=%v, Quality=%s, 视频数=%d, SelectedIndices=%v", colID, col.Title, req.AutoDownload, req.Quality, len(col.Videos), req.SelectedIndices)
	h.addLog("INFO", colID, fmt.Sprintf("合集已创建: %s, AutoDownload=%v, Quality=%s", col.Title, req.AutoDownload, req.Quality))

	if req.AutoDownload {
		// 如果指定了 selected_indices，只为选中的视频启动下载
		if len(req.SelectedIndices) > 0 {
			log.Printf("[INFO] %s: 启动选中视频下载，共 %d 个...", colID, len(req.SelectedIndices))
			h.addLog("INFO", colID, fmt.Sprintf("启动选中视频下载，共 %d 个", len(req.SelectedIndices)))
			go h.downloadSelectedCollectionVideos(colID, req.SelectedIndices, req.Quality)
		} else {
			// 没有指定选中索引，下载全部
			log.Printf("[INFO] %s: 启动合集下载（全部）...", colID)
			h.addLog("INFO", colID, "启动合集下载...")
			go h.downloadCollectionVideos(colID, req.Quality)
		}
	} else {
		log.Printf("[INFO] %s: AutoDownload=false, 不启动下载", colID)
	}

	// 持久化合集数据
	if err := h.saveCollections(); err != nil {
		log.Printf("[WARN] 保存合集数据失败: %v", err)
	}

	c.JSON(http.StatusOK, col)
}

// downloadCollectionVideoFile 下载合集中单个视频的核心逻辑（提取自三个重复函数）
func (h *Handlers) downloadCollectionVideoFile(colID string, video *CollectionVideoInfo, idx int, quality string) {
	videoURL := video.URL
	if videoURL == "" && video.VideoID != "" {
		videoURL = fmt.Sprintf("https://www.douyin.com/video/%s", video.VideoID)
	}
	if videoURL == "" && video.BVID != "" {
		videoURL = fmt.Sprintf("https://www.bilibili.com/video/%s", video.BVID)
	}
	if videoURL == "" {
		h.collectionsMu.Lock()
		video.Status = "failed"
		video.ErrorMessage = "无有效URL"
		h.collectionsMu.Unlock()
		return
	}

	h.collectionsMu.Lock()
	video.Status = "downloading"
	video.Progress = 0
	video.ErrorMessage = ""
	h.collectionsMu.Unlock()

	var lastDL int64
	lastTS := time.Now()
	updateProgress := func(downloaded, total int64) {
		h.collectionsMu.Lock()
		defer h.collectionsMu.Unlock()
		if total > 0 {
			video.Progress = int(downloaded * 100 / total)
		}
		now := time.Now()
		dt := now.Sub(lastTS).Seconds()
		if dt >= 0.5 {
			video.Speed = int64(float64(downloaded-lastDL) / dt)
			lastDL = downloaded
			lastTS = now
		}
	}

	h.addLog("INFO", colID, fmt.Sprintf("开始下载: %s", video.Title))

	// 解析视频
	videoInfo, err := h.mgr.ParseVideo(videoURL, quality)
	if err != nil {
		h.collectionsMu.Lock()
		video.Status = "failed"
		video.ErrorMessage = err.Error()
		h.collectionsMu.Unlock()
		h.addLog("ERROR", colID, fmt.Sprintf("解析失败: %s - %v", video.Title, err))
		return
	}

	h.collectionsMu.Lock()
	if videoInfo["title"] != nil {
		video.Title = videoInfo["title"].(string)
	}
	if videoInfo["author"] != nil {
		video.Author = videoInfo["author"].(string)
	}
	if videoInfo["cover_url"] != nil {
		video.CoverURL = videoInfo["cover_url"].(string)
	}
	h.collectionsMu.Unlock()

	// 生成输出路径
	safeTitle := sanitizeFilename(video.Title)
	if safeTitle == "" {
		safeTitle = fmt.Sprintf("video_%d", idx+1)
	}
	colFolder := sanitizeFilename(h.getCollectionTitle(colID))
	if colFolder == "" {
		colFolder = colID
	}
	colDir := filepath.Join(h.cfg.DownloadDir, colFolder)
	os.MkdirAll(colDir, 0755)
	outputPath := filepath.Join(colDir, safeTitle+".mp4")

	// 下载
	platform := download.IdentifyPlatform(videoURL)
	var downloadErr error

	switch platform {
	case "bilibili":
		parser := bilibili.NewParser()
		if h.cfg.BilibiliCookie != "" {
			parser.SetCookies(h.cfg.BilibiliCookie)
		}
		biliInfo, err := parser.Parse(videoURL, quality)
		if err != nil {
			downloadErr = err
			break
		}
		downloader := bilibili.NewDownloader()
		if h.cfg.BilibiliCookie != "" {
			downloader.SetCookies(h.cfg.BilibiliCookie)
		}
		if biliInfo.AudioURL != "" {
			downloadErr = downloader.DownloadWithMerge(biliInfo.VideoURL, biliInfo.AudioURL, outputPath, func(downloaded, total int64) {
				updateProgress(downloaded, total)
			})
		} else {
			downloadErr = downloader.Download(biliInfo.VideoURL, outputPath, func(downloaded, total int64) {
				updateProgress(downloaded, total)
			})
		}
	case "douyin":
		douyinDl := douyin.NewDouyinDownloader(h.cfg.Proxy)
		if h.cfg.DouyinCookie != "" {
			douyinDl.SetCookies(h.cfg.DouyinCookie)
		}
		douyinInfo, err := douyinDl.Parse(videoURL)
		if err != nil {
			downloadErr = err
			break
		}
		cookies := make(map[string]string)
		downloadErr = douyinDl.DownloadVideo(douyinInfo.VideoURL, outputPath, cookies, func(downloaded, total int64) {
			updateProgress(downloaded, total)
		})
	default:
		downloadErr = fmt.Errorf("不支持的平台")
	}

	h.collectionsMu.Lock()
	if downloadErr != nil {
		video.Status = "failed"
		video.ErrorMessage = downloadErr.Error()
		h.addLog("ERROR", colID, fmt.Sprintf("下载失败: %s - %v", video.Title, downloadErr))
	} else {
		video.Status = "completed"
		video.Speed = 0
		video.Progress = 100
		video.FilePath = outputPath
		if info, err := os.Stat(outputPath); err == nil {
			video.FileSize = info.Size()
		}
		h.addLog("INFO", colID, fmt.Sprintf("下载完成: %s", video.Title))
	}
	h.collectionsMu.Unlock()
}

// previewCollection 预览合集（支持B站和抖音）
func (h *Handlers) previewCollection(url string) (gin.H, error) {
	platform := download.IdentifyPlatform(url)
	switch platform {
	case "bilibili":
		info, err := h.bilibiliCollection.ParseCollection(url)
		if err != nil {
			return nil, err
		}
		return gin.H{
			"type":   "collection",
			"title":  info.Title,
			"author": info.Author,
			"count":  info.TotalCount,
			"videos": info.Videos,
		}, nil
	case "douyin":
		info, err := h.douyinCollection.ParseCollection(url)
		if err != nil {
			return nil, err
		}
		return gin.H{
			"type":   "collection",
			"title":  info.Title,
			"author": info.Author,
			"count":  info.TotalCount,
			"videos": info.Videos,
		}, nil
	default:
		return nil, fmt.Errorf("不支持的平台")
	}
}

// getCollectionTitle safely gets collection title by ID
func (h *Handlers) getCollectionTitle(colID string) string {
	h.collectionsMu.RLock()
	defer h.collectionsMu.RUnlock()
	if col, exists := h.collections[colID]; exists {
		return col.Title
	}
	return ""
}

// downloadSelectedCollectionVideos 下载合集中选中的视频
func (h *Handlers) downloadSelectedCollectionVideos(colID string, indices []int, quality string) {
	h.collectionsMu.RLock()
	col, exists := h.collections[colID]
	h.collectionsMu.RUnlock()
	if !exists {
		return
	}

	log.Printf("[INFO] %s: 开始下载选中视频: %s, 选中数: %d", colID, col.Title, len(indices))
	h.addLog("INFO", colID, fmt.Sprintf("开始下载选中视频: %s, 选中数: %d", col.Title, len(indices)))
	h.collectionsMu.Lock()
	col.Status = "downloading"
	h.collectionsMu.Unlock()

	selectedSet := make(map[int]bool)
	for _, idx := range indices {
		selectedSet[idx] = true
	}

	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for i, v := range col.Videos {
		if !selectedSet[i] || v.Status == "completed" {
			continue
		}

		wg.Add(1)
		go func(idx int, video *CollectionVideoInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			h.downloadCollectionVideoFile(colID, video, idx, quality)
		}(i, v)
	}

	go func() {
		wg.Wait()
		h.collectionsMu.Lock()
		col.Status = "completed"
		for _, v := range col.Videos {
			if v.Status == "failed" {
				col.Status = "partial"
				break
			}
		}
		h.collectionsMu.Unlock()
		log.Printf("[INFO] %s: 选中视频下载完成", colID)
		h.addLog("INFO", colID, "选中视频下载完成")
	}()
}

func (h *Handlers) GetCollections(c *gin.Context) {
	sourceFilter := c.Query("source")
	h.collectionsMu.RLock()
	defer h.collectionsMu.RUnlock()

	cols := make([]*CollectionInfo, 0, len(h.collections))
	for _, col := range h.collections {
		if sourceFilter == "" || col.Source == sourceFilter {
			cols = append(cols, col)
		}
	}
	sort.Slice(cols, func(i, j int) bool {
		return cols[i].CreatedAt.After(cols[j].CreatedAt)
	})
	c.JSON(http.StatusOK, cols)
}

func (h *Handlers) GetCollection(c *gin.Context) {
	id := c.Param("id")
	h.collectionsMu.RLock()
	col, exists := h.collections[id]
	h.collectionsMu.RUnlock()
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"detail": "合集不存在"})
		return
	}
	c.JSON(http.StatusOK, col)
}

func (h *Handlers) GetCollectionVideos(c *gin.Context) {
	id := c.Param("id")
	h.collectionsMu.RLock()
	col, exists := h.collections[id]
	h.collectionsMu.RUnlock()
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"detail": "合集不存在"})
		return
	}
	c.JSON(http.StatusOK, col.Videos)
}

func (h *Handlers) DeleteCollection(c *gin.Context) {
	id := c.Param("id")
	deleteFile := c.Query("deleteFile") == "true"

	h.collectionsMu.Lock()
	col, exists := h.collections[id]
	if !exists {
		h.collectionsMu.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"detail": "合集不存在"})
		return
	}

	// 删除文件
	if deleteFile {
		for i, v := range col.Videos {
			if v.FilePath != "" {
				if err := os.Remove(v.FilePath); err != nil {
					log.Printf("[WARN] 删除文件失败: %s - %v", v.FilePath, err)
				} else {
					log.Printf("[INFO] 删除文件成功: %s", v.FilePath)
				}
			} else {
				log.Printf("[WARN] 视频 %d FilePath为空，跳过删除", i)
			}
		}
		// 尝试删除合集文件夹（如果为空）
		if len(col.Videos) > 0 {
			colFolder := sanitizeFilename(col.Title)
			if colFolder == "" {
				colFolder = id
			}
			colDir := filepath.Join(h.cfg.DownloadDir, colFolder)
			os.Remove(colDir) // 只在目录为空时删除
		}
	}

	delete(h.collections, id)
	h.collectionsMu.Unlock()

	// 持久化合集数据
	if err := h.saveCollections(); err != nil {
		log.Printf("[WARN] 保存合集数据失败: %v", err)
	}

	if deleteFile {
		log.Printf("[INFO] %s: 合集已删除（含本地文件）: %s", id, col.Title)
	} else {
		log.Printf("[INFO] %s: 合集已删除（保留文件）: %s", id, col.Title)
	}
	c.JSON(http.StatusOK, gin.H{"message": "合集已删除"})
}

func (h *Handlers) DeleteCollectionVideo(c *gin.Context) {
	videoID := c.Param("id")
	h.collectionsMu.Lock()
	defer h.collectionsMu.Unlock()

	for _, col := range h.collections {
		for i, v := range col.Videos {
			// VideoID(抖音)或BVID(B站)均可匹配
			if v.VideoID == videoID || (videoID != "" && v.BVID == videoID) {
				// 同时删除已下载的本地文件
				if v.FilePath != "" {
					if err := os.Remove(v.FilePath); err == nil {
						log.Printf("[合集] 已删除视频文件: %s", v.FilePath)
					}
				}
				col.Videos = append(col.Videos[:i], col.Videos[i+1:]...)
				col.TotalCount = len(col.Videos)
				c.JSON(http.StatusOK, gin.H{"message": "视频已删除"})
				return
			}
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"detail": "视频不存在"})
}

// PlayCollectionVideoFile 在线播放合集中的已下载视频（流式，不删文件）
func (h *Handlers) PlayCollectionVideoFile(c *gin.Context) {
	colID := c.Param("id")
	idx := 0
	fmt.Sscanf(c.Param("idx"), "%d", &idx)

	h.collectionsMu.RLock()
	col, exists := h.collections[colID]
	var filePath string
	if exists && idx >= 0 && idx < len(col.Videos) {
		filePath = col.Videos[idx].FilePath
	}
	h.collectionsMu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"detail": "合集不存在"})
		return
	}
	if filePath == "" {
		c.JSON(http.StatusNotFound, gin.H{"detail": "视频未下载"})
		return
	}
	if _, err := os.Stat(filePath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "文件已被删除"})
		return
	}
	c.Header("Content-Type", "video/mp4")
	c.File(filePath)
}

// DownloadCollectionVideo 下载合集中的单个视频
func (h *Handlers) DownloadCollectionVideo(c *gin.Context) {
	colID := c.Param("id")
	idxStr := c.Param("idx")
	idx := 0
	fmt.Sscanf(idxStr, "%d", &idx)

	h.collectionsMu.RLock()
	col, exists := h.collections[colID]
	h.collectionsMu.RUnlock()
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"detail": "合集不存在"})
		return
	}

	if idx < 0 || idx >= len(col.Videos) {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "视频索引无效"})
		return
	}

	video := col.Videos[idx]
	if video.Status == "completed" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "视频已下载"})
		return
	}
	if video.Status == "downloading" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "视频正在下载中"})
		return
	}

	// 重置状态为 pending
	h.collectionsMu.Lock()
	video.Status = "pending"
	video.Progress = 0
	video.ErrorMessage = ""
	h.collectionsMu.Unlock()

	quality := col.Quality
	if quality == "" {
		quality = "1080p"
	}

	// 启动单个视频下载
	go h.downloadSingleCollectionVideo(colID, idx, quality)

	c.JSON(http.StatusOK, gin.H{"message": "下载已启动", "index": idx})
}

// DeleteCollectionVideoFile 删除合集视频的本地文件并重置状态
func (h *Handlers) DeleteCollectionVideoFile(c *gin.Context) {
	colID := c.Param("id")
	idxStr := c.Param("idx")
	idx := 0
	fmt.Sscanf(idxStr, "%d", &idx)

	h.collectionsMu.Lock()
	defer h.collectionsMu.Unlock()

	col, exists := h.collections[colID]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"detail": "合集不存在"})
		return
	}

	if idx < 0 || idx >= len(col.Videos) {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "视频索引无效"})
		return
	}

	video := col.Videos[idx]
	if video.Status != "completed" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "视频未下载"})
		return
	}

	// 删除文件
	if video.FilePath != "" {
		os.Remove(video.FilePath)
	}

	// 重置状态
	video.Status = "pending"
	video.FilePath = ""
	video.FileSize = 0
	video.Progress = 0
	video.ErrorMessage = ""

	c.JSON(http.StatusOK, gin.H{"message": "文件已删除，状态已重置"})
}

// downloadSingleCollectionVideo 下载合集中的单个视频
func (h *Handlers) downloadSingleCollectionVideo(colID string, idx int, quality string) {
	h.collectionsMu.RLock()
	col, exists := h.collections[colID]
	if !exists {
		h.collectionsMu.RUnlock()
		return
	}
	if idx < 0 || idx >= len(col.Videos) {
		h.collectionsMu.RUnlock()
		return
	}
	video := col.Videos[idx]
	h.collectionsMu.RUnlock()

	h.downloadCollectionVideoFile(colID, video, idx, quality)
}

func (h *Handlers) DownloadCollection(c *gin.Context) {
	colID := c.Param("id")
	quality := c.Query("quality")
	if quality == "" {
		quality = "1080p"
	}

	h.collectionsMu.RLock()
	col, exists := h.collections[colID]
	h.collectionsMu.RUnlock()
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"detail": "合集不存在"})
		return
	}

	go h.downloadCollectionVideos(colID, quality)
	c.JSON(http.StatusOK, gin.H{"message": "下载任务已启动", "video_count": len(col.Videos)})
}

func (h *Handlers) downloadCollectionVideos(colID, quality string) {
	log.Printf("[INFO] %s: downloadCollectionVideos 开始执行", colID)
	h.collectionsMu.RLock()
	col, exists := h.collections[colID]
	h.collectionsMu.RUnlock()
	if !exists {
		log.Printf("[ERROR] %s: 合集不存在", colID)
		h.addLog("ERROR", colID, "合集不存在")
		return
	}

	log.Printf("[INFO] %s: 开始下载合集: %s, 视频数: %d", colID, col.Title, len(col.Videos))
	h.addLog("INFO", colID, fmt.Sprintf("开始下载合集: %s, 视频数: %d", col.Title, len(col.Videos)))
	h.collectionsMu.Lock()
	col.Status = "downloading"
	h.collectionsMu.Unlock()

	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for i, v := range col.Videos {
		if v.Status == "completed" {
			continue
		}

		wg.Add(1)
		go func(idx int, video *CollectionVideoInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			h.downloadCollectionVideoFile(colID, video, idx, quality)
		}(i, v)
	}

	go func() {
		wg.Wait()
		h.collectionsMu.Lock()
		col.Status = "completed"
		for _, v := range col.Videos {
			if v.Status == "failed" {
				col.Status = "partial"
				break
			}
		}
		h.collectionsMu.Unlock()
		// 持久化合集状态（否则重启后回退到pending，丢文件路径）
		if err := h.saveCollections(); err != nil {
			log.Printf("[ERROR] %s: 保存合集状态失败: %v", colID, err)
		}
		h.addLog("INFO", colID, fmt.Sprintf("合集下载完成: %s, 状态: %s", col.Title, col.Status))
	}()
}

// ==================== Settings (from old frontend) ====================

func (h *Handlers) SaveSettings(c *gin.Context) {
	var req struct {
		BilibiliCookie string `json:"bilibili_cookie"`
		DouyinCookie   string `json:"douyin_cookie"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效请求"})
		return
	}
	if req.BilibiliCookie != "" {
		h.cfg.BilibiliCookie = req.BilibiliCookie
		h.bilibiliParser.SetCookies(req.BilibiliCookie)
		h.bilibiliCollection.SetCookies(req.BilibiliCookie)
		h.mgr.SetBilibiliCookie(req.BilibiliCookie)
	}
	if req.DouyinCookie != "" {
		h.cfg.DouyinCookie = req.DouyinCookie
		h.douyinParser.SetCookies(req.DouyinCookie)
		h.douyinCollection.SetCookies(req.DouyinCookie)
		h.mgr.SetDouyinCookie(req.DouyinCookie)
	}
	h.cfg.Save()
	c.JSON(http.StatusOK, gin.H{"message": "设置已保存"})
}

// ==================== Helpers ====================

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_", "\n", "_", "\r", "_",
	)
	name = replacer.Replace(name)
	runes := []rune(name)
	if len(runes) > 100 {
		name = string(runes[:100])
	}
	return strings.TrimSpace(name)
}

// ==================== 订阅功能 ====================

func (h *Handlers) ToggleCollectionSubscribe(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Subscribe       bool `json:"subscribe"`
		RefreshInterval int  `json:"refresh_interval"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效请求"})
		return
	}

	h.collectionsMu.Lock()
	col, exists := h.collections[id]
	if !exists {
		h.collectionsMu.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"detail": "合集不存在"})
		return
	}

	col.Subscribed = req.Subscribe
	if req.Subscribe {
		col.LastRefresh = time.Now()
		if req.RefreshInterval > 0 {
			col.RefreshInterval = req.RefreshInterval
		} else {
			col.RefreshInterval = 60 // 默认60分钟
		}
	}
	h.collectionsMu.Unlock()

	// 持久化
	if err := h.saveCollections(); err != nil {
		log.Printf("[WARN] 保存合集数据失败: %v", err)
	}

	log.Printf("[INFO] %s: 订阅状态变更: %s -> subscribed=%v", id, col.Title, col.Subscribed)
	h.addLog("INFO", id, fmt.Sprintf("订阅状态变更: %s -> %v", col.Title, col.Subscribed))

	c.JSON(http.StatusOK, gin.H{
		"subscribed":       col.Subscribed,
		"last_refresh":     col.LastRefresh,
		"refresh_interval": col.RefreshInterval,
	})
}

func (h *Handlers) startSubscriptionChecker() {
	ticker := time.NewTicker(1 * time.Minute) // 每分钟检查一次
	go func() {
		for range ticker.C {
			h.checkSubscriptions()
		}
	}()
	log.Println("[INFO] 订阅检查器已启动（每分钟检查一次）")
}

func (h *Handlers) checkSubscriptions() {
	now := time.Now()
	var needRefresh []string

	h.collectionsMu.RLock()
	for id, col := range h.collections {
		if col.Subscribed && col.RefreshInterval > 0 {
			nextRefresh := col.LastRefresh.Add(time.Duration(col.RefreshInterval) * time.Minute)
			if now.After(nextRefresh) {
				needRefresh = append(needRefresh, id)
			}
		}
	}
	h.collectionsMu.RUnlock()

	for _, id := range needRefresh {
		go h.refreshCollection(id)
	}
}

func (h *Handlers) refreshCollection(colID string) {
	h.collectionsMu.RLock()
	col, exists := h.collections[colID]
	if !exists {
		h.collectionsMu.RUnlock()
		return
	}
	// 复制必要信息，避免长时间持有锁
	colURL := col.URL
	colTitle := col.Title
	colQuality := col.Quality
	h.collectionsMu.RUnlock()

	log.Printf("[INFO] %s: 开始刷新合集: %s", colID, colTitle)
	h.addLog("INFO", colID, fmt.Sprintf("开始刷新合集: %s", colTitle))

	// 解析合集获取最新视频列表
	var newVideos []*CollectionVideoInfo
	platform := download.IdentifyPlatform(colURL)

	switch platform {
	case "bilibili":
		parser := bilibili.NewCollectionParser()
		if h.cfg.BilibiliCookie != "" {
			parser.SetCookies(h.cfg.BilibiliCookie)
		}
		collectionInfo, err := parser.ParseCollection(colURL)
		if err != nil {
			log.Printf("[ERROR] %s: 刷新合集失败: %v", colID, err)
			h.addLog("ERROR", colID, fmt.Sprintf("刷新合集失败: %v", err))
			return
		}
		for _, v := range collectionInfo.Videos {
			newVideos = append(newVideos, &CollectionVideoInfo{
				BVID:     v.BVID,
				URL:      v.URL,
				Title:    v.Title,
				Author:   v.Author,
				CoverURL: v.CoverURL,
				Duration: v.Duration,
				Status:   "pending",
			})
		}
	case "douyin":
		parser := douyin.NewCollectionParser()
		if h.cfg.DouyinCookie != "" {
			parser.SetCookies(h.cfg.DouyinCookie)
		}
		collectionInfo, err := parser.ParseCollection(colURL)
		if err != nil {
			log.Printf("[ERROR] %s: 刷新合集失败: %v", colID, err)
			h.addLog("ERROR", colID, fmt.Sprintf("刷新合集失败: %v", err))
			return
		}
		for _, v := range collectionInfo.Videos {
			newVideos = append(newVideos, &CollectionVideoInfo{
				VideoID:  v.VideoID,
				URL:      v.URL,
				Title:    v.Title,
				Author:   v.Author,
				CoverURL: v.CoverURL,
				Duration: v.Duration,
				Status:   "pending",
			})
		}
	default:
		log.Printf("[ERROR] %s: 不支持的平台: %s", colID, colURL)
		h.addLog("ERROR", colID, fmt.Sprintf("不支持的平台: %s", colURL))
		return
	}

	// 对比现有视频，找出新增视频
	h.collectionsMu.Lock()
	col = h.collections[colID]
	existingURLs := make(map[string]bool)
	for _, v := range col.Videos {
		url := v.URL
		if url == "" {
			if v.BVID != "" {
				url = v.BVID
			} else if v.VideoID != "" {
				url = v.VideoID
			}
		}
		existingURLs[url] = true
	}

	var addedVideos []*CollectionVideoInfo
	for _, v := range newVideos {
		url := v.URL
		if url == "" {
			if v.BVID != "" {
				url = v.BVID
			} else if v.VideoID != "" {
				url = v.VideoID
			}
		}
		if !existingURLs[url] {
			v.Page = len(col.Videos) + 1
			col.Videos = append(col.Videos, v)
			addedVideos = append(addedVideos, v)
		}
	}
	col.TotalCount = len(col.Videos)
	col.LastRefresh = time.Now()
	h.collectionsMu.Unlock()

	if len(addedVideos) == 0 {
		log.Printf("[INFO] %s: 合集无新视频: %s", colID, colTitle)
		h.addLog("INFO", colID, fmt.Sprintf("合集无新视频: %s", colTitle))
		// 持久化（更新 LastRefresh）
		h.saveCollections()
		return
	}

	log.Printf("[INFO] %s: 发现 %d 个新视频: %s", colID, len(addedVideos), colTitle)
	h.addLog("INFO", colID, fmt.Sprintf("发现 %d 个新视频: %s", len(addedVideos), colTitle))

	// 为新增视频启动下载
	for _, v := range addedVideos {
		videoURL := v.URL
		if videoURL == "" && v.VideoID != "" {
			videoURL = fmt.Sprintf("https://www.douyin.com/video/%s", v.VideoID)
		}
		if videoURL == "" && v.BVID != "" {
			videoURL = fmt.Sprintf("https://www.bilibili.com/video/%s", v.BVID)
		}
		if videoURL == "" {
			v.Status = "failed"
			v.ErrorMessage = "无有效URL"
			continue
		}

		// 启动下载
		go func(video *CollectionVideoInfo, url string) {
			h.downloadSingleCollectionVideo(colID, video.Page-1, colQuality)
		}(v, videoURL)
	}

	// 持久化
	if err := h.saveCollections(); err != nil {
		log.Printf("[WARN] 保存合集数据失败: %v", err)
	}
}

func extractFirstURL(text string) string {
	re := regexp.MustCompile(`https?://[^\s<>"']+`)
	match := re.FindString(text)
	if match != "" {
		return strings.TrimRight(match, ".,;!?")
	}
	return text
}

// ==================== 持久化 ====================

func (h *Handlers) getCollectionsFilePath() string {
	return filepath.Join(h.cfg.LogDir, "collections.json")
}

func (h *Handlers) saveCollections() error {
	h.collectionsMu.RLock()
	defer h.collectionsMu.RUnlock()

	data, err := json.MarshalIndent(h.collections, "", "  ")
	if err != nil {
		return err
	}

	filePath := h.getCollectionsFilePath()
	os.MkdirAll(filepath.Dir(filePath), 0755)
	return fsutil.WriteFileAtomic(filePath, data)
}

func (h *Handlers) loadCollections() error {
	filePath := h.getCollectionsFilePath()
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil // 文件不存在，跳过
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	h.collectionsMu.Lock()
	defer h.collectionsMu.Unlock()

	return json.Unmarshal(data, &h.collections)
}

// ==================== Auto Update ====================

// verifySHA256File 下载 "<hash>  <filename>" 格式的校验文件并比对本地文件。
func verifySHA256File(path, sumsURL string) error {
	resp, err := http.Get(sumsURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("校验和下载失败: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return fmt.Errorf("校验和文件为空")
	}
	expected := strings.ToLower(strings.TrimSpace(fields[0]))

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	digest := sha256.New()
	if _, err := io.Copy(digest, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(digest.Sum(nil))
	if actual != expected {
		return fmt.Errorf("SHA256 不匹配: 期望 %s 实际 %s", expected, actual)
	}
	return nil
}
func (h *Handlers) TriggerUpdate(c *gin.Context) {
	if runtime.GOOS != "windows" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "自动更新仅支持Windows"})
		return
	}

	// 获取最新release的setup.exe下载链接
	resp, err := http.Get("https://api.github.com/repos/Star-wsc/ccnew-vdl/releases/latest")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法获取版本信息"})
		return
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解析版本信息失败"})
		return
	}

	// 已是最新版本则不重复下载安装（dev 版本跳过比较）
	local := strings.TrimPrefix(Version, "v")
	remote := strings.TrimPrefix(release.TagName, "v")
	if Version != "dev" && local != "" && local == remote {
		c.JSON(http.StatusOK, gin.H{"message": "已是最新版本，无需更新", "version": Version})
		return
	}

	var setupURL, sumURL string
	for _, asset := range release.Assets {
		switch asset.Name {
		case "ccnew-vdl-setup.exe":
			setupURL = asset.BrowserDownloadURL
		case "ccnew-vdl-setup.exe.sha256":
			sumURL = asset.BrowserDownloadURL
		}
	}

	if setupURL == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到安装包"})
		return
	}

	// 下载安装包到临时目录
	tmpDir := os.TempDir()
	setupPath := filepath.Join(tmpDir, "ccnew-vdl-setup.exe")

	dlResp, err := http.Get(setupURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("下载安装包失败: %v", err)})
		return
	}
	defer dlResp.Body.Close()

	outFile, err := os.Create(setupPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建临时文件失败"})
		return
	}
	written, err := io.Copy(outFile, dlResp.Body)
	outFile.Close()
	if err != nil {
		os.Remove(setupPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "下载安装包失败"})
		return
	}
	if written < 1024 {
		os.Remove(setupPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "安装包文件异常"})
		return
	}

	// 优先校验完整性，防止被篡改的安装包被执行
	if sumURL == "" {
		log.Println("[WARN] 最新版本未提供SHA256校验文件，跳过完整性校验")
	} else if err := verifySHA256File(setupPath, sumURL); err != nil {
		os.Remove(setupPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "安装包校验失败，已取消更新: " + err.Error()})
		return
	}

	// 启动安装程序（静默安装模式）
	cmd := exec.Command(setupPath, "/SILENT", "/NORESTART")
	if err := cmd.Start(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("启动安装程序失败: %v", err)})
		return
	}

	// 安装程序启动后，延迟退出当前程序
	go func() {
		time.Sleep(3 * time.Second)
		os.Exit(0)
	}()

	c.JSON(http.StatusOK, gin.H{"message": "正在下载并安装更新，程序即将重启..."})
}
