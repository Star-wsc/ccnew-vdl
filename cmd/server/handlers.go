package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Star-wsc/ccnew-vdl/internal/bilibili"
	"github.com/Star-wsc/ccnew-vdl/internal/config"
	"github.com/Star-wsc/ccnew-vdl/internal/download"
	"github.com/Star-wsc/ccnew-vdl/internal/douyin"
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
	BVID     string `json:"bvid,omitempty"`
	VideoID  string `json:"video_id"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Author   string `json:"author"`
	CoverURL string `json:"cover_url"`
	Duration int    `json:"duration"`
	Page     int    `json:"page"`
}

type CollectionInfo struct {
	ID         string               `json:"id"`
	URL        string               `json:"url"`
	Title      string               `json:"title"`
	Author     string               `json:"author"`
	CoverURL   string               `json:"cover_url"`
	TotalCount int                  `json:"total_count"`
	Videos     []*CollectionVideoInfo `json:"videos"`
	Status     string               `json:"status"`
	Quality    string               `json:"quality"`
	CreatedAt  time.Time            `json:"created_at"`
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
	h.logsMu.Lock()
	defer h.logsMu.Unlock()
	h.logs = append(h.logs, LogEntry{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Level:     level,
		TaskID:    taskID,
		Message:   message,
	})
	if len(h.logs) > 1000 {
		h.logs = h.logs[len(h.logs)-1000:]
	}
}

// ==================== Index ====================

func (h *Handlers) Index(c *gin.Context) {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	c.File(filepath.Join(exeDir, "static", "index.html"))
}

// ==================== Config ====================

func (h *Handlers) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"download_dir": h.cfg.DownloadDir,
		"version":      "v1.0.0",
		"first_run":    false,
		"total_tasks":  len(h.mgr.GetAllTasks()),
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
	script := `
	Add-Type -AssemblyName System.Windows.Forms
	$dialog = New-Object System.Windows.Forms.FolderBrowserDialog
	$dialog.Description = "选择下载目录"
	$dialog.ShowNewFolderButton = $true
	if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
		Write-Output $dialog.SelectedPath
	} else {
		Write-Output "CANCELLED"
	}`
	cmd := exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass", "-Command", script)
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
	c.JSON(http.StatusOK, gin.H{"message": "B站Cookie已保存"})
}

func (h *Handlers) LoginBilibili(c *gin.Context) {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	cookiePath := filepath.Join(os.TempDir(), "bilibili_cookies.txt")
	os.Remove(cookiePath)

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

func (h *Handlers) GetDouyinCookie(c *gin.Context) {
	cookie := h.cfg.DouyinCookie
	masked := ""
	if len(cookie) > 20 {
		masked = cookie[:10] + "..." + cookie[len(cookie)-10:]
	} else if len(cookie) > 0 {
		masked = cookie[:5] + "..."
	}
	c.JSON(http.StatusOK, gin.H{"cookie_masked": masked, "has_cookie": cookie != ""})
}

func (h *Handlers) ClearDouyinCookie(c *gin.Context) {
	var req struct {
		Cookie string `json:"cookie"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效请求"})
		return
	}
	h.cfg.DouyinCookie = req.Cookie
	h.cfg.Save()
	h.douyinParser.SetCookies(req.Cookie)
	c.JSON(http.StatusOK, gin.H{"message": "抖音Cookie已保存"})
}

func (h *Handlers) LoginDouyin(c *gin.Context) {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	cookiePath := filepath.Join(os.TempDir(), "douyin_cookies.txt")
	os.Remove(cookiePath)

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
public class DouyinLoginWin{
    public static Window Create(string url,string cp){
        var w=new Window();w.Title="抖音登录";w.Width=500;w.Height=700;w.WindowStartupLocation=WindowStartupLocation.CenterScreen;
        var v=new WebView2();w.Content=v;
        w.Loaded+=async(s,e)=>{
            try{
                var env=await Microsoft.Web.WebView2.Core.CoreWebView2Environment.CreateAsync(null,System.IO.Path.Combine(System.IO.Path.GetTempPath(),"vd-wv2-douyin"));
                await v.EnsureCoreWebView2Async(env);
                v.CoreWebView2.Navigate(url);
                System.Windows.Threading.DispatcherTimer timer=null;
                timer=new System.Windows.Threading.DispatcherTimer();
                timer.Interval=TimeSpan.FromSeconds(2);
                timer.Tick+=async(sender,args)=>{
                    try{
                        var cookies=await v.CoreWebView2.CookieManager.GetCookiesAsync(url);
                        bool found=false;
                        foreach(var c in cookies){if(c.Name=="sessionid"){found=true;break;}}
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
$win=[DouyinLoginWin]::Create('https://www.douyin.com',$cookiePath)
$app=New-Object System.Windows.Application
$app.Run($win)`, wv2Core, wv2Wpf, cookiePathEsc)

	cmd := exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[ERROR] LoginDouyin PS error: %v, output: %s\n", err, string(output))
	}

	cookieBytes, readErr := os.ReadFile(cookiePath)
	if readErr != nil || len(cookieBytes) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "未获取到Cookie，可能未登录或超时"})
		return
	}

	cookieStr := string(cookieBytes)
	h.cfg.DouyinCookie = cookieStr
	h.cfg.Save()
	h.douyinParser.SetCookies(cookieStr)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "登录成功", "cookie_length": len(cookieStr)})
}

// ==================== Proxy ====================

func (h *Handlers) ProxyImage(c *gin.Context) {
	imageURL := c.Query("url")
	if imageURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "缺少图片URL"})
		return
	}

	client := &http.Client{Timeout: 15 * time.Second}
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

	client := &http.Client{Timeout: 600 * time.Second}
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
	tasks := h.mgr.GetAllTasks()
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
	c.JSON(http.StatusOK, gin.H{
		"total":       total,
		"pending":     pending,
		"parsing":     parsing,
		"downloading": downloading,
		"completed":   completed,
		"failed":      failed,
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

	// 预览模式：解析视频信息并返回，不创建下载任务
	if req.Quality == "preview" {
		videoInfo, err := h.mgr.ParseVideo(extractedURL, "4k")
		if err != nil {
			// 单视频解析失败，尝试合集解析
			colInfo, colErr := h.douyinCollection.ParseCollection(extractedURL)
			if colErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("解析失败: %v", err)})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"type":     "collection",
				"title":    colInfo.Title,
				"author":   colInfo.Author,
				"count":    colInfo.TotalCount,
				"videos":   colInfo.Videos,
			})
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

	task := h.mgr.CreateTask(extractedURL, req.Quality)
	go h.mgr.ExecuteTask(context.Background(), task.ID)

	h.addLog("INFO", task.ID, "任务创建: "+extractedURL)

	c.JSON(http.StatusOK, task)
}

func (h *Handlers) CreateTaskFromPreview(c *gin.Context) {
	var req struct {
		URL         string                 `json:"url"`
		Quality     string                 `json:"quality"`
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

	existing := h.mgr.FindTaskByURL(req.URL)
	if existing != nil {
		c.JSON(http.StatusOK, existing)
		return
	}

	task := h.mgr.CreateTask(req.URL, req.Quality)

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
		if platform, ok := req.PreviewData["platform"].(string); ok && platform != "" {
			task.Platform = platform
		}
	}

	go h.mgr.ExecuteTask(context.Background(), task.ID)

	c.JSON(http.StatusOK, task)
}

func (h *Handlers) GetTasks(c *gin.Context) {
	statusFilter := c.Query("status")
	tasks := h.mgr.GetAllTasks()

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
	task.Status = StatusPending
	task.ErrorMessage = ""
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

	// 根据URL判断平台
	isDouyin := strings.Contains(req.URL, "douyin.com") || strings.Contains(req.URL, "iesdouyin.com") || strings.Contains(req.URL, "v.douyin.com")
	isBilibili := strings.Contains(req.URL, "bilibili.com") || strings.Contains(req.URL, "b23.tv")

	if isDouyin {
		info, err := h.douyinCollection.ParseCollection(req.URL)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": "解析抖音合集失败: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"type":       "collection",
			"platform":   "douyin",
			"title":      info.Title,
			"author":     info.Author,
			"count":      info.TotalCount,
			"total_count": info.TotalCount,
			"videos":     info.Videos,
		})
	} else if isBilibili {
		info, err := h.bilibiliCollection.ParseCollection(req.URL)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": "解析B站合集失败: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"type":       "collection",
			"platform":   "bilibili",
			"title":      info.Title,
			"author":     info.Author,
			"count":      info.TotalCount,
			"total_count": info.TotalCount,
			"videos":     info.Videos,
		})
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "不支持的链接格式"})
	}
}

func (h *Handlers) CreateCollection(c *gin.Context) {
	var req struct {
		URL           string                   `json:"url"`
		Title         string                   `json:"title"`
		Videos        []map[string]interface{} `json:"videos"`
		Quality       string                   `json:"quality"`
		AutoDownload  bool                     `json:"auto_download"`
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
		})
	}

	col := &CollectionInfo{
		ID:         colID,
		URL:        req.URL,
		Title:      req.Title,
		TotalCount: len(videos),
		Videos:     videos,
		Status:     "pending",
		Quality:    req.Quality,
		CreatedAt:  time.Now(),
	}

	h.collectionsMu.Lock()
	h.collections[colID] = col
	h.collectionsMu.Unlock()

	if req.AutoDownload {
		go h.downloadCollectionVideos(colID, req.Quality)
	}

	c.JSON(http.StatusOK, col)
}

func (h *Handlers) GetCollections(c *gin.Context) {
	h.collectionsMu.RLock()
	defer h.collectionsMu.RUnlock()

	cols := make([]*CollectionInfo, 0, len(h.collections))
	for _, col := range h.collections {
		cols = append(cols, col)
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
	h.collectionsMu.Lock()
	if _, exists := h.collections[id]; !exists {
		h.collectionsMu.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"detail": "合集不存在"})
		return
	}
	delete(h.collections, id)
	h.collectionsMu.Unlock()
	c.JSON(http.StatusOK, gin.H{"message": "合集已删除"})
}

func (h *Handlers) DeleteCollectionVideo(c *gin.Context) {
	videoID := c.Param("id")
	h.collectionsMu.Lock()
	defer h.collectionsMu.Unlock()

	for _, col := range h.collections {
		for i, v := range col.Videos {
			if v.VideoID == videoID {
				col.Videos = append(col.Videos[:i], col.Videos[i+1:]...)
				col.TotalCount = len(col.Videos)
				c.JSON(http.StatusOK, gin.H{"message": "视频已删除"})
				return
			}
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"detail": "视频不存在"})
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
	h.collectionsMu.RLock()
	col, exists := h.collections[colID]
	h.collectionsMu.RUnlock()
	if !exists {
		return
	}

	col.Status = "downloading"
	taskCount := 0
	for _, v := range col.Videos {
		videoURL := v.URL
		if videoURL == "" && v.VideoID != "" {
			videoURL = fmt.Sprintf("https://www.douyin.com/video/%s", v.VideoID)
		}
		if videoURL == "" && v.BVID != "" {
			videoURL = fmt.Sprintf("https://www.bilibili.com/video/%s", v.BVID)
		}
		if videoURL == "" {
			continue
		}

		// 检查是否已有相同URL的任务
		existing := h.mgr.FindTaskByURL(videoURL)
		if existing != nil {
			if existing.Status == StatusFailed {
				existing.Status = StatusPending
				existing.ErrorMessage = ""
				go h.mgr.ExecuteTask(context.Background(), existing.ID)
			}
			continue
		}

		task := h.mgr.CreateTask(videoURL, quality)
		go h.mgr.ExecuteTask(context.Background(), task.ID)
		taskCount++
	}

	h.addLog("INFO", colID, fmt.Sprintf("合集下载已启动: %d个视频", taskCount))
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
	}
	if req.DouyinCookie != "" {
		h.cfg.DouyinCookie = req.DouyinCookie
		h.douyinParser.SetCookies(req.DouyinCookie)
	}
	h.cfg.Save()
	c.JSON(http.StatusOK, gin.H{"message": "设置已保存"})
}

// ==================== Helpers ====================

func extractFirstURL(text string) string {
	re := regexp.MustCompile(`https?://[^\s<>"']+`)
	match := re.FindString(text)
	if match != "" {
		return strings.TrimRight(match, ".,;!?")
	}
	return text
}
