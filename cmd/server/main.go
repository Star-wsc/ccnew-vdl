package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Star-wsc/ccnew-vdl/internal/config"
	"github.com/Star-wsc/ccnew-vdl/internal/download"
	"github.com/gin-gonic/gin"
)

// Version is set at build time via ldflags
var Version = "1.3.5"

// killPortProcess 仅强杀同名的自身旧实例，避免误伤占用端口的其他程序。
func killPortProcess(port string) {
	cmd := exec.Command("cmd", "/c", fmt.Sprintf("netstat -ano | findstr :%s | findstr LISTENING", port))
	output, _ := cmd.Output()
	if len(output) == 0 {
		return
	}

	selfName := ""
	if exe, err := os.Executable(); err == nil {
		selfName = strings.ToLower(filepath.Base(exe))
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 5 {
			continue
		}
		pid := fields[len(fields)-1]
		if pid == "0" {
			continue
		}

		imgOut, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %s", pid), "/FO", "CSV", "/NH").Output()
		if err != nil || len(imgOut) == 0 {
			continue
		}
		cols := strings.Split(strings.TrimSpace(string(imgOut)), ",")
		if len(cols) < 1 {
			continue
		}
		name := strings.Trim(strings.ToLower(cols[0]), "\"")
		if selfName == "" || !strings.EqualFold(name, selfName) {
			log.Printf("端口 %s 被 %s(PID=%s) 占用且非本程序实例，跳过强杀", port, name, pid)
			continue
		}
		log.Printf("杀掉占用端口%s的旧实例: PID=%s", port, pid)
		exec.Command("taskkill", "/F", "/PID", pid).Run()
	}
}

// cleanupStaleTempFiles 清理上次运行残留的临时分片（崩溃/断电遗留）。
func cleanupStaleTempFiles() {
	patterns := []string{
		"temp_video_*.m4s",
		"temp_audio_*.m4s",
		"temp_video_*.m4s.clean.mp4",
		"temp_audio_*.m4s.clean.m4a",
	}
	removed := 0
	for _, p := range patterns {
		matches, _ := filepath.Glob(p)
		for _, f := range matches {
			if os.Remove(f) == nil {
				removed++
			}
		}
	}
	if removed > 0 {
		log.Printf("清理了 %d 个上次运行残留的临时文件", removed)
	}
}

func main() {
	// 立即隐藏控制台窗口（在任何输出之前）
	hideConsoleWindow()

	cleanupStaleTempFiles()

	cfg := config.Load()

	// 初始化文件日志
	os.MkdirAll(cfg.LogDir, 0755)
	if logFile, logErr := os.OpenFile(filepath.Join(cfg.LogDir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); logErr == nil {
		log.SetOutput(logFile)
		log.Printf("[启动] 日志文件: %s", logFile.Name())
		defer logFile.Close()
	}

	os.MkdirAll(cfg.DownloadDir, 0755)
	os.MkdirAll(cfg.LogDir, 0755)

	mgr := download.NewManager(cfg)
	h := NewHandlers(cfg, mgr)

	// 加载持久化的合集数据
	if err := h.loadCollections(); err != nil {
		log.Printf("[WARN] 加载合集数据失败: %v", err)
	}

	// 启动订阅检查器
	h.startSubscriptionChecker()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(corsMiddleware(cfg.Port))

	// 所有 API 路由
	r.GET("/", h.Index)
	r.GET("/api/config", h.GetConfig)
	r.POST("/api/browse-folder", h.BrowseFolder)
	r.POST("/api/download-dir", h.SetDownloadDir)
	r.GET("/api/download-dir", h.GetDownloadDir)
	r.POST("/api/bilibili/cookie", h.SetBilibiliCookie)
	r.GET("/api/bilibili/cookie", h.GetBilibiliCookie)
	r.POST("/api/bilibili/login", h.LoginBilibili)
	r.GET("/api/bilibili/login/poll", h.PollBilibiliLogin)
	r.GET("/api/proxy/image", h.ProxyImage)
	r.GET("/api/proxy/file", h.ProxyFileDownload)
	r.POST("/api/tasks", h.CreateTask)
	r.POST("/api/tasks/create-from-preview", h.CreateTaskFromPreview)
	r.GET("/api/tasks", h.GetTasks)
	r.GET("/api/tasks/:task_id", h.GetTask)
	r.DELETE("/api/tasks/:task_id", h.DeleteTask)
	r.POST("/api/tasks/:task_id/retry", h.RetryTask)
	r.GET("/api/tasks/:task_id/download", h.DownloadTaskFile)
	r.GET("/api/tasks/:task_id/proxy-download", h.ProxyDownload)
	r.GET("/api/stats", h.GetStats)
	r.GET("/api/logs", h.GetLogs)
	r.DELETE("/api/logs", h.ClearLogs)
	r.POST("/api/settings", h.SaveSettings)
	r.GET("/api/console/visible", h.GetConsoleVisible)
	r.POST("/api/update", h.TriggerUpdate)
	r.POST("/api/console/toggle", h.ToggleConsoleWindow)

	r.POST("/api/collections/preview", h.PreviewCollection)
	r.POST("/api/collections", h.CreateCollection)
	r.POST("/api/collections/:id/download", h.DownloadCollection)
	r.GET("/api/collections", h.GetCollections)
	r.GET("/api/collections/:id", h.GetCollection)
	r.GET("/api/collections/:id/videos", h.GetCollectionVideos)
	r.DELETE("/api/collections/:id", h.DeleteCollection)
	r.DELETE("/api/collections/videos/:id", h.DeleteCollectionVideo)
	r.POST("/api/collections/:id/videos/:idx/download", h.DownloadCollectionVideo)
	r.DELETE("/api/collections/:id/videos/:idx/file", h.DeleteCollectionVideoFile)
	r.POST("/api/collections/:id/subscribe", h.ToggleCollectionSubscribe)

	// 获取可执行文件所在目录
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	r.Static("/static", filepath.Join(exeDir, "static"))

	addr := fmt.Sprintf(":%s", cfg.Port)
	killPortProcess(cfg.Port)

	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		log.Printf("服务器启动: http://127.0.0.1%s", addr)
		log.Printf("下载目录: %s", cfg.DownloadDir)
		for i := 0; i < 5; i++ {
			if err := srv.ListenAndServe(); err != nil {
				if err == http.ErrServerClosed {
					return
				}
				log.Printf("端口绑定失败，%d秒后重试... (%v)", i+1, err)
				time.Sleep(time.Duration(i+1) * time.Second)
				killPortProcess(cfg.Port)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			return
		}
		log.Fatalf("服务器启动失败: 端口 %s 无法绑定", cfg.Port)
	}()

	// 等待服务器启动后打开桌面窗口
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	time.Sleep(1 * time.Second)
	launchDesktopWindow(cfg.Port, quit)

	<-quit

	log.Println("正在关闭服务器...")
	killChildProcess() // 清理前端窗口和WebView2
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	log.Println("服务器已关闭")
}

// corsMiddleware 仅对本机来源（含端口）放行 CORS，拒绝前缀伪装域名。
func corsMiddleware(port string) gin.HandlerFunc {
	allowed := map[string]bool{
		"http://127.0.0.1:" + port: true,
		"http://localhost:" + port: true,
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
