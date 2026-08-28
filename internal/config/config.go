package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port           string `json:"port"`
	DownloadDir    string `json:"download_dir"`
	LogDir         string `json:"log_dir"`
	BilibiliCookie string `json:"bilibili_cookie"`
	DouyinCookie   string `json:"douyin_cookie"`
	Proxy          string `json:"proxy"`
}

func Load() *Config {
	cfg := &Config{
		Port:        "18000",
		DownloadDir: getDefaultDownloadDir(),
		LogDir:      "./logs",
	}

	// 从配置文件加载（优先用户目录，其次程序目录）
	configFiles := []string{getConfigFilePath()}
	// 检查程序所在目录的 config.json
	if exePath, err := os.Executable(); err == nil {
		configFiles = append(configFiles, filepath.Join(filepath.Dir(exePath), "config.json"))
	}
	// 检查当前工作目录的 config.json
	if wd, err := os.Getwd(); err == nil {
		localCfg := filepath.Join(wd, "config.json")
		if localCfg != configFiles[0] && (len(configFiles) < 2 || localCfg != configFiles[1]) {
			configFiles = append(configFiles, localCfg)
		}
	}

	for _, configFile := range configFiles {
		if _, err := os.Stat(configFile); err == nil {
			data, err := os.ReadFile(configFile)
			if err == nil {
				var fileCfg Config
				if err := json.Unmarshal(data, &fileCfg); err == nil {
					if fileCfg.Port != "" {
						cfg.Port = fileCfg.Port
					}
					if fileCfg.DownloadDir != "" {
						cfg.DownloadDir = fileCfg.DownloadDir
					}
					if fileCfg.BilibiliCookie != "" {
						cfg.BilibiliCookie = sanitizeCookie(fileCfg.BilibiliCookie)
					}
					if fileCfg.DouyinCookie != "" {
						cfg.DouyinCookie = sanitizeCookie(fileCfg.DouyinCookie)
					}
					if fileCfg.Proxy != "" {
						cfg.Proxy = fileCfg.Proxy
					}
					break // 找到第一个有效配置文件就停止
				}
			}
		}
	}

	// 环境变量最后加载，优先级最高于配置文件
	// （Docker 场景下 compose 中的 DOWNLOAD_DIR 等不能被 config.json 覆盖）
	if port := os.Getenv("PORT"); port != "" {
		cfg.Port = port
	}
	if dir := os.Getenv("DOWNLOAD_DIR"); dir != "" {
		cfg.DownloadDir = dir
	}
	if dir := os.Getenv("LOG_DIR"); dir != "" {
		cfg.LogDir = dir
	}
	if cookie := os.Getenv("BILIBILI_COOKIE"); cookie != "" {
		cfg.BilibiliCookie = sanitizeCookie(cookie)
	}
	if cookie := os.Getenv("DOUYIN_COOKIE"); cookie != "" {
		cfg.DouyinCookie = sanitizeCookie(cookie)
	}
	if proxy := os.Getenv("PROXY"); proxy != "" {
		cfg.Proxy = proxy
	}

	return cfg
}

func (c *Config) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	configFile := getConfigFilePath()
	dir := filepath.Dir(configFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// 0600：配置含 Cookie，仅限当前用户可读
	return os.WriteFile(configFile, data, 0600)
}

// sanitizeCookie 清理Cookie中的非法HTTP头字符
func sanitizeCookie(cookie string) string {
	// 去除换行符、回车符、前后空白
	cookie = strings.ReplaceAll(cookie, "\n", "")
	cookie = strings.ReplaceAll(cookie, "\r", "")
	cookie = strings.ReplaceAll(cookie, "\t", "")
	cookie = strings.TrimSpace(cookie)
	// 去除可能的引号包裹
	cookie = strings.Trim(cookie, "\"'")
	return cookie
}

func getDefaultDownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./downloads"
	}
	return filepath.Join(home, "Downloads", "video-downloader")
}

func getConfigFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./config.json"
	}
	return filepath.Join(home, ".config", "ccnew-vdl", "config.json")
}
