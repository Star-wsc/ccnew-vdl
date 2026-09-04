package download

import (
	"context"
	"encoding/json"
	"fmt"
	"crypto/rand"
	"log"
	"os/exec"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Star-wsc/ccnew-vdl/internal/bilibili"
	"github.com/Star-wsc/ccnew-vdl/internal/config"
	"github.com/Star-wsc/ccnew-vdl/internal/douyin"
)

type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusParsing    TaskStatus = "parsing"
	StatusDownloading TaskStatus = "downloading"
	StatusCompleted  TaskStatus = "completed"
	StatusFailed     TaskStatus = "failed"
)

type Task struct {
	ID           string     `json:"id"`
	URL          string     `json:"url"`
	Title        string     `json:"title"`
	Author       string     `json:"author"`
	CoverURL     string     `json:"cover_url"`
	VideoURL     string     `json:"video_url,omitempty"`
	AudioURL     string     `json:"audio_url,omitempty"`
	Quality      string     `json:"quality"`
	Platform     string     `json:"platform"`
	Status       TaskStatus `json:"status"`
	Progress     int        `json:"progress"`
	Speed        int64      `json:"speed"`
	FilePath     string     `json:"file_path"`
	FileSize     int64      `json:"file_size"`
	ErrorMessage string     `json:"error_message"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type Manager struct {
	cfg              *config.Config
	bilibiliParser   *bilibili.Parser
	bilibiliDownloader *bilibili.Downloader
	douyinDownloader *douyin.DouyinDownloader
	douyinCollection *douyin.CollectionParser
	tasks            map[string]*Task
	tasksFile        string
	mu               sync.RWMutex
}

func NewManager(cfg *config.Config) *Manager {
	// 任务持久化文件路径
	tasksFile := filepath.Join(cfg.LogDir, "tasks.json")

	m := &Manager{
		cfg:                cfg,
		bilibiliParser:     bilibili.NewParser(),
		bilibiliDownloader: bilibili.NewDownloader(),
		douyinDownloader:   douyin.NewDouyinDownloader(cfg.Proxy),
		douyinCollection:   douyin.NewCollectionParser(),
		tasks:              make(map[string]*Task),
		tasksFile:          tasksFile,
	}

	if cfg.BilibiliCookie != "" {
		m.bilibiliParser.SetCookies(cfg.BilibiliCookie)
		m.bilibiliDownloader.SetCookies(cfg.BilibiliCookie)
	}
	if cfg.DouyinCookie != "" {
		m.douyinDownloader.SetCookies(cfg.DouyinCookie)
		m.douyinCollection.SetCookies(cfg.DouyinCookie)
	}

	os.MkdirAll(cfg.DownloadDir, 0755)
	os.MkdirAll(cfg.LogDir, 0755)

	// 加载持久化的任务
	m.loadTasks()

	return m
}

// loadTasks 从文件加载持久化的任务
func (m *Manager) loadTasks() {
	data, err := os.ReadFile(m.tasksFile)
	if err != nil {
		return // 文件不存在或读取失败，忽略
	}

	var tasks []*Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return // 解析失败，忽略
	}

	for _, task := range tasks {
		// 重置进行中的任务状态为失败（因为服务器重启了）
		if task.Status == StatusParsing || task.Status == StatusDownloading {
			task.Status = StatusFailed
			task.ErrorMessage = "服务器重启，任务中断"
		}
		m.tasks[task.ID] = task
	}
}

// saveTasks 保存任务到文件
func (m *Manager) saveTasks() {
	m.mu.RLock()
	tasks := make([]Task, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, *task) // 复制值，不是指针
	}
	m.mu.RUnlock()

	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return
	}

	os.MkdirAll(filepath.Dir(m.tasksFile), 0755)
	os.WriteFile(m.tasksFile, data, 0644)
}

// CreateTask 创建下载任务
func (m *Manager) CreateTask(url, quality string) *Task {
	task := &Task{
		ID:        generateID(),
		URL:       url,
		Quality:   quality,
		Status:    StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	m.mu.Lock()
	m.tasks[task.ID] = task
	m.mu.Unlock()

	m.saveTasks()
	return task
}

// ExecuteTask 执行下载任务
func (m *Manager) ExecuteTask(ctx context.Context, taskID string) {
	task := m.GetTask(taskID)
	if task == nil {
		return
	}

	// 如果已有预览数据（来自 create-from-preview），跳过解析直接下载
	if task.VideoURL != "" {
		videoInfo := &videoInfo{
			Title:    task.Title,
			Author:   task.Author,
			CoverURL: task.CoverURL,
			VideoURL: task.VideoURL,
			AudioURL: task.AudioURL,
			Platform: task.Platform,
			Quality:  task.Quality,
		}
		// 如果预览数据没有音频URL，尝试重新解析获取
		if videoInfo.AudioURL == "" {
			if parsed, parseErr := m.parseVideo(task.URL, task.Quality); parseErr == nil && parsed != nil && parsed.AudioURL != "" {
				videoInfo.AudioURL = parsed.AudioURL
			}
		}
		m.updateTaskStatus(taskID, StatusDownloading, "")
		m.saveTasks()
		outputPath := m.generateOutputPath(task.Title, task.Platform)
		var lastDL int64
		var lastTS = time.Now()
		err := m.downloadVideo(videoInfo, outputPath, func(downloaded, total int64) {
			m.mu.Lock()
			if total > 0 {
				task.Progress = int(downloaded * 100 / total)
			} else if downloaded > 0 {
				task.Progress = int(downloaded / (1024 * 1024))
				if task.Progress > 99 { task.Progress = 99 }
			}
			now := time.Now()
			dt := now.Sub(lastTS).Seconds()
			if dt >= 0.5 {
				task.Speed = int64(float64(downloaded - lastDL) / dt)
				lastDL = downloaded
				lastTS = now
			}
			task.UpdatedAt = now
			m.mu.Unlock()
		})
		if err != nil {
			m.updateTaskStatus(taskID, StatusFailed, err.Error())
			m.saveTasks()
			return
		}
		fileInfo, _ := os.Stat(outputPath)
		m.mu.Lock()
		task.Status = StatusCompleted
		task.FilePath = outputPath
		if fileInfo != nil { task.FileSize = fileInfo.Size() }
		task.Progress = 100
		task.Speed = 0
		task.UpdatedAt = time.Now()
		m.mu.Unlock()
		m.saveTasks()
		return
	}

	m.updateTaskStatus(taskID, StatusParsing, "")
	m.saveTasks()

	// 解析视频（最多重试5次，间隔3秒）
	var videoInfo *videoInfo
	var err error
	for attempt := 1; attempt <= 5; attempt++ {
		videoInfo, err = m.parseVideo(task.URL, task.Quality)
		if err == nil {
			break
		}
		if attempt < 5 {
			log.Printf("[重试] 解析失败(第%d次): %v，3秒后重试...", attempt, err)
			time.Sleep(3 * time.Second)
		}
	}
	if err != nil {
		m.updateTaskStatus(taskID, StatusFailed, fmt.Sprintf("解析失败(重试5次): %v", err))
		m.saveTasks()
		return
	}

	// 更新任务信息
	m.mu.Lock()
	task.Title = videoInfo.Title
	task.Author = videoInfo.Author
	task.CoverURL = videoInfo.CoverURL
	task.VideoURL = videoInfo.VideoURL
	task.Platform = videoInfo.Platform
	task.Quality = videoInfo.Quality // 使用实际下载的画质
	task.UpdatedAt = time.Now()
	m.mu.Unlock()

	// 下载视频
	m.updateTaskStatus(taskID, StatusDownloading, "")
	m.saveTasks()

	outputPath := m.generateOutputPath(task.Title, task.Platform)
	var lastDL2 int64
	var lastTS2 = time.Now()
	// 下载视频（最多重试5次，间隔3秒）
	for attempt := 1; attempt <= 5; attempt++ {
		lastDL2 = 0
		lastTS2 = time.Now()
		err = m.downloadVideo(videoInfo, outputPath, func(downloaded, total int64) {
			m.mu.Lock()
			if total > 0 {
				task.Progress = int(downloaded * 100 / total)
			} else if downloaded > 0 {
				task.Progress = int(downloaded / (1024 * 1024))
				if task.Progress > 99 {
					task.Progress = 99
				}
			}
			now := time.Now()
			dt := now.Sub(lastTS2).Seconds()
			if dt >= 0.5 {
				task.Speed = int64(float64(downloaded - lastDL2) / dt)
				lastDL2 = downloaded
				lastTS2 = now
			}
			task.UpdatedAt = now
			m.mu.Unlock()
		})
		if err == nil {
			break
		}
		// 清理失败的临时文件
		os.Remove(outputPath)
		os.Remove(outputPath + ".video.tmp")
		os.Remove(outputPath + ".audio.tmp")
		if attempt < 5 {
			log.Printf("[重试] 下载失败(第%d次): %v，3秒后重试...", attempt, err)
			time.Sleep(3 * time.Second)
		}
	}

	if err != nil {
		m.updateTaskStatus(taskID, StatusFailed, fmt.Sprintf("下载失败(重试5次): %v", err))
		m.saveTasks()
		return
	}

	// 获取文件大小
	fileInfo, _ := os.Stat(outputPath)
	m.mu.Lock()
	task.Status = StatusCompleted
	task.FilePath = outputPath
	if fileInfo != nil {
		task.FileSize = fileInfo.Size()
	}
	task.Progress = 100
	task.Speed = 0
	task.UpdatedAt = time.Now()
	m.mu.Unlock()

	m.saveTasks()
}

// GetTask 获取任务
func (m *Manager) GetTask(taskID string) *Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[taskID]
}

// GetAllTasks 获取所有任务（按创建时间倒序排列，最新的在前面）
func (m *Manager) GetAllTasks() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]*Task, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, task)
	}

	// 按创建时间倒序排列（最新的在前面）
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
	})

	return tasks
}

// DeleteTask 删除任务
func (m *Manager) DeleteTask(taskID string, deleteFile bool) bool {
	m.mu.Lock()
	task, exists := m.tasks[taskID]
	if !exists {
		m.mu.Unlock()
		return false
	}

	if deleteFile && task.FilePath != "" {
		os.Remove(task.FilePath)
	}

	delete(m.tasks, taskID)
	m.mu.Unlock()

	m.saveTasks()
	return true
}

// FindTaskByURL 根据URL查找任务
func (m *Manager) FindTaskByURL(url string) *Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, task := range m.tasks {
		if task.URL == url {
			return task
		}
	}
	return nil
}

// ParseVideo 解析视频信息，用于预览。返回前端期望的格式。
func (m *Manager) ParseVideo(url, quality string) (map[string]interface{}, error) {
	videoInfo, err := m.parseVideo(url, quality)
	if err != nil {
		return nil, err
	}

	platform := identifyPlatform(url)
	q := quality
	if q == "" || q == "best" || q == "preview" {
		if platform == "bilibili" {
			q = "4k"
		} else {
			q = "1080p"
		}
	}

	result := map[string]interface{}{
		"title":              videoInfo.Title,
		"author":             videoInfo.Author,
		"cover_url":          videoInfo.CoverURL,
		"video_url":          videoInfo.VideoURL,
		"platform":           platform,
		"quality":            q,
		"available_qualities": []string{"4k", "2k", "1080p", "720p", "480p"},
	}
	if videoInfo.AudioURL != "" {
		result["audio_url"] = videoInfo.AudioURL
	}
	return result, nil
}

// ParseLink 解析链接，返回单视频或合集信息
func (m *Manager) ParseLink(url string) (interface{}, error) {
	platform := identifyPlatform(url)

	switch platform {
	case "bilibili":
		info, err := m.bilibiliParser.Parse(url, "4k")
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"type":     "video",
			"platform": "bilibili",
			"title":    info.Title,
			"author":   info.Author,
			"cover":    info.CoverURL,
			"quality":  info.Quality,
		}, nil

	case "douyin":
		// 先尝试单视频解析
		info, err := m.douyinDownloader.Parse(url)
		if err == nil && info.VideoURL != "" {
			return map[string]interface{}{
				"type":     "video",
				"platform": "douyin",
				"title":    info.Title,
				"author":   info.Author,
				"cover":    info.CoverURL,
			}, nil
		}

		// 如果是明确的合集URL，尝试合集解析
		if douyin.IsDouyinCollectionURL(url) {
			colInfo, err := m.douyinCollection.ParseCollection(url)
			if err != nil {
				return nil, fmt.Errorf("合集解析失败: %w", err)
			}

			videos := make([]map[string]interface{}, 0, len(colInfo.Videos))
			for _, v := range colInfo.Videos {
				videos = append(videos, map[string]interface{}{
					"video_id": v.VideoID,
					"url":      v.URL,
					"title":    v.Title,
					"author":   v.Author,
				})
			}

			return map[string]interface{}{
				"type":     "collection",
				"platform": "douyin",
				"title":    colInfo.Title,
				"count":    colInfo.TotalCount,
				"videos":   videos,
			}, nil
		}

		// 单视频解析失败
		return nil, fmt.Errorf("单视频解析失败: %w", err)

	default:
		return nil, fmt.Errorf("不支持的平台")
	}
}

func (m *Manager) parseVideo(url, quality string) (*videoInfo, error) {
	platform := identifyPlatform(url)

	switch platform {
	case "bilibili":
		info, err := m.bilibiliParser.Parse(url, quality)
		if err != nil {
			return nil, err
		}
		actualQuality := info.Quality
		if actualQuality == "" {
			actualQuality = quality
		}
		return &videoInfo{
			Title:    info.Title,
			Author:   info.Author,
			CoverURL: info.CoverURL,
			VideoURL: info.VideoURL,
			AudioURL: info.AudioURL,
			Platform: "bilibili",
			Quality:  actualQuality,
		}, nil

	case "douyin":
		info, err := m.douyinDownloader.Parse(url)
		if err != nil {
			return nil, err
		}
		actualQuality := info.SelectedQuality
		if actualQuality == "" {
			actualQuality = "1080p"
		}
		return &videoInfo{
			Title:    info.Title,
			Author:   info.Author,
			CoverURL: info.CoverURL,
			VideoURL: info.VideoURL,
			AudioURL: info.AudioURL,
			Platform: "douyin",
			Quality:  actualQuality,
		}, nil

	default:
		return nil, fmt.Errorf("不支持的平台")
	}
}

type videoInfo struct {
	Title    string
	Author   string
	CoverURL string
	VideoURL string
	AudioURL string
	Platform string
	Quality  string
}

func (m *Manager) downloadVideo(info *videoInfo, outputPath string, progressFunc func(int64, int64)) error {
	switch info.Platform {
	case "bilibili":
		if info.AudioURL != "" {
			return m.bilibiliDownloader.DownloadWithMerge(info.VideoURL, info.AudioURL, outputPath, progressFunc)
		}
		return m.bilibiliDownloader.Download(info.VideoURL, outputPath, progressFunc)

	case "douyin":
		cookies := make(map[string]string)
		if m.cfg.DouyinCookie != "" {
			for _, part := range strings.Split(m.cfg.DouyinCookie, ";") {
				part = strings.TrimSpace(part)
				if kv := strings.SplitN(part, "=", 2); len(kv) == 2 {
					cookies[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
				}
			}
		}
		if info.AudioURL != "" {
			log.Printf("[抖音] DASH音视频分离，下载并合并...")
			videoTemp := outputPath + ".video.tmp"
			audioTemp := outputPath + ".audio.tmp"
			defer os.Remove(videoTemp)
			defer os.Remove(audioTemp)
			err := m.douyinDownloader.DownloadVideo(info.VideoURL, videoTemp, cookies, progressFunc)
			if err != nil { return fmt.Errorf("下载视频流失败: %w", err) }
			err = m.douyinDownloader.DownloadVideo(info.AudioURL, audioTemp, cookies, progressFunc)
			if err != nil { return fmt.Errorf("下载音频流失败: %w", err) }
			if err := mergeAudioVideo(videoTemp, audioTemp, outputPath); err != nil {
				return fmt.Errorf("合并音视频失败: %w", err)
			}
			log.Printf("[抖音] 音视频合并成功")
			return nil
		}
		return m.douyinDownloader.DownloadVideo(info.VideoURL, outputPath, cookies, progressFunc)

	default:
		return fmt.Errorf("不支持的平台")
	}
}


func hasAudioTrack(filePath string) bool {
	cmd := exec.Command("ffprobe", "-v", "quiet", "-select_streams", "a", "-show_entries", "stream=codec_type", "-of", "csv=p=0", filePath)
	out, err := cmd.Output()
	if err != nil { return true }
	return strings.TrimSpace(string(out)) != ""
}

func mergeAudioVideo(videoPath, audioPath, outputPath string) error {
	ffmpegPath, err := findFFmpegForDownload()
	if err != nil {
		return fmt.Errorf("FFmpeg未找到: %w", err)
	}
	return exec.Command(ffmpegPath, "-i", videoPath, "-i", audioPath, "-c:v", "copy", "-c:a", "copy", "-y", outputPath).Run()
}

// findFFmpegForDownload 在 download 包内查找 FFmpeg，逻辑与 bilibili.findFFmpeg 一致。
func findFFmpegForDownload() (string, error) {
	// 1. PATH
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path, nil
	}
	// 2. 可执行文件同级目录
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		candidates := getFFmpegCandidatesForDownload()
		for _, name := range candidates {
			path := filepath.Join(exeDir, name)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
		ffmpegDir := filepath.Join(exeDir, "ffmpeg")
		for _, name := range candidates {
			path := filepath.Join(ffmpegDir, name)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}
	// 3. 当前目录
	candidates := getFFmpegCandidatesForDownload()
	for _, name := range candidates {
		if _, err := os.Stat(name); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("未找到FFmpeg")
}

func getFFmpegCandidatesForDownload() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"ffmpeg-windows-amd64.exe", "ffmpeg.exe"}
	case "linux":
		if runtime.GOARCH == "arm64" {
			return []string{"ffmpeg-linux-arm64", "ffmpeg"}
		}
		return []string{"ffmpeg-linux-amd64", "ffmpeg"}
	case "android":
		return []string{"libffmpeg.so", "ffmpeg"}
	default:
		return []string{"ffmpeg"}
	}
}

func (m *Manager) generateOutputPath(title, platform string) string {
	// 清理文件名
	safeTitle := sanitizeFilename(title)
	if safeTitle == "" {
		safeTitle = fmt.Sprintf("video_%d", time.Now().Unix())
	}

	// 添加平台前缀
	prefix := map[string]string{
		"bilibili": "bz_",
		"douyin":   "dy_",
	}[platform]
	if prefix == "" {
		prefix = "vd_"
	}

	// 限制文件名长度（避免路径过长）
	runes := []rune(safeTitle)
	if len(runes) > 80 {
		safeTitle = string(runes[:80])
	}

	return filepath.Join(m.cfg.DownloadDir, prefix+safeTitle+".mp4")
}

// SetBilibiliCookie 更新Manager内部的B站Cookie
func (m *Manager) SetBilibiliCookie(cookie string) {
	m.bilibiliParser.SetCookies(cookie)
	m.bilibiliDownloader.SetCookies(cookie)
}

// SetDouyinCookie 更新Manager内部的抖音Cookie
func (m *Manager) SetDouyinCookie(cookie string) {
	m.douyinDownloader.SetCookies(cookie)
	m.douyinCollection.SetCookies(cookie)
}


// ResetTaskForRetry 在锁内把失败任务重置为待执行
func (m *Manager) ResetTaskForRetry(taskID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, exists := m.tasks[taskID]
	if !exists || task.Status != StatusFailed { return false }
	task.Status = StatusPending
	task.ErrorMessage = ""
	task.UpdatedAt = time.Now()
	return true
}

func (m *Manager) updateTaskStatus(taskID string, status TaskStatus, errorMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if task, exists := m.tasks[taskID]; exists {
		task.Status = status
		task.ErrorMessage = errorMsg
		task.UpdatedAt = time.Now()
	}
}

func sanitizeFilename(name string) string {
	// 替换非法字符
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		"\n", "_",
		"\r", "_",
	)
	return replacer.Replace(name)
}

func identifyPlatform(url string) string {
	if strings.Contains(url, "bilibili.com") || strings.Contains(url, "b23.tv") {
		return "bilibili"
	}
	if strings.Contains(url, "douyin.com") || strings.Contains(url, "iesdouyin.com") || strings.Contains(url, "v.douyin.com") {
		return "douyin"
	}
	return ""
}

// IdentifyPlatform identifies platform from URL (exported for use by handlers)
func IdentifyPlatform(url string) string {
	return identifyPlatform(url)
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
