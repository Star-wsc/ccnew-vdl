package douyin

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Star-wsc/ccnew-vdl/internal/models"
)

// DouyinDownloader 抖音下载器，包含多种解析策略
type DouyinDownloader struct {
	parser *Parser
}

func NewDouyinDownloader(proxy string) *DouyinDownloader {
	return &DouyinDownloader{
		parser: NewParser(proxy),
	}
}

func (d *DouyinDownloader) SetCookies(cookies string) {
	d.parser.SetCookies(cookies)
}

// Parse 解析抖音视频，使用多种策略
func (d *DouyinDownloader) Parse(rawURL string) (*models.VideoInfo, error) {
	videoURL := rawURL

	// 解析短链接
	if strings.Contains(videoURL, "v.douyin.com") {
		resolved, err := d.parser.resolveShortURL(videoURL)
		if err == nil && resolved != "" {
			videoURL = resolved
			log.Printf("[抖音] 短链接解析为: %s", videoURL)
		}
	}

	// 提取视频ID
	videoID := d.parser.extractVideoID(videoURL)
	if videoID == "" {
		return nil, fmt.Errorf("无法提取视频ID: %s", videoURL)
	}
	log.Printf("[抖音] 视频ID: %s", videoID)

	// 策略1: 用iesdouyin.com分享页 + 移动端UA（最有效！douyin.com返回SPA空壳）
	shareURL := fmt.Sprintf("https://www.iesdouyin.com/share/video/%s/", videoID)
	log.Printf("[抖音] 策略1: iesdouyin分享页+移动端UA: %s", shareURL)
	info, err := d.parseWithMobileUA(shareURL)
	if err == nil && info.VideoURL != "" {
		d.enrichAudioURL(info, videoID)
		log.Printf("[抖音] 策略1成功: title=%s, author=%s", info.Title, info.Author)
		return info, nil
	}
	log.Printf("[抖音] 策略1失败: %v", err)

	// 策略2: douyin.com + 移动端UA
	douyinURL := fmt.Sprintf("https://www.douyin.com/video/%s", videoID)
	log.Printf("[抖音] 策略2: douyin.com+移动端UA")
	info, err = d.parseWithMobileUA(douyinURL)
	if err == nil && info.VideoURL != "" {
		d.enrichAudioURL(info, videoID)
		log.Printf("[抖音] 策略2成功")
		return info, nil
	}
	log.Printf("[抖音] 策略2失败: %v", err)

	// 策略3: 桌面UA解析 (RENDER_DATA + Detail API + HTML Regex)
	log.Printf("[抖音] 策略3: 桌面UA快速解析")
	info, err = d.parser.Parse(douyinURL)
	if err == nil && info.VideoURL != "" {
		d.enrichAudioURL(info, videoID)
		log.Printf("[抖音] 策略3成功")
		return info, nil
	}
	log.Printf("[抖音] 策略3失败: %v", err)

	// 策略4: 多UA尝试
	log.Printf("[抖音] 策略4: 多UA尝试")
	info, err = d.parseWithAlternateUA(shareURL)
	if err == nil && info.VideoURL != "" {
		d.enrichAudioURL(info, videoID)
		log.Printf("[抖音] 策略4成功")
		return info, nil
	}
	log.Printf("[抖音] 策略4失败: %v", err)

	// 策略5: 第三方API
	log.Printf("[抖音] 策略5: 第三方API")
	info, err = d.parseViaAPI(douyinURL)
	if err == nil && info.VideoURL != "" {
		d.enrichAudioURL(info, videoID)
		log.Printf("[抖音] 策略5成功")
		return info, nil
	}
	log.Printf("[抖音] 策略5失败: %v", err)

	return nil, fmt.Errorf("all parse strategies failed for: %s", rawURL)
}

// parseWithMobileUA 使用移动端UA解析（关键策略）
func (d *DouyinDownloader) parseWithMobileUA(videoURL string) (*models.VideoInfo, error) {
	// 使用与原始项目相同的移动端UA
	mobileUA := "Mozilla/5.0 (Linux; Android 10; SM-G981B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36"
	return d.parseWithUA(videoURL, mobileUA)
}

func (d *DouyinDownloader) parseWithAlternateUA(videoURL string) (*models.VideoInfo, error) {
	userAgents := []string{
		"com.ss.android.ugc.aweme/330201 (Linux; U; Android 13; zh_CN; SM-G991B; Build/TP1A.220624.014)",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X; zh_CN; Scale/3.00)",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15",
	}

	for _, ua := range userAgents {
		info, err := d.parseWithUA(videoURL, ua)
		if err == nil && info.VideoURL != "" {
			return info, nil
		}
	}

	return nil, fmt.Errorf("all alternate UA parse strategies failed")
}

func (d *DouyinDownloader) parseWithUA(videoURL, userAgent string) (*models.VideoInfo, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
	}

	req, err := http.NewRequest("GET", videoURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	html, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	cookies := make(map[string]string)
	for _, c := range resp.Cookies() {
		cookies[c.Name] = c.Value
	}

	htmlStr := string(html)

	// 尝试提取视频URL
	videoURLs := make(map[string]string)
	downloadURLs := make(map[string]string)

	patterns := []struct {
		regex   string
		urlType string
	}{
		{`"download_addr"[^}]*"url_list"\s*:\s*\["([^"]+)"`, "download"},
		{`"download"[^}]*"url_list"\s*:\s*\["([^"]+)"`, "download"},
		{`"play_addr"[^}]*"url_list"\s*:\s*\["([^"]+)"`, "play"},
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p.regex)
		matches := re.FindAllStringSubmatch(htmlStr, -1)
		for _, match := range matches {
			if len(match) > 1 {
				u := decodeUnicodeURL(match[1])
				u = processVideoURL(u)
				if p.urlType == "download" {
					downloadURLs["1080p"] = u
				} else {
					videoURLs["1080p"] = u
				}
				break
			}
		}
	}

	// 提取bit_rate信息（多清晰度）
	bitRateMatch := regexp.MustCompile(`"bit_rate"\s*:\s*\[([^\]]+)\]`).FindStringSubmatch(htmlStr)
	if bitRateMatch != nil {
		bitRates := bitRateMatch[1]
		qualityMap := map[string][]string{
			"4k":    {"4k", "2160p", "uhd"},
			"2k":    {"2k", "1440p", "qhd"},
			"1080p": {"1080p", "fhd", "full_hd"},
			"720p":  {"720p", "hd", "high"},
			"480p":  {"480p", "sd", "normal"},
		}

		for q, keywords := range qualityMap {
			for _, keyword := range keywords {
				pattern := fmt.Sprintf(`"gear_name"\s*:\s*"[^"]*%s[^"]*"[^}}]*"play_addr"[^}}]*"url_list"\s*:\s*\["([^"]+)"`, keyword)
				re := regexp.MustCompile(`(?i)` + pattern)
				match := re.FindStringSubmatch(bitRates)
				if len(match) > 1 {
					u := decodeUnicodeURL(match[1])
					u = processVideoURL(u)
					videoURLs[q] = u
					break
				}
			}
		}
	}

	// 合并所有URL
	allURLs := make(map[string]string)
	for k, v := range videoURLs {
		allURLs[k] = v
	}
	for k, v := range downloadURLs {
		allURLs[k] = v
	}

	if len(allURLs) == 0 {
		return nil, fmt.Errorf("no video URL found")
	}

	// 选择最佳URL
	qualityPriority := []string{"4k", "2k", "1080p", "720p", "480p"}
	selectedURL := ""
	selectedQuality := ""
	for _, q := range qualityPriority {
		if u, ok := allURLs[q]; ok && u != "" {
			selectedURL = u
			selectedQuality = q
			break
		}
	}
	if selectedURL == "" {
		for q, u := range allURLs {
			selectedURL = u
			selectedQuality = q
			break
		}
	}

	// 提取标题
	title := ""
	descMatch := regexp.MustCompile(`"desc"\s*:\s*"([^"]*)"`).FindStringSubmatch(htmlStr)
	if len(descMatch) > 1 && descMatch[1] != "" {
		title = descMatch[1]
	} else {
		titleRe := regexp.MustCompile(`<title[^>]*>([^<]+)</title>`)
		if m := titleRe.FindStringSubmatch(htmlStr); len(m) > 1 {
			title = strings.TrimSuffix(m[1], " - 抖音")
		}
	}

	// 提取作者
	author := "未知作者"
	authorRe := regexp.MustCompile(`"nickname"\s*:\s*"([^"]+)"`)
	if m := authorRe.FindStringSubmatch(htmlStr); len(m) > 1 {
		author = m[1]
	}

	// 提取封面URL
	coverURL := ""
	coverMatch := regexp.MustCompile(`"cover"[^}]*"url_list"\s*:\s*\["([^"]+)"`).FindStringSubmatch(htmlStr)
	if len(coverMatch) > 1 {
		coverURL = decodeUnicodeURL(coverMatch[1])
		if strings.HasPrefix(coverURL, "//") {
			coverURL = "https:" + coverURL
		}
	}

	log.Printf("[抖音-UA] 选择清晰度: %s (可用: %v)", selectedQuality, getKeys(allURLs))

	return &models.VideoInfo{
		Title:           title,
		Author:          author,
		CoverURL:        coverURL,
		VideoURL:        selectedURL,
		SelectedQuality: selectedQuality,
		Cookies:         cookies,
		Platform:        models.PlatformDouyin,
	}, nil
}

func (d *DouyinDownloader) parseViaAPI(videoURL string) (*models.VideoInfo, error) {
	apis := []string{
		"https://api.douyin.wtf/api?url=%s",
		"https://www.douyin.wtf/api?url=%s",
	}

	for _, api := range apis {
		apiURL := fmt.Sprintf(api, videoURL)
		info, err := d.fetchFromAPI(apiURL)
		if err == nil && info.VideoURL != "" {
			return info, nil
		}
	}

	return nil, fmt.Errorf("all third-party APIs failed")
}

func (d *DouyinDownloader) fetchFromAPI(apiURL string) (*models.VideoInfo, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			VideoURL string `json:"video_url"`
			Title    string `json:"title"`
			Author   string `json:"author"`
			CoverURL string `json:"cover_url"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.Code != 0 || result.Data.VideoURL == "" {
		return nil, fmt.Errorf("API error or no video URL")
	}

	return &models.VideoInfo{
		Title:           result.Data.Title,
		Author:          result.Data.Author,
		CoverURL:        result.Data.CoverURL,
		VideoURL:        result.Data.VideoURL,
		SelectedQuality: "1080p",
		Platform:        models.PlatformDouyin,
	}, nil
}

// DownloadVideo 下载抖音视频
func (d *DouyinDownloader) DownloadVideo(videoURL, outputPath string, cookies map[string]string, progressFunc func(int64, int64)) error {
	client := &http.Client{Timeout: 600 * time.Second}

	req, err := http.NewRequest("GET", videoURL, nil)
	if err != nil {
		return err
	}

	// 设置抖音视频下载专用请求头
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.douyin.com/")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Connection", "keep-alive")

	// 添加 Cookie
	for name, value := range cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// 检查 Content-Type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.Contains(contentType, "video") && !strings.Contains(contentType, "mp4") && !strings.Contains(contentType, "octet-stream") {
		// 读取前 100 字节检查是否是 HTML
		buf := make([]byte, 100)
		n, _ := io.ReadFull(resp.Body, buf)
		if n > 0 && (string(buf[:n]) == "<!DOCTYPE" || string(buf[:n])[:5] == "<html") {
			return fmt.Errorf("服务器返回 HTML 而非视频流")
		}
		// 如果不是 HTML，继续下载
		outFile, err := os.Create(outputPath)
		if err != nil {
			return err
		}
		defer outFile.Close()
		outFile.Write(buf[:n])
		written := int64(n)
		totalSize := resp.ContentLength
		if progressFunc != nil {
			progressFunc(written, totalSize)
		}
		buf2 := make([]byte, 32*1024)
		for {
			nr, err := resp.Body.Read(buf2)
			if nr > 0 {
				nw, ew := outFile.Write(buf2[:nr])
				if ew != nil {
					return ew
				}
				written += int64(nw)
				if progressFunc != nil {
					progressFunc(written, totalSize)
				}
			}
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
		}
		return nil
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	totalSize := resp.ContentLength
	var written int64
	buf := make([]byte, 32*1024)

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			nw, ew := outFile.Write(buf[:n])
			if ew != nil {
				return ew
			}
			written += int64(nw)
			if progressFunc != nil {
				progressFunc(written, totalSize)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	return nil
}

// enrichAudioURL 从API补充音频流URL（抖音DASH格式音视频分离）
func (d *DouyinDownloader) enrichAudioURL(info *models.VideoInfo, videoID string) {
	if info.AudioURL != "" { return }
	apiInfo, err := d.parser.parseDetailAPI(videoID)
	if err == nil && apiInfo != nil && apiInfo.AudioURL != "" {
		info.AudioURL = apiInfo.AudioURL
		log.Printf("[抖音-enrich] 已补充音频流URL")
	}
}
