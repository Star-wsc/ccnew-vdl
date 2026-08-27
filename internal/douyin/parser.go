package douyin

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Star-wsc/ccnew-vdl/internal/models"
)

var douyinCookieWarningLogged bool

type Parser struct {
	client  *http.Client
	cookies string
}

func NewParser(proxy string) *Parser {
	transport := &http.Transport{}
	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return &Parser{
		client: &http.Client{
			Timeout:   15 * time.Second,
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

func (p *Parser) SetCookies(cookies string) {
	p.cookies = cookies
}

func (p *Parser) Parse(rawURL string) (*models.VideoInfo, error) {
	videoURL := rawURL
	log.Printf("[抖音] 开始解析: %s", rawURL)

	if strings.Contains(videoURL, "v.douyin.com") {
		resolved, err := p.resolveShortURL(videoURL)
		if err == nil && resolved != "" {
			videoURL = resolved
			log.Printf("[抖音] 短链接解析为: %s", videoURL)
		} else {
			log.Printf("[抖音] 短链接解析失败: %v", err)
		}
	}

	videoID := p.extractVideoID(videoURL)
	fullURL := videoURL
	if videoID != "" {
		fullURL = fmt.Sprintf("https://www.douyin.com/video/%s", videoID)
		log.Printf("[抖音] 提取到视频ID: %s, 构造URL: %s", videoID, fullURL)
	} else {
		log.Printf("[抖音] 无法提取视频ID，使用原始URL: %s", videoURL)
	}

	info, err := p.parseDetailAPI(videoID)
	if err == nil && info.VideoURL != "" {
		log.Printf("[抖音] 策略1(parseDetailAPI+ttwid)成功")
		return info, nil
	} else {
		log.Printf("[抖音] 策略1(parseDetailAPI+ttwid)失败: %v", err)
	}

	info, err = p.parseRenderData(fullURL)
	if err == nil && info.VideoURL != "" {
		log.Printf("[抖音] 策略2(parseRenderData)成功")
		return info, nil
	} else {
		log.Printf("[抖音] 策略2(parseRenderData)失败: %v", err)
	}

	info, err = p.parseHTMLRegex(fullURL)
	if err == nil && info.VideoURL != "" {
		log.Printf("[抖音] 策略3(parseHTMLRegex)成功")
		return info, nil
	} else {
		log.Printf("[抖音] 策略3(parseHTMLRegex)失败: %v", err)
	}

	return nil, fmt.Errorf("all parse strategies failed for: %s", rawURL)
}

func (p *Parser) resolveShortURL(shortURL string) (string, error) {
	req, err := http.NewRequest("GET", shortURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 13; SM-G991B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 301 || resp.StatusCode == 302 {
		location := resp.Header.Get("Location")
		if location != "" {
			return location, nil
		}
	}

	return "", fmt.Errorf("no redirect")
}

// ExtractVideoID 是 extractVideoID 的公共版本
func (p *Parser) ExtractVideoID(videoURL string) string {
	return p.extractVideoID(videoURL)
}

func (p *Parser) extractVideoID(videoURL string) string {
	// 匹配 douyin.com/video/xxx
	re := regexp.MustCompile(`douyin\.com/video/(\d+)`)
	matches := re.FindStringSubmatch(videoURL)
	if len(matches) > 1 {
		return matches[1]
	}

	// 匹配 iesdouyin.com/share/video/xxx/ (分享页URL)
	re2 := regexp.MustCompile(`iesdouyin\.com/share/video/(\d+)`)
	matches2 := re2.FindStringSubmatch(videoURL)
	if len(matches2) > 1 {
		return matches2[1]
	}

	// 匹配 modal_id=xxx
	re3 := regexp.MustCompile(`modal_id=(\d+)`)
	matches3 := re3.FindStringSubmatch(videoURL)
	if len(matches3) > 1 {
		return matches3[1]
	}

	return ""
}

func (p *Parser) fetchHTML(videoURL string) (string, map[string]string, error) {
	req, err := http.NewRequest("GET", videoURL, nil)
	if err != nil {
		return "", nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	if p.cookies != "" {
		req.Header.Set("Cookie", p.cookies)
	} else {
		// 未提供真实Cookie，抖音只返回720p视频
		if !douyinCookieWarningLogged {
			log.Println("[警告] 未提供抖音登录Cookie，清晰度受限(最高720p)")
			log.Println("[提示] 获取Cookie方法:")
			log.Println("  1. 浏览器打开 www.douyin.com 并登录")
			log.Println("  2. 按F12打开开发者工具 → Network标签")
			log.Println("  3. 刷新页面，点击任意请求，复制Cookie值")
			log.Println("  4. 通过配置文件或参数传入程序")
			douyinCookieWarningLogged = true
		}
		req.Header.Set("Cookie", "msToken=abcdefg")
	}
	req.Header.Set("Referer", "https://www.douyin.com/")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}

	cookies := make(map[string]string)
	for _, c := range resp.Cookies() {
		cookies[c.Name] = c.Value
	}

	return string(body), cookies, nil
}

// ============ Strategy 1: RENDER_DATA ============

func (p *Parser) parseRenderData(videoURL string) (*models.VideoInfo, error) {
	html, cookies, err := p.fetchHTML(videoURL)
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`<script id="RENDER_DATA" type="application/json">([^<]+)</script>`)
	match := re.FindStringSubmatch(html)
	if len(match) < 2 {
		return nil, fmt.Errorf("RENDER_DATA not found")
	}

	decoded, err := url.QueryUnescape(match[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode RENDER_DATA: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(decoded), &data); err != nil {
		return nil, fmt.Errorf("failed to parse RENDER_DATA JSON: %w", err)
	}

	info := p.extractFromRenderData(data)
	if info != nil && info.VideoURL != "" {
		info.Cookies = cookies
		return info, nil
	}

	return nil, fmt.Errorf("video info not found in RENDER_DATA")
}

func (p *Parser) extractFromRenderData(data map[string]interface{}) *models.VideoInfo {
	var detail map[string]interface{}

	if aweme, ok := data["aweme"].(map[string]interface{}); ok {
		if d, ok := aweme["detail"].(map[string]interface{}); ok {
			detail = d
		}
	}

	if detail == nil {
		if app, ok := data["app"].(map[string]interface{}); ok {
			if vi, ok := app["videoInfo"].(map[string]interface{}); ok {
				detail = vi
			}
		}
	}

	if detail == nil {
		for _, v := range data {
			if m, ok := v.(map[string]interface{}); ok {
				if d, ok := m["detail"].(map[string]interface{}); ok {
					detail = d
					break
				}
				if vi, ok := m["videoInfo"].(map[string]interface{}); ok {
					detail = vi
					break
				}
			}
		}
	}

	if detail == nil {
		return nil
	}

	videoData, _ := detail["video"].(map[string]interface{})
	if videoData == nil {
		return nil
	}

	videoURLs := p.extractVideoURLs(videoData)

	title, _ := detail["desc"].(string)
	if title == "" {
		if si, ok := detail["share_info"].(map[string]interface{}); ok {
			title, _ = si["share_title"].(string)
		}
	}

	author := ""
	authorID := ""
	if authorInfo, ok := detail["author"].(map[string]interface{}); ok {
		author, _ = authorInfo["nickname"].(string)
		authorID, _ = authorInfo["unique_id"].(string)
		if authorID == "" {
			authorID, _ = authorInfo["short_id"].(string)
		}
	}

	coverURL := ""
	if cover, ok := videoData["cover"].(map[string]interface{}); ok {
		if urlList, ok := cover["url_list"].([]interface{}); ok && len(urlList) > 0 {
			coverURL, _ = urlList[0].(string)
		}
	}
	if coverURL == "" {
		if originCover, ok := videoData["origin_cover"].(map[string]interface{}); ok {
			if urlList, ok := originCover["url_list"].([]interface{}); ok && len(urlList) > 0 {
				coverURL, _ = urlList[0].(string)
			}
		}
	}
	if coverURL != "" && strings.HasPrefix(coverURL, "//") {
		coverURL = "https:" + coverURL
	}

	selectedURL := ""
	selectedQuality := ""
	qualityPriority := []string{"4k", "2k", "1080p", "720p", "480p"}
	for _, q := range qualityPriority {
		if u, ok := videoURLs[q]; ok && u != "" {
			selectedURL = u
			selectedQuality = q
			break
		}
	}
	if selectedURL == "" {
		for q, u := range videoURLs {
			selectedURL = u
			selectedQuality = q
			break
		}
	}

	if selectedURL == "" {
		return nil
	}

	audioURL := videoURLs["_audio"]
	delete(videoURLs, "_audio")

	log.Printf("[抖音] 选择清晰度: %s (可用: %v)", selectedQuality, getKeys(videoURLs))

	return &models.VideoInfo{
		Title:              title,
		Author:             author,
		AuthorID:           authorID,
		CoverURL:           coverURL,
		VideoURL:           selectedURL,
		AudioURL:		audioURL,
		SelectedQuality:    selectedQuality,
		AvailableQualities: getKeys(videoURLs),
	}
}

func (p *Parser) extractVideoURLs(videoData map[string]interface{}) map[string]string {
	urls := make(map[string]string)

	bitRate, _ := videoData["bit_rate"].([]interface{})
	bitRateQualities := make(map[string]string)

	for _, br := range bitRate {
		brMap, ok := br.(map[string]interface{})
		if !ok {
			continue
		}

		gearName, _ := brMap["gear_name"].(string)
		qualityType, _ := brMap["quality_type"].(float64)
		width, _ := brMap["width"].(float64)
		height, _ := brMap["height"].(float64)

		brPlayAddr, _ := brMap["play_addr"].(map[string]interface{})
		if brPlayAddr == nil {
			continue
		}

		brURLList, _ := brPlayAddr["url_list"].([]interface{})
		if len(brURLList) > 0 {
			u, _ := brURLList[0].(string)
			u = processVideoURL(u)

			q := mapQualityAdvanced(gearName, qualityType, int(width), int(height))
			if q != "" {
				bitRateQualities[q] = u
			}
		}
	}

	for q, u := range bitRateQualities {
		urls[q] = u
	}

	if bitRateAudio, ok := videoData["bit_rate_audio"].([]interface{}); ok && len(bitRateAudio) > 0 {
		for _, bra := range bitRateAudio {
			braMap, ok := bra.(map[string]interface{})
			if !ok { continue }
			audioMeta, ok := braMap["audio_meta"].(map[string]interface{})
			if !ok { continue }
			urlListObj, ok := audioMeta["url_list"].(map[string]interface{})
			if !ok { continue }
			for _, key := range []string{"main_url", "backup_url", "fallback_url"} {
				if u, ok := urlListObj[key].(string); ok && u != "" {
					if strings.HasPrefix(u, "//") { u = "https:" + u }
					urls["_audio"] = u
					log.Printf("[抖音] 找到音频流URL")
					break
				}
			}
			if _, has := urls["_audio"]; has { break }
		}
	}

	playAddr, _ := videoData["play_addr"].(map[string]interface{})
	if playAddr == nil {
		playAddr, _ = videoData["playAddr"].(map[string]interface{})
	}

	if playAddr != nil {
		urlList, _ := playAddr["url_list"].([]interface{})
		if len(urlList) > 0 {
			u, _ := urlList[0].(string)
			u = processVideoURL(u)
			if u != "" && len(urls) == 0 {
				urls["default"] = u
			}
		}
	}

	downloadAddr, _ := videoData["download_addr"].(map[string]interface{})
	if downloadAddr != nil {
		urlList, _ := downloadAddr["url_list"].([]interface{})
		if len(urlList) > 0 {
			u, _ := urlList[0].(string)
			u = processVideoURL(u)
			if u != "" {
				urls["download"] = u
			}
		}
	}

	for _, key := range []string{"play_addr_h265", "play_addr_h264", "play_addr_bytevc1"} {
		if v, ok := videoData[key].(map[string]interface{}); ok {
			if urlList, ok := v["url_list"].([]interface{}); ok && len(urlList) > 0 {
				u, _ := urlList[0].(string)
				u = processVideoURL(u)
				if u != "" {
					if _, exists := urls["4k"]; !exists {
						urls["4k_h265"] = u
					}
				}
			}
		}
	}

	return urls
}

func mapQualityAdvanced(gearName string, qualityType float64, width, height int) string {
	if height >= 2160 || width >= 3840 {
		return "4k"
	}
	if height >= 1440 || width >= 2560 {
		return "2k"
	}
	if height >= 1080 || width >= 1920 {
		return "1080p"
	}
	if height >= 720 || width >= 1280 {
		return "720p"
	}
	if height >= 480 || width >= 854 {
		return "480p"
	}
	if height >= 360 || width >= 640 {
		return "360p"
	}

	lower := strings.ToLower(gearName)
	switch {
	case strings.Contains(lower, "4k") || strings.Contains(lower, "2160") || strings.Contains(lower, "uhd"):
		return "4k"
	case strings.Contains(lower, "2k") || strings.Contains(lower, "1440") || strings.Contains(lower, "qhd"):
		return "2k"
	case strings.Contains(lower, "1080") || strings.Contains(lower, "fhd") || strings.Contains(lower, "full_hd"):
		return "1080p"
	case strings.Contains(lower, "720") || strings.Contains(lower, "hd"):
		return "720p"
	case strings.Contains(lower, "480") || strings.Contains(lower, "sd") || strings.Contains(lower, "normal"):
		return "480p"
	case strings.Contains(lower, "360"):
		return "360p"
	}

	if qualityType >= 10 {
		return "4k"
	}
	if qualityType >= 8 {
		return "2k"
	}
	if qualityType >= 2 {
		return "1080p"
	}
	if qualityType >= 1 {
		return "720p"
	}

	return ""
}


// getTtwid 从字节跳动获取ttwid cookie（Detail API必需）
func (p *Parser) getTtwid() string {
	body := `{"region":"cn","aid":1768,"needFid":false,"service":"www.ixigua.com","migrate_priority":0,"cbUrlProtocol":"https","union":true}`
	req, err := http.NewRequest("POST", "https://ttwid.bytedance.com/ttwid/union/register/", strings.NewReader(body))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "ttwid" {
			return cookie.Value
		}
	}
	return ""
}
// ============ Strategy 2: Detail API ============

func (p *Parser) parseDetailAPI(videoID string) (*models.VideoInfo, error) {
	if videoID == "" {
		return nil, fmt.Errorf("videoID is empty")
	}
	apiURL := fmt.Sprintf("https://www.douyin.com/aweme/v1/web/aweme/detail/?aweme_id=%s&aid=6383&cookie_enabled=true&browser_language=zh-CN&browser_platform=Win32&browser_name=Chrome&browser_version=126.0.0.0", videoID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Referer", fmt.Sprintf("https://www.douyin.com/video/%s", videoID))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")

	// 添加Cookie支持（优先使用用户Cookie，否则自动获取ttwid）
	cookieStr := p.cookies
	if cookieStr == "" {
		ttwid := p.getTtwid()
		if ttwid != "" {
			cookieStr = "ttwid=" + ttwid
		}
	}
	if cookieStr != "" {
		req.Header.Set("Cookie", cookieStr)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if len(body) > 0 && body[0] == '<' {
		return nil, fmt.Errorf("API returned HTML instead of JSON")
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	awemeDetail, ok := result["aweme_detail"].(map[string]interface{})
	if !ok || awemeDetail == nil {
		return nil, fmt.Errorf("aweme_detail not found in API response")
	}

	return p.parseAwemeDetail(awemeDetail), nil
}

func (p *Parser) parseAwemeDetail(detail map[string]interface{}) *models.VideoInfo {
	videoData, _ := detail["video"].(map[string]interface{})
	if videoData == nil {
		return nil
	}

	videoURLs := p.extractVideoURLs(videoData)

	title, _ := detail["desc"].(string)

	author := ""
	authorID := ""
	if authorInfo, ok := detail["author"].(map[string]interface{}); ok {
		author, _ = authorInfo["nickname"].(string)
		authorID, _ = authorInfo["unique_id"].(string)
	}

	coverURL := ""
	if cover, ok := videoData["cover"].(map[string]interface{}); ok {
		if urlList, ok := cover["url_list"].([]interface{}); ok && len(urlList) > 0 {
			coverURL, _ = urlList[0].(string)
		}
	}
	if coverURL != "" && strings.HasPrefix(coverURL, "//") {
		coverURL = "https:" + coverURL
	}

	selectedURL := ""
	selectedQuality := ""
	qualityPriority := []string{"4k", "2k", "1080p", "720p", "480p"}
	for _, q := range qualityPriority {
		if u, ok := videoURLs[q]; ok && u != "" {
			selectedURL = u
			selectedQuality = q
			break
		}
	}
	if selectedURL == "" {
		for q, u := range videoURLs {
			if q != "download" && q != "4k_h265" && u != "" {
				selectedURL = u
				selectedQuality = q
				break
			}
		}
	}
	if selectedURL == "" {
		if u, ok := videoURLs["4k_h265"]; ok && u != "" {
			selectedURL = u
			selectedQuality = "4k"
		}
	}

	if selectedURL == "" {
		return nil
	}

	audioURL2 := videoURLs["_audio"]
	delete(videoURLs, "_audio")

	log.Printf("[抖音-API] 选择清晰度: %s (可用: %v)", selectedQuality, getKeys(videoURLs))

	return &models.VideoInfo{
		Title:              title,
		Author:             author,
		AuthorID:           authorID,
		CoverURL:           coverURL,
		VideoURL:           selectedURL,
		AudioURL:           audioURL2,
		SelectedQuality:    selectedQuality,
		AvailableQualities: getKeys(videoURLs),
	}
}

// ============ Strategy 3: HTML Regex (improved) ============

func (p *Parser) parseHTMLRegex(videoURL string) (*models.VideoInfo, error) {
	html, cookies, err := p.fetchHTML(videoURL)
	if err != nil {
		return nil, err
	}

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

	for _, p2 := range patterns {
		re := regexp.MustCompile(p2.regex)
		matches := re.FindAllStringSubmatch(html, -1)
		for _, match := range matches {
			if len(match) > 1 {
				u := decodeUnicodeURL(match[1])
				u = processVideoURL(u)
				if p2.urlType == "download" {
					downloadURLs["1080p"] = u
				} else {
					videoURLs["1080p"] = u
				}
				break
			}
		}
	}

	bitRateMatch := regexp.MustCompile(`"bit_rate"\s*:\s*\[([^\]]+)\]`).FindStringSubmatch(html)
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

	allURLs := make(map[string]string)
	for k, v := range videoURLs {
		allURLs[k] = v
	}
	for k, v := range downloadURLs {
		allURLs[k] = v
	}

	if len(allURLs) == 0 {
		return nil, fmt.Errorf("no video URL found in HTML")
	}

	selectedURL := ""
	qualityPriority := []string{"4k", "2k", "1080p", "720p", "480p"}
	for _, q := range qualityPriority {
		if u, ok := allURLs[q]; ok && u != "" {
			selectedURL = u
			break
		}
	}
	if selectedURL == "" {
		for _, u := range allURLs {
			selectedURL = u
			break
		}
	}

	descMatch := regexp.MustCompile(`"desc"\s*:\s*"([^"]*)"`).FindStringSubmatch(html)
	authorMatch := regexp.MustCompile(`"nickname"\s*:\s*"([^"]*)"`).FindStringSubmatch(html)
	coverMatch := regexp.MustCompile(`"cover"[^}]*"url_list"\s*:\s*\["([^"]+)"`).FindStringSubmatch(html)

	title := ""
	if len(descMatch) > 1 {
		title = descMatch[1]
	}

	author := "未知作者"
	if len(authorMatch) > 1 {
		author = authorMatch[1]
	}

	coverURL := ""
	if len(coverMatch) > 1 {
		coverURL = decodeUnicodeURL(coverMatch[1])
		if strings.HasPrefix(coverURL, "//") {
			coverURL = "https:" + coverURL
		}
	}

	selectedQuality := ""
	for q := range allURLs {
		if q != "download" && q != "4k_h265" {
			selectedQuality = q
			break
		}
	}

	log.Printf("[抖音-HTML] 选择清晰度: %s (可用: %v)", selectedQuality, getKeys(allURLs))

	return &models.VideoInfo{
		Title:              title,
		Author:             author,
		CoverURL:           coverURL,
		VideoURL:           selectedURL,
		SelectedQuality:    selectedQuality,
		AvailableQualities: getKeys(allURLs),
		Cookies:            cookies,
	}, nil
}

// ============ Helper Functions ============

func processVideoURL(u string) string {
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "//") {
		u = "https:" + u
	}
	u = strings.Replace(u, "playwm", "play", -1)
	re := regexp.MustCompile(`[?&]watermark=\d+`)
	u = re.ReplaceAllString(u, "")
	re2 := regexp.MustCompile(`[?&]ratio=\w+`)
	u = re2.ReplaceAllString(u, "")
	return u
}

func decodeUnicodeURL(u string) string {
	if !strings.Contains(u, "\\u") {
		return u
	}
	var result strings.Builder
	for i := 0; i < len(u); i++ {
		if i+5 < len(u) && u[i:i+2] == "\\u" {
			var code int
			fmt.Sscanf(u[i+2:i+6], "%x", &code)
			result.WriteRune(rune(code))
			i += 5
		} else if u[i] == '\\' {
			continue
		} else {
			result.WriteByte(u[i])
		}
	}
	return result.String()
}

func getKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
