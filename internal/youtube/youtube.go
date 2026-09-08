package youtube

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// IsYouTubeURL 判断是否YouTube链接(含youtu.be/Shorts/音乐站)
func IsYouTubeURL(u string) bool {
	l := strings.ToLower(u)
	for _, h := range []string{"youtube.com", "youtu.be", "youtube-nocookie.com"} {
		if strings.Contains(l, h) {
			return true
		}
	}
	return false
}

// FindYTDLP 查找yt-dlp: PATH → exe目录/yt-dlp/ → exe目录 → 当前目录
func FindYTDLP() (string, error) {
	var names []string
	switch runtime.GOOS {
	case "windows":
		names = []string{"yt-dlp-windows-amd64.exe", "yt-dlp.exe", "yt-dlp"}
	default:
		names = []string{"yt-dlp-linux-amd64", "yt-dlp"}
	}
	if p, err := exec.LookPath("yt-dlp"); err == nil {
		return p, nil
	}
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		dirs = append(dirs, filepath.Join(d, "yt-dlp"), d)
	}
	dirs = append(dirs, ".")
	for _, dir := range dirs {
		for _, n := range names {
			p := filepath.Join(dir, n)
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("未找到yt-dlp, 请将其放入程序目录的yt-dlp子目录")
}

// VideoInfo 元数据(yt-dlp -J)
type VideoInfo struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Uploader    string `json:"uploader"`
	Thumbnail   string `json:"thumbnail"`
	DurationSec int    `json:"duration"`
	Height      int    `json:"height"`
	Fps         int    `json:"fps"`
}

func baseArgs(proxy string) []string {
	a := []string{"--no-playlist", "--no-warnings", "--no-progress"}
	if proxy != "" {
		a = append(a, "--proxy", proxy)
	}
	return a
}

// ParseInfo 解析元数据(用于预览/任务标题)
func ParseInfo(url, proxy string) (*VideoInfo, error) {
	bin, err := FindYTDLP()
	if err != nil {
		return nil, err
	}
	args := append(baseArgs(proxy), "-J", url)
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp解析失败: %w", err)
	}
	var vi VideoInfo
	if err := json.Unmarshal(out, &vi); err != nil {
		return nil, fmt.Errorf("yt-dlp输出解析失败: %w", err)
	}
	if vi.Title == "" {
		return nil, fmt.Errorf("未获取到视频信息")
	}
	return &vi, nil
}

// Version 当前yt-dlp版本号
func Version() (string, error) {
	bin, err := FindYTDLP()
	if err != nil {
		return "", err
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// SelfUpdate 执行yt-dlp -U自更新(独立二进制自带能力), 返回更新输出与更新后版本
func SelfUpdate() (string, string, error) {
	bin, err := FindYTDLP()
	if err != nil {
		return "", "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "-U").CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return output, "", fmt.Errorf("yt-dlp更新失败: %w", err)
	}
	newVer, _ := Version()
	return output, newVer, nil
}

// QualityToHeight 档位→最大高度
func QualityToHeight(q string) int {
	switch strings.ToLower(q) {
	case "4k":
		return 2160
	case "2k":
		return 1440
	case "1080p":
		return 1080
	case "720p":
		return 720
	case "480p":
		return 480
	default:
		return 1080
	}
}

// findFFmpeg 查找ffmpeg(合并用), 与download包逻辑一致的精简版
func findFFmpeg() string {
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	var dirs []string
	var names []string
	if runtime.GOOS == "windows" {
		names = []string{"ffmpeg-windows-amd64.exe", "ffmpeg.exe", "ffmpeg"}
	} else if runtime.GOARCH == "arm64" {
		names = []string{"ffmpeg-linux-arm64", "ffmpeg"}
	} else {
		names = []string{"ffmpeg-linux-amd64", "ffmpeg"}
	}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		dirs = append(dirs, filepath.Join(d, "ffmpeg"), d, filepath.Join(d, "yt-dlp"))
	}
	dirs = append(dirs, "ffmpeg", ".")
	for _, dir := range dirs {
		for _, n := range names {
			p := filepath.Join(dir, n)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

var (
	rePct    = regexp.MustCompile(`(\d+(?:\.\d+)?)%`)
	reOf     = regexp.MustCompile(`of\s*~?\s*([\d.]+)(KiB|MiB|GiB)`)
	reAtSize = regexp.MustCompile(`\]\s*([\d.]+)(KiB|MiB|GiB)\s+at`)
)

func unitBytes(u string) int64 {
	switch u {
	case "GiB":
		return 1 << 30
	case "MiB":
		return 1 << 20
	default:
		return 1 << 10
	}
}

// parseProgress 从 "[download] 45.2% of ~8.50MiB at 1.2MiB/s" 提取字节进度
func parseProgress(line string) (downloaded, total int64) {
	if m := rePct.FindStringSubmatch(line); m != nil {
		pct, _ := strconv.ParseFloat(m[1], 64)
		if t := reOf.FindStringSubmatch(line); t != nil {
			v, _ := strconv.ParseFloat(t[1], 64)
			total = int64(v * float64(unitBytes(t[2])))
			downloaded = int64(pct / 100 * float64(total))
			return
		}
	}
	if m := reAtSize.FindStringSubmatch(line); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		downloaded = int64(v * float64(unitBytes(m[2])))
	}
	return
}

// Download 用yt-dlp下载并合并为mp4。
// url: 视频页地址; quality: 4k/2k/1080p/720p/480p; outputPath: 最终.mp4路径
// progress: (已下载字节, 总字节估计)
func Download(url, quality, outputPath, proxy string, progress func(downloaded, total int64)) error {
	bin, err := FindYTDLP()
	if err != nil {
		return err
	}
	h := QualityToHeight(quality)
	// 优先m4a(AAC)音轨: Opus-in-MP4部分播放器不兼容
	format := fmt.Sprintf("bv*[height<=%d]+ba[ext=m4a]/bv*[height<=%d]+ba/b[height<=%d]/b", h, h, h)
	base := strings.TrimSuffix(outputPath, filepath.Ext(outputPath))
	outTpl := base + ".%(ext)s"

	args := append(baseArgs(proxy),
		"-f", format,
		"--merge-output-format", "mp4",
		"--newline",
	)
	if ff := findFFmpeg(); ff != "" {
		args = append(args, "--ffmpeg-location", ff)
	}
	args = append(args, "-o", outTpl, url)

	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
		} // 丢弃stderr, 防止管道阻塞
	}()
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 256*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "[download]") && progress != nil {
			if d, t := parseProgress(line); d > 0 || t > 0 {
				progress(d, t)
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("yt-dlp下载失败: %w", err)
	}

	// 产物确认: 合并成功即.mp4; 若为其他容器(webm/mkv)用ffmpeg转封装
	if _, err := os.Stat(outputPath); err == nil {
		return nil
	}
	for _, ext := range []string{".mkv", ".webm"} {
		alt := base + ext
		if _, err := os.Stat(alt); err == nil {
			return remuxToMP4(alt, outputPath)
		}
	}
	return fmt.Errorf("下载完成但未找到输出文件: %s", base+".*")
}

// remuxToMP4 无损转封装到mp4, 失败则直接改名兜底
func remuxToMP4(src, dst string) error {
	ff := findFFmpeg()
	if ff != "" {
		cmd := exec.Command(ff, "-i", src, "-c", "copy", "-y", dst)
		if err := cmd.Run(); err == nil {
			os.Remove(src)
			return nil
		}
	}
	return os.Rename(src, dst)
}
