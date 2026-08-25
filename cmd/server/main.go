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
var Version = "dev"

func killPortProcess(port string) {
	cmd := exec.Command("cmd", "/c", fmt.Sprintf("netstat -ano | findstr :%s | findstr LISTENING", port))
	output, _ := cmd.Output()
	if len(output) == 0 {
		return
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 5 {
			pid := fields[len(fields)-1]
			if pid != "0" {
				log.Printf("杀掉占用端口%s的进程: PID=%s", port, pid)
				exec.Command("taskkill", "/F", "/PID", pid).Run()
			}
		}
	}
}

func main() {
	// 立即隐藏控制台窗口（在任何输出之前）
	hideConsoleWindow()

	cfg := config.Load()

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
	r.Use(corsMiddleware())

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	log.Println("服务器已关闭")
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowedOrigins := []string{"http://127.0.0.1", "http://localhost", "http://0.0.0.0"}
		allowed := false
		for _, ao := range allowedOrigins {
			if strings.HasPrefix(origin, ao) {
				allowed = true
				break
			}
		}
		if origin == "" {
			allowed = true
		}
		if allowed {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		}
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}