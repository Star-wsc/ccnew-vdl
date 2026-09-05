package bilibili

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// MergeMP4 使用FFmpeg合并音视频流
func MergeMP4(videoPath, audioPath, outputPath string) error {
	ffmpegPath, err := findFFmpeg()
	if err != nil {
		return fmt.Errorf("FFmpeg未找到: %w", err)
	}

	args := []string{
		"-i", videoPath,
		"-i", audioPath,
		"-c:v", "copy",
		"-c:a", "copy",
		"-y",
		outputPath,
	}

	cmd := exec.Command(ffmpegPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Windows下隐藏控制台窗口
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = getSysProcAttr()
	}

	return cmd.Run()
}

// findFFmpeg 查找FFmpeg可执行文件
func findFFmpeg() (string, error) {
	// 1. 检查PATH中的ffmpeg
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path, nil
	}

	// 2. 检查可执行文件同级目录
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		candidates := getFFmpegCandidates()
		for _, name := range candidates {
			path := filepath.Join(exeDir, name)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}

		// 3. 检查可执行文件同级目录的ffmpeg子目录
		ffmpegDir := filepath.Join(exeDir, "ffmpeg")
		for _, name := range candidates {
			path := filepath.Join(ffmpegDir, name)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}

	// 4. 检查当前目录
	candidates := getFFmpegCandidates()
	for _, name := range candidates {
		if _, err := os.Stat(name); err == nil {
			return name, nil
		}
	}

	// 5. 检查当前目录的ffmpeg子目录
	for _, name := range candidates {
		path := filepath.Join("ffmpeg", name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("未找到FFmpeg")
}

// getFFmpegCandidates 获取平台对应的FFmpeg文件名
func getFFmpegCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"ffmpeg-windows-amd64.exe", "ffmpeg.exe"}
	case "linux":
		if runtime.GOARCH == "arm64" {
			return []string{"ffmpeg-linux-arm64", "ffmpeg"}
		}
		return []string{"ffmpeg-linux-amd64", "ffmpeg"}
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return []string{"ffmpeg-darwin-arm64", "ffmpeg"}
		}
		return []string{"ffmpeg-darwin-amd64", "ffmpeg"}
	case "android":
		return []string{"libffmpeg.so", "ffmpeg"}
	default:
		return []string{"ffmpeg"}
	}
}
