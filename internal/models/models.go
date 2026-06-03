package models

import (
	"crypto/rand"
	"fmt"
	"time"
)

type TaskStatus string

const (
	StatusPending     TaskStatus = "pending"
	StatusParsing     TaskStatus = "parsing"
	StatusDownloading TaskStatus = "downloading"
	StatusCompleted   TaskStatus = "completed"
	StatusFailed      TaskStatus = "failed"
)

type DownloadTask struct {
	ID            string     `json:"id"`
	URL           string     `json:"url"`
	Title         string     `json:"title"`
	Author        string     `json:"author"`
	AuthorID      string     `json:"author_id"`
	CoverURL      string     `json:"cover_url"`
	VideoURL      string     `json:"video_url"`
	Quality       string     `json:"quality"`
	ActualQuality string     `json:"actual_quality"`
	Platform      string     `json:"platform"`
	Status        TaskStatus `json:"status"`
	Progress      int        `json:"progress"`
	Speed         int64      `json:"speed"`
	FilePath      string     `json:"file_path"`
	FileSize      int64      `json:"file_size"`
	ErrorMessage  string     `json:"error_message"`
	RetryCount    int        `json:"retry_count"`
	MaxRetries    int        `json:"max_retries"`
	CreatedAt     string     `json:"created_at"`
	UpdatedAt     string     `json:"updated_at"`
}

func NewDownloadTask(url, quality string) *DownloadTask {
	now := time.Now().Format(time.RFC3339)
	return &DownloadTask{
		ID:         generateID(),
		URL:        url,
		Quality:    quality,
		Status:     StatusPending,
		Progress:   0,
		MaxRetries: 3,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type CreateTaskRequest struct {
	URL     string `json:"url"`
	Quality string `json:"quality"`
}

type ConfigResponse struct {
	DownloadDir string `json:"download_dir"`
	TotalTasks  int    `json:"total_tasks"`
	Version     string `json:"version"`
	FirstRun    bool   `json:"first_run"`
}

type StatsResponse struct {
	Total       int `json:"total"`
	Pending     int `json:"pending"`
	Downloading int `json:"downloading"`
	Completed   int `json:"completed"`
	Failed      int `json:"failed"`
}

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	TaskID    string `json:"task_id"`
	Message   string `json:"message"`
	Details   string `json:"details"`
}

type VideoInfo struct {
	Title              string
	Author             string
	AuthorID           string
	CoverURL           string
	VideoURL           string
	AudioURL           string
	DownloadURL        string
	SelectedQuality    string
	AvailableQualities []string
	Cookies            map[string]string
	Platform           Platform
}

type NotificationEvent struct {
	Type    string `json:"type"` // "completed" | "failed"
	TaskID  string `json:"task_id"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

type SearchRequest struct {
	Keyword  string `json:"keyword"`
	Status   string `json:"status"`
	Platform string `json:"platform"`
	Limit    int    `json:"limit"`
}

type SettingsRequest struct {
	DownloadDir   string `json:"download_dir"`
	Proxy         string `json:"proxy"`
	MaxConcurrent int    `json:"max_concurrent"`
	SpeedLimit    int    `json:"speed_limit"`
	FileTemplate  string `json:"file_template"`
	AutoClassify  bool   `json:"auto_classify"`
	APIToken      string `json:"api_token"`
}

type SettingsResponse struct {
	DownloadDir   string `json:"download_dir"`
	Proxy         string `json:"proxy"`
	MaxConcurrent int    `json:"max_concurrent"`
	SpeedLimit    int    `json:"speed_limit"`
	FileTemplate  string `json:"file_template"`
	AutoClassify  bool   `json:"auto_classify"`
	APIToken      string `json:"api_token"`
	Version       string `json:"version"`
}
