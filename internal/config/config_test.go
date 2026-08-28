package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTestConfig 在隔离的用户目录下写一份 config.json
func writeTestConfig(t *testing.T, content string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home) // Windows UserHomeDir
	t.Setenv("HOME", home)        // Linux/macOS
	cfgFile := filepath.Join(home, ".config", "ccnew-vdl", "config.json")
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

// 环境变量必须优先于 config.json（Docker 场景：compose 中的 DOWNLOAD_DIR
// 不能被 UI 保存过的 config.json 覆盖）
func TestEnvOverridesConfigFile(t *testing.T) {
	writeTestConfig(t, `{"download_dir":"/from/config-file","port":"19999"}`)
	t.Setenv("DOWNLOAD_DIR", "/from/env")

	cfg := Load()
	if cfg.DownloadDir != "/from/env" {
		t.Fatalf("环境变量应优先于配置文件: got download_dir=%q", cfg.DownloadDir)
	}
}

// 没有环境变量时，config.json 仍然生效（桌面版场景）
func TestConfigFileUsedWhenNoEnv(t *testing.T) {
	writeTestConfig(t, `{"download_dir":"/from/config-file"}`)

	cfg := Load()
	if cfg.DownloadDir != "/from/config-file" {
		t.Fatalf("无环境变量时应使用配置文件: got download_dir=%q", cfg.DownloadDir)
	}
}

// LOG_DIR 环境变量应被读取（此前代码从未读取过该变量）
func TestLogDirEnv(t *testing.T) {
	writeTestConfig(t, `{}`)
	t.Setenv("LOG_DIR", "/from/env-logs")

	cfg := Load()
	if cfg.LogDir != "/from/env-logs" {
		t.Fatalf("LOG_DIR 环境变量应生效: got log_dir=%q", cfg.LogDir)
	}
}
